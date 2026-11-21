package supervisor

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trzsz/trzsz-ssh/contracts"
)

type fakeRunner struct {
	mu          sync.Mutex
	runs        []*fakeRun
	starts      int
	startErr    error
	startErrs   []error
	lastOptions contracts.SessionOptions
}

func (r *fakeRunner) Start(_ context.Context, _ string, cols, rows int, options contracts.SessionOptions) (RunSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts++
	r.lastOptions = options
	if len(r.startErrs) > 0 {
		err := r.startErrs[0]
		r.startErrs = r.startErrs[1:]
		return nil, err
	}
	if r.startErr != nil {
		return nil, r.startErr
	}
	run := &fakeRun{
		output: make(chan []byte, 16),
		done:   make(chan struct{}),
		cols:   cols,
		rows:   rows,
	}
	r.runs = append(r.runs, run)
	return run, nil
}

func (r *fakeRunner) last() *fakeRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runs[len(r.runs)-1]
}

type fakeRun struct {
	output chan []byte
	done   chan struct{}
	once   sync.Once

	mu       sync.Mutex
	written  bytes.Buffer
	cols     int
	rows     int
	signals  []string
	closed   bool
	exitCode int
}

func (r *fakeRun) Output() <-chan []byte { return r.output }
func (r *fakeRun) Done() <-chan struct{} { return r.done }

func (r *fakeRun) ExitCode() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exitCode
}

func (r *fakeRun) Write(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrSessionClosed
	}
	r.written.Write(data)
	return nil
}

func (r *fakeRun) Resize(cols, rows int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrSessionClosed
	}
	r.cols, r.rows = cols, rows
	return nil
}

func (r *fakeRun) Signal(signal string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrSessionClosed
	}
	r.signals = append(r.signals, signal)
	return nil
}

func (r *fakeRun) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.finish(0)
	return nil
}

func (r *fakeRun) finish(code int) {
	r.once.Do(func() {
		r.mu.Lock()
		r.exitCode = code
		r.mu.Unlock()
		close(r.output)
		close(r.done)
	})
}

