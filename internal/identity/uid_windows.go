//go:build windows

package identity

// userID is not meaningful on Windows; the IPC transport uses a named pipe.
func userID() int { return 0 }
