//go:build !windows

package transfer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func testManager(t *testing.T, scpScript, sftpScript string) *Manager {
	t.Helper()
	dir := t.TempDir()
	if scpScript == "" {
		scpScript = writeScript(t, dir, "scp", "exit 0\n")
	}
	if sftpScript == "" {
		sftpScript = writeScript(t, dir, "sftp", "cat >/dev/null\nexit 0\n")
	}
	manager := NewManager(Options{ScpBin: scpScript, SftpBin: sftpScript, MaxActive: 2, KeepDone: 5})
	t.Cleanup(manager.Close)
	return manager
}

func waitState(t *testing.T, manager *Manager, id string, want State) Info {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, item := range manager.List() {
			if item.ID == id {
				if item.State == want {
					return item
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("transfer %s did not reach %s: %+v", id, want, manager.List())
	return Info{}
}

func TestSCPUploadAndDownloadArgs(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	scp := writeScript(t, dir, "scp", "printf '%s\\n' \"$*\" > "+log+"\nexit 0\n")
	manager := testManager(t, scp, "")
	local := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	started, err := manager.Start(context.Background(), Request{
		Protocol: "scp", Direction: "upload", Target: "lab", Port: 2222,
		LocalPath: local, RemotePath: "/tmp/file.txt",
	})
	if err != nil {
		t.Fatalf("Start upload: %v", err)
	}
	waitState(t, manager, started.ID, StateCompleted)
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"-B", "BatchMode=yes", "-P 2222", "--", local, "lab:/tmp/file.txt"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("args missing %q: %s", expected, data)
		}
	}

	started, err = manager.Start(context.Background(), Request{
		Protocol: "scp", Direction: "download", Target: "lab", LocalPath: local, RemotePath: "/tmp/file.txt",
	})
	if err != nil {
		t.Fatalf("Start download: %v", err)
	}
	waitState(t, manager, started.ID, StateCompleted)
	data, _ = os.ReadFile(log)
	if !strings.Contains(string(data), "lab:/tmp/file.txt "+local) {
		t.Fatalf("download args = %s", data)
	}
}

func TestSFTPBatchUsesStdin(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	batchLog := filepath.Join(dir, "batch.log")
	sftp := writeScript(t, dir, "sftp",
		"printf '%s\\n' \"$*\" > "+log+"\ncat > "+batchLog+"\nexit 0\n")
	manager := testManager(t, "", sftp)
	local := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	started, err := manager.Start(context.Background(), Request{
		Protocol: "sftp", Direction: "upload", Target: "lab", LocalPath: local, RemotePath: "/tmp/x y",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, manager, started.ID, StateCompleted)
	args, _ := os.ReadFile(log)
	batch, _ := os.ReadFile(batchLog)
	if !strings.Contains(string(args), "-b -") || !strings.Contains(string(args), "lab") {
		t.Fatalf("sftp args = %s", args)
	}
	if !strings.Contains(string(batch), `put "`+local+`" "/tmp/x y"`) {
		t.Fatalf("batch = %s", batch)
	}
}

func TestTransferFailureKeepsBoundedMessage(t *testing.T) {
	dir := t.TempDir()
	scp := writeScript(t, dir, "scp", "echo 'permission denied' >&2\nexit 1\n")
	manager := testManager(t, scp, "")
	local := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	started, err := manager.Start(context.Background(), Request{
		Protocol: "scp", Direction: "upload", Target: "lab", LocalPath: local, RemotePath: "/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	item := waitState(t, manager, started.ID, StateFailed)
	if item.ErrorCode != "transfer_failed" || !strings.Contains(item.ErrorLabel, "permission denied") {
		t.Fatalf("failure = %+v", item)
	}
}

func TestTransferCancelKillsProcess(t *testing.T) {
	dir := t.TempDir()
	scp := writeScript(t, dir, "scp", "sleep 30\n")
	manager := testManager(t, scp, "")
	local := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	started, err := manager.Start(context.Background(), Request{
		Protocol: "scp", Direction: "upload", Target: "lab", LocalPath: local, RemotePath: "/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := manager.Cancel(started.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	item := waitState(t, manager, started.ID, StateCancelled)
	if item.FinishedAt.IsZero() {
		t.Fatalf("cancel did not set finishedAt: %+v", item)
	}
}

func TestTransferValidationAndLimits(t *testing.T) {
	manager := testManager(t, "", "")
	local := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), Request{Protocol: "trzsz", Direction: "upload", Target: "lab", LocalPath: local, RemotePath: "/x"}); err == nil {
		t.Fatal("trzsz must be reported as unavailable")
	}
	if _, err := manager.Start(context.Background(), Request{Protocol: "scp", Direction: "upload", Target: "-oProxyCommand=x", LocalPath: local, RemotePath: "/x"}); err == nil {
		t.Fatal("option-like target must fail")
	}
	if _, err := manager.Start(context.Background(), Request{Protocol: "scp", Direction: "upload", Target: "lab", LocalPath: filepath.Join(filepath.Dir(local), "missing"), RemotePath: "/x"}); err == nil {
		t.Fatal("missing local file must fail")
	}
	if _, err := manager.Start(context.Background(), Request{Protocol: "scp", Direction: "upload", Target: "lab", LocalPath: filepath.Dir(local), RemotePath: "/x"}); err == nil {
		t.Fatal("directory without recursive must fail")
	}
	if _, err := manager.Start(context.Background(), Request{Protocol: "scp", Direction: "upload", Target: "lab", LocalPath: local, RemotePath: "/x; touch /tmp/pwned"}); err == nil {
		t.Fatal("unsafe scp remote path must fail")
	}
	if _, err := manager.Start(context.Background(), Request{Protocol: "sftp", Direction: "upload", Target: "lab", LocalPath: local, RemotePath: "/x with spaces"}); err != nil {
		t.Fatalf("sftp with spaces must be accepted: %v", err)
	}
}
