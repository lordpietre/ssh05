//go:build linux

package frp

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestConfigureProcessSetsDeathSignalOnLinux(t *testing.T) {
	cmd := exec.Command("true")
	configureProcess(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid || cmd.SysProcAttr.Pdeathsig != syscall.SIGKILL {
		t.Fatalf("SysProcAttr = %+v", cmd.SysProcAttr)
	}
}
