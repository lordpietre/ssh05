package frp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/trzsz/trzsz-ssh/contracts"
)

func fakeFrpc(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frpc")
	script := "#!/bin/sh\nif [ \"$1\" = \"verify\" ]; then exit 0; fi\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	manager, err := NewManager(Options{
		ConfigPath: filepath.Join(dir, "frp.json"), KeyPath: filepath.Join(dir, "frp.key"),
		WorkDir: filepath.Join(dir, "work"), FrpcBin: fakeFrpc(t),
		EndpointAddress: "127.0.0.1", EndpointPort: 7000, EndpointToken: "endpoint-token",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Close)
	return manager
}

func TestManagerApplyStopAndEncrypts(t *testing.T) {
	manager := testManager(t)
	ctx := context.Background()
	applied, err := manager.Apply(ctx, contracts.FrpProxy{
		Name: "ssh", Type: contracts.FrpTCP, ServicePort: 22, RemotePort: 22022,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if applied.State != contracts.FrpOnline || applied.ID == "" {
		t.Fatalf("applied = %+v", applied)
	}
	config, err := os.ReadFile(filepath.Join(manager.opts.WorkDir, applied.ID+".toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "remotePort = 22022") {
		t.Fatalf("config = %s", config)
	}
	store, err := os.ReadFile(manager.opts.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(store), "endpoint-token") || strings.Contains(string(store), "secret") {
		t.Fatalf("secret in clear: %s", store)
	}
	if err := manager.Stop(ctx, applied.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	list := manager.List()
	if list.Proxies[0].State != contracts.FrpStopped {
		t.Fatalf("state = %s", list.Proxies[0].State)
	}
}

func TestManagerSTCPVisitorRoute(t *testing.T) {
	manager := testManager(t)
	applied, err := manager.Apply(context.Background(), contracts.FrpProxy{
		Name: "visitor", Type: contracts.FrpSTCP, ServicePort: 22, BindPort: 22023, HostProfileID: "lab",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if port, ok := manager.RouteForHost("lab"); !ok || port != 22023 {
		t.Fatalf("route = %d/%v", port, ok)
	}
	config, _ := os.ReadFile(filepath.Join(manager.opts.WorkDir, applied.ID+".toml"))
	if !strings.Contains(string(config), "[[visitors]]") {
		t.Fatalf("config = %s", config)
	}
}

func TestManagerRotate(t *testing.T) {
	manager := testManager(t)
	applied, err := manager.Apply(context.Background(), contracts.FrpProxy{
		Name: "udp", Type: contracts.FrpUDP, ServicePort: 53, RemotePort: 22024,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(manager.opts.WorkDir, applied.ID+".toml")
	before, _ := os.ReadFile(path)
	if err := manager.RotateEndpoint(context.Background(), applied.EndpointID); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) == string(after) {
		t.Fatal("rotation did not change token")
	}
}

func TestManagerVerifiesPinnedBinaryHash(t *testing.T) {
	bin := fakeFrpc(t)
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:])
	dir := t.TempDir()
	manager, err := NewManager(Options{
		ConfigPath: filepath.Join(dir, "frp.json"), KeyPath: filepath.Join(dir, "frp.key"),
		WorkDir: filepath.Join(dir, "work"), FrpcBin: bin, FrpcSHA256: "sha256:" + good,
	})
	if err != nil {
		t.Fatalf("matching hash: %v", err)
	}
	manager.Close()
	actual, err := manager.BinarySHA256()
	if err != nil || actual != good {
		t.Fatalf("BinarySHA256 = %q/%v", actual, err)
	}

	_, err = NewManager(Options{
		ConfigPath: filepath.Join(dir, "bad.json"), KeyPath: filepath.Join(dir, "bad.key"),
		WorkDir: filepath.Join(dir, "badwork"), FrpcBin: bin, FrpcSHA256: strings.Repeat("0", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("mismatched hash error = %v", err)
	}
}

func TestKillProcessGroupReapsChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups differ on windows")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := "#!/bin/sh\nsleep 30 &\necho $! > " + pidFile + "\nwait\n"
	path := filepath.Join(dir, "spawn")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path)
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var childPID int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			if childPID > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if childPID == 0 {
		killProcess(cmd)
		_ = cmd.Wait()
		t.Fatal("child pid was not published")
	}

	started := time.Now()
	killProcess(cmd)
	_ = cmd.Wait()
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("process group kill took %s", elapsed)
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if err := syscall.Kill(childPID, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child %d survived the process-group kill", childPID)
}

func fakeFrps(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frps")
	script := "#!/bin/sh\nif [ \"$1\" = \"verify\" ]; then exit 0; fi\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestManagerManagesLocalFrpsLifecycle(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManager(Options{
		ConfigPath: filepath.Join(dir, "frp.json"), KeyPath: filepath.Join(dir, "frp.key"),
		WorkDir: filepath.Join(dir, "work"), FrpcBin: fakeFrpc(t), FrpsBin: fakeFrps(t),
		EndpointListen: "127.0.0.1:17000", EndpointName: "local", EndpointToken: "server-token",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(manager.Close)

	list := manager.List()
	if len(list.Endpoints) != 1 {
		t.Fatalf("endpoints = %+v", list.Endpoints)
	}
	endpoint := list.Endpoints[0]
	if !endpoint.Managed || endpoint.State != contracts.FrpOnline || endpoint.Port != 17000 {
		t.Fatalf("endpoint = %+v", endpoint)
	}
	config, err := os.ReadFile(filepath.Join(dir, "work", endpoint.ID+"-frps.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "bindPort = 17000") || !strings.Contains(string(config), "auth.token") {
		t.Fatalf("config = %s", config)
	}

	applied, err := manager.Apply(context.Background(), contracts.FrpProxy{
		Name: "lab", Type: contracts.FrpTCP, ServicePort: 22, RemotePort: 22022,
	})
	if err != nil {
		t.Fatalf("Apply with local frps: %v", err)
	}
	if applied.State != contracts.FrpOnline {
		t.Fatalf("applied = %+v", applied)
	}

	if err := manager.StopEndpoint(context.Background(), endpoint.ID); err != nil {
		t.Fatalf("StopEndpoint: %v", err)
	}
	list = manager.List()
	if list.Endpoints[0].State != contracts.FrpStopped {
		t.Fatalf("endpoint after stop = %+v", list.Endpoints[0])
	}
	for _, proxy := range list.Proxies {
		if proxy.State != contracts.FrpStopped {
			t.Fatalf("dependent proxy = %+v", proxy)
		}
	}
}
