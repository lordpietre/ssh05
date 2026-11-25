package ssh05

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeema/knownhosts"
	"golang.org/x/crypto/ssh"
)

func startHostKeyServer(t *testing.T) (string, ssh.PublicKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer func() { _ = conn.Close() }()
				serverConn, chans, reqs, err := ssh.NewServerConn(conn, serverConfig)
				if err != nil {
					return
				}
				defer func() { _ = serverConn.Close() }()
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					_ = ch.Reject(ssh.UnknownChannelType, "test server")
				}
			}(conn)
		}
	}()
	return listener.Addr().String(), signer.PublicKey()
}

func hostKeyTestConfig(t *testing.T, address, knownHosts, strict string) string {
	t.Helper()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split address: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ssh_config")
	content := fmt.Sprintf(`Host ssh05-hostkey-test
  HostName %s
  Port %s
  User tester
  UserKnownHostsFile %s
  GlobalKnownHostsFile /dev/null
  StrictHostKeyChecking %s
  BatchMode yes
  ConnectTimeout 5
`, host, port, knownHosts, strict)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestTrustHostKeyWritesApprovedKey(t *testing.T) {
	address, serverKey := startHostKeyServer(t)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(knownHosts, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	config := hostKeyTestConfig(t, address, knownHosts, "ask")

	info, err := TrustHostKey("ssh05-hostkey-test", config, ssh.FingerprintSHA256(serverKey))
	if err != nil {
		t.Fatalf("TrustHostKey: %v", err)
	}
	if info.Fingerprint != ssh.FingerprintSHA256(serverKey) {
		t.Fatalf("fingerprint = %q", info.Fingerprint)
	}
	data, err := os.ReadFile(knownHosts)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(ssh.MarshalAuthorizedKey(serverKey)))
	if len(fields) < 2 || !strings.Contains(string(data), fields[1]) {
		t.Fatalf("known_hosts does not contain the approved key: %s", data)
	}
}

func TestTrustHostKeyRejectsDifferentFingerprint(t *testing.T) {
	address, _ := startHostKeyServer(t)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(knownHosts, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	config := hostKeyTestConfig(t, address, knownHosts, "ask")

	_, err := TrustHostKey("ssh05-hostkey-test", config, "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	var changed *HostKeyChangedError
	if !errors.As(err, &changed) {
		t.Fatalf("err = %v, want HostKeyChangedError", err)
	}
	data, err := os.ReadFile(knownHosts)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("a rejected key must not be written: %s", data)
	}
}

func TestUnknownHostKeyUsesPrompt(t *testing.T) {
	address, serverKey := startHostKeyServer(t)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(knownHosts, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	config := hostKeyTestConfig(t, address, knownHosts, "ask")

	var prompted ssh.PublicKey
	abort := errors.New("abort after prompt")
	_, err := SshLogin(&SshArgs{
		Destination: "ssh05-hostkey-test",
		ConfigFile:  config,
		HostKeyPrompt: func(_ string, key ssh.PublicKey, _ string) error {
			prompted = key
			return abort
		},
	})
	if !errors.Is(err, abort) {
		t.Fatalf("err = %v, want prompt abort", err)
	}
	if prompted == nil || ssh.FingerprintSHA256(prompted) != ssh.FingerprintSHA256(serverKey) {
		t.Fatalf("prompt key = %v", prompted)
	}
}

func TestStrictUnknownHostKeyReturnsTypedError(t *testing.T) {
	address, _ := startHostKeyServer(t)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(knownHosts, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	config := hostKeyTestConfig(t, address, knownHosts, "yes")

	_, err := SshLogin(&SshArgs{
		Destination: "ssh05-hostkey-test",
		ConfigFile:  config,
		HostKeyPrompt: func(string, ssh.PublicKey, string) error {
			t.Fatal("prompt must not run with StrictHostKeyChecking=yes")
			return nil
		},
	})
	var unknown *HostKeyUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("err = %v, want HostKeyUnknownError", err)
	}
	if unknown.CanTrust {
		t.Fatal("strict unknown key must not be trustable")
	}
}

func TestChangedHostKeyReturnsTypedError(t *testing.T) {
	address, _ := startHostKeyServer(t)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	_, wrongPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wrongSigner, err := ssh.NewSignerFromKey(wrongPrivate)
	if err != nil {
		t.Fatal(err)
	}
	line := knownhosts.Line([]string{knownhosts.Normalize(address)}, wrongSigner.PublicKey())
	if err := os.WriteFile(knownHosts, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := hostKeyTestConfig(t, address, knownHosts, "ask")

	_, err = SshLogin(&SshArgs{Destination: "ssh05-hostkey-test", ConfigFile: config})
	var changed *HostKeyChangedError
	if !errors.As(err, &changed) {
		t.Fatalf("err = %v, want HostKeyChangedError", err)
	}
}
