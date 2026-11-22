// Package transfer runs typed SCP/SFTP transfers without a shell.
package transfer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// State is the observable lifecycle of a transfer.
type State string

const (
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Request is a validated transfer request. Target is an SSH destination
// (alias or user@host); Port is optional.
type Request struct {
	Protocol   string
	Direction  string
	Target     string
	Port       int
	LocalPath  string
	RemotePath string
	Recursive  bool
}

// Info is a non-secret transfer snapshot.
type Info struct {
	ID         string    `json:"transferId"`
	Protocol   string    `json:"protocol"`
	Direction  string    `json:"direction"`
	Target     string    `json:"target"`
	LocalPath  string    `json:"localPath"`
	RemotePath string    `json:"remotePath"`
	Recursive  bool      `json:"recursive"`
	State      State     `json:"state"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	ErrorCode  string    `json:"errorCode,omitempty"`
	ErrorLabel string    `json:"errorLabel,omitempty"`
}

// Options configures the transfer manager.
type Options struct {
	SSHConfig string
	ScpBin    string
	SftpBin   string
	MaxActive int
	KeepDone  int
}

// Manager owns local scp/sftp processes.
type Manager struct {
	mu     sync.Mutex
	opts   Options
	items  map[string]*transfer
	order  []string
	procs  map[string]*exec.Cmd
	next   atomic.Uint64
	closed bool
}

type transfer struct {
	info   Info
	cmd    *exec.Cmd
	stderr *boundedBuffer
}

// NewManager creates a transfer manager.
func NewManager(options Options) *Manager {
	if strings.TrimSpace(options.ScpBin) == "" {
		options.ScpBin = "scp"
	}
	if strings.TrimSpace(options.SftpBin) == "" {
		options.SftpBin = "sftp"
	}
	if options.MaxActive <= 0 {
		options.MaxActive = 4
	}
	if options.KeepDone <= 0 {
		options.KeepDone = 50
	}
	return &Manager{
		opts:  options,
		items: make(map[string]*transfer),
		procs: make(map[string]*exec.Cmd),
	}
}

// Start validates and launches one transfer.
func (m *Manager) Start(ctx context.Context, request Request) (Info, error) {
	request.Protocol = strings.ToLower(strings.TrimSpace(request.Protocol))
	request.Direction = strings.ToLower(strings.TrimSpace(request.Direction))
	if request.Protocol != "scp" && request.Protocol != "sftp" {
		if request.Protocol == "trzsz" || request.Protocol == "zmodem" {
			return Info{}, fmt.Errorf("transfer protocol %q is not enabled in this build", request.Protocol)
		}
		return Info{}, fmt.Errorf("unsupported transfer protocol %q", request.Protocol)
	}
	if request.Direction != "upload" && request.Direction != "download" {
		return Info{}, fmt.Errorf("invalid transfer direction %q", request.Direction)
	}
	target := strings.TrimSpace(request.Target)
	if target == "" || strings.HasPrefix(target, "-") || strings.ContainsAny(target, " \t\r\n") {
		return Info{}, fmt.Errorf("invalid transfer target %q", request.Target)
	}
	for name, value := range map[string]string{
		"local path":  request.LocalPath,
		"remote path": request.RemotePath,
	} {
		if strings.TrimSpace(value) == "" {
			return Info{}, fmt.Errorf("%s is required", name)
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return Info{}, fmt.Errorf("%s contains an invalid character", name)
		}
	}
	if request.Protocol == "scp" && !scpSafeRemotePath(request.RemotePath) {
		return Info{}, fmt.Errorf("scp remote path contains unsupported characters; use sftp for spaces or metacharacters")
	}
	local, err := resolveLocalPath(request.LocalPath)
	if err != nil {
		return Info{}, err
	}
	request.LocalPath = local
	if request.Direction == "upload" {
		info, statErr := os.Stat(local)
		if statErr != nil {
			return Info{}, fmt.Errorf("local path: %w", statErr)
		}
		if info.IsDir() && !request.Recursive {
			return Info{}, fmt.Errorf("local path is a directory; enable recursive")
		}
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Info{}, fmt.Errorf("transfer manager is closed")
	}
	active := 0
	for _, item := range m.items {
		if item.info.State == StateRunning {
			active++
		}
	}
	if active >= m.opts.MaxActive {
		m.mu.Unlock()
		return Info{}, fmt.Errorf("too many active transfers (max %d)", m.opts.MaxActive)
	}
	id := fmt.Sprintf("xfer-%d", m.next.Add(1))
	item := &transfer{
		info: Info{
			ID:         id,
			Protocol:   request.Protocol,
			Direction:  request.Direction,
			Target:     target,
			LocalPath:  local,
			RemotePath: request.RemotePath,
			Recursive:  request.Recursive,
			State:      StateRunning,
			StartedAt:  time.Now().UTC(),
		},
		stderr: &boundedBuffer{limit: 4096},
	}
	m.items[id] = item
	m.order = append(m.order, id)
	m.mu.Unlock()

	command, args, stdin := m.build(request)
	cmd := exec.CommandContext(ctx, command, args...)
	configureProcess(cmd)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	cmd.Stderr = item.stderr
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		m.finish(item, StateCancelled, "closed", "supervisor is shutting down")
		return m.snapshot(id), fmt.Errorf("transfer manager is closed")
	}
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		m.finish(item, StateFailed, "start_failed", err.Error())
		return m.snapshot(id), err
	}
	item.cmd = cmd
	m.procs[id] = cmd
	m.mu.Unlock()
	go m.wait(id, item, cmd)
	return m.snapshot(id), nil
}

func (m *Manager) build(request Request) (string, []string, string) {
	if request.Protocol == "scp" {
		args := []string{"-B", "-o", "BatchMode=yes"}
		if m.opts.SSHConfig != "" {
			args = append(args, "-F", m.opts.SSHConfig)
		}
		if request.Port > 0 {
			args = append(args, "-P", fmt.Sprintf("%d", request.Port))
		}
		if request.Recursive {
			args = append(args, "-r")
		}
		args = append(args, "--")
		remote := request.Target + ":" + request.RemotePath
		if request.Direction == "upload" {
			args = append(args, request.LocalPath, remote)
		} else {
			args = append(args, remote, request.LocalPath)
		}
		return m.opts.ScpBin, args, ""
	}

	args := []string{"-q", "-b", "-", "-o", "BatchMode=yes"}
	if m.opts.SSHConfig != "" {
		args = append(args, "-F", m.opts.SSHConfig)
	}
	if request.Port > 0 {
		args = append(args, "-P", fmt.Sprintf("%d", request.Port))
	}
	args = append(args, request.Target)
	operation := "put"
	if request.Direction == "download" {
		operation = "get"
	}
	if request.Recursive {
		operation += " -r"
	}
	first, second := request.LocalPath, request.RemotePath
	if request.Direction == "download" {
		first, second = request.RemotePath, request.LocalPath
	}
	batch := fmt.Sprintf("%s %s %s\n", operation, sftpQuote(first), sftpQuote(second))
	return m.opts.SftpBin, args, batch
}

func scpSafeRemotePath(value string) bool {
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("._/-~", char) {
			continue
		}
		return false
	}
	return true
}

func sftpQuote(value string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func (m *Manager) wait(id string, item *transfer, cmd *exec.Cmd) {
	err := cmd.Wait()
	m.mu.Lock()
	if item.cmd != cmd {
		m.mu.Unlock()
		return
	}
	delete(m.procs, id)
	state := item.info.State
	message := strings.TrimSpace(item.stderr.String())
	m.mu.Unlock()
	if state == StateCancelled {
		return
	}
	if err != nil {
		if message == "" {
			message = err.Error()
		}
		m.finish(item, StateFailed, "transfer_failed", message)
		return
	}
	m.finish(item, StateCompleted, "", "")
}

func (m *Manager) finish(item *transfer, state State, code, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item.info.State = state
	item.info.FinishedAt = time.Now().UTC()
	item.info.ErrorCode = code
	item.info.ErrorLabel = message
	m.pruneLocked()
}

func (m *Manager) snapshot(id string) Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item := m.items[id]; item != nil {
		return item.info
	}
	return Info{ID: id}
}

func (m *Manager) pruneLocked() {
	finished := make([]string, 0, len(m.order))
	for _, id := range m.order {
		if item := m.items[id]; item != nil && item.info.State != StateRunning {
			finished = append(finished, id)
		}
	}
	for len(finished) > m.opts.KeepDone {
		id := finished[0]
		finished = finished[1:]
		delete(m.items, id)
	}
	if len(m.order) > m.opts.KeepDone*2 {
		kept := make([]string, 0, len(m.order))
		for _, id := range m.order {
			if _, ok := m.items[id]; ok {
				kept = append(kept, id)
			}
		}
		m.order = kept
	}
}

// List returns newest first.
func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.order))
	for index := len(m.order) - 1; index >= 0; index-- {
		if item := m.items[m.order[index]]; item != nil {
			out = append(out, item.info)
		}
	}
	return out
}

// Cancel stops one running transfer.
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	item := m.items[id]
	cmd := m.procs[id]
	if item == nil {
		m.mu.Unlock()
		return fmt.Errorf("transfer not found")
	}
	if item.info.State != StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("transfer is not running")
	}
	item.info.State = StateCancelled
	item.info.FinishedAt = time.Now().UTC()
	delete(m.procs, id)
	m.mu.Unlock()
	if cmd != nil {
		killProcess(cmd)
	}
	return nil
}

// Close cancels every running transfer.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	ids := make([]string, 0, len(m.procs))
	for id := range m.procs {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Cancel(id)
	}
}

type boundedBuffer struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

func resolveLocalPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, value[2:])
	}
	if !filepath.IsAbs(value) {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return "", err
		}
		return absolute, nil
	}
	return value, nil
}
