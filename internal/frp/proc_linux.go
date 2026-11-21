//go:build linux

package frp

import (
	"os/exec"
	"syscall"
)

// configureProcess puts frpc/frps in their own process group and asks the
// kernel to kill them if the supervisor dies unexpectedly.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
}
