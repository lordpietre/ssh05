//go:build !windows

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trzsz/trzsz-ssh/contracts"
)

func startTestServer(t *testing.T, handler Handler) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "run", "control.sock")
	server, err := Listen(socket, handler)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
		<-done
	})
	return socket
}

func TestServerClientRoundTrip(t *testing.T) {
	handler := HandlerFunc(func(_ context.Context, _ *Conn, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		if req.Method != contracts.MethodHostsList {
			return nil, &contracts.RPCError{Code: CodeMethodNotFound, Message: "unexpected method"}
		}
		data, err := json.Marshal(contracts.HostsListResult{
			Hosts: []contracts.HostProfile{{ID: "host-1", Alias: "demo", Target: contracts.HostTarget{Hostname: "demo", Port: 22}}},
		})
		if err != nil {
			return nil, &contracts.RPCError{Code: CodeInternal, Message: "encode result"}
		}
		return data, nil
	})
	socket := startTestServer(t, handler)

	client, err := Dial(socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := CallTyped[contracts.HostsListParams, contracts.HostsListResult](
		ctx, client, contracts.MethodHostsList, contracts.HostsListParams{},
	)
	if err != nil {
		t.Fatalf("CallTyped: %v", err)
	}
	if len(result.Hosts) != 1 || result.Hosts[0].Alias != "demo" {
		t.Fatalf("result = %+v", result)
	}
}

func TestServerRejectsVersionMismatch(t *testing.T) {
	socket := startTestServer(t, HandlerFunc(func(context.Context, *Conn, contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		return nil, nil
	}))

	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	request := contracts.RPCRequest{
		ProtocolVersion: contracts.ProtocolVersion + 1,
		RequestID:       "bad-version",
		Method:          contracts.MethodHostsList,
		Timestamp:       time.Now().UTC(),
	}
	if err := WriteJSONFrame(conn, request); err != nil {
		t.Fatalf("WriteJSONFrame: %v", err)
	}
	var response contracts.RPCResponse
	if err := ReadJSONFrame(bufio.NewReader(conn), &response); err != nil {
		t.Fatalf("ReadJSONFrame: %v", err)
	}
	if response.Error == nil || response.Error.Code != CodeVersionMismatch {
		t.Fatalf("response = %+v", response)
	}
}

func TestServerRejectsUnknownMethod(t *testing.T) {
	socket := startTestServer(t, HandlerFunc(func(context.Context, *Conn, contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		return nil, nil
	}))

	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	request := contracts.RPCRequest{
		ProtocolVersion: contracts.ProtocolVersion,
		RequestID:       "shell",
		Method:          contracts.Method("shell.exec"),
		Timestamp:       time.Now().UTC(),
	}
	if err := WriteJSONFrame(conn, request); err != nil {
		t.Fatalf("WriteJSONFrame: %v", err)
	}
	var response contracts.RPCResponse
	if err := ReadJSONFrame(bufio.NewReader(conn), &response); err != nil {
		t.Fatalf("ReadJSONFrame: %v", err)
	}
	if response.Error == nil || response.Error.Code != CodeMethodNotFound {
		t.Fatalf("response = %+v", response)
	}
}

func TestServerRecoversHandlerPanic(t *testing.T) {
	handler := HandlerFunc(func(context.Context, *Conn, contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		panic("secret value that must not leak")
	})
	socket := startTestServer(t, handler)

	client, err := Dial(socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = client.Call(ctx, contracts.MethodHostsList, contracts.HostsListParams{})
	var rpcError *contracts.RPCError
	if !errors.As(err, &rpcError) {
		t.Fatalf("error = %v, want RPCError", err)
	}
	if rpcError.Code != CodeInternal {
		t.Fatalf("code = %d, want %d", rpcError.Code, CodeInternal)
	}
	if rpcError.Message == "secret value that must not leak" {
		t.Fatal("panic value leaked into the response")
	}
}

func TestSocketPermissions(t *testing.T) {
	socket := startTestServer(t, HandlerFunc(func(context.Context, *Conn, contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		return nil, nil
	}))

	dirInfo, err := os.Stat(filepath.Dir(socket))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("socket dir mode = %o, want 700", dirInfo.Mode().Perm())
	}

	socketInfo, err := os.Stat(socket)
	if err != nil {
		t.Fatal(err)
	}
	if socketInfo.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %o, want 600", socketInfo.Mode().Perm())
	}
}

