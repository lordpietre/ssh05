package ipc

import (
	"context"
	"encoding/json"

	"github.com/lordpietre/ssh05/contracts"
)

// Handler dispatches one validated control request. Returning a non-nil
// RPCError produces a typed error response; the transport never exposes a Go
// panic or a secret in the message. The Conn can be retained for binary PTY
// streaming while the connection stays open.
type Handler interface {
	Handle(ctx context.Context, conn *Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, conn *Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError)

func (f HandlerFunc) Handle(ctx context.Context, conn *Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	return f(ctx, conn, req)
}
