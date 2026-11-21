package ipc

import (
	"context"
	"encoding/json"

	"github.com/trzsz/trzsz-ssh/contracts"
)

// Handler dispatches one validated control request. Returning a non-nil
// RPCError produces a typed error response; the transport never exposes a Go
// panic or a secret in the message.
type Handler interface {
	Handle(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError)

func (f HandlerFunc) Handle(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	return f(ctx, req)
}