func (r *fakeRun) emit(data string) {
	r.output <- []byte(data)
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestBackendOpenDataWriteResizeClose(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{})
	ctx := context.Background()

	id, err := backend.Open(ctx, "host-1", "", contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if runner.starts != 1 {
		t.Fatalf("starts = %d", runner.starts)
	}
	run := runner.last()

	statuses := make(chan contracts.SessionStatus, 8)
	unsubscribeStatus := backend.OnStatus(id, func(status contracts.SessionStatus) {
		statuses <- status
	})
	defer unsubscribeStatus()
	if status := <-statuses; status.Phase != contracts.PhaseConnected {
		t.Fatalf("current status = %q", status.Phase)
	}

	data := make(chan string, 4)
	unsubscribeData := backend.OnData(id, func(bytes []byte) {
		data <- string(bytes)
	})
	defer unsubscribeData()

	run.emit("hello")
	select {
	case value := <-data:
		if value != "hello" {
			t.Fatalf("data = %q", value)
		}
	case <-time.After(time.Second):
		t.Fatal("data not delivered")
	}

	if err := backend.Write(ctx, id, []byte("input")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := run.written.String(); got != "input" {
		t.Fatalf("written = %q", got)
	}
	if err := backend.Resize(ctx, id, 120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if run.cols != 120 || run.rows != 40 {
		t.Fatalf("size = %dx%d", run.cols, run.rows)
	}
	if err := backend.Signal(ctx, id, "INT"); err != nil {
		t.Fatalf("Signal: %v", err)
	}
	run.mu.Lock()
	if len(run.signals) != 1 || run.signals[0] != "INT" {
		t.Fatalf("signals = %v", run.signals)
	}
	run.mu.Unlock()

	if err := backend.RequestClose(ctx, id, contracts.CloseView); err != nil {
		t.Fatalf("RequestClose: %v", err)
	}
	run.finish(0)

	deadline := time.After(2 * time.Second)
	for {
		select {
		case status := <-statuses:
			if status.Phase == contracts.PhaseClosed {
				return
			}
		case <-deadline:
			t.Fatal("closed status not emitted")
		}
	}
}

func TestBackendBuffersDataBeforeAttach(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{})
	ctx := context.Background()

	id, err := backend.Open(ctx, "host-1", "", contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	run := runner.last()
	run.emit("early")

	eventually(t, func() bool {
		backend.mu.RLock()
		defer backend.mu.RUnlock()
		return len(backend.sessions[id].pending) == len("early")
	})

	data := make(chan string, 1)
	unsubscribe := backend.OnData(id, func(bytes []byte) {
		data <- string(bytes)
	})
	defer unsubscribe()

	select {
	case value := <-data:
		if value != "early" {
			t.Fatalf("buffered data = %q", value)
		}
	case <-time.After(time.Second):
		t.Fatal("buffered data not delivered")
	}
}

func TestBackendRejectsUnsupportedDetach(t *testing.T) {
	backend := NewBackend(&fakeRunner{}, Options{})
	id, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	err = backend.RequestClose(context.Background(), id, contracts.DetachRemoteTmux)
	if !errors.Is(err, ErrUnsupportedIntent) {
		t.Fatalf("error = %v, want ErrUnsupportedIntent", err)
	}
}

func TestBackendBoundsSessions(t *testing.T) {
	backend := NewBackend(&fakeRunner{}, Options{MaxSessions: 1})
	if _, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{}); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if _, err := backend.Open(context.Background(), "host-2", "", contracts.SessionOptions{}); !errors.Is(err, ErrTooManySessions) {
		t.Fatalf("second Open error = %v, want ErrTooManySessions", err)
	}
}

func TestBackendClosedSessionReleasesSlot(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{MaxSessions: 1})
	id, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := backend.RequestClose(context.Background(), id, contracts.CloseView); err != nil {
		t.Fatalf("RequestClose: %v", err)
	}
	runner.last().finish(0)
	eventually(t, func() bool { return len(backend.List()) == 0 })
	if _, err := backend.Open(context.Background(), "host-2", "", contracts.SessionOptions{}); err != nil {
		t.Fatalf("Open after close: %v", err)
	}
}

func TestBackendStartFailureDoesNotLeakSlot(t *testing.T) {
	runner := &fakeRunner{startErr: errors.New("ssh unavailable")}
	backend := NewBackend(runner, Options{MaxSessions: 1})
	if _, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{}); err == nil {
		t.Fatal("Open must fail")
	}
	runner.mu.Lock()
	runner.startErr = nil
	runner.mu.Unlock()
	if _, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{}); err != nil {
		t.Fatalf("Open after failure: %v", err)
	}
}

func TestBackendUnknownSession(t *testing.T) {
	backend := NewBackend(&fakeRunner{}, Options{})
	if err := backend.Write(context.Background(), "missing", []byte("x")); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Write error = %v", err)
	}
	if err := backend.Resize(context.Background(), "missing", 80, 24); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Resize error = %v", err)
	}
	if err := backend.Attach(context.Background(), "missing", nil); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Attach error = %v", err)
	}
}

