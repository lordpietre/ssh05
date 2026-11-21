//go:build !windows

package frp

import (
	"os/exec"
	"syscall"
)

// configureProcess puts frpc in its own process group so the supervisor can
// terminate it and any children as a unit.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcess terminates the whole process group, avoiding orphans.
func killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
