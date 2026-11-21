//go:build !windows

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// DefaultSocketPath returns $XDG_RUNTIME_DIR/trss/control.sock when available,
// otherwise a per-user directory under os.TempDir().
func DefaultSocketPath() string {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "trss", "control.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("trss-%d", os.Getuid()), "control.sock")
}

func listenUnix(path string) (net.Listener, error) {
	if path == "" {
		return nil, fmt.Errorf("ipc: empty socket path")
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("ipc: create socket dir: %w", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, fmt.Errorf("ipc: secure socket dir: %w", err)
		}
	}

	if _, err := os.Stat(path); err == nil {
		conn, dialErr := net.DialTimeout("unix", path, 200*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, ErrAlreadyRunning
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("ipc: remove stale socket: %w", err)
		}
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen: %w", err)
	}
	if unixListener, ok := ln.(*net.UnixListener); ok {
		unixListener.SetUnlinkOnClose(true)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("ipc: secure socket: %w", err)
	}
	return ln, nil
}
