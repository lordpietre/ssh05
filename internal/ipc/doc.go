// Package ipc implements the local, authenticated control transport for ssh05.
//
// It uses a Unix domain socket, a length-prefixed JSON control frame and a
// separate binary frame for PTY bytes. The package deliberately does not know
// about SSH or the UI: a supervisor supplies a Handler and a UI (or test code)
// uses Client.
//
// Security properties:
//   - the socket directory is forced to 0700 and the socket to 0600;
//   - Linux peers are checked with SO_PEERCRED (same UID only);
//   - frames are bounded (1 MiB JSON, 4 MiB binary);
//   - handler panics become a generic internal error without leaking values;
//   - only methods declared in contracts.Method are accepted (no shell.exec).
package ipc
