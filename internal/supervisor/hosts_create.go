package supervisor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lordpietre/ssh05/contracts"
)

var hostAliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// CreateHost appends a Host block to the configured ssh_config and reloads the
// inventory. It never writes passwords or private key material.
func (i *Inventory) CreateHost(ctx context.Context, params contracts.HostCreateParams) (contracts.HostProfile, error) {
	i.ensureLoaded(ctx)
	params.Alias = strings.TrimSpace(params.Alias)
	params.Hostname = strings.TrimSpace(params.Hostname)
	params.Username = strings.TrimSpace(params.Username)
	params.IdentityFile = strings.TrimSpace(params.IdentityFile)
	if !hostAliasPattern.MatchString(params.Alias) {
		return contracts.HostProfile{}, fmt.Errorf("invalid alias %q", params.Alias)
	}
	if params.Hostname == "" || strings.ContainsAny(params.Hostname, " \t\r\n\x00") {
		return contracts.HostProfile{}, fmt.Errorf("invalid hostname")
	}
	if strings.ContainsAny(params.Username, " \t\r\n\x00") || strings.ContainsAny(params.IdentityFile, "\r\n\x00") {
		return contracts.HostProfile{}, fmt.Errorf("invalid user or identity file")
	}
	if params.Port == 0 {
		params.Port = 22
	}
	if params.Port < 1 || params.Port > 65535 {
		return contracts.HostProfile{}, fmt.Errorf("invalid port %d", params.Port)
	}
	path := i.sshConfigPath()
	if path == "" {
		return contracts.HostProfile{}, fmt.Errorf("SSH config is disabled")
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	for _, profile := range i.profiles {
		if profile.Alias == params.Alias || profile.ID == params.Alias {
			return contracts.HostProfile{}, fmt.Errorf("host %q already exists", params.Alias)
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return contracts.HostProfile{}, err
	}
	block := renderHostBlock(params)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return contracts.HostProfile{}, err
	}
	info, statErr := file.Stat()
	if statErr == nil && info.Size() > 0 {
		block = "\n" + block
	}
	if _, err := file.WriteString(block); err != nil {
		_ = file.Close()
		return contracts.HostProfile{}, err
	}
	if err := file.Close(); err != nil {
		return contracts.HostProfile{}, err
	}
	i.reloadLocked(ctx)
	for _, profile := range i.profiles {
		if profile.Alias == params.Alias {
			return profile, nil
		}
	}
	return contracts.HostProfile{}, fmt.Errorf("host %q was written but not discovered", params.Alias)
}

func (i *Inventory) sshConfigPath() string {
	path := strings.TrimSpace(i.config.SSHConfigPath)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".ssh", "config")
	}
	if strings.EqualFold(path, "none") {
		return ""
	}
	return expandHome(path)
}

func renderHostBlock(params contracts.HostCreateParams) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Host %s\n", params.Alias)
	fmt.Fprintf(&builder, "    HostName %s\n", params.Hostname)
	fmt.Fprintf(&builder, "    Port %d\n", params.Port)
	if params.Username != "" {
		fmt.Fprintf(&builder, "    User %s\n", params.Username)
	}
	if params.IdentityFile != "" {
		fmt.Fprintf(&builder, "    IdentityFile %s\n", params.IdentityFile)
	}
	return builder.String()
}
