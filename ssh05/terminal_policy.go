package ssh05

import (
	"errors"
	"sync/atomic"
)

// nonInteractiveLogins counts active logins that must never read /dev/tty.
// The supervisor is the only caller that enables it, but the counter keeps
// concurrent sessions and library users independent.
var nonInteractiveLogins atomic.Int32

var errNonInteractive = errors.New("ssh05: interactive terminal input is disabled")

func enterNonInteractive(enabled bool) func() {
	if !enabled {
		return func() {}
	}
	nonInteractiveLogins.Add(1)
	return func() { nonInteractiveLogins.Add(-1) }
}

func terminalInputAllowed() bool {
	return nonInteractiveLogins.Load() == 0
}
