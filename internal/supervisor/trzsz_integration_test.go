package supervisor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lordpietre/ssh05/contracts"
)

// TestSessionTrzszIntegration is opt-in because it needs a reachable SSH alias
// whose remote PATH includes the trzsz `tsz` tool.
//
//	SSH05_TEST_SSH_ALIAS=ssh05-test \
//	SSH05_TEST_SSH_CONFIG=/tmp/ssh05-ssh-config \
//	SSH05_TEST_TRZSZ_DOWNLOAD=/tmp/ssh05-trzsz-download \
//	go test -run TestSessionTrzszIntegration -v ./internal/supervisor
func TestSessionTrzszIntegration(t *testing.T) {
	alias := os.Getenv("SSH05_TEST_SSH_ALIAS")
	if alias == "" {
		t.Skip("set SSH05_TEST_SSH_ALIAS to run the trzsz integration test")
	}
	downloadDir := os.Getenv("SSH05_TEST_TRZSZ_DOWNLOAD")
	if downloadDir == "" {
		t.Skip("set SSH05_TEST_TRZSZ_DOWNLOAD to run the trzsz integration test")
	}
	if err := os.MkdirAll(downloadDir, 0o700); err != nil {
		t.Fatalf("create download dir: %v", err)
	}
	configPath := os.Getenv("SSH05_TEST_SSH_CONFIG")

	runner := &SSHRunner{Config: SSHConfig{
		ConfigPath:   configPath,
		Terminal:     "xterm-256color",
		Trzsz:        true,
		DownloadPath: downloadDir,
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	run, err := runner.Start(ctx, alias, 100, 30, contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer run.Close()

	name := fmt.Sprintf("ssh05-trzsz-%d.txt", time.Now().UnixNano())
	remote := filepath.Join(os.TempDir(), name)
	payload := "SSH05_TRZSZ_OK"
	script := fmt.Sprintf("printf '%s' > %s && tsz -q %s\n", payload, remote, remote)
	if err := run.Write([]byte(script)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	local := filepath.Join(downloadDir, name)
	deadline := time.After(20 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var output strings.Builder
	for {
		select {
		case chunk, ok := <-run.Output():
			if ok {
				output.Write(chunk)
			}
		case <-ticker.C:
			data, readErr := os.ReadFile(local)
			if readErr == nil && string(data) == payload {
				_ = os.Remove(remote)
				return
			}
		case <-deadline:
			t.Fatalf("trzsz download timed out; output: %q", output.String())
		}
	}
}
