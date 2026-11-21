package supervisor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trzsz/trzsz-ssh/contracts"
	"github.com/trzsz/trzsz-ssh/internal/ipc"
)

// TestServiceSSHIntegration is an opt-in end-to-end check:
//
//	TRSS_TEST_SSH_ALIAS=trss-test TRSS_TEST_SSH_CONFIG=/tmp/config \
//	go test -run TestServiceSSHIntegration -v ./internal/supervisor
func TestServiceSSHIntegration(t *testing.T) {
	alias := os.Getenv("TRSS_TEST_SSH_ALIAS")
	if alias == "" {
		t.Skip("set TRSS_TEST_SSH_ALIAS to run the SSH service integration test")
	}
	configPath := os.Getenv("TRSS_TEST_SSH_CONFIG")

	backend := NewBackend(&SSHRunner{Config: SSHConfig{ConfigPath: configPath}}, Options{})
	service := NewService(backend, SSHHosts{ConfigPath: configPath})
	socket := filepath.Join(t.TempDir(), "run", "control.sock")
	server, err := ipc.Listen(socket, service)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
		<-done
		backend.CloseAll()
	})

	client, err := ipc.Dial(socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	output := make(chan string, 64)
	client.SetBinaryHandler(func(_ string, data []byte) {
		output <- string(data)
	})
	statuses := make(chan contracts.SessionStatus, 8)
	client.SetEventHandler(func(event contracts.Event) {
		var status contracts.SessionStatus
		if err := json.Unmarshal(event.Payload, &status); err == nil {
			statuses <- status
		}
	})

	callCtx, callCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer callCancel()

	hosts, err := ipc.CallTyped[contracts.HostsListParams, contracts.HostsListResult](
		callCtx, client, contracts.MethodHostsList, contracts.HostsListParams{},
	)
	if err != nil {
		t.Fatalf("hosts.list: %v", err)
	}
	found := false
	for _, host := range hosts.Hosts {
		if host.Alias == alias {
			found = true
		}
	}
	if !found {
		t.Fatalf("alias %q not in hosts.list", alias)
	}

	opened, err := ipc.CallTyped[contracts.SessionsOpenParams, contracts.SessionsOpenResult](
		callCtx, client, contracts.MethodSessionsOpen, contracts.SessionsOpenParams{HostProfileID: alias},
	)
	if err != nil {
		t.Fatalf("sessions.open: %v", err)
	}
	if _, err := ipc.CallTyped[contracts.SessionsResizeParams, struct{}](
		callCtx, client, contracts.MethodSessionsResize,
		contracts.SessionsResizeParams{SessionID: opened.SessionID, Cols: 100, Rows: 30},
	); err != nil {
		t.Fatalf("sessions.resize: %v", err)
	}

	if err := client.SendBinary(string(opened.SessionID), []byte("echo TRSS_SERVICE_OK; stty size\n")); err != nil {
		t.Fatalf("SendBinary: %v", err)
	}
	transcript := waitForOutput(t, output, "30 100")
	if !strings.Contains(transcript, "TRSS_SERVICE_OK") {
		t.Fatalf("command output missing from transcript: %q", transcript)
	}

	if _, err := ipc.CallTyped[contracts.SessionsResizeParams, struct{}](
		callCtx, client, contracts.MethodSessionsResize,
		contracts.SessionsResizeParams{SessionID: opened.SessionID, Cols: 90, Rows: 28},
	); err != nil {
		t.Fatalf("second resize: %v", err)
	}
	if err := client.SendBinary(string(opened.SessionID), []byte("stty size\n")); err != nil {
		t.Fatalf("SendBinary after resize: %v", err)
	}
	transcript += waitForOutput(t, output, "28 90")

	if _, err := ipc.CallTyped[contracts.SessionsCloseParams, struct{}](
		callCtx, client, contracts.MethodSessionsClose,
		contracts.SessionsCloseParams{SessionID: opened.SessionID, Intent: contracts.CloseView},
	); err != nil {
		t.Fatalf("sessions.close: %v", err)
	}
	select {
	case status := <-statuses:
		if status.Phase == contracts.PhaseClosed {
			return
		}
	case <-time.After(5 * time.Second):
		t.Fatal("closed status not received")
	}
}

func waitForOutput(t *testing.T, output <-chan string, marker string) string {
	t.Helper()
	var transcript strings.Builder
	deadline := time.After(10 * time.Second)
	for {
		select {
		case chunk := <-output:
			transcript.WriteString(chunk)
			if strings.Contains(transcript.String(), marker) {
				return transcript.String()
			}
		case <-deadline:
			t.Fatalf("timeout waiting for %q, transcript=%q", marker, transcript.String())
		}
	}
}
