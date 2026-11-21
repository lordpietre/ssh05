package tssh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListHostEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_config")
	content := `Host trss-test-a
    HostName 127.0.0.1
    Port 2222
    User tester

Host trss-test-b trss-test-c
    HostName example.invalid
    User admin
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := ListHostEntries(path)
	if err != nil {
		t.Fatalf("ListHostEntries: %v", err)
	}
	found := map[string]HostEntry{}
	for _, entry := range entries {
		found[entry.Alias] = entry
	}
	for _, alias := range []string{"trss-test-a", "trss-test-b", "trss-test-c"} {
		if _, ok := found[alias]; !ok {
			t.Errorf("missing alias %q in %+v", alias, entries)
		}
	}
	if got := found["trss-test-a"]; got.Host != "127.0.0.1" || got.Port != "2222" || got.User != "tester" {
		t.Errorf("trss-test-a = %+v", got)
	}
	if _, ok := found["*"]; ok {
		t.Error("wildcard host must not be listed")
	}
}
