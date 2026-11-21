package supervisor

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/trzsz/trzsz-ssh/contracts"
	"github.com/trzsz/trzsz-ssh/tssh"
)

// InventoryConfig configures the canonical, non-secret host inventory.
type InventoryConfig struct {
	SSHConfigPath string
	Keepass       KeePassConfig
}

// Inventory merges ssh_config and KDBX entries with provenance.
type Inventory struct {
	config InventoryConfig

	mu        sync.RWMutex
	loaded    bool
	sshHosts  []contracts.HostProfile
	kpHosts   []contracts.HostProfile
	profiles  []contracts.HostProfile
	conflicts []contracts.HostConflict
	vault     contracts.VaultStatusResult
	lastErr   error
}

// NewInventory creates a lazy inventory. KDBX is imported on first List/Import
// so a missing or locked database does not prevent ssh_config from loading.
func NewInventory(config InventoryConfig) *Inventory {
	return &Inventory{config: config}
}

func (i *Inventory) ensureLoaded(ctx context.Context) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.loaded {
		return
	}
	i.loaded = true
	i.reloadLocked(ctx)
}

func (i *Inventory) reloadLocked(ctx context.Context) {
	sshHosts, err := tssh.ListHostEntries(i.config.SSHConfigPath)
	if err != nil {
		i.lastErr = err
	} else {
		i.sshHosts = make([]contracts.HostProfile, 0, len(sshHosts))
		for _, entry := range sshHosts {
			i.sshHosts = append(i.sshHosts, sshProfile(entry))
		}
	}

	i.kpHosts = nil
	i.vault = contracts.VaultStatusResult{Provider: "none", Locked: true}
	if i.config.Keepass.enabled() {
		i.vault.Provider = "keepassxc"
		entries, err := i.config.Keepass.listEntries(ctx)
		if err != nil {
			i.lastErr = err
			i.vault.Locked = true
		} else {
			i.vault.Locked = false
			for _, entry := range entries {
				if profile, ok := keepassProfile(entry); ok {
					i.kpHosts = append(i.kpHosts, profile)
				}
			}
			i.vault.Profiles = len(i.kpHosts)
		}
	}
	i.mergeLocked()
}

func (i *Inventory) mergeLocked() {
	byAlias := make(map[string]int)
	i.profiles = nil
	i.conflicts = nil

	add := func(profile contracts.HostProfile) {
		if index, ok := byAlias[profile.Alias]; ok {
			existing := &i.profiles[index]
			if targetKey(existing.Target) == targetKey(profile.Target) {
				mergeProfile(existing, profile)
				return
			}
			i.conflicts = append(i.conflicts, contracts.HostConflict{
				HostProfileID: existing.ID,
				Field:         "target",
				LocalValue:    targetLabel(existing.Target),
				RemoteValue:   targetLabel(profile.Target),
				Sources:       []string{string(existing.SourceRefs[0].Kind), string(profile.SourceRefs[0].Kind)},
			})
		}
		byAlias[profile.Alias] = len(i.profiles)
		i.profiles = append(i.profiles, profile)
	}

	for _, profile := range i.sshHosts {
		add(profile)
	}
	for _, profile := range i.kpHosts {
		add(profile)
	}
	sort.Slice(i.profiles, func(a, b int) bool {
		return strings.ToLower(i.profiles[a].Alias) < strings.ToLower(i.profiles[b].Alias)
	})
}

func mergeProfile(destination *contracts.HostProfile, source contracts.HostProfile) {
	for _, ref := range source.SourceRefs {
		duplicate := false
		for _, existing := range destination.SourceRefs {
			if existing.Kind == ref.Kind && existing.EntryID == ref.EntryID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			destination.SourceRefs = append(destination.SourceRefs, ref)
		}
	}
	if destination.AuthRef == nil {
		destination.AuthRef = source.AuthRef
	}
	for _, label := range source.Labels {
		found := false
		for _, existing := range destination.Labels {
			if existing == label {
				found = true
				break
			}
		}
		if !found {
			destination.Labels = append(destination.Labels, label)
		}
	}
	if destination.Color == "" {
		destination.Color = source.Color
	}
}

// List implements HostProvider.
func (i *Inventory) List(ctx context.Context) ([]contracts.HostProfile, error) {
	i.ensureLoaded(ctx)
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := append([]contracts.HostProfile(nil), i.profiles...)
	return out, nil
}

// Resolve finds a profile by canonical ID or alias.
func (i *Inventory) Resolve(ctx context.Context, id string) (contracts.HostProfile, bool) {
	i.ensureLoaded(ctx)
	i.mu.RLock()
	defer i.mu.RUnlock()
	for _, profile := range i.profiles {
		if profile.ID == id || profile.Alias == id {
			return profile, true
		}
	}
	return contracts.HostProfile{}, false
}

// Conflicts returns the last merge conflicts.
func (i *Inventory) Conflicts() []contracts.HostConflict {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return append([]contracts.HostConflict(nil), i.conflicts...)
}

// VaultStatus reports KeePass lock/provider state without secrets.
func (i *Inventory) VaultStatus() contracts.VaultStatusResult {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.vault
}

