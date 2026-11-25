package supervisor

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"testing"

	"github.com/lordpietre/ssh05/contracts"
	"github.com/lordpietre/ssh05/internal/ipc"
	"github.com/lordpietre/ssh05/ssh05"
	"golang.org/x/crypto/ssh"
)

func TestHostKeyErrorMapping(t *testing.T) {
	unknown := &ssh05.HostKeyUnknownError{
		Host:           "example.test:22",
		KeyType:        "ED25519",
		Fingerprint:    "SHA256:abc",
		KnownHostsPath: "/home/test/.ssh/known_hosts",
		CanTrust:       true,
	}
	rpcError := hostKeyError(unknown)
	if rpcError.Code != ipc.CodeHostKeyUnknown {
		t.Fatalf("code = %d", rpcError.Code)
	}
	var challenge contracts.HostKeyChallenge
	if err := json.Unmarshal(rpcError.Data, &challenge); err != nil {
		t.Fatalf("data: %v", err)
	}
	if challenge.Fingerprint != unknown.Fingerprint || !challenge.CanTrust {
		t.Fatalf("challenge = %+v", challenge)
	}
	if code := hostKeyErrorCode(unknown); code != "host_key_unknown" {
		t.Fatalf("audit code = %q", code)
	}

	changed := &ssh05.HostKeyChangedError{
		Host:        "example.test:22",
		KeyType:     "RSA",
		Fingerprint: "SHA256:def",
	}
	rpcError = hostKeyError(changed)
	if rpcError.Code != ipc.CodeHostKeyChanged {
		t.Fatalf("code = %d", rpcError.Code)
	}
	var changedChallenge contracts.HostKeyChallenge
	if err := json.Unmarshal(rpcError.Data, &changedChallenge); err != nil {
		t.Fatalf("data: %v", err)
	}
	if changedChallenge.CanTrust {
		t.Fatal("changed keys must not be trustable")
	}
	if code := hostKeyErrorCode(changed); code != "host_key_changed" {
		t.Fatalf("audit code = %q", code)
	}

	wrapped := errors.Join(errors.New("outer"), unknown)
	if rpcError := hostKeyError(wrapped); rpcError.Code != ipc.CodeHostKeyUnknown {
		t.Fatalf("wrapped code = %d", rpcError.Code)
	}
}

func TestSupervisorHostKeyPromptNeverUsesTTY(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	err = supervisorHostKeyPrompt("example.test:22", signer.PublicKey(), "/tmp/known_hosts")
	var unknown *ssh05.HostKeyUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("err = %v, want HostKeyUnknownError", err)
	}
	if !unknown.CanTrust {
		t.Fatal("a default unknown key must be trustable")
	}
	if unknown.Fingerprint != ssh.FingerprintSHA256(signer.PublicKey()) {
		t.Fatalf("fingerprint = %q", unknown.Fingerprint)
	}
	if unknown.KeyType != "ED25519" {
		t.Fatalf("key type = %q", unknown.KeyType)
	}
}
