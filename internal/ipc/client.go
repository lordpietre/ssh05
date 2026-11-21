package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/trzsz/trzsz-ssh/contracts"
)

// BinaryHandler receives PTY bytes from the binary channel.
type BinaryHandler func(sessionID string, data []byte)

// Client is a local control-channel client with one reader goroutine and
// request/response correlation by requestId.
type Client struct {
	conn net.Conn

	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[contracts.RequestID]chan *contracts.RPCResponse
	nextID    atomic.Uint64

	closeOnce sync.Once
	closed    chan struct{}

	errMu sync.Mutex
	err   error

	binaryMu      sync.Mutex
	binaryHandler BinaryHandler
}

// Dial connects to an existing local socket.
func Dial(path string) (*Client, error) {
	return DialContext(context.Background(), path)
}

// DialContext connects with a bounded timeout and context cancellation.
func DialContext(ctx context.Context, path string) (*Client, error) {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("ipc: dial: %w", err)
	}
	client := &Client{
		conn:    conn,
		pending: make(map[contracts.RequestID]chan *contracts.RPCResponse),
		closed:  make(chan struct{}),
	}
	go client.readLoop()
	return client, nil
}

// SetBinaryHandler installs the callback for the binary PTY channel.
func (c *Client) SetBinaryHandler(handler BinaryHandler) {
	c.binaryMu.Lock()
	c.binaryHandler = handler
	c.binaryMu.Unlock()
}

// Call sends a typed control request and waits for its response. A remote error
// is returned as *contracts.RPCError; transport errors are ordinary errors.
func (c *Client) Call(ctx context.Context, method contracts.Method, params any) (json.RawMessage, error) {
	if !method.Valid() {
		return nil, &contracts.RPCError{Code: CodeMethodNotFound, Message: "unknown method"}
	}
	rawParams, err := marshalParams(params)
	if err != nil {
		return nil, err
	}

	requestID := contracts.RequestID(fmt.Sprintf("req-%d", c.nextID.Add(1)))
	req := contracts.RPCRequest{
		ProtocolVersion: contracts.ProtocolVersion,
		RequestID:       requestID,
		Method:          method,
		Params:          rawParams,
		Timestamp:       time.Now().UTC(),
	}
	return c.call(ctx, req)
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		return raw, nil
	}
	data, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("ipc: encode params: %w", err)
	}
	return data, nil
}

func (c *Client) call(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, error) {
	response := make(chan *contracts.RPCResponse, 1)
	if err := c.register(req.RequestID, response); err != nil {
		return nil, err
	}

	if err := c.write(req); err != nil {
		c.remove(req.RequestID)
		return nil, err
	}

	select {
	case resp := <-response:
		if resp == nil {
			return nil, c.getErr()
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.remove(req.RequestID)
		return nil, ctx.Err()
	case <-c.closed:
		c.remove(req.RequestID)
		return nil, c.getErr()
	}
}

func (c *Client) register(requestID contracts.RequestID, ch chan *contracts.RPCResponse) error {
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if c.pending == nil {
		return ErrClosed
	}
	c.pending[requestID] = ch
	return nil
}

func (c *Client) remove(requestID contracts.RequestID) {
	c.pendingMu.Lock()
	delete(c.pending, requestID)
	c.pendingMu.Unlock()
}

func (c *Client) write(req contracts.RPCRequest) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	if err := WriteJSONFrame(c.conn, req); err != nil {
		c.fail(fmt.Errorf("ipc: write: %w", err))
		return c.getErr()
	}
	return nil
}

func (c *Client) readLoop() {
	for {
		kind, payload, err := readFrame(c.conn)
		if err != nil {
			c.fail(fmt.Errorf("ipc: read: %w", err))
			return
		}
		switch kind {
		case FrameBinary:
			sessionID, data, err := ReadBinaryFrame(payload)
			if err != nil {
				c.fail(err)
				return
			}
			c.binaryMu.Lock()
			handler := c.binaryHandler
			c.binaryMu.Unlock()
			if handler != nil {
				handler(sessionID, data)
			}
		case FrameJSON:
			var resp contracts.RPCResponse
			if err := json.Unmarshal(payload, &resp); err != nil {
				c.fail(fmt.Errorf("ipc: decode response: %w", err))
				return
			}
			if resp.ProtocolVersion != contracts.ProtocolVersion {
				c.fail(fmt.Errorf("ipc: response protocol version %d", resp.ProtocolVersion))
				return
			}
			c.pendingMu.Lock()
			ch := c.pending[resp.RequestID]
			delete(c.pending, resp.RequestID)
			c.pendingMu.Unlock()
			if ch != nil {
				ch <- &resp
			}
		default:
			c.fail(ErrUnknownFrame)
			return
		}
	}
}

func (c *Client) fail(err error) {
	c.closeOnce.Do(func() {
		c.errMu.Lock()
		c.err = err
		c.errMu.Unlock()
		close(c.closed)
		_ = c.conn.Close()

		c.pendingMu.Lock()
		for requestID, ch := range c.pending {
			delete(c.pending, requestID)
			ch <- nil
		}
		c.pendingMu.Unlock()
	})
}

func (c *Client) getErr() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	if c.err != nil {
		return c.err
	}
	return ErrClosed
}

// Close closes the connection and fails pending calls.
func (c *Client) Close() error {
	c.fail(ErrClosed)
	return nil
}

// CallTyped is a small generic helper for typed method contracts.
func CallTyped[P any, R any](ctx context.Context, client *Client, method contracts.Method, params P) (R, error) {
	var zero R
	raw, err := client.Call(ctx, method, params)
	if err != nil {
		return zero, err
	}
	if len(raw) == 0 {
		return zero, nil
	}
	var result R
	if err := json.Unmarshal(raw, &result); err != nil {
		return zero, fmt.Errorf("ipc: decode result: %w", err)
	}
	return result, nil
}
