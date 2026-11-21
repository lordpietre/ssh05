package supervisor

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/trzsz/trzsz-ssh/contracts"
	"github.com/trzsz/trzsz-ssh/internal/audit"
	"github.com/trzsz/trzsz-ssh/internal/ipc"
)

// HostProvider supplies the current non-secret host inventory.
type HostProvider interface {
	List(ctx context.Context) ([]contracts.HostProfile, error)
}

// HostInventory is implemented by Inventory for import/conflicts/vault status.
type HostInventory interface {
	HostProvider
	Resolve(ctx context.Context, id string) (contracts.HostProfile, bool)
	Conflicts() []contracts.HostConflict
	VaultStatus() contracts.VaultStatusResult
	Import(ctx context.Context, source string, path string) (contracts.HostsImportResult, error)
}

// Service maps the typed IPC methods onto the supervisor backend.
type Service struct {
	backend *Backend
	hosts   HostProvider
	audit   *audit.Logger
}

// ServiceOption configures optional service integrations.
type ServiceOption func(*Service)

// WithAudit enables sanitized operational audit records.
func WithAudit(logger *audit.Logger) ServiceOption {
	return func(service *Service) { service.audit = logger }
}

// NewService creates the IPC handler for a supervisor backend.
func NewService(backend *Backend, hosts HostProvider, options ...ServiceOption) *Service {
	service := &Service{backend: backend, hosts: hosts}
	for _, option := range options {
		option(service)
	}
	return service
}

// Handle implements ipc.Handler.
func (s *Service) Handle(ctx context.Context, conn *ipc.Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	switch req.Method {
	case contracts.MethodHostsList:
		return s.listHosts(ctx, req)
	case contracts.MethodHostsImport:
		return s.importHosts(ctx, req)
	case contracts.MethodHostsResolve:
		return s.resolveHosts(ctx, req)
	case contracts.MethodSessionsOpen:
		return s.openSession(ctx, conn, req)
	case contracts.MethodSessionsWrite:
		return s.writeSession(ctx, conn, req)
	case contracts.MethodSessionsResize:
		return s.resizeSession(ctx, req)
	case contracts.MethodSessionsClose:
		return s.closeSession(ctx, req)
	case contracts.MethodSessionsDetachTmux:
		return s.detachTmux(ctx, req)
	case contracts.MethodSessionsAttachTmux:
		return s.attachTmux(ctx, req)
	case contracts.MethodSessionsSignal:
		return s.signalSession(ctx, req)
	case contracts.MethodSessionsList:
		return marshal(contracts.SessionsListResult{Sessions: s.backend.List()})
	case contracts.MethodSessionsAttach:
		return s.attachSession(ctx, conn, req)
	case contracts.MethodRoutesPlan:
		return s.planRoute(req)
	case contracts.MethodRoutesConnect:
		return s.connectRoute(ctx, conn, req)
	case contracts.MethodFrpInstancesList:
		return marshal(contracts.FrpInstancesListResult{})
	case contracts.MethodVaultStatus:
		status := contracts.VaultStatusResult{Provider: "none", Locked: true}
		if inventory, ok := s.hosts.(HostInventory); ok {
			status = inventory.VaultStatus()
		}
		return marshal(status)
	default:
		return nil, notImplemented()
	}
}

