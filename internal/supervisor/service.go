package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/trzsz/trzsz-ssh/contracts"
	"github.com/trzsz/trzsz-ssh/internal/audit"
	frpmanager "github.com/trzsz/trzsz-ssh/internal/frp"
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
	frp     *frpmanager.Manager
}

// ServiceOption configures optional service integrations.
type ServiceOption func(*Service)

// WithAudit enables sanitized operational audit records.
func WithAudit(logger *audit.Logger) ServiceOption {
	return func(service *Service) { service.audit = logger }
}

// WithFRP enables the local FRP supervisor.
func WithFRP(manager *frpmanager.Manager) ServiceOption {
	return func(service *Service) { service.frp = manager }
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
	case contracts.MethodRoutesProbe:
		return s.probeRoute(ctx, req)
	case contracts.MethodFrpInstancesList:
		if s.frp == nil {
			return marshal(contracts.FrpInstancesListResult{})
		}
		return marshal(s.frp.List())
	case contracts.MethodFrpProxiesApply:
		return s.applyFRP(ctx, req)
	case contracts.MethodFrpProxiesStop:
		return s.stopFRP(ctx, req)
	case contracts.MethodFrpProxiesRemove:
		return s.removeFRP(ctx, req)
	case contracts.MethodFrpRotate:
		return s.rotateFRP(ctx, req)
	case contracts.MethodFrpServerStart:
		return s.startFRPServer(ctx, req)
	case contracts.MethodFrpServerStop:
		return s.stopFRPServer(ctx, req)
	case contracts.MethodVaultStatus:
		status := contracts.VaultStatusResult{Provider: "none", Locked: true}
		if inventory, ok := s.hosts.(HostInventory); ok {
			status = inventory.VaultStatus()
		}
		return marshal(status)
	case contracts.MethodVaultUnlock:
		return s.unlockVault(ctx, req)
	default:
		return nil, notImplemented()
	}
}