func TestBackendCapabilities(t *testing.T) {
	backend := NewBackend(&fakeRunner{}, Options{})
	caps, err := backend.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caps.Detach || caps.Reattach || caps.RemoteTmux || caps.PersistentProcess {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
	if !caps.BinaryStreaming {
		t.Fatal("BinaryStreaming must be true")
	}
}

func TestBackendPassesTmuxOptions(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{})
	id, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{TmuxSession: "trss-demo"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if id == "" {
		t.Fatal("empty session id")
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.lastOptions.TmuxSession != "trss-demo" {
		t.Fatalf("tmux option = %q", runner.lastOptions.TmuxSession)
	}
	infos := backend.List()
	if len(infos) != 1 || infos[0].ID != id || infos[0].ProfileID != "host-1" || infos[0].TmuxSession != "trss-demo" {
		t.Fatalf("List = %+v", infos)
	}
}

func TestBackendLargeOutputIsBoundedBeforeAttach(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{MaxPending: 1 << 20})
	id, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	run := runner.last()

	const total = 100 << 20
	chunk := bytes.Repeat([]byte("x"), 32<<10)
	for written := 0; written < total; written += len(chunk) {
		run.emit(string(chunk))
	}
	run.finish(0)

	eventually(t, func() bool {
		backend.mu.RLock()
		defer backend.mu.RUnlock()
		pending := len(backend.sessions[id].pending)
		return pending > 0 && pending <= (1<<20)
	})
}

func TestBackendLargeOutputReachesHandler(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{})
	id, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	run := runner.last()

	var received atomic.Int64
	unsubscribe := backend.OnData(id, func(data []byte) {
		received.Add(int64(len(data)))
	})
	defer unsubscribe()

	const total = 100 << 20
	chunk := bytes.Repeat([]byte("x"), 32<<10)
	for written := 0; written < total; written += len(chunk) {
		run.emit(string(chunk))
	}
	run.finish(0)

	eventually(t, func() bool {
		return received.Load() >= total
	})
}

func TestBackendReconnectsWithBackoff(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{ReconnectMax: 2, ReconnectBase: time.Millisecond})
	id, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	statuses := make(chan contracts.SessionStatus, 16)
	unsubscribe := backend.OnStatus(id, func(status contracts.SessionStatus) { statuses <- status })
	defer unsubscribe()

	runner.last().finish(1)
	eventually(t, func() bool {
		runner.mu.Lock()
		defer runner.mu.Unlock()
		return len(runner.runs) == 2
	})

	sawReconnecting := false
	sawConnected := false
	deadline := time.After(2 * time.Second)
	for !sawReconnecting || !sawConnected {
		select {
		case status := <-statuses:
			if status.Phase == contracts.PhaseReconnecting {
				sawReconnecting = true
			}
			if status.Phase == contracts.PhaseConnected {
				sawConnected = true
			}
		case <-deadline:
			t.Fatalf("reconnect statuses not observed: reconnecting=%v connected=%v", sawReconnecting, sawConnected)
		}
	}

	runner.mu.Lock()
	second := runner.runs[1]
	runner.mu.Unlock()
	second.emit("after-reconnect")
	second.finish(0)
	eventually(t, func() bool { return len(backend.List()) == 0 })
}

func TestBackendReconnectExhaustionFails(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{ReconnectMax: 2, ReconnectBase: time.Millisecond})
	id, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	runner.mu.Lock()
	runner.startErrs = []error{errors.New("down"), errors.New("down")}
	runner.mu.Unlock()
	statuses := make(chan contracts.SessionStatus, 16)
	unsubscribe := backend.OnStatus(id, func(status contracts.SessionStatus) { statuses <- status })
	defer unsubscribe()

	runner.last().finish(1)
	eventually(t, func() bool { return len(backend.List()) == 0 })

	sawFailed := false
	for !sawFailed {
		select {
		case status := <-statuses:
			if status.Phase == contracts.PhaseFailed {
				sawFailed = true
			}
		case <-time.After(2 * time.Second):
			t.Fatal("failed status not observed")
		}
	}
}

func TestBackendReconnectCanBeCancelled(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{ReconnectMax: 3, ReconnectBase: 40 * time.Millisecond})
	id, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	statuses := make(chan contracts.SessionStatus, 16)
	unsubscribe := backend.OnStatus(id, func(status contracts.SessionStatus) { statuses <- status })
	defer unsubscribe()

	runner.last().finish(1)
	for {
		select {
		case status := <-statuses:
			if status.Phase == contracts.PhaseReconnecting {
				if err := backend.RequestClose(context.Background(), id, contracts.CloseView); err != nil {
					t.Fatalf("RequestClose: %v", err)
				}
				eventually(t, func() bool { return len(backend.List()) == 0 })
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatal("reconnecting status not observed")
		}
	}
}
