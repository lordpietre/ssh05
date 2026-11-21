// Package supervisor implements the TRSS application service: session
// lifecycle, PTY data fan-out and the typed IPC method handler.
//
// It is transport-oriented, not a UI: Backend implements
// contracts.SessionBackend, SSHRunner starts the in-repo Go SSH client and
// Service maps contracts methods onto that backend. The daemon entry point is
// cmd/trssd.
package supervisor
