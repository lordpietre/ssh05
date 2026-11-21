package supervisor

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// agentInfo is a non-secret view of the SSH agent in use.
type agentInfo struct {
	Socket     string
	Provider   string
	Identities int
	Message    string
}

// detectSSHAgent inspects SSH_AUTH_SOCK and `ssh-add -l`. It never reads or
// exports private key material: only socket metadata and fingerprints.
func detectSSHAgent(ctx context.Context) agentInfo {
	socket := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	info := agentInfo{Socket: socket, Provider: "none"}
	if socket == "" {
		info.Message = "SSH_AUTH_SOCK no definido"
		return info
	}
	lowerSocket := strings.ToLower(socket)
	switch {
	case strings.Contains(lowerSocket, "kpxc"), strings.Contains(lowerSocket, "keepass"):
		info.Provider = "keepassxc"
	case os.Getenv("KEEPASSXC_SSH_AGENT") != "":
		info.Provider = "keepassxc"
	default:
		info.Provider = "openssh"
	}

	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, "ssh-add", "-l").CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err == nil {
		info.Identities = countIdentities(text)
		return info
	}
	if strings.Contains(strings.ToLower(text), "the agent has no identities") {
		info.Message = "el agente no tiene identidades"
		return info
	}
	info.Message = text
	if info.Message == "" {
		info.Message = err.Error()
	}
	return info
}

func countIdentities(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
