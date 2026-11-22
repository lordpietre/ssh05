package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvPrefersSSH05AndFallsBackToTRSS(t *testing.T) {
	t.Setenv("SSH05_PROBE", "new")
	t.Setenv("TRSS_PROBE", "old")
	if got := Env("PROBE"); got != "new" {
		t.Fatalf("Env = %q, want new", got)
	}
	t.Setenv("SSH05_PROBE", "")
	if got := Env("PROBE"); got != "old" {
		t.Fatalf("legacy Env = %q, want old", got)
	}
}

func TestConfigDirAdoptsLegacyDirectory(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, LegacyName)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "hosts.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"XDG_CONFIG_HOME": base}
	got := ConfigDir(env)
	want := filepath.Join(base, Name)
	if got != want {
		t.Fatalf("ConfigDir = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(want, "hosts.json")); err != nil {
		t.Fatalf("adopted file missing: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy directory should be gone, err=%v", err)
	}
}

func TestStateDirUsesProductName(t *testing.T) {
	base := t.TempDir()
	env := map[string]string{"XDG_STATE_HOME": base, "HOME": base}
	if got, want := StateDir(env), filepath.Join(base, Name); got != want {
		t.Fatalf("StateDir = %q, want %q", got, want)
	}
}

func TestSocketPathHonorsOverride(t *testing.T) {
	t.Setenv("SSH05_SOCKET", "/tmp/custom.sock")
	t.Setenv("TRSS_SOCKET", "")
	if got := UserSocketPath(); got != "/tmp/custom.sock" {
		t.Fatalf("socket = %q", got)
	}
}

func TestDownloadDirOnlyWhenPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := DownloadDir(); got != "" {
		t.Fatalf("DownloadDir without folder = %q, want empty", got)
	}
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := DownloadDir(); got != downloads {
		t.Fatalf("DownloadDir = %q, want %q", got, downloads)
	}
}