func (s *Service) unlockVault(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	inventory, ok := s.hosts.(HostInventory)
	if !ok {
		return nil, notImplemented()
	}
	params, rpcError := decodeParams[contracts.VaultUnlockParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if params.Provider != "" && params.Provider != "keepassxc" && params.Provider != "keepass" {
		return nil, &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: "only KeePassXC is supported"}
	}
	if _, err := inventory.Import(ctx, "keepass", ""); err != nil {
		s.log(audit.Event{Action: "vault.unlock", Result: "error", ErrorCode: "internal", Detail: params.Reason})
		return nil, internalError(err)
	}
	status := inventory.VaultStatus()
	s.log(audit.Event{Action: "vault.unlock", Result: "ok", Detail: params.Reason})
	return marshal(status)
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
	destination := s.destinationForRoute(ctx, params.HostProfileID, params.RouteID)
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

func (s *Service) destinationForRoute(ctx context.Context, hostProfileID, routeID string) string {
	if strings.HasPrefix(routeID, "frp:") {
		routes, _ := s.routeCandidates(ctx, hostProfileID)
		for _, route := range routes {
			if route.ID != routeID || len(route.Via) == 0 {
				continue
			}
			user := ""
			if inventory, ok := s.hosts.(HostInventory); ok {
				if profile, found := inventory.Resolve(ctx, hostProfileID); found && profile.Target.Username != "" {
					user = profile.Target.Username + "@"
				}
			}
			return user + route.Via[0]
		}
	}
	return s.destinationFor(ctx, hostProfileID)
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

func (s *Service) probeRoute(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.RoutesProbeParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	routes, _ := s.routeCandidates(ctx, params.HostProfileID)
	var route contracts.ConnectionRoute
	found := false
	for _, candidate := range routes {
		if candidate.ID == params.RouteID {
			route, found = candidate, true
			break
		}
	}
	if !found {
		if len(routes) == 0 {
			return nil, internalError(fmt.Errorf("no authorized route for %q", params.HostProfileID))
		}
		route = routes[len(routes)-1]
	}
	address, err := s.routeAddress(ctx, route)
	if err != nil {
		return nil, internalError(err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	started := time.Now()
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(probeCtx, "tcp", address)
	latency := time.Since(started).Milliseconds()
	if conn != nil {
		_ = conn.Close()
	}
	state := classifyProbeError(err)
	message := ""
	if err != nil {
		message = err.Error()
		if state == "timeout" {
			message = "sin respuesta dentro del plazo; no demuestra que el puerto esté cerrado"
		}
	}
	return marshal(contracts.RoutesProbeResult{
		RouteID: route.ID, Kind: route.Kind, Address: address,
		State: state, LatencyMS: latency, Message: message,
	})
}

func (s *Service) routeAddress(ctx context.Context, route contracts.ConnectionRoute) (string, error) {
	switch route.Kind {
	case contracts.RouteFRP:
		if len(route.Via) > 0 {
			return route.Via[0], nil
		}
	case contracts.RouteProxyJump:
		if len(route.Via) > 0 {
			host, port := splitJump(route.Via[0])
			return net.JoinHostPort(host, port), nil
		}
	case contracts.RouteDirect:
		if inventory, ok := s.hosts.(HostInventory); ok {
			if profile, found := inventory.Resolve(ctx, route.HostProfileID); found {
				port := profile.Target.Port
				if port <= 0 {
					port = 22
				}
				return net.JoinHostPort(profile.Target.Hostname, fmt.Sprintf("%d", port)), nil
			}
		}
	}
	return "", fmt.Errorf("route %q has no probe address", route.ID)
}

func splitJump(value string) (string, string) {
	hostPart := value
	if at := strings.LastIndex(hostPart, "@"); at >= 0 {
		hostPart = hostPart[at+1:]
	}
	if strings.HasPrefix(hostPart, "[") {
		if end := strings.Index(hostPart, "]"); end >= 0 {
			host := strings.Trim(hostPart[:end+1], "[]")
			port := "22"
			if rest := strings.TrimPrefix(hostPart[end+1:], ":"); rest != "" {
				port = rest
			}
			return host, port
		}
	}
	host, port, err := net.SplitHostPort(hostPart)
	if err == nil {
		return host, port
	}
	return hostPart, "22"
}

func classifyProbeError(err error) string {
	if err == nil {
		return "reachable"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns_failure"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "refused"
	}
	return "unknown"
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
	if s.frp != nil {
		if port, ok := s.frp.RouteForHost(hostProfileID); ok {
			routes = append(routes, contracts.ConnectionRoute{
				ID:            fmt.Sprintf("frp:%s:%d", hostProfileID, port),
				Kind:          contracts.RouteFRP,
				HostProfileID: hostProfileID,
				Via:           []string{fmt.Sprintf("127.0.0.1:%d", port)},
				Authorized:    true,
			})
			reason = fmt.Sprintf("FRP visitor en 127.0.0.1:%d", port)
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
		Destination: s.destinationForRoute(ctx, params.HostProfileID, params.RouteID),
	})
	if err != nil {
		return nil, internalError(err)
	}
	s.attach(conn, id)
	return marshal(contracts.SessionsOpenResult{SessionID: id})
}

func (s *Service) applyFRP(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	if s.frp == nil {
		return nil, notImplemented()
	}
	params, rpcError := decodeParams[contracts.FrpProxiesApplyParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	applied, err := s.frp.Apply(ctx, params.Proxy)
	if err != nil {
		s.log(audit.Event{Action: "frp.apply", Result: "error", ErrorCode: "internal", Detail: params.Proxy.Name})
		return nil, internalError(err)
	}
	s.log(audit.Event{Action: "frp.apply", Result: "ok", Detail: applied.ID})
	return marshal(contracts.FrpProxiesApplyResult{Proxy: applied})
}

func (s *Service) stopFRP(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	if s.frp == nil {
		return nil, notImplemented()
	}
	params, rpcError := decodeParams[contracts.FrpProxiesStopParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.frp.Stop(ctx, params.ProxyID); err != nil {
		s.log(audit.Event{Action: "frp.stop", Result: "error", ErrorCode: "internal", Detail: params.ProxyID})
		return nil, internalError(err)
	}
	s.log(audit.Event{Action: "frp.stop", Result: "ok", Detail: params.ProxyID})
	return json.RawMessage(`{}`), nil
}

func (s *Service) removeFRP(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	if s.frp == nil {
		return nil, notImplemented()
	}
	params, rpcError := decodeParams[contracts.FrpProxiesRemoveParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.frp.Remove(ctx, params.ProxyID); err != nil {
		s.log(audit.Event{Action: "frp.remove", Result: "error", ErrorCode: "internal", Detail: params.ProxyID})
		return nil, internalError(err)
	}
	s.log(audit.Event{Action: "frp.remove", Result: "ok", Detail: params.ProxyID})
	return json.RawMessage(`{}`), nil
}

func (s *Service) rotateFRP(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	if s.frp == nil {
		return nil, notImplemented()
	}
	params, rpcError := decodeParams[contracts.FrpRotateParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.frp.RotateEndpoint(ctx, params.EndpointID); err != nil {
		s.log(audit.Event{Action: "frp.rotate", Result: "error", ErrorCode: "internal", Detail: params.EndpointID})
		return nil, internalError(err)
	}
	s.log(audit.Event{Action: "frp.rotate", Result: "ok", Detail: params.EndpointID})
	return json.RawMessage(`{}`), nil
}

func (s *Service) startFRPServer(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	if s.frp == nil {
		return nil, notImplemented()
	}
	params, rpcError := decodeParams[contracts.FrpServerParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.frp.StartEndpoint(ctx, params.EndpointID); err != nil {
		s.log(audit.Event{Action: "frp.server.start", Result: "error", ErrorCode: "internal", Detail: params.EndpointID})
		return nil, internalError(err)
	}
	s.log(audit.Event{Action: "frp.server.start", Result: "ok", Detail: params.EndpointID})
	for _, endpoint := range s.frp.List().Endpoints {
		if endpoint.ID == params.EndpointID {
			return marshal(endpoint)
		}
	}
	return nil, internalError(fmt.Errorf("FRP endpoint not found after start"))
}

func (s *Service) stopFRPServer(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	if s.frp == nil {
		return nil, notImplemented()
	}
	params, rpcError := decodeParams[contracts.FrpServerParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if err := s.frp.StopEndpoint(ctx, params.EndpointID); err != nil {
		s.log(audit.Event{Action: "frp.server.stop", Result: "error", ErrorCode: "internal", Detail: params.EndpointID})
		return nil, internalError(err)
	}
	s.log(audit.Event{Action: "frp.server.stop", Result: "ok", Detail: params.EndpointID})
	return json.RawMessage(`{}`), nil
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
