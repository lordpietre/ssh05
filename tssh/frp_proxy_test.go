package tssh

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestEnsurePrivateDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are not meaningful on Windows")
	}
	dir := filepath.Join(t.TempDir(), "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDir(dir); err != nil {
		t.Fatalf("ensurePrivateDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %o, want 700", info.Mode().Perm())
	}
}

func TestFRPTokenSealRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, frpKeySize)
	sealed, err := sealFRPTokenWithKey(key, "super-secret-token")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if strings.Contains(sealed, "super-secret-token") {
		t.Fatal("ciphertext contains the plaintext token")
	}
	opened, err := openFRPTokenWithKey(key, sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened != "super-secret-token" {
		t.Fatalf("opened = %q", opened)
	}

	wrongKey := bytes.Repeat([]byte{0x24}, frpKeySize)
	if _, err := openFRPTokenWithKey(wrongKey, sealed); err == nil {
		t.Fatal("wrong key must fail")
	}
}

func TestFRPSaveLoadEncryptsToken(t *testing.T) {
	originalKeyPath := frpKeyPath
	keyPath := filepath.Join(t.TempDir(), "frp.key")
	frpKeyPath = func() string { return keyPath }
	t.Cleanup(func() { frpKeyPath = originalKeyPath })

	path := filepath.Join(t.TempDir(), "frp.json")
	entry := &frpProxyEntry{
		Alias:       "a",
		Name:        "proxy",
		ServicePort: 80,
		ExposedPort: 8080,
		Token:       "super-secret-token",
	}
	if err := frpSaveConfig([]*frpProxyEntry{entry}, path); err != nil {
		t.Fatalf("frpSaveConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "super-secret-token") {
		t.Fatalf("token persisted in plaintext: %s", data)
	}
	if !strings.Contains(string(data), "token_enc") {
		t.Fatalf("token_enc missing: %s", data)
	}

	loaded, err := frpLoadConfig(path, "a")
	if err != nil {
		t.Fatalf("frpLoadConfig: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Token != "super-secret-token" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if loaded[0].LegacyToken != "" {
		t.Fatalf("legacy token not cleared: %+v", loaded[0])
	}

	if info, err := os.Stat(keyPath); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %o, want 600", info.Mode().Perm())
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestFRPLoadMigratesLegacyPlaintextToken(t *testing.T) {
	originalKeyPath := frpKeyPath
	frpKeyPath = func() string { return filepath.Join(t.TempDir(), "frp.key") }
	t.Cleanup(func() { frpKeyPath = originalKeyPath })

	path := filepath.Join(t.TempDir(), "frp.json")
	legacy := `[{"alias":"a","name":"proxy","token":"legacy-secret","service_port":80,"exposed_port":8080}]`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := frpLoadConfig(path, "a")
	if err != nil {
		t.Fatalf("frpLoadConfig: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Token != "legacy-secret" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if loaded[0].LegacyToken != "" {
		t.Fatalf("legacy token not cleared after hydration: %+v", loaded[0])
	}

	if err := frpSaveConfig(loaded, path); err != nil {
		t.Fatalf("frpSaveConfig: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "legacy-secret") {
		t.Fatalf("legacy token persisted in plaintext: %s", data)
	}
	if !strings.Contains(string(data), "token_enc") {
		t.Fatalf("token_enc missing after migration: %s", data)
	}
}

func TestFRPTokenSealFailsOnRNGError(t *testing.T) {
	original := cryptoRandRead
	t.Cleanup(func() { cryptoRandRead = original })
	cryptoRandRead = func([]byte) (int, error) {
		return 0, errors.New("rng unavailable")
	}

	if _, err := sealFRPTokenWithKey(bytes.Repeat([]byte{0x42}, frpKeySize), "token"); err == nil {
		t.Fatal("seal must fail when the CSPRNG fails")
	}
}

func TestNormalizeFRPProxyType(t *testing.T) {
	cases := map[string]string{
		"":      "tcp",
		"tcp":   "tcp",
		"TCP":   "tcp",
		" udp ": "udp",
		"stcp":  "stcp",
		"xtcp":  "xtcp",
		"http":  "tcp",
	}
	for input, want := range cases {
		if got := normalizeFRPProxyType(input); got != want {
			t.Errorf("normalizeFRPProxyType(%q) = %q, want %q", input, got, want)
		}
	}
	if nextFRPProxyType("tcp") != "udp" || nextFRPProxyType("udp") != "stcp" ||
		nextFRPProxyType("stcp") != "xtcp" || nextFRPProxyType("xtcp") != "tcp" {
		t.Fatal("proxy type cycle is incorrect")
	}
}

func TestFRPCProxyBlockTypes(t *testing.T) {
	entry := &frpProxyEntry{
		Name:        "proxy",
		ServicePort: 8080,
		ExposedPort: 9000,
		Token:       "auth-token",
		Secret:      "dedicated-secret",
	}

	entry.Type = "tcp"
	block := frpcProxyBlock(entry)
	if !strings.Contains(block, `type = "tcp"`) || !strings.Contains(block, "remotePort = 9000") {
		t.Fatalf("tcp block = %s", block)
	}
	if strings.Contains(block, "secretKey") {
		t.Fatalf("tcp block must not use secretKey: %s", block)
	}

	entry.Type = "udp"
	block = frpcProxyBlock(entry)
	if !strings.Contains(block, `type = "udp"`) || !strings.Contains(block, "remotePort = 9000") {
		t.Fatalf("udp block = %s", block)
	}

	for _, proxyType := range []string{"stcp", "xtcp"} {
		entry.Type = proxyType
		block = frpcProxyBlock(entry)
		if !strings.Contains(block, `type = "`+proxyType+`"`) ||
			!strings.Contains(block, `secretKey = "dedicated-secret"`) {
			t.Fatalf("%s block = %s", proxyType, block)
		}
		if strings.Contains(block, "auth-token") {
			t.Fatalf("%s block reused the auth token: %s", proxyType, block)
		}
		if strings.Contains(block, "remotePort") {
			t.Fatalf("%s block must not use remotePort: %s", proxyType, block)
		}
	}
}

func TestEnsureFRPProxySecret(t *testing.T) {
	entry := &frpProxyEntry{Type: "tcp"}
	if err := ensureFRPProxySecret(entry); err != nil || entry.Secret != "" {
		t.Fatalf("tcp secret = %q, err = %v", entry.Secret, err)
	}

	entry.Type = "stcp"
	if err := ensureFRPProxySecret(entry); err != nil {
		t.Fatalf("ensure secret: %v", err)
	}
	if len(entry.Secret) != 64 {
		t.Fatalf("secret length = %d, want 64", len(entry.Secret))
	}
	first := entry.Secret
	if err := ensureFRPProxySecret(entry); err != nil {
		t.Fatal(err)
	}
	if entry.Secret != first {
		t.Fatal("existing secret must be preserved")
	}
}

func TestParseFRPVersion(t *testing.T) {
	cases := map[string]string{
		"0.69.1":              "0.69.1",
		"frpc version 0.31.0": "0.31.0",
		"v0.52.3\n":           "0.52.3",
		"frps 1.2.3 extra":    "1.2.3",
	}
	for input, want := range cases {
		version, ok := parseFRPVersion(input)
		if !ok {
			t.Errorf("parseFRPVersion(%q) failed", input)
			continue
		}
		if version.String() != want {
			t.Errorf("parseFRPVersion(%q) = %s, want %s", input, version, want)
		}
	}
	for _, input := range []string{"", "no version", "0.31"} {
		if _, ok := parseFRPVersion(input); ok {
			t.Errorf("parseFRPVersion(%q) should fail", input)
		}
	}
}

func TestFRPTypeSupported(t *testing.T) {
	for _, proxyType := range []string{"tcp", "udp"} {
		if !frpTypeSupported("0.30.0", proxyType) {
			t.Errorf("%s should be supported on 0.30.0", proxyType)
		}
	}
	for _, proxyType := range []string{"stcp", "xtcp"} {
		if frpTypeSupported("0.30.0", proxyType) {
			t.Errorf("%s should not be supported on 0.30.0", proxyType)
		}
		if !frpTypeSupported("0.31.0", proxyType) {
			t.Errorf("%s should be supported on 0.31.0", proxyType)
		}
	}
	if !frpTypeSupported("", "xtcp") || !frpTypeSupported("unknown", "xtcp") {
		t.Fatal("unknown versions must not block the user")
	}
}

func TestNextSupportedFRPType(t *testing.T) {
	if got := nextSupportedFRPType("tcp", "0.30.0"); got != "udp" {
		t.Fatalf("0.30.0 tcp -> %s, want udp", got)
	}
	if got := nextSupportedFRPType("udp", "0.30.0"); got != "tcp" {
		t.Fatalf("0.30.0 udp -> %s, want tcp", got)
	}
	if got := nextSupportedFRPType("udp", "0.69.1"); got != "stcp" {
		t.Fatalf("0.69.1 udp -> %s, want stcp", got)
	}
	if got := nextSupportedFRPType("xtcp", "0.69.1"); got != "tcp" {
		t.Fatalf("0.69.1 xtcp -> %s, want tcp", got)
	}
}

func TestFRPProxySecretPersistsEncrypted(t *testing.T) {
	originalKeyPath := frpKeyPath
	keyPath := filepath.Join(t.TempDir(), "frp.key")
	frpKeyPath = func() string { return keyPath }
	t.Cleanup(func() { frpKeyPath = originalKeyPath })

	path := filepath.Join(t.TempDir(), "frp.json")
	entry := &frpProxyEntry{
		Alias:       "a",
		Name:        "stcp-proxy",
		Type:        "stcp",
		ServicePort: 22,
		Token:       "auth-token",
		Secret:      "dedicated-secret",
	}
	if err := frpSaveConfig([]*frpProxyEntry{entry}, path); err != nil {
		t.Fatalf("frpSaveConfig: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"auth-token", "dedicated-secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("secret %q persisted in plaintext: %s", secret, data)
		}
	}
	if !strings.Contains(string(data), "secret_enc") {
		t.Fatalf("secret_enc missing: %s", data)
	}

	loaded, err := frpLoadConfig(path, "a")
	if err != nil {
		t.Fatalf("frpLoadConfig: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Secret != "dedicated-secret" || loaded[0].Token != "auth-token" {
		t.Fatalf("loaded = %+v", loaded)
	}
}
