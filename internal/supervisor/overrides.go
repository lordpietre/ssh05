package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lordpietre/ssh05/contracts"
)

// hostOverride is the user's explicit metadata for a canonical host. It lives
// outside ssh_config/KDBX so imports never overwrite manual choices.
type hostOverride struct {
	Labels   []string `json:"labels,omitempty"`
	Favorite *bool    `json:"favorite,omitempty"`
	Color    string   `json:"color,omitempty"`
}

type overrideFile struct {
	Hosts map[string]hostOverride `json:"hosts"`
}

func defaultOverridesPath() string {
	if value := strings.TrimSpace(os.Getenv("TRSS_HOST_OVERRIDES")); value != "" {
		return expandHome(value)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "trss", "hosts.json")
}

func (i *Inventory) loadOverridesLocked() {
	i.overrides = make(map[string]hostOverride)
	path := i.config.OverridesPath
	if path == "" {
		path = defaultOverridesPath()
	} else {
		path = expandHome(path)
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		i.lastErr = err
		return
	}
	var file overrideFile
	if err := json.Unmarshal(data, &file); err != nil {
		i.lastErr = fmt.Errorf("host overrides: %w", err)
		return
	}
	if file.Hosts != nil {
		i.overrides = file.Hosts
	}
}

func (i *Inventory) saveOverridesLocked() error {
	path := i.config.OverridesPath
	if path == "" {
		path = defaultOverridesPath()
	} else {
		path = expandHome(path)
	}
	data, err := json.MarshalIndent(overrideFile{Hosts: i.overrides}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (i *Inventory) applyOverridesLocked() {
	for index := range i.profiles {
		override, ok := i.overrides[i.profiles[index].ID]
		if !ok {
			continue
		}
		profile := &i.profiles[index]
		if len(override.Labels) > 0 {
			for _, label := range override.Labels {
				if !containsString(profile.Labels, label) {
					profile.Labels = append(profile.Labels, label)
				}
			}
		}
		if override.Favorite != nil {
			profile.Favorite = *override.Favorite
		}
		if override.Color != "" {
			profile.Color = override.Color
		}
		if !hasManualRef(profile.SourceRefs) {
			profile.SourceRefs = append(profile.SourceRefs, contracts.SourceReference{
				Kind:    contracts.SourceManual,
				EntryID: "override",
			})
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func hasManualRef(refs []contracts.SourceReference) bool {
	for _, ref := range refs {
		if ref.Kind == contracts.SourceManual {
			return true
		}
	}
	return false
}

// SetOverride applies explicit user metadata and persists it. Fields left nil
// are unchanged; an empty color clears the override color.
func (i *Inventory) SetOverride(ctx context.Context, patch contracts.HostOverridePatch) (contracts.HostProfile, error) {
	i.ensureLoaded(ctx)
	i.mu.Lock()
	defer i.mu.Unlock()
	index := -1
	for pos := range i.profiles {
		if i.profiles[pos].ID == patch.HostProfileID || i.profiles[pos].Alias == patch.HostProfileID {
			index = pos
			break
		}
	}
	if index < 0 {
		return contracts.HostProfile{}, fmt.Errorf("host %q not found", patch.HostProfileID)
	}
	id := i.profiles[index].ID
	override := i.overrides[id]
	if patch.Labels != nil {
		override.Labels = sanitizeLabels(*patch.Labels)
	}
	if patch.Favorite != nil {
		value := *patch.Favorite
		override.Favorite = &value
	}
	if patch.Color != nil {
		color, err := sanitizeColor(*patch.Color)
		if err != nil {
			return contracts.HostProfile{}, err
		}
		override.Color = color
	}
	if len(override.Labels) == 0 && override.Favorite == nil && override.Color == "" {
		delete(i.overrides, id)
	} else {
		i.overrides[id] = override
	}
	if err := i.saveOverridesLocked(); err != nil {
		return contracts.HostProfile{}, err
	}
	i.reloadLocked(ctx)
	for pos := range i.profiles {
		if i.profiles[pos].ID == id {
			return i.profiles[pos], nil
		}
	}
	return contracts.HostProfile{}, fmt.Errorf("host %q disappeared after reload", id)
}

func sanitizeLabels(labels []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" || len([]rune(label)) > 64 || seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
		if len(out) == 32 {
			break
		}
	}
	return out
}

func sanitizeColor(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	raw := strings.TrimPrefix(value, "#")
	if len(raw) == 3 {
		raw = string([]byte{raw[0], raw[0], raw[1], raw[1], raw[2], raw[2]})
	}
	if len(raw) != 6 {
		return "", fmt.Errorf("invalid color %q", value)
	}
	for _, char := range raw {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return "", fmt.Errorf("invalid color %q", value)
		}
	}
	return "#" + strings.ToUpper(raw), nil
}
