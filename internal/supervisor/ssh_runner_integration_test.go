package supervisor

import (
	"context"
	"github.com/trzsz/trzsz-ssh/contracts"
	"os"
	"strings"
	"testing"
	"time"
)

// TestSSHRunnerIntegration is opt-in because it needs a reachable SSH alias.
//
//	TRSS_TEST_SSH_ALIAS=trss-test \
//	TRSS_TEST_SSH_CONFIG=/tmp/trss-ssh-config \
//	go test -run TestSSHRunnerIntegration -v ./internal/supervisor
func TestSSHRunnerIntegration(t *testing.T) {
	alias := os.Getenv("TRSS_TEST_SSH_ALIAS")
	if alias == "" {
		t.Skip("set TRSS_TEST_SSH_ALIAS to run the SSH integration test")
	}
	configPath := os.Getenv("TRSS_TEST_SSH_CONFIG")

	runner := &SSHRunner{Config: SSHConfig{ConfigPath: configPath}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	run, err := runner.Start(ctx, alias, 100, 30, contracts.SessionOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer run.Close()

	if err := run.Write([]byte("echo TRSS_SUPERVISOR_OK; stty size\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var output strings.Builder
	deadline := time.After(10 * time.Second)
	for {
		select {
		case chunk, ok := <-run.Output():
			if !ok {
				t.Fatalf("output closed early: %q", output.String())
			}
			output.Write(chunk)
			if strings.Contains(output.String(), "TRSS_SUPERVISOR_OK") &&
				strings.Contains(output.String(), "30 100") {
				if err := run.Resize(90, 28); err != nil {
					t.Fatalf("Resize: %v", err)
				}
				if err := run.Write([]byte("stty size\n")); err != nil {
					t.Fatalf("Write after resize: %v", err)
				}
				for {
					select {
					case chunk, ok := <-run.Output():
						if !ok {
							t.Fatalf("output closed during resize check: %q", output.String())
						}
						output.Write(chunk)
						if strings.Contains(output.String(), "28 90") {
							return
						}
					case <-time.After(5 * time.Second):
						t.Fatalf("resize did not reach the remote PTY: %q", output.String())
					}
				}
			}
		case <-deadline:
			t.Fatalf("timeout waiting for SSH output: %q", output.String())
		}
	}
}
