//go:build !windows

package ssh05

import (
	"os"
	"os/exec"
	"syscall"
)

// launchOpenTUI replaces the current process with the packaged OpenTUI
// frontend when it is available. It returns handled=false when ssh05-ui is not
// installed, so the classic TUI can run instead.
func launchOpenTUI() (handled bool, code int) {
	path, err := exec.LookPath("ssh05-ui")
	if err != nil {
		return false, 0
	}
	args := append([]string{path}, os.Args[1:]...)
	if err := syscall.Exec(path, args, os.Environ()); err != nil {
		return false, 0
	}
	return true, 0
}
