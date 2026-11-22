package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lordpietre/ssh05/contracts"
)

func TestInventoryCreateHostAppendsAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_config")
	if err := os.WriteFile(path, []byte("Host existing\n    HostName 10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inventory := NewInventory(InventoryConfig{SSHConfigPath: path, OverridesPath: filepath.Join(dir, "hosts.json")})
	profile, err := inventory.CreateHost(context.Background(), contracts.HostCreateParams{
		Alias: "new-lab", Hostname: "10.0.0.2", Port: 2222, Username: "admin",
		IdentityFile: "~/.ssh/id_ed25519",
	})
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if profile.Alias != "new-lab" || profile.Target.Hostname != "10.0.0.2" || profile.Target.Port != 2222 {
		t.Fatalf("profile = %+v", profile)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "Host new-lab") || !strings.Contains(string(data), "IdentityFile ~/.ssh/id_ed25519") {
		t.Fatalf("config = %s", data)
	}
	if _, err := inventory.CreateHost(context.Background(), contracts.HostCreateParams{Alias: "new-lab", Hostname: "x"}); err == nil {
		t.Fatal("duplicate alias must fail")
	}
	if _, err := inventory.CreateHost(context.Background(), contracts.HostCreateParams{Alias: "bad alias", Hostname: "x"}); err == nil {
		t.Fatal("invalid alias must fail")
	}
}
