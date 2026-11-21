//go:build !windows && !linux

package frp

import (
	"os/exec"
	"syscall"
)

// configureProcess puts frpc/frps in their own process group so the supervisor
// can terminate them and their children as a unit.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
