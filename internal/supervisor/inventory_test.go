package supervisor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/trzsz/trzsz-ssh/contracts"
)

func keepassFixture(t *testing.T) (db, key string) {
	t.Helper()
	cli, err := exec.LookPath("keepassxc-cli")
	if err != nil {
		t.Skip("keepassxc-cli not available")
	}
	dir := t.TempDir()
	db = filepath.Join(dir, "test.kdbx")
	key = filepath.Join(dir, "test.key")
	if err := os.WriteFile(key, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(cli, args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", cli, args, err, output)
		}
	}
	run("db-create", "--set-key-file", key, db)
	run("add", "--no-password", "-k", key, "-u", "admin",
		"--url", "ssh://lab.example.net:2222", "--notes", "group=lab", db, "lab-arm64")
	run("add", "--no-password", "-k", key, "-u", "root",
		"--url", "ssh://other.example.net", db, "other")
	run("add", "--no-password", "-k", key, "-u", "admin",
		"--url", "ssh://conflict.example.net:2222", db, "ssh-only")
	return db, key
}

func writeSSHConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh_config")
	content := `Host lab-arm64
    HostName lab.example.net
    Port 2222
    User admin
    ProxyJump bastion.example.net

Host ssh-only
    HostName ssh-only.example.net
    User admin
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInventoryMergesSSHConfigAndKeePass(t *testing.T) {
	db, key := keepassFixture(t)
	inventory := NewInventory(InventoryConfig{
		SSHConfigPath: writeSSHConfig(t),
		Keepass:       KeePassConfig{Database: db, KeyFile: key},
	})

	profiles, err := inventory.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byAlias := map[string]contracts.HostProfile{}
	for _, profile := range profiles {
		byAlias[profile.Alias] = profile
	}

	merged, ok := byAlias["lab-arm64"]
	if !ok {
		t.Fatalf("lab-arm64 missing: %+v", profiles)
	}
	if len(merged.SourceRefs) != 2 {
		t.Fatalf("provenance = %+v", merged.SourceRefs)
	}
	if merged.AuthRef == nil || merged.AuthRef.Provider != "keepassxc" {
		t.Fatalf("auth ref = %+v", merged.AuthRef)
	}
	if len(merged.Labels) != 1 || merged.Labels[0] != "lab" {
		t.Fatalf("labels = %+v", merged.Labels)
	}
	if _, ok := byAlias["other"]; !ok {
		t.Fatalf("KDBX-only host missing: %+v", profiles)
	}
	if got := destinationFor(merged); got != "lab-arm64" {
		t.Fatalf("ssh destination = %q", got)
	}
	if got := destinationFor(byAlias["other"]); got != "root@other.example.net:22" {
		t.Fatalf("keepass destination = %q", got)
	}

	conflicts := inventory.Conflicts()
	if len(conflicts) != 1 || conflicts[0].HostProfileID != "ssh-only" {
		t.Fatalf("conflicts = %+v", conflicts)
	}

	status := inventory.VaultStatus()
	if status.Provider != "keepassxc" || status.Locked || status.Profiles != 3 {
		t.Fatalf("vault status = %+v", status)
	}
}

func TestInventoryImportKeePass(t *testing.T) {
	db, key := keepassFixture(t)
	inventory := NewInventory(InventoryConfig{
		SSHConfigPath: writeSSHConfig(t),
		Keepass:       KeePassConfig{Database: "nonexistent.kdbx", KeyFile: key},
	})
	if _, err := inventory.List(context.Background()); err != nil {
		t.Fatalf("initial List: %v", err)
	}
	if status := inventory.VaultStatus(); !status.Locked {
		t.Fatalf("vault should be locked without DB: %+v", status)
	}

	result, err := inventory.Import(context.Background(), "keepass", db)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Imported < 3 {
		t.Fatalf("imported = %+v", result)
	}
	if status := inventory.VaultStatus(); status.Locked {
		t.Fatalf("vault should be unlocked after import: %+v", status)
	}
}
