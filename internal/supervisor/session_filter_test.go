package supervisor

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// TestSessionFilterPassThrough verifies that ordinary terminal traffic crosses
// the trzsz filter unchanged in both directions.
func TestSessionFilterPassThrough(t *testing.T) {
	serverInReader, serverIn := io.Pipe()
	serverOut, serverOutWriter := io.Pipe()
	filter := newSessionFilter(serverIn, serverOut, 80, false, "")
	defer filter.Close()

	input := []byte("echo SSH05_TRZSZ_OK\r")
	go func() { _, _ = filter.input.Write(input) }()
	gotInput := make([]byte, len(input))
	if _, err := io.ReadFull(serverInReader, gotInput); err != nil {
		t.Fatalf("read server input: %v", err)
	}
	if !bytes.Equal(gotInput, input) {
		t.Fatalf("server input mismatch: got %q want %q", gotInput, input)
	}

	output := []byte("SSH05_TRZSZ_OK\r\n")
	go func() { _, _ = serverOutWriter.Write(output) }()
	gotOutput := make([]byte, len(output))
	if _, err := io.ReadFull(filter.output, gotOutput); err != nil {
		t.Fatalf("read client output: %v", err)
	}
	if !bytes.Equal(gotOutput, output) {
		t.Fatalf("client output mismatch: got %q want %q", gotOutput, output)
	}
}

// TestSessionFilterCloseUnblocks verifies that closing the filter releases the
// server input pipe instead of leaving the session goroutines blocked.
func TestSessionFilterCloseUnblocks(t *testing.T) {
	serverInReader, serverIn := io.Pipe()
	serverOut, _ := io.Pipe()
	filter := newSessionFilter(serverIn, serverOut, 80, false, "")

	filter.Close()

	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(serverInReader)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("read closed server input: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server input stayed blocked after Close")
	}
}
