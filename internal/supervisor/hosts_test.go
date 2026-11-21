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
