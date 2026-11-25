// Command ssh05d is the ssh05 local supervisor.
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

	"github.com/lordpietre/ssh05/internal/audit"
	frpmanager "github.com/lordpietre/ssh05/internal/frp"
	"github.com/lordpietre/ssh05/internal/identity"
	"github.com/lordpietre/ssh05/internal/ipc"
	"github.com/lordpietre/ssh05/internal/supervisor"
	transfermanager "github.com/lordpietre/ssh05/internal/transfer"
)

func defaultAuditPath() string {
	return filepath.Join(identity.UserStateDir(), "audit.log")
}

func main() {
	version := flag.Bool("version", false, "print version and support contact, then exit")
	socket := flag.String("socket", ipc.DefaultSocketPath(), "control socket path")
	sshConfig := flag.String("ssh-config", "", "ssh_config path (default: ~/.ssh/config)")
	keepassDB := flag.String("keepass-db", "", "KDBX database to import (read-only)")
	keepassKey := flag.String("keepass-key-file", "", "KDBX key file")
	keepassPasswordCommand := flag.String("keepass-password-command", "", "command that prints the KDBX password to stdout")
	vaultIdle := flag.Duration("vault-idle-timeout", 15*time.Minute, "lock the vault after inactivity (0 disables)")
	terminal := flag.String("terminal", "xterm-256color", "TERM value for remote PTYs")
	maxSessions := flag.Int("max-sessions", 16, "maximum concurrent sessions")
	maxPending := flag.Int("max-pending-bytes", 1<<20, "per-session pending PTY bytes before dropping old output")
	reconnectMax := flag.Int("reconnect-max", 3, "reconnect attempts before failing")
	reconnectDelay := flag.Duration("reconnect-delay", 500*time.Millisecond, "initial reconnect backoff")
	auditPath := flag.String("audit-log", defaultAuditPath(), "sanitized audit log path (empty disables)")
	auditMaxBytes := flag.Int64("audit-max-bytes", 10<<20, "audit log rotation threshold")
	auditMaxFiles := flag.Int("audit-max-files", 3, "audit files kept (current + rotations)")
	hostsOverrides := flag.String("hosts-overrides", "", "host metadata override path (default: ~/.config/ssh05/hosts.json)")
	frpServer := flag.String("frp-server", "", "optional frps address for local frpc proxies")
	frpPort := flag.Int("frp-port", 7000, "frps control port")
	frpToken := flag.String("frp-token", "", "initial frps token (generated if empty)")
	frpTokenForce := flag.Bool("frp-token-force", false, "overwrite the stored frps token (recovery)")
	frpsListen := flag.String("frps-listen", "", "start a managed local frps at host:port")
	frpsBin := flag.String("frps", "frps", "frps binary")
	frpsSHA256 := flag.String("frps-sha256", "", "pinned SHA-256 for frps (verified before start)")
	frpcBin := flag.String("frpc", "frpc", "frpc binary")
	frpcSHA256 := flag.String("frpc-sha256", "", "pinned SHA-256 for frpc (verified before start)")
	frpConfig := flag.String("frp-config", "", "FRP manager config path")
	scpBin := flag.String("scp", "scp", "scp binary for file transfers")
	sftpBin := flag.String("sftp", "sftp", "sftp binary for file transfers")
	nmapBin := flag.String("nmap", "", "optional external nmap binary; the embedded one is used by default")
	dockerHosts := flag.Bool("docker", false, "include Docker containers as hosts")
	trzszEnabled := flag.Bool("trzsz", true, "enable trz/tsz transfers inside sessions")
	zmodemEnabled := flag.Bool("zmodem", false, "enable rz/sz transfers inside sessions")
	downloadPath := flag.String("download-path", identity.DownloadDir(), "default folder for trzsz/zmodem downloads (empty: desktop picker)")
	flag.Parse()

	if *version {
		fmt.Printf("%s %s\n%s\nSoporte: %s\n", identity.Supervisor, identity.Version, identity.Vendor, identity.SupportEmail)
		return
	}

	if !*dockerHosts {
		_ = os.Setenv("TSSH_DISABLE_DOCKER", "1")
		_ = os.Setenv("SSH05_DISABLE_DOCKER", "1")
	}

	logger, err := audit.NewRotating(*auditPath, *auditMaxBytes, *auditMaxFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ssh05d: audit log: %v\n", err)
		os.Exit(1)
	}

	backend := supervisor.NewBackend(
		&supervisor.SSHRunner{Config: supervisor.SSHConfig{
			ConfigPath:   *sshConfig,
			Terminal:     *terminal,
			Trzsz:        *trzszEnabled,
			Zmodem:       *zmodemEnabled,
			DownloadPath: *downloadPath,
		}},
		supervisor.Options{
			MaxSessions:   *maxSessions,
			MaxPending:    *maxPending,
			ReconnectMax:  *reconnectMax,
			ReconnectBase: *reconnectDelay,
		},
	)
	defer backend.CloseAll()

	inventory := supervisor.NewInventory(supervisor.InventoryConfig{
		SSHConfigPath:    *sshConfig,
		OverridesPath:    *hostsOverrides,
		VaultIdleTimeout: *vaultIdle,
		Keepass: supervisor.KeePassConfig{
			Database:        *keepassDB,
			KeyFile:         *keepassKey,
			PasswordCommand: *keepassPasswordCommand,
		},
	})
	frpManager, err := frpmanager.NewManager(frpmanager.Options{
		ConfigPath:         *frpConfig,
		FrpcBin:            *frpcBin,
		FrpcSHA256:         *frpcSHA256,
		FrpsBin:            *frpsBin,
		FrpsSHA256:         *frpsSHA256,
		EndpointListen:     *frpsListen,
		EndpointAddress:    *frpServer,
		EndpointPort:       *frpPort,
		EndpointToken:      *frpToken,
		EndpointTokenForce: *frpTokenForce,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ssh05d: frp: %v\n", err)
		os.Exit(1)
	}
	defer frpManager.Close()
	transferManager := transfermanager.NewManager(transfermanager.Options{
		SSHConfig: *sshConfig,
		ScpBin:    *scpBin,
		SftpBin:   *sftpBin,
	})
	defer transferManager.Close()
	service := supervisor.NewService(backend, inventory,
		supervisor.WithAudit(logger), supervisor.WithFRP(frpManager),
		supervisor.WithTransfers(transferManager), supervisor.WithNmap(*nmapBin),
		supervisor.WithSSHConfig(*sshConfig))
	server, err := ipc.Listen(*socket, service)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ssh05d: %v\n", err)
		os.Exit(1)
	}
	defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "ssh05d: listening on %s\n", *socket)
	if err := server.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ssh05d: %v\n", err)
		os.Exit(1)
	}
}
