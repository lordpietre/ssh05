package tssh

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateTokenUsesCSPRNG(t *testing.T) {
	first, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}
	second, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() second error = %v", err)
	}

	if len(first) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars (256 bits)", len(first))
	}
	if strings.Trim(first, "0123456789abcdef") != "" {
		t.Fatalf("token %q is not lowercase hex", first)
	}
	if first == second {
		t.Fatal("two generated tokens are identical")
	}
	if first == "tssh-frp-default-token" {
		t.Fatal("generateToken() returned the removed static fallback token")
	}
}

func TestGenerateTokenFailsClosedOnRNGError(t *testing.T) {
	original := cryptoRandRead
	t.Cleanup(func() { cryptoRandRead = original })

	cryptoRandRead = func([]byte) (int, error) {
		return 0, errors.New("rng unavailable")
	}

	token, err := generateToken()
	if err == nil {
		t.Fatal("generateToken() must fail when the CSPRNG fails")
	}
	if token != "" {
		t.Fatalf("generateToken() returned token %q on RNG error", token)
	}
	if !strings.Contains(err.Error(), "rng unavailable") {
		t.Fatalf("error %q does not wrap the RNG failure", err)
	}
}

func TestGenerateTokenFailsClosedOnShortRead(t *testing.T) {
	original := cryptoRandRead
	t.Cleanup(func() { cryptoRandRead = original })

	cryptoRandRead = func(buf []byte) (int, error) {
		return len(buf) / 2, nil
	}

	token, err := generateToken()
	if err == nil {
		t.Fatal("generateToken() must fail on a short CSPRNG read")
	}
	if token != "" {
		t.Fatalf("generateToken() returned token %q on short read", token)
	}
}

func TestValidRemoteFileMode(t *testing.T) {
	for _, mode := range []string{"644", "755", "0644", "0600"} {
		if !validRemoteFileMode(mode) {
			t.Errorf("mode %q should be valid", mode)
		}
	}
	for _, mode := range []string{"", "64", "999", "64;id", "-x"} {
		if validRemoteFileMode(mode) {
			t.Errorf("mode %q should be invalid", mode)
		}
	}
}

func TestFrpCopyToRemoteRejectsInvalidInputBeforeConnecting(t *testing.T) {
	if err := frpCopyToRemote("unused", nil, "", "644"); err == nil {
		t.Fatal("empty remote path must be rejected")
	}
	if err := frpCopyToRemote("unused", nil, "/tmp/x\x00y", "644"); err == nil {
		t.Fatal("remote path with NUL must be rejected")
	}
	if err := frpCopyToRemote("unused", nil, "/tmp/x", "644;id"); err == nil {
		t.Fatal("invalid mode must be rejected")
	}
}

func TestFrpRemoteCopyCommandIsSafeForRemoteShell(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	home := t.TempDir()
	// Spaces, a command substitution and a single quote must remain a literal
	// filename, never a second command.
	remotePath := "~/dir with space/$(touch pwned)'s.toml"
	cmd := frpRemoteCopyCommand(remotePath, "640")

	command := exec.Command("sh", "-c", cmd)
	command.Env = append(os.Environ(), "HOME="+home)
	command.Stdin = strings.NewReader("safe payload")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("remote copy script failed: %v\n%s", err, out)
	}

	expected := filepath.Join(home, "dir with space", "$(touch pwned)'s.toml")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("expected file %q not created: %v", expected, err)
	}
	if string(data) != "safe payload" {
		t.Fatalf("file content = %q", data)
	}
	info, err := os.Stat(expected)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("file mode = %o, want 640", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(home, "pwned")); err == nil {
		t.Fatal("command injection executed: pwned file exists")
	}
}

func TestFrpRemoteCopyCommandHandlesPlainTildePath(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	home := t.TempDir()
	cmd := frpRemoteCopyCommand("~/trss.conf", "600")
	command := exec.Command("sh", "-c", cmd)
	command.Env = append(os.Environ(), "HOME="+home)
	command.Stdin = strings.NewReader("x")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("remote copy script failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, "trss.conf")); err != nil {
		t.Fatalf("tilde path not expanded into HOME: %v", err)
	}
}
