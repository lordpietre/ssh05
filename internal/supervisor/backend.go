package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/trzsz/trzsz-ssh/contracts"
)

var (
	ErrSessionNotFound   = errors.New("supervisor: session not found")
	ErrSessionClosed     = errors.New("supervisor: session closed")
	ErrUnsupportedIntent = errors.New("supervisor: unsupported close intent")
	ErrTooManySessions   = errors.New("supervisor: too many sessions")
)

const (
	defaultMaxSessions = 16
	defaultMaxPending  = 1 << 20 // 1 MiB before the first data handler attaches
)

// Options bounds the supervisor resources.
type Options struct {
	MaxSessions int
	MaxPending  int
}

// Backend is the in-process implementation of contracts.SessionBackend.
type Backend struct {
	runner  Runner
	options Options

	mu       sync.RWMutex
	sessions map[contracts.SessionID]*session
	nextID   atomic.Uint64
}

type session struct {
	id        contracts.SessionID
	profileID string
	phase     contracts.SessionPhase
	seq       uint64
	run       RunSession
	exitCode  int

	nextHandler    uint64
	dataHandlers   map[uint64]contracts.DataHandler
	statusHandlers map[uint64]contracts.StatusHandler

	pending          []byte
	pendingDelivered bool
	maxPending       int
	errorCode        string
	message          string
}

// NewBackend creates a supervisor backend around a Runner.
func NewBackend(runner Runner, options Options) *Backend {
	if options.MaxSessions <= 0 {
		options.MaxSessions = defaultMaxSessions
	}
	if options.MaxPending <= 0 {
		options.MaxPending = defaultMaxPending
	}
	return &Backend{
		runner:   runner,
		options:  options,
		sessions: make(map[contracts.SessionID]*session),
	}
}

// Open creates a session and starts its transport with a provisional 80x24
// PTY. The UI must call Resize after receiving the real viewport size.
func (b *Backend) Open(ctx context.Context, profileID string, routeID string) (contracts.SessionID, error) {
	if profileID == "" {
		return "", errors.New("supervisor: profileID is required")
	}

	b.mu.Lock()
	if len(b.sessions) >= b.options.MaxSessions {
		b.mu.Unlock()
		return "", ErrTooManySessions
	}
	id := contracts.SessionID(fmt.Sprintf("ses-%d", b.nextID.Add(1)))
	s := &session{
		id:             id,
		profileID:      profileID,
		phase:          contracts.PhaseConnecting,
		dataHandlers:   make(map[uint64]contracts.DataHandler),
		statusHandlers: make(map[uint64]contracts.StatusHandler),
		maxPending:     b.options.MaxPending,
	}
	b.sessions[id] = s
	b.mu.Unlock()

	b.emitStatus(s, contracts.PhaseConnecting, "", "")

	run, err := b.runner.Start(ctx, profileID, 80, 24)
	if err != nil {
		b.emitStatus(s, contracts.PhaseFailed, "start_failed", err.Error())
		b.mu.Lock()
		delete(b.sessions, id)
		b.mu.Unlock()
		return "", err
	}

	b.mu.Lock()
	s.run = run
	b.mu.Unlock()
	b.emitStatus(s, contracts.PhaseConnected, "", "")

	go b.readSession(s, run)
	return id, nil
}

func (b *Backend) readSession(s *session, run RunSession) {
	for data := range run.Output() {
		b.emitData(s, data)
	}
	<-run.Done()

	code := run.ExitCode()
	b.mu.Lock()
	s.run = nil
	s.exitCode = code
	b.mu.Unlock()

	if code == 0 {
		b.emitStatus(s, contracts.PhaseClosed, "", "")
		return
	}
	b.emitStatus(s, contracts.PhaseFailed, "exit_nonzero", fmt.Sprintf("exit code %d", code))
}

func (b *Backend) Write(ctx context.Context, id contracts.SessionID, data []byte) error {
	run, err := b.runFor(id)
	if err != nil {
		return err
	}
	return run.Write(data)
}

func (b *Backend) Resize(ctx context.Context, id contracts.SessionID, cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("supervisor: invalid size %dx%d", cols, rows)
	}
	run, err := b.runFor(id)
	if err != nil {
		return err
	}
	return run.Resize(cols, rows)
}

func (b *Backend) Signal(ctx context.Context, id contracts.SessionID, signal string) error {
	if signal == "" {
		return fmt.Errorf("supervisor: empty signal")
	}
	run, err := b.runFor(id)
	if err != nil {
		return err
	}
	return run.Signal(signal)
}

func (b *Backend) RequestClose(ctx context.Context, id contracts.SessionID, intent contracts.CloseIntent) error {
	if !intent.Valid() {
		return ErrUnsupportedIntent
	}
	if intent == contracts.DetachRemoteTmux {
		return ErrUnsupportedIntent
	}

	b.mu.RLock()
	s := b.sessions[id]
	if s == nil {
		b.mu.RUnlock()
		return ErrSessionNotFound
	}
	run := s.run
	b.mu.RUnlock()
	if run == nil {
		return ErrSessionClosed
	}

	b.emitStatus(s, contracts.PhaseClosing, "", "")
	return run.Close()
}

