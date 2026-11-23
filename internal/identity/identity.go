// Package identity centralizes the product name, vendor details and the
// on-disk locations shared by the ssh05 client, its supervisor and the UI.
//
// The former "trss" names are still accepted as input: environment variables
// fall back to TRSS_* and an existing ~/.config/trss (or ~/.local/state/trss)
// directory is adopted once by renaming it to ssh05.
package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// Name is the product and command name used in paths and messages.
	Name = "ssh05"
	// Supervisor is the name of the local supervisor daemon.
	Supervisor = "ssh05d"
	// Version is the client and supervisor version.
	Version = "0.3.28"
	// Vendor owns this distribution.
	Vendor = "Dinámicas Electrónicas"
	// SupportEmail is the public support contact.
	SupportEmail = "dinamicas.electronicas@gmail.com"
	// LegacyName is the previous internal name, kept for migration only.
	LegacyName = "trss"
)

// Env returns the value of SSH05_<suffix>, falling back to the legacy
// TRSS_<suffix> variable so existing scripts and services keep working.
func Env(suffix string) string {
	if value := strings.TrimSpace(os.Getenv("SSH05_" + suffix)); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("TRSS_" + suffix))
}

// ConfigDir returns the private configuration directory, adopting a legacy
// ~/.config/trss directory the first time it is observed.
func ConfigDir(env map[string]string) string {
	return adoptedDir(env, "XDG_CONFIG_HOME", ".config")
}

// StateDir returns the per-user state directory, adopting a legacy
// ~/.local/state/trss directory the first time it is observed.
func StateDir(env map[string]string) string {
	return adoptedDir(env, "XDG_STATE_HOME", filepath.Join(".local", "state"))
}

// BaseDir resolves an XDG base directory from env (falling back to HOME) and
// appends the product name without performing migration.
func BaseDir(env map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(env[key]); value != "" {
		return filepath.Join(value, Name)
	}
	home := strings.TrimSpace(env["HOME"])
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, fallback, Name)
}

// LegacyDir is the former product directory for the given XDG base.
func LegacyDir(env map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(env[key]); value != "" {
		return filepath.Join(value, LegacyName)
	}
	home := strings.TrimSpace(env["HOME"])
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, fallback, LegacyName)
}

// DownloadDir returns the user's existing Downloads folder, or an empty
// string when it does not exist. It is used as the default destination for
// trzsz/zmodem downloads without creating directories.
func DownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	dir := filepath.Join(home, "Downloads")
	if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
		return dir
	}
	return ""
}

// RuntimeDir returns a private per-user runtime directory for the control
// socket. It is never adopted from the legacy tree because sockets are tied to
// the live session.
func RuntimeDir(env map[string]string) string {
	if value := strings.TrimSpace(env["XDG_RUNTIME_DIR"]); value != "" {
		return filepath.Join(value, Name)
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d", Name, userID()))
}

// SocketPath is the default control socket path used by the supervisor and UI.
func SocketPath(env map[string]string) string {
	if value := envValue(env, "SOCKET"); value != "" {
		return expandHome(value)
	}
	return filepath.Join(RuntimeDir(env), "control.sock")
}

// LegacySocketPath is the socket path used before the rename.
func LegacySocketPath(env map[string]string) string {
	if value := strings.TrimSpace(env["XDG_RUNTIME_DIR"]); value != "" {
		return filepath.Join(value, LegacyName, "control.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d", LegacyName, userID()), "control.sock")
}

// EnvMap returns the process environment as a map, useful for the helpers.
func EnvMap() map[string]string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

// UserConfigDir is ConfigDir for the current process environment.
func UserConfigDir() string { return ConfigDir(EnvMap()) }

// UserStateDir is StateDir for the current process environment.
func UserStateDir() string { return StateDir(EnvMap()) }

// UserSocketPath is SocketPath for the current process environment.
func UserSocketPath() string { return SocketPath(EnvMap()) }

// LegacyUserSocketPath is LegacySocketPath for the current process environment.
func LegacyUserSocketPath() string { return LegacySocketPath(EnvMap()) }

func adoptedDir(env map[string]string, key, fallback string) string {
	adoptionMu.Lock()
	defer adoptionMu.Unlock()
	target := BaseDir(env, key, fallback)
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		return target
	}
	legacy := LegacyDir(env, key, fallback)
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		if err := os.Rename(legacy, target); err != nil {
			// A concurrent process may have completed the migration first.
			if info, statErr := os.Stat(target); statErr == nil && info.IsDir() {
				return target
			}
			// A read-only or cross-device home keeps the legacy directory.
			return legacy
		}
		return target
	}
	return target
}

var adoptionMu sync.Mutex

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

func envValue(env map[string]string, suffix string) string {
	if value := strings.TrimSpace(env["SSH05_"+suffix]); value != "" {
		return value
	}
	return strings.TrimSpace(env["TRSS_"+suffix])
}
