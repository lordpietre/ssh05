package supervisor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lordpietre/ssh05/contracts"
	"github.com/lordpietre/ssh05/ssh05"
)

var hostAliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// CreateHost appends a Host block to the configured ssh_config and reloads the
// inventory. An optional password is stored as an encoded `#!! encPassword`
// entry (same format as the classic client); private keys are never written.
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
	if strings.ContainsAny(params.Password, "\r\n\x00") {
		return contracts.HostProfile{}, fmt.Errorf("invalid password")
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
	block, err := renderHostBlock(params)
	if err != nil {
		return contracts.HostProfile{}, err
	}
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

// UpdateHost rewrites the Host block for an existing alias. An empty password
// keeps the encoded secret already present in the block.
func (i *Inventory) UpdateHost(ctx context.Context, params contracts.HostUpdateParams) (contracts.HostProfile, error) {
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
	if strings.ContainsAny(params.Password, "\r\n\x00") {
		return contracts.HostProfile{}, fmt.Errorf("invalid password")
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
	data, err := os.ReadFile(path)
	if err != nil {
		return contracts.HostProfile{}, err
	}
	lines := strings.Split(string(data), "\n")
	start, end := findHostBlock(lines, params.Alias)
	if start < 0 {
		return contracts.HostProfile{}, fmt.Errorf("host %q not found", params.Alias)
	}
	block, err := renderHostBlock(contracts.HostCreateParams{
		Alias:        params.Alias,
		Hostname:     params.Hostname,
		Port:         params.Port,
		Username:     params.Username,
		IdentityFile: params.IdentityFile,
		Password:     params.Password,
	})
	if err != nil {
		return contracts.HostProfile{}, err
	}
	if params.IdentityFile == "" {
		if kept := keepConfigLine(lines[start:end], "IdentityFile"); kept != "" {
			block = strings.TrimRight(block, "\n") + "\n" + kept + "\n"
		}
	}
	if params.Password == "" {
		kept := keepSecretLines(lines[start:end])
		if len(kept) > 0 {
			block = strings.TrimRight(block, "\n") + "\n" + strings.Join(kept, "\n") + "\n"
		}
	}
	rewritten := make([]string, 0, len(lines)+8)
	rewritten = append(rewritten, lines[:start]...)
	rewritten = append(rewritten, strings.Split(strings.TrimRight(block, "\n"), "\n")...)
	rewritten = append(rewritten, lines[end:]...)
	if err := writeConfigAtomic(path, []byte(strings.Join(rewritten, "\n"))); err != nil {
		return contracts.HostProfile{}, err
	}
	i.reloadLocked(ctx)
	for _, profile := range i.profiles {
		if profile.Alias == params.Alias {
			return profile, nil
		}
	}
	return contracts.HostProfile{}, fmt.Errorf("host %q was updated but not discovered", params.Alias)
}

// DeleteHost removes the Host block for an alias from ssh_config.
func (i *Inventory) DeleteHost(ctx context.Context, params contracts.HostDeleteParams) error {
	alias := strings.TrimSpace(params.Alias)
	if !hostAliasPattern.MatchString(alias) {
		return fmt.Errorf("invalid alias %q", alias)
	}
	path := i.sshConfigPath()
	if path == "" {
		return fmt.Errorf("SSH config is disabled")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	start, end := findHostBlock(lines, alias)
	if start < 0 {
		return fmt.Errorf("host %q not found", alias)
	}
	rewritten := append([]string{}, lines[:start]...)
	rewritten = append(rewritten, lines[end:]...)
	if err := writeConfigAtomic(path, []byte(strings.Join(rewritten, "\n"))); err != nil {
		return err
	}
	i.reloadLocked(ctx)
	return nil
}

func findHostBlock(lines []string, alias string) (int, int) {
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "host ") {
			continue
		}
		matched := false
		for _, pattern := range strings.Fields(trimmed)[1:] {
			if pattern == alias {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		end := len(lines)
		for next := index + 1; next < len(lines); next++ {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lines[next])), "host ") {
				end = next
				break
			}
		}
		return index, end
	}
	return -1, -1
}

func keepConfigLine(block []string, key string) string {
	prefix := strings.ToLower(key) + " "
	for _, line := range block {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), prefix) {
			return "    " + trimmed
		}
	}
	return ""
}

func keepSecretLines(block []string) []string {
	var kept []string
	for _, line := range block {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#!! encPassword ") || strings.HasPrefix(trimmed, "#!! encQuestionAnswer1 ") {
			kept = append(kept, "    "+trimmed)
		}
	}
	return kept
}

func writeConfigAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".ssh05-config-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
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

func renderHostBlock(params contracts.HostCreateParams) (string, error) {
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
	if params.Password != "" {
		secret, err := ssh05.EncodeSecret(params.Password)
		if err != nil {
			return "", fmt.Errorf("encode password: %w", err)
		}
		fmt.Fprintf(&builder, "    #!! encPassword %s\n", secret)
		fmt.Fprintf(&builder, "    #!! encQuestionAnswer1 %s\n", secret)
	}
	return builder.String(), nil
}