func (s *Service) listHosts(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	if s.hosts == nil {
		return marshal(contracts.HostsListResult{})
	}
	params, rpcError := decodeParams[contracts.HostsListParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	hosts, err := s.hosts.List(ctx)
	if err != nil {
		return nil, internalError(err)
	}
	if params.Query != "" {
		needle := strings.ToLower(params.Query)
		filtered := hosts[:0]
		for _, host := range hosts {
			haystack := strings.ToLower(host.Alias + " " + host.Target.Hostname + " " + host.Target.Username)
			if strings.Contains(haystack, needle) {
				filtered = append(filtered, host)
			}
		}
		hosts = filtered
	}
	result := contracts.HostsListResult{Hosts: hosts}
	if inventory, ok := s.hosts.(HostInventory); ok {
		result.Conflicts = inventory.Conflicts()
	}
	return marshal(result)
}

func (s *Service) importHosts(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	inventory, ok := s.hosts.(HostInventory)
	if !ok {
		return nil, notImplemented()
	}
	params, rpcError := decodeParams[contracts.HostsImportParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	result, err := inventory.Import(ctx, params.Source, params.Path)
	if err != nil {
		s.log(audit.Event{Action: "hosts.import", Result: "error", ErrorCode: "internal", Detail: params.Source})
		return nil, internalError(err)
	}
	s.log(audit.Event{Action: "hosts.import", Result: "ok", Detail: params.Source})
	return marshal(result)
}

func (s *Service) resolveHosts(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	if s.hosts == nil {
		return nil, &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: "host inventory unavailable"}
	}
	params, rpcError := decodeParams[contracts.HostsListParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	hosts, err := s.hosts.List(ctx)
	if err != nil {
		return nil, internalError(err)
	}
	needle := strings.ToLower(params.Query)
	for _, host := range hosts {
		haystack := strings.ToLower(host.Alias + " " + host.Target.Hostname + " " + host.Target.Username)
		if needle == "" || strings.Contains(haystack, needle) {
			routeID := "direct:" + host.ID
			return marshal(contracts.RoutesPlanResult{
				Routes: []contracts.ConnectionRoute{{
					ID:            routeID,
					Kind:          contracts.RouteDirect,
					HostProfileID: host.ID,
					Authorized:    true,
				}},
				Selected: routeID,
				Reason:   "direct SSH",
			})
		}
	}
	return nil, &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: "host not found"}
}

func (s *Service) openSession(ctx context.Context, conn *ipc.Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.SessionsOpenParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	destination := s.destinationFor(ctx, params.HostProfileID)
	id, err := s.backend.Open(ctx, params.HostProfileID, params.RoutePolicyID, contracts.SessionOptions{
		TmuxSession: params.TmuxSession,
		Destination: destination,
	})
	if err != nil {
		s.log(audit.Event{Action: "session.open", HostProfileID: params.HostProfileID, Result: "error", ErrorCode: "internal"})
		return nil, internalError(err)
	}
	s.log(audit.Event{Action: "session.open", HostProfileID: params.HostProfileID, SessionID: string(id), Result: "ok"})
	s.attach(conn, id)
	return marshal(contracts.SessionsOpenResult{SessionID: id})
}

func (s *Service) destinationFor(ctx context.Context, hostProfileID string) string {
	if inventory, ok := s.hosts.(HostInventory); ok {
		if profile, found := inventory.Resolve(ctx, hostProfileID); found {
			return destinationFor(profile)
		}
	}
	return hostProfileID
}

func (s *Service) writeSession(ctx context.Context, conn *ipc.Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.SessionsWriteParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	s.installBinaryInput(conn)
	if len(params.Data) > 0 {
		if err := s.backend.Write(ctx, params.SessionID, params.Data); err != nil {
			return nil, internalError(err)
		}
	}
	return json.RawMessage(`{}`), nil
}

func (s *Service) resizeSession(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.SessionsResizeParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.backend.Resize(ctx, params.SessionID, params.Cols, params.Rows); err != nil {
		return nil, internalError(err)
	}
	return json.RawMessage(`{}`), nil
}

func (s *Service) closeSession(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.SessionsCloseParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.backend.RequestClose(ctx, params.SessionID, params.Intent); err != nil {
		s.log(audit.Event{Action: "session.close", SessionID: string(params.SessionID), Result: "error", ErrorCode: "internal", Detail: string(params.Intent)})
		return nil, internalError(err)
	}
	s.log(audit.Event{Action: "session.close", SessionID: string(params.SessionID), Result: "ok", Detail: string(params.Intent)})
	return json.RawMessage(`{}`), nil
}

func (s *Service) detachTmux(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.SessionsTmuxParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.backend.RequestClose(ctx, params.SessionID, contracts.DetachRemoteTmux); err != nil {
		return nil, internalError(err)
	}
	return json.RawMessage(`{}`), nil
}

func (s *Service) attachTmux(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.SessionsTmuxParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.backend.Attach(ctx, params.SessionID, nil); err != nil {
		return nil, internalError(err)
	}
	return json.RawMessage(`{}`), nil
}

func (s *Service) signalSession(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.SessionsSignalParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.backend.Signal(ctx, params.SessionID, params.Signal); err != nil {
		return nil, internalError(err)
	}
	return json.RawMessage(`{}`), nil
}

func (s *Service) attachSession(ctx context.Context, conn *ipc.Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.SessionsAttachParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.backend.Attach(ctx, params.SessionID, nil); err != nil {
		return nil, internalError(err)
	}
	s.attach(conn, params.SessionID)
	return json.RawMessage(`{}`), nil
}

func (s *Service) planRoute(req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.RoutesPlanParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	ctx := context.Background()
	routes, reason := s.routeCandidates(ctx, params.HostProfileID)
	selected := ""
	if len(routes) > 0 {
		selected = routes[len(routes)-1].ID
	}
	return marshal(contracts.RoutesPlanResult{Routes: routes, Selected: selected, Reason: reason})
}

func (s *Service) routeCandidates(ctx context.Context, hostProfileID string) ([]contracts.ConnectionRoute, string) {
	directID := "direct:" + hostProfileID
	routes := []contracts.ConnectionRoute{{
		ID:            directID,
		Kind:          contracts.RouteDirect,
		HostProfileID: hostProfileID,
		Authorized:    true,
	}}
	reason := "direct SSH"

	if inventory, ok := s.hosts.(HostInventory); ok {
		if profile, found := inventory.Resolve(ctx, hostProfileID); found && len(profile.ProxyJump) > 0 {
			routes = append(routes, contracts.ConnectionRoute{
				ID:            "proxy_jump:" + hostProfileID,
				Kind:          contracts.RouteProxyJump,
				HostProfileID: hostProfileID,
				Via:           append([]string(nil), profile.ProxyJump...),
				Authorized:    true,
			})
			reason = "ProxyJump via " + strings.Join(profile.ProxyJump, ", ")
		}
	}
	return routes, reason
}

func (s *Service) connectRoute(ctx context.Context, conn *ipc.Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.RoutesConnectParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	routes, _ := s.routeCandidates(ctx, params.HostProfileID)
	if params.RouteID != "" {
		authorized := false
		for _, route := range routes {
			if route.ID == params.RouteID && route.Authorized {
				authorized = true
				break
			}
		}
		if !authorized {
			return nil, &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: "route not authorized"}
		}
	}
	id, err := s.backend.Open(ctx, params.HostProfileID, "", contracts.SessionOptions{
		Destination: s.destinationFor(ctx, params.HostProfileID),
	})
	if err != nil {
		return nil, internalError(err)
	}
	s.attach(conn, id)
	return marshal(contracts.SessionsOpenResult{SessionID: id})
}