func TestSecondServerIsRejected(t *testing.T) {
	socket := startTestServer(t, HandlerFunc(func(context.Context, *Conn, contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		return nil, nil
	}))

	second, err := Listen(socket, HandlerFunc(func(context.Context, *Conn, contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		return nil, nil
	}))
	if second != nil {
		_ = second.Close()
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("error = %v, want ErrAlreadyRunning", err)
	}
}

func TestPublicSocketDirectoryIsRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	server, err := Listen(filepath.Join(dir, "control.sock"), HandlerFunc(func(context.Context, *Conn, contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		return nil, nil
	}))
	if server != nil {
		_ = server.Close()
	}
	if err == nil || errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("error = %v, want a private-directory error", err)
	}
}

func TestMalformedJSONIsRejected(t *testing.T) {
	socket := startTestServer(t, HandlerFunc(func(context.Context, *Conn, contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		return nil, nil
	}))

	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if err := writeFrame(conn, FrameJSON, []byte("{")); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	var response contracts.RPCResponse
	if err := ReadJSONFrame(bufio.NewReader(conn), &response); err != nil {
		t.Fatalf("ReadJSONFrame: %v", err)
	}
	if response.Error == nil || response.Error.Code != CodeParseError {
		t.Fatalf("response = %+v", response)
	}
}

func TestCallContextCancellation(t *testing.T) {
	blocked := make(chan struct{})
	socket := startTestServer(t, HandlerFunc(func(context.Context, *Conn, contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		<-blocked
		return nil, nil
	}))
	t.Cleanup(func() { close(blocked) })

	client, err := Dial(socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := client.Call(ctx, contracts.MethodHostsList, contracts.HostsListParams{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline", err)
	}
}

func TestServerSendsBinaryToClient(t *testing.T) {
	socket := startTestServer(t, HandlerFunc(func(_ context.Context, conn *Conn, _ contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		if err := conn.SendBinary("ses-1", []byte("output")); err != nil {
			return nil, &contracts.RPCError{Code: CodeInternal, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil
	}))

	client, err := Dial(socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	received := make(chan string, 1)
	client.SetBinaryHandler(func(sessionID string, data []byte) {
		received <- sessionID + ":" + string(data)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Call(ctx, contracts.MethodHostsList, contracts.HostsListParams{}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	select {
	case value := <-received:
		if value != "ses-1:output" {
			t.Fatalf("binary = %q", value)
		}
	case <-time.After(time.Second):
		t.Fatal("binary frame not received")
	}
}

func TestClientSendsBinaryToServer(t *testing.T) {
	received := make(chan string, 1)
	socket := startTestServer(t, HandlerFunc(func(_ context.Context, conn *Conn, _ contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
		conn.SetBinaryHandler(func(sessionID string, data []byte) {
			received <- sessionID + ":" + string(data)
		})
		return json.RawMessage(`{}`), nil
	}))

	client, err := Dial(socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Call(ctx, contracts.MethodSessionsWrite, contracts.SessionsWriteParams{SessionID: "ses-1"}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if err := client.SendBinary("ses-1", []byte("input")); err != nil {
		t.Fatalf("SendBinary: %v", err)
	}
	select {
	case value := <-received:
		if value != "ses-1:input" {
			t.Fatalf("binary = %q", value)
		}
	case <-time.After(time.Second):
		t.Fatal("binary frame not received by server")
	}
}
