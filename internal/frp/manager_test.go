package frp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
