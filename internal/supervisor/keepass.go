package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// KeePassConfig configures the keepassxc-cli importer. The master password is
// never stored; it is read from an explicit command (askpass pattern) when the
// database is not key-file-only.
type KeePassConfig struct {
	Database        string
	KeyFile         string
	PasswordCommand string
	CLI             string
}

func (c KeePassConfig) enabled() bool {
	return strings.TrimSpace(c.Database) != ""
}

func (c KeePassConfig) cli() string {
	if strings.TrimSpace(c.CLI) != "" {
		return c.CLI
	}
	return "keepassxc-cli"
}

func expandHome(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// keepassEntry is a non-secret KDBX entry view.
type keepassEntry struct {
	Title string
	UUID  string
	URL   string
	User  string
	Host  string
	Port  string
	Notes string
}

func (c KeePassConfig) commandArgs(db, keyFile string) []string {
	args := make([]string, 0, 5)
	if keyFile != "" {
		args = append(args, "--no-password", "-k", keyFile)
	}
	args = append(args, db)
	return args
}

func (c KeePassConfig) command(ctx context.Context, subcommand string, subOptions []string, trailing ...string) (*exec.Cmd, error) {
	db := expandHome(c.Database)
	keyFile := expandHome(c.KeyFile)
	if db == "" {
		return nil, fmt.Errorf("keepass database is not configured")
	}
	if _, err := os.Stat(db); err != nil {
		return nil, fmt.Errorf("keepass database: %w", err)
	}
	if keyFile != "" {
		if _, err := os.Stat(keyFile); err != nil {
			return nil, fmt.Errorf("keepass key file: %w", err)
		}
	}
	if keyFile == "" && strings.TrimSpace(c.PasswordCommand) == "" {
		return nil, fmt.Errorf("keepass database requires a key file or password command")
	}

	args := []string{subcommand}
	args = append(args, subOptions...)
	args = append(args, c.commandArgs(db, keyFile)...)
	args = append(args, trailing...)
	cmd := exec.CommandContext(ctx, c.cli(), args...)
	if keyFile == "" {
		password, err := c.readPassword(ctx)
		if err != nil {
			return nil, err
		}
		cmd.Stdin = strings.NewReader(password + "\n")
	}
	return cmd, nil
}

func (c KeePassConfig) readPassword(ctx context.Context) (string, error) {
	command := strings.TrimSpace(c.PasswordCommand)
	if command == "" {
		return "", fmt.Errorf("keepass password command is empty")
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("keepass password command: %w", err)
	}
	return strings.TrimRight(string(output), "\r\n"), nil
}

func (c KeePassConfig) listEntries(ctx context.Context) ([]keepassEntry, error) {
	cmd, err := c.command(ctx, "ls", []string{"-q", "-R", "-f"})
	if err != nil {
		return nil, err
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("keepass ls: %w", err)
	}

	var entries []keepassEntry
	for _, line := range strings.Split(string(output), "\n") {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		entry, err := c.showEntry(ctx, path)
		if err != nil {
			// One unreadable entry must not hide the rest.
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (c KeePassConfig) showEntry(ctx context.Context, path string) (keepassEntry, error) {
	cmd, err := c.command(ctx, "show", []string{"-q", "--all"}, path)
	if err != nil {
		return keepassEntry{}, err
	}
	output, err := cmd.Output()
	if err != nil {
		return keepassEntry{}, fmt.Errorf("keepass show %q: %w", path, err)
	}
	entry := keepassEntry{Title: path}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Title":
			entry.Title = value
		case "Uuid":
			entry.UUID = strings.Trim(value, "{}")
		case "UserName":
			entry.User = value
		case "URL":
			entry.URL = value
		case "Notes":
			entry.Notes = value
		case "HostName", "Host":
			entry.Host = value
		case "Port":
			entry.Port = value
		}
	}
	if entry.UUID == "" {
		entry.UUID = path
	}
	return entry, nil
}
