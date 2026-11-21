//go:build !windows

package frp

import (
	"os/exec"
	"syscall"
)

// killProcess terminates the whole process group, avoiding orphans.
func killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
