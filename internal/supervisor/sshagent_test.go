package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSSHAgentReadsOnlyFingerprints(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho '256 SHA256:abc123 comment (ED25519)'\necho '256 SHA256:def456 other (RSA)'\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh-add"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("SSH_AUTH_SOCK", "/run/user/1000/kpxc-ssh-agent.sock")

	info := detectSSHAgent(context.Background())
	if info.Provider != "keepassxc" || info.Identities != 2 || info.Message != "" {
		t.Fatalf("info = %+v", info)
	}
	if info.Socket != "/run/user/1000/kpxc-ssh-agent.sock" {
		t.Fatalf("socket = %q", info.Socket)
	}
}

func TestDetectSSHAgentWithoutSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	info := detectSSHAgent(context.Background())
	if info.Provider != "none" || info.Identities != 0 || info.Message == "" {
		t.Fatalf("info = %+v", info)
	}
}
