// Command trssd is the TRSS local supervisor.
//
// It exposes the typed IPC contract over an authenticated Unix socket. The UI
// is a separate process; see contracts/ and internal/ipc/.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/trzsz/trzsz-ssh/internal/audit"
	frpmanager "github.com/trzsz/trzsz-ssh/internal/frp"
	"github.com/trzsz/trzsz-ssh/internal/ipc"
	"github.com/trzsz/trzsz-ssh/internal/supervisor"
)

func defaultAuditPath() string {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "trss", "audit.log")
}

func main() {
	socket := flag.String("socket", ipc.DefaultSocketPath(), "control socket path")
	sshConfig := flag.String("ssh-config", "", "ssh_config path (default: ~/.ssh/config)")
	keepassDB := flag.String("keepass-db", "", "KDBX database to import (read-only)")
	keepassKey := flag.String("keepass-key-file", "", "KDBX key file")
	keepassPasswordCommand := flag.String("keepass-password-command", "", "command that prints the KDBX password to stdout")
	vaultIdle := flag.Duration("vault-idle-timeout", 15*time.Minute, "lock the vault after inactivity (0 disables)")
	terminal := flag.String("terminal", "xterm-256color", "TERM value for remote PTYs")
	maxSessions := flag.Int("max-sessions", 16, "maximum concurrent sessions")
	reconnectMax := flag.Int("reconnect-max", 3, "reconnect attempts before failing")
	reconnectDelay := flag.Duration("reconnect-delay", 500*time.Millisecond, "initial reconnect backoff")
	auditPath := flag.String("audit-log", defaultAuditPath(), "sanitized audit log path (empty disables)")
	auditMaxBytes := flag.Int64("audit-max-bytes", 10<<20, "audit log rotation threshold")
	frpServer := flag.String("frp-server", "", "optional frps address for local frpc proxies")
	frpPort := flag.Int("frp-port", 7000, "frps control port")
	frpToken := flag.String("frp-token", "", "initial frps token (generated if empty)")
	frpcBin := flag.String("frpc", "frpc", "frpc binary")
	frpcSHA256 := flag.String("frpc-sha256", "", "pinned SHA-256 for frpc (verified before start)")
	frpConfig := flag.String("frp-config", "", "FRP manager config path")
	flag.Parse()

	logger, err := audit.New(*auditPath, *auditMaxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trssd: audit log: %v\n", err)
		os.Exit(1)
	}

	backend := supervisor.NewBackend(
		&supervisor.SSHRunner{Config: supervisor.SSHConfig{ConfigPath: *sshConfig, Terminal: *terminal}},
		supervisor.Options{
			MaxSessions:   *maxSessions,
			ReconnectMax:  *reconnectMax,
			ReconnectBase: *reconnectDelay,
		},
	)
	defer backend.CloseAll()

	inventory := supervisor.NewInventory(supervisor.InventoryConfig{
		SSHConfigPath:    *sshConfig,
		VaultIdleTimeout: *vaultIdle,
		Keepass: supervisor.KeePassConfig{
			Database:        *keepassDB,
			KeyFile:         *keepassKey,
			PasswordCommand: *keepassPasswordCommand,
		},
	})
	frpManager, err := frpmanager.NewManager(frpmanager.Options{
		ConfigPath:      *frpConfig,
		FrpcBin:         *frpcBin,
		FrpcSHA256:      *frpcSHA256,
		EndpointAddress: *frpServer,
		EndpointPort:    *frpPort,
		EndpointToken:   *frpToken,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "trssd: frp: %v\n", err)
		os.Exit(1)
	}
	defer frpManager.Close()
	service := supervisor.NewService(backend, inventory,
		supervisor.WithAudit(logger), supervisor.WithFRP(frpManager))
	server, err := ipc.Listen(*socket, service)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trssd: %v\n", err)
		os.Exit(1)
	}
	defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "trssd: listening on %s\n", *socket)
	if err := server.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "trssd: %v\n", err)
		os.Exit(1)
	}
}