func (s *Service) log(event audit.Event) {
	if s.audit != nil {
		s.audit.Log(event)
	}
}

func (s *Service) attach(conn *ipc.Conn, id contracts.SessionID) {
	s.installBinaryInput(conn)
	unsubscribeData := s.backend.OnData(id, func(data []byte) {
		_ = conn.SendBinary(string(id), data)
	})

	var cleanupOnce sync.Once
	var unsubscribeStatus func()
	ready := make(chan struct{})
	cleanup := func() {
		<-ready
		cleanupOnce.Do(func() {
			unsubscribeData()
			unsubscribeStatus()
		})
	}
	unsubscribeStatus = s.backend.OnStatus(id, func(status contracts.SessionStatus) {
		payload, err := json.Marshal(status)
		if err != nil {
			return
		}
		_ = conn.SendEvent(contracts.Event{
			ProtocolVersion: contracts.ProtocolVersion,
			Event:           "session.status",
			SessionID:       id,
			Seq:             status.Seq,
			Timestamp:       status.At,
			Payload:         payload,
		})
		if status.Phase == contracts.PhaseClosed || status.Phase == contracts.PhaseFailed {
			go cleanup()
		}
	})
	close(ready)

	go func() {
		<-conn.Done()
		cleanup()
	}()
}

func (s *Service) installBinaryInput(conn *ipc.Conn) {
	conn.SetBinaryHandler(func(sessionID string, data []byte) {
		_ = s.backend.Write(context.Background(), contracts.SessionID(sessionID), data)
	})
}

func decodeParams[T any](req contracts.RPCRequest) (T, *contracts.RPCError) {
	var params T
	if len(req.Params) == 0 || string(req.Params) == "null" {
		return params, nil
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return params, &contracts.RPCError{Code: ipc.CodeInvalidParams, Message: "invalid params"}
	}
	return params, nil
}

func marshal(value any) (json.RawMessage, *contracts.RPCError) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, internalError(err)
	}
	return data, nil
}

func internalError(err error) *contracts.RPCError {
	return &contracts.RPCError{Code: ipc.CodeInternal, Message: err.Error()}
}

func notImplemented() *contracts.RPCError {
	return &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: "method not implemented in this supervisor"}
}
