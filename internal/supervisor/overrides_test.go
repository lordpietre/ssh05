package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lordpietre/ssh05/contracts"
)

func TestInventoryHostOverridesPersist(t *testing.T) {
	dir := t.TempDir()
	sshConfig := filepath.Join(dir, "ssh_config")
	if err := os.WriteFile(sshConfig, []byte("Host lab\n    HostName 10.0.0.9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	overrides := filepath.Join(dir, "hosts.json")
	inventory := NewInventory(InventoryConfig{SSHConfigPath: sshConfig, OverridesPath: overrides})
	hosts, err := inventory.List(context.Background())
	if err != nil || len(hosts) != 1 {
		t.Fatalf("List = %+v/%v", hosts, err)
	}

	favorite := true
	color := "5878cc"
	labels := []string{" prod ", "lab", "prod", ""}
	updated, err := inventory.SetOverride(context.Background(), contracts.HostOverridePatch{
		HostProfileID: "lab",
		Labels:        &labels,
		Favorite:      &favorite,
		Color:         &color,
	})
	if err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if !updated.Favorite || updated.Color != "#5878CC" {
		t.Fatalf("updated = %+v", updated)
	}
	if len(updated.Labels) != 2 || updated.Labels[0] != "prod" || updated.Labels[1] != "lab" {
		t.Fatalf("labels = %+v", updated.Labels)
	}
	if !hasManualRef(updated.SourceRefs) {
		t.Fatalf("manual source missing: %+v", updated.SourceRefs)
	}
	if info, err := os.Stat(overrides); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("overrides mode: %v/%v", info, err)
	}

	reloaded := NewInventory(InventoryConfig{SSHConfigPath: sshConfig, OverridesPath: overrides})
	hosts, err = reloaded.List(context.Background())
	if err != nil || len(hosts) != 1 || !hosts[0].Favorite || hosts[0].Color != "#5878CC" {
		t.Fatalf("reloaded = %+v/%v", hosts, err)
	}

	cleared := ""
	if _, err := reloaded.SetOverride(context.Background(), contracts.HostOverridePatch{
		HostProfileID: "lab", Color: &cleared,
	}); err != nil {
		t.Fatalf("clear color: %v", err)
	}
	hosts, _ = reloaded.List(context.Background())
	if hosts[0].Color != "" {
		t.Fatalf("color after clear = %q", hosts[0].Color)
	}
}

func TestInventoryHostOverrideRejectsInvalidColor(t *testing.T) {
	dir := t.TempDir()
	sshConfig := filepath.Join(dir, "ssh_config")
	if err := os.WriteFile(sshConfig, []byte("Host lab\n    HostName 10.0.0.9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inventory := NewInventory(InventoryConfig{
		SSHConfigPath: sshConfig,
		OverridesPath: filepath.Join(dir, "hosts.json"),
	})
	bad := "not-a-color"
	if _, err := inventory.SetOverride(context.Background(), contracts.HostOverridePatch{
		HostProfileID: "lab", Color: &bad,
	}); err == nil {
		t.Fatal("invalid color must fail")
	}
}
