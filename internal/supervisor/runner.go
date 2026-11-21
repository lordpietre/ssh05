package supervisor

import "context"

// Runner starts one concrete transport/session. The supervisor does not care
// whether it is backed by the in-repo Go SSH client or an external adapter.
type Runner interface {
	Start(ctx context.Context, profileID string, cols, rows int) (RunSession, error)
}

// RunSession is the transport-level view of one open session.
type RunSession interface {
	Output() <-chan []byte
	Done() <-chan struct{}
	ExitCode() int
	Write(data []byte) error
	Resize(cols, rows int) error
	Signal(signal string) error
	Close() error
}