// Import reloads the requested source. Path is optional for ssh_config and
// KDBX; when empty the configured path is reused.
func (i *Inventory) Import(ctx context.Context, source string, path string) (contracts.HostsImportResult, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	switch source {
	case "ssh_config":
		if strings.TrimSpace(path) != "" {
			i.config.SSHConfigPath = path
		}
	case "keepass":
		if strings.TrimSpace(path) != "" {
			i.config.Keepass.Database = path
		}
	default:
		return contracts.HostsImportResult{}, fmt.Errorf("unsupported import source %q", source)
	}
	i.reloadLocked(ctx)
	return contracts.HostsImportResult{
		Imported:  len(i.profiles),
		Conflicts: append([]contracts.HostConflict(nil), i.conflicts...),
	}, nil
}

func destinationFor(profile contracts.HostProfile) string {
	for _, ref := range profile.SourceRefs {
		if ref.Kind == contracts.SourceSSHConfig && profile.Alias != "" {
			return profile.Alias
		}
	}
	port := profile.Target.Port
	if port == 0 {
		port = 22
	}
	host := profile.Target.Hostname
	if strings.Contains(host, ":") {
		host = "[" + strings.Trim(host, "[]") + "]"
	}
	user := ""
	if profile.Target.Username != "" {
		user = profile.Target.Username + "@"
	}
	return fmt.Sprintf("%s%s:%d", user, host, port)
}

func targetKey(target contracts.HostTarget) string {
	user := strings.ToLower(target.Username)
	host := strings.ToLower(target.Hostname)
	port := target.Port
	if port == 0 {
		port = 22
	}
	return fmt.Sprintf("%s@%s:%d", user, host, port)
}

func targetLabel(target contracts.HostTarget) string {
	user := ""
	if target.Username != "" {
		user = target.Username + "@"
	}
	return fmt.Sprintf("%s%s:%d", user, target.Hostname, target.Port)
}

func sshProfile(entry tssh.HostEntry) contracts.HostProfile {
	port := 22
	if parsed, err := strconv.Atoi(entry.Port); err == nil && parsed > 0 && parsed <= 65535 {
		port = parsed
	}
	hostname := entry.Host
	if hostname == "" {
		hostname = entry.Alias
	}
	profile := contracts.HostProfile{
		ID:    entry.Alias,
		Alias: entry.Alias,
		Target: contracts.HostTarget{
			Hostname: hostname,
			Port:     port,
			Username: entry.User,
		},
		SourceRefs: []contracts.SourceReference{{
			Kind:    contracts.SourceSSHConfig,
			EntryID: entry.Alias,
		}},
		Discovered:       true,
		VerifiedIdentity: false,
	}
	for _, jump := range strings.Split(entry.ProxyJump, ",") {
		if jump = strings.TrimSpace(jump); jump != "" {
			profile.ProxyJump = append(profile.ProxyJump, jump)
		}
	}
	return profile
}

func keepassProfile(entry keepassEntry) (contracts.HostProfile, bool) {
	hostname := strings.TrimSpace(entry.Host)
	username := strings.TrimSpace(entry.User)
	portText := strings.TrimSpace(entry.Port)
	if entry.URL != "" {
		parsed, err := url.Parse(entry.URL)
		if err != nil {
			return contracts.HostProfile{}, false
		}
		if parsed.Scheme != "" && !strings.EqualFold(parsed.Scheme, "ssh") {
			// Do not interpret arbitrary URLs as SSH hosts.
			if hostname == "" {
				return contracts.HostProfile{}, false
			}
		} else {
			if hostname == "" {
				hostname = parsed.Hostname()
			}
			if username == "" && parsed.User != nil {
				username = parsed.User.Username()
			}
			if portText == "" {
				portText = parsed.Port()
			}
		}
	}
	if hostname == "" {
		return contracts.HostProfile{}, false
	}
	port := 22
	if parsed, err := strconv.Atoi(portText); err == nil && parsed > 0 && parsed <= 65535 {
		port = parsed
	}
	alias := strings.TrimSpace(entry.Title)
	if alias == "" {
		alias = hostname
	}
	id := "keepass:" + entry.UUID
	profile := contracts.HostProfile{
		ID:    id,
		Alias: alias,
		Target: contracts.HostTarget{
			Hostname: hostname,
			Port:     port,
			Username: username,
		},
		SourceRefs: []contracts.SourceReference{{
			Kind:        contracts.SourceKeePass,
			EntryID:     entry.UUID,
			Fingerprint: entry.UUID,
		}},
		AuthRef: &contracts.CredentialReference{
			Provider:    "keepassxc",
			EntryID:     entry.UUID,
			Fingerprint: entry.UUID,
		},
		Discovered:       true,
		VerifiedIdentity: false,
	}
	for _, label := range parseKeePassLabels(entry.Notes) {
		profile.Labels = append(profile.Labels, label)
	}
	profile.Color = parseKeePassColor(entry.Notes)
	return profile, true
}

func parseKeePassColor(notes string) string {
	for _, token := range strings.FieldsFunc(notes, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	}) {
		token = strings.TrimSpace(token)
		if strings.HasPrefix(strings.ToLower(token), "color=") {
			value := strings.TrimSpace(token[len("color="):])
			if strings.HasPrefix(value, "#") && (len(value) == 7 || len(value) == 4) {
				return value
			}
		}
	}
	return ""
}

func parseKeePassLabels(notes string) []string {
	var labels []string
	for _, token := range strings.FieldsFunc(notes, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	}) {
		token = strings.TrimSpace(token)
		for _, prefix := range []string{"group=", "label=", "tag="} {
			if strings.HasPrefix(strings.ToLower(token), prefix) {
				value := strings.TrimSpace(token[len(prefix):])
				if value != "" {
					labels = append(labels, value)
				}
			}
		}
	}
	return labels
}
