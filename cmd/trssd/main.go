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
	"syscall"
	"time"

	"github.com/trzsz/trzsz-ssh/internal/ipc"
	"github.com/trzsz/trzsz-ssh/internal/supervisor"
)

func main() {
	socket := flag.String("socket", ipc.DefaultSocketPath(), "control socket path")
	sshConfig := flag.String("ssh-config", "", "ssh_config path (default: ~/.ssh/config)")
	keepassDB := flag.String("keepass-db", "", "KDBX database to import (read-only)")
	keepassKey := flag.String("keepass-key-file", "", "KDBX key file")
	keepassPasswordCommand := flag.String("keepass-password-command", "", "command that prints the KDBX password to stdout")
	terminal := flag.String("terminal", "xterm-256color", "TERM value for remote PTYs")
	maxSessions := flag.Int("max-sessions", 16, "maximum concurrent sessions")
	reconnectMax := flag.Int("reconnect-max", 3, "reconnect attempts before failing")
	reconnectDelay := flag.Duration("reconnect-delay", 500*time.Millisecond, "initial reconnect backoff")
	flag.Parse()

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
		SSHConfigPath: *sshConfig,
		Keepass: supervisor.KeePassConfig{
			Database:        *keepassDB,
			KeyFile:         *keepassKey,
			PasswordCommand: *keepassPasswordCommand,
		},
	})
	service := supervisor.NewService(backend, inventory)
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
