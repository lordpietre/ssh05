//go:build windows

package ssh05

import (
	"errors"
	"os"
	"os/exec"
)

// launchOpenTUI runs the packaged OpenTUI frontend when it is available. It
// returns handled=false when ssh05-ui is not installed, so the classic TUI can
// run instead.
func launchOpenTUI() (handled bool, code int) {
	path, err := exec.LookPath("ssh05-ui.exe")
	if err != nil {
		path, err = exec.LookPath("ssh05-ui")
		if err != nil {
			return false, 0
		}
	}
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return true, exitErr.ExitCode()
		}
		return true, 1
	}
	return true, 0
}
