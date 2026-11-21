package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/trzsz/trzsz-ssh/contracts"
)

func TestSSHHostsList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_config")
	content := `Host provider-test
    HostName 127.0.0.1
    Port 2222
    User tester
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	hosts, err := (SSHHosts{ConfigPath: path}).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found *contracts.HostProfile
	for index := range hosts {
		if hosts[index].Alias == "provider-test" {
			found = &hosts[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("provider-test not found in %+v", hosts)
	}
	if found.Target.Hostname != "127.0.0.1" || found.Target.Port != 2222 || found.Target.Username != "tester" {
		t.Fatalf("profile = %+v", found)
	}
	if found.VerifiedIdentity || !found.Discovered {
		t.Fatalf("imported host must be discovered/unverified: %+v", found)
	}
	if len(found.SourceRefs) != 1 || found.SourceRefs[0].Kind != contracts.SourceSSHConfig {
		t.Fatalf("source refs = %+v", found.SourceRefs)
	}
}

func TestSSHHostsInventoryMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_config")
	content := `Host lab
    HostName 10.0.0.5
    User admin
    #!! Color 5878CC
    #!! Favorite Yes
    #!! GroupLabels lab prod
Host plain
    HostName 10.0.0.6
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	hosts, err := (SSHHosts{ConfigPath: path}).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byAlias := map[string]contracts.HostProfile{}
	for _, host := range hosts {
		byAlias[host.Alias] = host
	}
	lab, ok := byAlias["lab"]
	if !ok {
		t.Fatalf("lab missing from %+v", hosts)
	}
	if lab.Color != "5878CC" {
		t.Fatalf("color = %q", lab.Color)
	}
	if !lab.Favorite {
		t.Fatalf("favorite = %v", lab.Favorite)
	}
	if len(lab.Labels) != 2 || lab.Labels[0] != "lab" || lab.Labels[1] != "prod" {
		t.Fatalf("labels = %+v", lab.Labels)
	}
	if byAlias["plain"].Favorite || byAlias["plain"].Color != "" {
		t.Fatalf("plain = %+v", byAlias["plain"])
	}
}

func TestSSHHostsInclude(t *testing.T) {
	dir := t.TempDir()
	include := filepath.Join(dir, "extra.conf")
	if err := os.WriteFile(include, []byte("Host included\n    HostName 10.0.0.7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "ssh_config")
	content := "Include " + include + "\nHost main\n    HostName 10.0.0.8\n"
	if err := os.WriteFile(main, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts, err := (SSHHosts{ConfigPath: main}).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := map[string]bool{}
	for _, host := range hosts {
		found[host.Alias] = true
	}
	if !found["included"] || !found["main"] {
		t.Fatalf("include not honored: %+v", found)
	}
}
