package supervisor

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/lordpietre/ssh05/contracts"
	"github.com/lordpietre/ssh05/ssh05"
	"github.com/trzsz/shellescape"
	"github.com/trzsz/trzsz-go/trzsz"
	"golang.org/x/crypto/ssh"
)

// SSHConfig configures the Go SSH runner.
type SSHConfig struct {
	ConfigPath string
	Terminal   string
	// Trzsz enables trz/tsz transfers inside the session stream.
	Trzsz bool
	// Zmodem enables rz/sz transfers inside the session stream.
	Zmodem bool
	// DownloadPath is the default folder for trzsz/zmodem downloads. An empty
	// value keeps the desktop file picker.
	DownloadPath string
}

// sessionFilter bridges a supervisor session with the trzsz/zmodem filter.
//
// The filter inspects server output for trz/tsz/rz/sz triggers and, when one
// is found, performs the transfer over the same PTY stream. Bytes outside a
// transfer pass through unchanged.
type sessionFilter struct {
	input  *io.PipeWriter
	output *io.PipeReader
	filter *trzsz.TrzszFilter
}

func newSessionFilter(serverIn io.WriteCloser, serverOut io.Reader, columns int32, zmodem bool, downloadPath string) *sessionFilter {
	clientIn, clientInWriter := io.Pipe()
	clientOutReader, clientOut := io.Pipe()
	filter := trzsz.NewTrzszFilter(clientIn, clientOut, serverIn, serverOut, trzsz.TrzszOptions{
		TerminalColumns: columns,
		EnableZmodem:    zmodem,
		EnableOSC52:     false,
	})
	if downloadPath != "" {
		filter.SetDefaultDownloadPath(downloadPath)
	}
	return &sessionFilter{input: clientInWriter, output: clientOutReader, filter: filter}
}

func (f *sessionFilter) Resize(cols int32) { f.filter.SetTerminalColumns(cols) }

// Close stops the filter and unblocks its read loop.
func (f *sessionFilter) Close() {
	f.filter.Close()
	_ = f.input.Close()
	_ = f.output.Close()
}

func tmuxStartCommand(sessionName string) string {
	return "tmux new-session -A -s " + shellescape.Quote(sessionName)
}

func tmuxAttachCommand(sessionName string) string {
	return "tmux attach-session -t " + shellescape.Quote(sessionName)
}

func tmuxSessionExists(client ssh05.SshClient, sessionName string) bool {
	probe, err := client.NewSession()
	if err != nil {
		return false
	}
	defer func() { _ = probe.Close() }()
	_, err = probe.CombinedOutput("tmux has-session -t " + shellescape.Quote(sessionName))
	return err == nil
}

// supervisorHostKeyPrompt never reads /dev/tty. It converts an unknown key
// into a typed IPC challenge for the UI, even when ssh05d was started from a
// terminal.
func supervisorHostKeyPrompt(host string, key ssh.PublicKey, knownHostsPath string) error {
	return &ssh05.HostKeyUnknownError{
		Host:           host,
		KeyType:        ssh05.HostKeyType(key),
		Fingerprint:    ssh.FingerprintSHA256(key),
		KnownHostsPath: knownHostsPath,
		CanTrust:       true,
	}
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
	destination := options.Destination
	if destination == "" {
		destination = profileID
	}

	client, err := ssh05.SshLogin(&ssh05.SshArgs{
		Destination:    destination,
		ConfigFile:     r.Config.ConfigPath,
		HostKeyPrompt:  supervisorHostKeyPrompt,
		NonInteractive: true,
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
	attachedTmux := ""
	command := ""
	if options.TmuxSession != "" {
		if options.TmuxCreate {
			attachedTmux = options.TmuxSession
			command = tmuxStartCommand(options.TmuxSession)
		} else if tmuxSessionExists(client, options.TmuxSession) {
			attachedTmux = options.TmuxSession
			command = tmuxAttachCommand(options.TmuxSession)
		}
	}
	if command != "" {
		if err := session.Start(command); err != nil {
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
		tmux:    attachedTmux,
	}
	if r.Config.Trzsz || r.Config.Zmodem {
		run.filter = newSessionFilter(stdin, stdout, int32(cols), r.Config.Zmodem, r.Config.DownloadPath)
		run.stdin = run.filter.input
		run.stdout = run.filter.output
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
	client  ssh05.SshClient
	session ssh05.SshSession
	stdin   io.WriteCloser
	stdout  io.Reader
	tmux    string
	filter  *sessionFilter

	output chan []byte
	done   chan struct{}

	mu        sync.Mutex
	closed    bool
	exitCode  int
	closeOnce sync.Once
}

// AttachedTmux reports the tmux session actually attached; empty means the
// requested session was missing and a plain shell was opened instead.
func (r *sshRun) AttachedTmux() string { return r.tmux }

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
	if r.filter != nil {
		r.filter.Resize(int32(cols))
	}
	return r.session.WindowChange(rows, cols)
}

func (r *sshRun) Signal(signal string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrSessionClosed
	}
	sender, ok := r.session.(ssh05.SshSignalSender)
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
		if r.filter != nil {
			r.filter.Close()
		}
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