func (b *Backend) Attach(ctx context.Context, id contracts.SessionID, cursor *uint64) error {
	b.mu.RLock()
	s := b.sessions[id]
	b.mu.RUnlock()
	if s == nil {
		return ErrSessionNotFound
	}
	// This backend does not retain a detached process; Attach only makes the
	// existing live session visible to a new view.
	return nil
}

func (b *Backend) OnData(id contracts.SessionID, cb contracts.DataHandler) func() {
	if cb == nil {
		return func() {}
	}
	b.mu.Lock()
	s := b.sessions[id]
	if s == nil {
		b.mu.Unlock()
		return func() {}
	}
	key := s.nextHandler
	s.nextHandler++
	s.dataHandlers[key] = cb
	var pending []byte
	if !s.pendingDelivered && len(s.pending) > 0 {
		pending = s.pending
		s.pending = nil
		s.pendingDelivered = true
	}
	b.mu.Unlock()

	if len(pending) > 0 {
		cb(pending)
	}
	return func() {
		b.mu.Lock()
		delete(s.dataHandlers, key)
		if len(s.dataHandlers) == 0 {
			// Start a fresh buffered window for the next viewer.
			s.pending = nil
			s.pendingDelivered = false
		}
		b.mu.Unlock()
	}
}

func (b *Backend) OnStatus(id contracts.SessionID, cb contracts.StatusHandler) func() {
	if cb == nil {
		return func() {}
	}
	b.mu.Lock()
	s := b.sessions[id]
	if s == nil {
		b.mu.Unlock()
		return func() {}
	}
	key := s.nextHandler
	s.nextHandler++
	s.statusHandlers[key] = cb
	current := s.status()
	b.mu.Unlock()

	cb(current)
	return func() {
		b.mu.Lock()
		delete(s.statusHandlers, key)
		b.mu.Unlock()
	}
}

func (b *Backend) Capabilities(ctx context.Context) (contracts.Capabilities, error) {
	return contracts.Capabilities{
		Detach:            false,
		Reattach:          false,
		RemoteTmux:        false,
		PersistentProcess: false,
		BinaryStreaming:   true,
	}, nil
}

// CloseAll closes every live transport. It is used on supervisor shutdown.
func (b *Backend) CloseAll() {
	b.mu.Lock()
	runs := make([]RunSession, 0, len(b.sessions))
	for _, s := range b.sessions {
		if s.run != nil {
			runs = append(runs, s.run)
		}
	}
	b.mu.Unlock()
	for _, run := range runs {
		_ = run.Close()
	}
}

func (b *Backend) runFor(id contracts.SessionID) (RunSession, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s := b.sessions[id]
	if s == nil {
		return nil, ErrSessionNotFound
	}
	if s.run == nil {
		return nil, ErrSessionClosed
	}
	return s.run, nil
}

func (b *Backend) emitStatus(s *session, phase contracts.SessionPhase, code string, message string) {
	b.mu.Lock()
	s.phase = phase
	s.seq++
	s.errorCode = code
	s.message = message
	status := s.status()
	handlers := make([]contracts.StatusHandler, 0, len(s.statusHandlers))
	for _, handler := range s.statusHandlers {
		handlers = append(handlers, handler)
	}
	b.mu.Unlock()

	for _, handler := range handlers {
		handler(status)
	}
}

func (b *Backend) emitData(s *session, data []byte) {
	copied := append([]byte(nil), data...)

	b.mu.Lock()
	if len(s.dataHandlers) == 0 {
		remaining := s.maxPending - len(s.pending)
		if remaining <= 0 {
			// Bound memory: drop the oldest bytes and keep the newest screen.
			drop := len(copied)
			if drop > len(s.pending) {
				drop = len(s.pending)
			}
			s.pending = s.pending[drop:]
			remaining = s.maxPending - len(s.pending)
		}
		if remaining > 0 {
			take := len(copied)
			if take > remaining {
				take = remaining
			}
			s.pending = append(s.pending, copied[:take]...)
		}
		b.mu.Unlock()
		return
	}

	handlers := make([]contracts.DataHandler, 0, len(s.dataHandlers))
	for _, handler := range s.dataHandlers {
		handlers = append(handlers, handler)
	}
	b.mu.Unlock()

	for _, handler := range handlers {
		handler(copied)
	}
}

func (s *session) status() contracts.SessionStatus {
	return contracts.SessionStatus{
		ID:        s.id,
		Phase:     s.phase,
		Seq:       s.seq,
		At:        time.Now().UTC(),
		ErrorCode: s.errorCode,
		Message:   s.message,
	}
}
