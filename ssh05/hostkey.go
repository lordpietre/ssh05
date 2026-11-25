package ssh05

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// HostKeyPrompt is called for a host key that is not present in known_hosts.
// Returning nil accepts and persists the key; returning an error aborts the
// handshake. The supervisor uses it to turn the unknown key into a typed IPC
// challenge instead of depending on /dev/tty.
type HostKeyPrompt func(host string, key ssh.PublicKey, knownHostsPath string) error

// HostKeyUnknownError describes a server key that is not in known_hosts.
type HostKeyUnknownError struct {
	Host           string
	KeyType        string
	Fingerprint    string
	KnownHostsPath string
	// CanTrust is false when StrictHostKeyChecking=yes forbids adding the key.
	CanTrust bool
}

func (e *HostKeyUnknownError) Error() string {
	if !e.CanTrust {
		return fmt.Sprintf("host key for %s is unknown and StrictHostKeyChecking forbids trusting it (fingerprint %s)", e.Host, e.Fingerprint)
	}
	return fmt.Sprintf("host key for %s is not trusted (fingerprint %s)", e.Host, e.Fingerprint)
}

// HostKeyChangedError describes a key that differs from the known_hosts entry.
type HostKeyChangedError struct {
	Host           string
	KeyType        string
	Fingerprint    string
	KnownHostsPath string
}

func (e *HostKeyChangedError) Error() string {
	return fmt.Sprintf("host key for %s has changed (fingerprint %s)", e.Host, e.Fingerprint)
}

// HostKeyInfo is the non-secret result of verifying or trusting a key.
type HostKeyInfo struct {
	Host           string
	KeyType        string
	Fingerprint    string
	KnownHostsPath string
}

// HostKeyType returns the short OpenSSH name for a key type.
func HostKeyType(key ssh.PublicKey) string { return shortKeyType(key.Type()) }

var errHostKeyCaptured = errors.New("ssh05: host key captured")

// TrustHostKey re-probes destination with the current ssh_config and writes
// the server key to known_hosts only when its SHA-256 fingerprint matches the
// value the user approved. A changed key is never written.
func TrustHostKey(destination, configFile, fingerprint string) (HostKeyInfo, error) {
	var (
		key  ssh.PublicKey
		host string
		path string
	)
	client, err := SshLogin(&SshArgs{
		Destination:    destination,
		ConfigFile:     configFile,
		NonInteractive: true,
		HostKeyPrompt: func(promptHost string, promptKey ssh.PublicKey, knownHostsPath string) error {
			key, host, path = promptKey, promptHost, knownHostsPath
			return errHostKeyCaptured
		},
	})
	if client != nil {
		_ = client.Close()
	}
	if key == nil {
		if err == nil {
			err = errors.New("ssh05: server did not present a host key")
		}
		return HostKeyInfo{}, err
	}

	actual := ssh.FingerprintSHA256(key)
	if fingerprint != "" && actual != fingerprint {
		return HostKeyInfo{}, &HostKeyChangedError{
			Host:           host,
			KeyType:        shortKeyType(key.Type()),
			Fingerprint:    actual,
			KnownHostsPath: path,
		}
	}
	if path == "" {
		return HostKeyInfo{}, errors.New("ssh05: no writable known_hosts path in ssh_config")
	}
	if err := writeKnownHost(path, host, key); err != nil {
		return HostKeyInfo{}, fmt.Errorf("ssh05: write known_hosts: %w", err)
	}
	return HostKeyInfo{
		Host:           host,
		KeyType:        shortKeyType(key.Type()),
		Fingerprint:    actual,
		KnownHostsPath: path,
	}, nil
}
