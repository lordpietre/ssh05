package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lordpietre/ssh05/contracts"
	"github.com/lordpietre/ssh05/ssh05"
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

func TestInventoryCreateHostStoresEncodedPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_config")
	inventory := NewInventory(InventoryConfig{SSHConfigPath: path, OverridesPath: filepath.Join(dir, "hosts.json")})
	const password = "s3cret-pass"
	if _, err := inventory.CreateHost(context.Background(), contracts.HostCreateParams{
		Alias: "web", Hostname: "10.0.0.3", Password: password,
	}); err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	if !strings.Contains(config, "#!! encPassword ") || !strings.Contains(config, "#!! encQuestionAnswer1 ") {
		t.Fatalf("encoded entries missing:\n%s", config)
	}
	if strings.Contains(config, password) {
		t.Fatalf("plaintext password written:\n%s", config)
	}
	encoded := strings.TrimSpace(strings.SplitN(strings.SplitN(config, "#!! encPassword ", 2)[1], "\n", 2)[0])
	decoded, err := ssh05.DecodeSecret(encoded)
	if err != nil || decoded != password {
		t.Fatalf("decode = %q, %v", decoded, err)
	}
}

func TestInventoryUpdateAndDeleteHost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_config")
	inventory := NewInventory(InventoryConfig{SSHConfigPath: path, OverridesPath: filepath.Join(dir, "hosts.json")})
	const password = "s3cret-pass"
	if _, err := inventory.CreateHost(context.Background(), contracts.HostCreateParams{
		Alias: "web", Hostname: "10.0.0.3", Port: 22, Username: "admin", Password: password,
	}); err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	profile, err := inventory.UpdateHost(context.Background(), contracts.HostUpdateParams{
		Alias: "web", Hostname: "10.0.0.9", Port: 2222, Username: "root",
	})
	if err != nil {
		t.Fatalf("UpdateHost: %v", err)
	}
	if profile.Target.Hostname != "10.0.0.9" || profile.Target.Port != 2222 || profile.Target.Username != "root" {
		t.Fatalf("profile = %+v", profile)
	}
	data, _ := os.ReadFile(path)
	config := string(data)
	if !strings.Contains(config, "HostName 10.0.0.9") || !strings.Contains(config, "#!! encPassword ") {
		t.Fatalf("updated config = %s", config)
	}
	encoded := strings.TrimSpace(strings.SplitN(strings.SplitN(config, "#!! encPassword ", 2)[1], "\n", 2)[0])
	if decoded, err := ssh05.DecodeSecret(encoded); err != nil || decoded != password {
		t.Fatalf("kept password = %q, %v", decoded, err)
	}

	const rotated = "new-pass"
	if _, err := inventory.UpdateHost(context.Background(), contracts.HostUpdateParams{
		Alias: "web", Hostname: "10.0.0.9", Port: 2222, Username: "root", Password: rotated,
	}); err != nil {
		t.Fatalf("UpdateHost password: %v", err)
	}
	data, _ = os.ReadFile(path)
	config = string(data)
	encoded = strings.TrimSpace(strings.SplitN(strings.SplitN(config, "#!! encPassword ", 2)[1], "\n", 2)[0])
	if decoded, err := ssh05.DecodeSecret(encoded); err != nil || decoded != rotated {
		t.Fatalf("rotated password = %q, %v", decoded, err)
	}

	if err := inventory.DeleteHost(context.Background(), contracts.HostDeleteParams{Alias: "web"}); err != nil {
		t.Fatalf("DeleteHost: %v", err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "Host web") {
		t.Fatalf("host block not removed:\n%s", data)
	}
	if _, err := inventory.UpdateHost(context.Background(), contracts.HostUpdateParams{Alias: "web", Hostname: "x"}); err == nil {
		t.Fatal("updating a missing host must fail")
	}
	if err := inventory.DeleteHost(context.Background(), contracts.HostDeleteParams{Alias: "web"}); err == nil {
		t.Fatal("deleting a missing host must fail")
	}
}
