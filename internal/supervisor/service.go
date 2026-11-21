package supervisor

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/trzsz/trzsz-ssh/contracts"
	"github.com/trzsz/trzsz-ssh/internal/ipc"
)

// HostProvider supplies the current non-secret host inventory.
type HostProvider interface {
	List(ctx context.Context) ([]contracts.HostProfile, error)
}

// Service maps the typed IPC methods onto the supervisor backend.
type Service struct {
	backend *Backend
	hosts   HostProvider
}

// NewService creates the IPC handler for a supervisor backend.
func NewService(backend *Backend, hosts HostProvider) *Service {
	return &Service{backend: backend, hosts: hosts}
}

// Handle implements ipc.Handler.
func (s *Service) Handle(ctx context.Context, conn *ipc.Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	switch req.Method {
	case contracts.MethodHostsList:
		return s.listHosts(ctx, req)
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
		return marshal(contracts.VaultStatusResult{Provider: "none", Locked: true})
	default:
		return nil, notImplemented()
	}
}

func (s *Service) listHosts(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	if s.hosts == nil {
		return marshal(contracts.HostsListResult{})
	}
	if _, err := decodeParams[contracts.HostsListParams](req); err != nil {
		return nil, err
	}
	hosts, err := s.hosts.List(ctx)
	if err != nil {
		return nil, internalError(err)
	}
	return marshal(contracts.HostsListResult{Hosts: hosts})
}

func (s *Service) openSession(ctx context.Context, conn *ipc.Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.SessionsOpenParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	id, err := s.backend.Open(ctx, params.HostProfileID, params.RoutePolicyID, contracts.SessionOptions{TmuxSession: params.TmuxSession})
	if err != nil {
		return nil, internalError(err)
	}
	s.attach(conn, id)
	return marshal(contracts.SessionsOpenResult{SessionID: id})
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
		return nil, internalError(err)
	}
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
	routeID := "direct:" + params.HostProfileID
	return marshal(contracts.RoutesPlanResult{
		Routes: []contracts.ConnectionRoute{{
			ID:            routeID,
			Kind:          contracts.RouteDirect,
			HostProfileID: params.HostProfileID,
			Authorized:    true,
		}},
		Selected: routeID,
		Reason:   "direct SSH",
	})
}

func (s *Service) connectRoute(ctx context.Context, conn *ipc.Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.RoutesConnectParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	id, err := s.backend.Open(ctx, params.HostProfileID, "", contracts.SessionOptions{})
	if err != nil {
		return nil, internalError(err)
	}
	s.attach(conn, id)
	return marshal(contracts.SessionsOpenResult{SessionID: id})
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
