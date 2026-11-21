// Package audit writes sanitized operational audit records.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event is deliberately explicit: there is no free-form payload where a
// caller could accidentally log a password or PTY bytes.
type Event struct {
	Time          time.Time `json:"time"`
	Action        string    `json:"action"`
	HostProfileID string    `json:"hostProfileId,omitempty"`
	SessionID     string    `json:"sessionId,omitempty"`
	Result        string    `json:"result"`
	ErrorCode     string    `json:"errorCode,omitempty"`
	Detail        string    `json:"detail,omitempty"`
}

// Logger appends JSON lines with rotation. A nil Logger is a no-op.
type Logger struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	maxFiles int
}

// New creates a logger at path. The parent directory is forced to 0700 and the
// file to 0600. An empty path returns nil (disabled).
func New(path string, maxBytes int64) (*Logger, error) {
	return NewRotating(path, maxBytes, 3)
}

// NewRotating keeps maxFiles total files (current plus .1 .. .N). A value <= 0
// defaults to 3; a value of 1 keeps only the current file.
func NewRotating(path string, maxBytes int64, maxFiles int) (*Logger, error) {
	if path == "" {
		return nil, nil
	}
	if maxFiles <= 0 {
		maxFiles = 3
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	if maxBytes <= 0 {
		maxBytes = 10 << 20
	}
	logger := &Logger{path: path, maxBytes: maxBytes, maxFiles: maxFiles}
	if err := logger.ensureFile(); err != nil {
		return nil, err
	}
	return logger, nil
}

func (l *Logger) ensureFile() error {
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(l.path, 0o600)
}

// Log appends one event. It never returns an error to callers: audit failures
// must not break a session, but they are also not silently persisted.
func (l *Logger) rotateLocked() {
	if l.maxFiles <= 1 {
		_ = os.Remove(l.path)
		return
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", l.path, l.maxFiles-1))
	for index := l.maxFiles - 2; index >= 1; index-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", l.path, index), fmt.Sprintf("%s.%d", l.path, index+1))
	}
	_ = os.Rename(l.path, l.path+".1")
}

func (l *Logger) Log(event Event) {
	if l == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if info, err := os.Stat(l.path); err == nil && info.Size()+int64(len(data)) > l.maxBytes {
		l.rotateLocked()
		if err := l.ensureFile(); err != nil {
			return
		}
	}
	file, err := os.OpenFile(l.path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_, _ = file.Write(data)
	_ = file.Close()
}
