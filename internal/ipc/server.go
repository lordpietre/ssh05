package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"

	"github.com/trzsz/trzsz-ssh/contracts"
)

// Server serves the authenticated local control channel.
type Server struct {
	path    string
	ln      net.Listener
	handler Handler

	mu        sync.Mutex
	conns     map[net.Conn]struct{}
	closed    bool
	closeOnce sync.Once
	closeErr  error
	wg        sync.WaitGroup
}

// Listen creates a local socket. On Unix the socket directory is forced to
// 0700 and the socket itself to 0600. A live socket at the same path produces
// ErrAlreadyRunning; a stale socket is removed.
func Listen(path string, handler Handler) (*Server, error) {
	if handler == nil {
		return nil, errors.New("ipc: nil handler")
	}
	ln, err := listenUnix(path)
	if err != nil {
		return nil, err
	}
	return &Server{
		path:    path,
		ln:      ln,
		handler: handler,
		conns:   make(map[net.Conn]struct{}),
	}, nil
}

// Path returns the socket path.
func (s *Server) Path() string {
	return s.path
}

// Serve accepts connections until the listener is closed or ctx is done.
func (s *Server) Serve(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-done:
		}
	}()

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if s.isClosed() || ctx.Err() != nil {
				return nil
			}
			return err
		}

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = conn.Close()
			return nil
		}
		s.conns[conn] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()

		go func(c net.Conn) {
			defer s.wg.Done()
			defer s.removeConn(c)
			s.handle(ctx, c)
		}(conn)
	}
}

// Close stops accepting, closes active connections and waits for handlers.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		for conn := range s.conns {
			_ = conn.Close()
		}
		s.conns = make(map[net.Conn]struct{})
		s.mu.Unlock()

		if s.ln != nil {
			s.closeErr = s.ln.Close()
		}
	})
	s.wg.Wait()
	return s.closeErr
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Server) removeConn(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	if err := checkPeer(conn); err != nil {
		return
	}

	reader := bufio.NewReaderSize(conn, 64*1024)
	writer := bufio.NewWriterSize(conn, 64*1024)
	clientConn := newConn(conn, writer)
	defer clientConn.close()

	for {
		kind, payload, err := readFrame(reader)
		if err != nil {
			return
		}
		switch kind {
		case FrameBinary:
			sessionID, data, err := ReadBinaryFrame(payload)
			if err != nil {
				return
			}
			clientConn.dispatchBinary(sessionID, data)
			continue
		case FrameJSON:
			// handled below
		default:
			return
		}

		var req contracts.RPCRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			resp := &contracts.RPCResponse{
				ProtocolVersion: contracts.ProtocolVersion,
				Error:           rpcErr(CodeParseError, "invalid JSON request"),
			}
			_ = clientConn.writeJSON(resp)
			return
		}

		resp := s.dispatch(ctx, clientConn, req)
		if err := clientConn.writeJSON(resp); err != nil {
			return
		}
		if req.ProtocolVersion != contracts.ProtocolVersion {
			return
		}
	}
}

func (s *Server) dispatch(ctx context.Context, conn *Conn, req contracts.RPCRequest) *contracts.RPCResponse {
	resp := &contracts.RPCResponse{
		ProtocolVersion: contracts.ProtocolVersion,
		RequestID:       req.RequestID,
	}
	if req.ProtocolVersion != contracts.ProtocolVersion {
		resp.Error = rpcErr(CodeVersionMismatch, "unsupported protocol version")
		return resp
	}
	if req.RequestID == "" {
		resp.Error = rpcErr(CodeInvalidRequest, "missing requestId")
		return resp
	}
	if !req.Method.Valid() {
		resp.Error = rpcErr(CodeMethodNotFound, "unknown method")
		return resp
	}

	result, rpcError := s.safeHandle(ctx, conn, req)
	if rpcError != nil {
		resp.Error = rpcError
		return resp
	}
	resp.Result = result
	return resp
}

func (s *Server) safeHandle(ctx context.Context, conn *Conn, req contracts.RPCRequest) (result json.RawMessage, rpcError *contracts.RPCError) {
	defer func() {
		if recover() != nil {
			result = nil
			// Never expose the panic value: it may contain host or secret data.
			rpcError = rpcErr(CodeInternal, "internal error")
		}
	}()
	return s.handler.Handle(ctx, conn, req)
}
