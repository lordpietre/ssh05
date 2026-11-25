package ipc

import (
	"errors"

	"github.com/lordpietre/ssh05/contracts"
)

// JSON-RPC 2.0 plus ssh05 application codes.
const (
	CodeParseError      = -32700
	CodeInvalidRequest  = -32600
	CodeMethodNotFound  = -32601
	CodeInvalidParams   = -32602
	CodeInternal        = -32603
	CodeUnauthorized    = -32000
	CodeVersionMismatch = -32001
	// CodeHostKeyUnknown asks the UI to confirm a new server host key.
	CodeHostKeyUnknown = -32010
	// CodeHostKeyChanged reports a key that differs from known_hosts. It must
	// never be auto-trusted.
	CodeHostKeyChanged = -32011
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
