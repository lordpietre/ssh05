package contracts

import (
	"context"
	"time"
)

// SessionID identifies one concrete connection managed by the supervisor.
type SessionID string

// TabID identifies the visual representation of a session.
type TabID string

// WorkspaceID identifies a restorable workspace.
type WorkspaceID string

// SessionPhase is the observable state of a session.
type SessionPhase string

const (
	PhaseCreated        SessionPhase = "created"
	PhaseResolving      SessionPhase = "resolving"
	PhaseConnecting     SessionPhase = "connecting"
	PhaseAuthenticating SessionPhase = "authenticating"
	PhaseConnected      SessionPhase = "connected"
	PhaseDegraded       SessionPhase = "degraded"
	PhaseReconnecting   SessionPhase = "reconnecting"
	PhaseDisconnected   SessionPhase = "disconnected"
	PhaseDetached       SessionPhase = "detached"
	PhaseFailed         SessionPhase = "failed"
	PhaseClosing        SessionPhase = "closing"
	PhaseClosed         SessionPhase = "closed"
)

// Valid reports whether p is one of the phases defined by the SDD.
func (p SessionPhase) Valid() bool {
	switch p {
	case PhaseCreated, PhaseResolving, PhaseConnecting, PhaseAuthenticating,
		PhaseConnected, PhaseDegraded, PhaseReconnecting, PhaseDisconnected,
		PhaseDetached, PhaseFailed, PhaseClosing, PhaseClosed:
		return true
	default:
		return false
	}
}

// CloseIntent distinguishes closing a view, dropping the SSH transport and
// detaching from remote tmux.
type CloseIntent string

const (
	CloseView        CloseIntent = "close-view"
	DisconnectSSH    CloseIntent = "disconnect-ssh"
	DetachRemoteTmux CloseIntent = "detach-remote-tmux"
)

// Valid reports whether i is one of the three supported close intents.
func (i CloseIntent) Valid() bool {
	switch i {
	case CloseView, DisconnectSSH, DetachRemoteTmux:
		return true
	default:
		return false
	}
}

// Capabilities is the backend's real feature set. The UI must not infer these
// values from tab indexes or host names.
type Capabilities struct {
	Detach            bool `json:"supportsDetach"`
	Reattach          bool `json:"supportsReattach"`
	RemoteTmux        bool `json:"supportsRemoteTmux"`
	PersistentProcess bool `json:"supportsPersistentProcess"`
	BinaryStreaming   bool `json:"supportsBinaryStreaming"`
}

// SessionStatus is emitted whenever a phase changes.
type SessionStatus struct {
	ID        SessionID    `json:"sessionId"`
	Phase     SessionPhase `json:"phase"`
	Seq       uint64       `json:"seq"`
	At        time.Time    `json:"at"`
	ErrorCode string       `json:"errorCode,omitempty"`
	Message   string       `json:"message,omitempty"`
}

// DataHandler receives raw PTY bytes. It must never be logged by default.
type DataHandler func(data []byte)

// StatusHandler receives ordered phase changes for one session.
type StatusHandler func(status SessionStatus)

// SessionOptions carries non-secret session preferences.
type SessionOptions struct {
	TmuxSession string `json:"tmuxSession,omitempty"`
}

// SessionBackend is the contract the supervisor implements and the UI consumes.
// Implementations must keep ordering per session and bound their queues.
type SessionBackend interface {
	Open(ctx context.Context, profileID string, routeID string, options SessionOptions) (SessionID, error)
	Write(ctx context.Context, id SessionID, data []byte) error
	Resize(ctx context.Context, id SessionID, cols, rows int) error
	RequestClose(ctx context.Context, id SessionID, intent CloseIntent) error
	Attach(ctx context.Context, id SessionID, cursor *uint64) error
	Signal(ctx context.Context, id SessionID, signal string) error
	OnData(id SessionID, cb DataHandler) (unsubscribe func())
	OnStatus(id SessionID, cb StatusHandler) (unsubscribe func())
	Capabilities(ctx context.Context) (Capabilities, error)
}
