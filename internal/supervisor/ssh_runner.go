package supervisor

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/trzsz/shellescape"
	"github.com/trzsz/trzsz-ssh/contracts"
	"github.com/trzsz/trzsz-ssh/tssh"
	"golang.org/x/crypto/ssh"
)

// SSHConfig configures the Go SSH runner.
type SSHConfig struct {
	ConfigPath string
	Terminal   string
}

func tmuxStartCommand(sessionName string) string {
	return "tmux new-session -A -s " + shellescape.Quote(sessionName)
}

// SSHRunner starts sessions with the in-repo Go SSH client.
type SSHRunner struct {
	Config SSHConfig
}

// Start opens an SSH client, requests a PTY and starts a remote shell.
func (r *SSHRunner) Start(ctx context.Context, profileID string, cols, rows int, options contracts.SessionOptions) (RunSession, error) {
	if profileID == "" {
		return nil, fmt.Errorf("supervisor: empty profileID")
	}

	client, err := tssh.SshLogin(&tssh.SshArgs{
		Destination: profileID,
		ConfigFile:  r.Config.ConfigPath,
	})
	if err != nil {
		return nil, fmt.Errorf("supervisor: ssh login %q: %w", profileID, err)
	}

	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("supervisor: new session: %w", err)
	}

	terminal := r.Config.Terminal
	if terminal == "" {
		terminal = "xterm-256color"
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty(terminal, rows, cols, modes); err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, fmt.Errorf("supervisor: request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, fmt.Errorf("supervisor: stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, fmt.Errorf("supervisor: stdout pipe: %w", err)
	}
	if options.TmuxSession != "" {
		if err := session.Start(tmuxStartCommand(options.TmuxSession)); err != nil {
			_ = session.Close()
			_ = client.Close()
			return nil, fmt.Errorf("supervisor: start tmux: %w", err)
		}
	} else if err := session.Shell(); err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, fmt.Errorf("supervisor: start shell: %w", err)
	}

	run := &sshRun{
		client:  client,
		session: session,
		stdin:   stdin,
		stdout:  stdout,
		output:  make(chan []byte, 64),
		done:    make(chan struct{}),
	}
	go run.pump()
	go run.wait()
	go func() {
		select {
		case <-ctx.Done():
			_ = run.Close()
		case <-run.done:
		}
	}()
	return run, nil
}

type sshRun struct {
	client  tssh.SshClient
	session tssh.SshSession
	stdin   io.WriteCloser
	stdout  io.Reader

	output chan []byte
	done   chan struct{}

	mu        sync.Mutex
	closed    bool
	exitCode  int
	closeOnce sync.Once
}

func (r *sshRun) Output() <-chan []byte {
	return r.output
}

func (r *sshRun) Done() <-chan struct{} {
	return r.done
}

func (r *sshRun) ExitCode() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exitCode
}

func (r *sshRun) Write(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrSessionClosed
	}
	_, err := r.stdin.Write(data)
	return err
}

func (r *sshRun) Resize(cols, rows int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrSessionClosed
	}
	return r.session.WindowChange(rows, cols)
}

func (r *sshRun) Signal(signal string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrSessionClosed
	}
	sender, ok := r.session.(tssh.SshSignalSender)
	if !ok {
		return fmt.Errorf("supervisor: transport does not support signals")
	}
	return sender.Signal(ssh.Signal(signal))
}

func (r *sshRun) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
		_ = r.session.Close()
		_ = r.client.Close()
	})
	return nil
}

func (r *sshRun) pump() {
	defer close(r.output)
	buffer := make([]byte, 32*1024)
	for {
		n, err := r.stdout.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			select {
			case r.output <- chunk:
			case <-r.done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (r *sshRun) wait() {
	defer close(r.done)
	err := r.session.Wait()
	code := r.session.GetExitCode()
	if err != nil && code == 0 {
		code = -1
	}
	r.mu.Lock()
	r.exitCode = code
	r.mu.Unlock()
}
