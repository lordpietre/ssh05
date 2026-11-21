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

	"github.com/trzsz/trzsz-ssh/internal/ipc"
	"github.com/trzsz/trzsz-ssh/internal/supervisor"
)

func main() {
	socket := flag.String("socket", ipc.DefaultSocketPath(), "control socket path")
	sshConfig := flag.String("ssh-config", "", "ssh_config path (default: ~/.ssh/config)")
	terminal := flag.String("terminal", "xterm-256color", "TERM value for remote PTYs")
	maxSessions := flag.Int("max-sessions", 16, "maximum concurrent sessions")
	flag.Parse()

	backend := supervisor.NewBackend(
		&supervisor.SSHRunner{Config: supervisor.SSHConfig{ConfigPath: *sshConfig, Terminal: *terminal}},
		supervisor.Options{MaxSessions: *maxSessions},
	)
	defer backend.CloseAll()

	service := supervisor.NewService(backend, supervisor.SSHHosts{ConfigPath: *sshConfig})
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
