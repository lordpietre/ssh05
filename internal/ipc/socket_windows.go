//go:build windows

package ipc

import "net"

// DefaultSocketPath is reserved for the future Windows named-pipe transport.
func DefaultSocketPath() string {
	return `\\.\pipe\trss-control`
}

func listenUnix(path string) (net.Listener, error) {
	return nil, ErrUnsupported
}
