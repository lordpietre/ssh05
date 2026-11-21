package tssh

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func writeTestSSHConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh_config")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestListHostEntriesReloadsSamePath(t *testing.T) {
	path := writeTestSSHConfig(t, "Host first\n    HostName 10.0.0.1\n")
	entries, err := ListHostEntries(path)
	if err != nil {
		t.Fatalf("first ListHostEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].Alias != "first" {
		t.Fatalf("first entries = %+v", entries)
	}

	if err := os.WriteFile(path, []byte("Host first\n    HostName 10.0.0.1\nHost second\n    HostName 10.0.0.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err = ListHostEntries(path)
	if err != nil {
		t.Fatalf("second ListHostEntries: %v", err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		found[entry.Alias] = true
	}
	if !found["first"] || !found["second"] {
		t.Fatalf("reload did not pick up changes: %+v", found)
	}
}

func TestAcquireConfigReadAllowsSharedReaders(t *testing.T) {
	path := writeTestSSHConfig(t, "Host shared\n    HostName 10.0.0.1\n")

	var active atomic.Int32
	var peak atomic.Int32
	var wg sync.WaitGroup
	for index := 0; index < 4; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := acquireConfigRead(path)
			if err != nil {
				t.Errorf("acquireConfigRead: %v", err)
				return
			}
			current := active.Add(1)
			if current > peak.Load() {
				peak.Store(current)
			}
			time.Sleep(50 * time.Millisecond)
			active.Add(-1)
			release()
		}()
	}
	wg.Wait()
	if peak.Load() < 2 {
		t.Fatalf("readers were serialized, peak = %d", peak.Load())
	}
}
