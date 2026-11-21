package ipc

import (
	"errors"

	"github.com/trzsz/trzsz-ssh/contracts"
)

// JSON-RPC 2.0 plus TRSS application codes.
const (
	CodeParseError      = -32700
	CodeInvalidRequest  = -32600
	CodeMethodNotFound  = -32601
	CodeInvalidParams   = -32602
	CodeInternal        = -32603
	CodeUnauthorized    = -32000
	CodeVersionMismatch = -32001
)

var (
	ErrUnsupported    = errors.New("ipc: unsupported platform")
	ErrAlreadyRunning = errors.New("ipc: server already running")
	ErrUnauthorized   = errors.New("ipc: unauthorized peer")
	ErrClosed         = errors.New("ipc: client closed")
)

func rpcErr(code int, message string) *contracts.RPCError {
	return &contracts.RPCError{Code: code, Message: message}
}
