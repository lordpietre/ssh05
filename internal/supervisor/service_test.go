package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/trzsz/trzsz-ssh/contracts"
	"github.com/trzsz/trzsz-ssh/internal/audit"
	frpmanager "github.com/trzsz/trzsz-ssh/internal/frp"
	"github.com/trzsz/trzsz-ssh/internal/ipc"
)

func startServiceServer(t *testing.T, service *Service) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "run", "control.sock")
	server, err := ipc.Listen(socket, service)
	if err != nil {
		t.Fatalf("ipc.Listen: %v", err)
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

func TestServiceSessionFlow(t *testing.T) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{})
	service := NewService(backend, nil)
	socket := startServiceServer(t, service)

	client, err := ipc.Dial(socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	events := make(chan contracts.SessionStatus, 8)
	client.SetEventHandler(func(event contracts.Event) {
		if event.Event != "session.status" {
			return
		}
		var status contracts.SessionStatus
		if err := json.Unmarshal(event.Payload, &status); err == nil {
			events <- status
		}
	})
	binary := make(chan string, 8)
	client.SetBinaryHandler(func(sessionID string, data []byte) {
		binary <- sessionID + ":" + string(data)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opened, err := ipc.CallTyped[contracts.SessionsOpenParams, contracts.SessionsOpenResult](
		ctx, client, contracts.MethodSessionsOpen, contracts.SessionsOpenParams{HostProfileID: "host-1"},
	)
	if err != nil {
		t.Fatalf("sessions.open: %v", err)
	}
	if opened.SessionID == "" {
		t.Fatal("empty session id")
	}

	select {
	case status := <-events:
		if status.Phase != contracts.PhaseConnected {
			t.Fatalf("first status = %q", status.Phase)
		}
	case <-time.After(time.Second):
		t.Fatal("connected event not received")
	}

	listed, err := ipc.CallTyped[struct{}, contracts.SessionsListResult](
		ctx, client, contracts.MethodSessionsList, struct{}{},
	)
	if err != nil {
		t.Fatalf("sessions.list: %v", err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].ID != opened.SessionID {
		t.Fatalf("sessions.list = %+v", listed.Sessions)
	}
	if _, err := ipc.CallTyped[contracts.SessionsAttachParams, struct{}](
		ctx, client, contracts.MethodSessionsAttach,
		contracts.SessionsAttachParams{SessionID: opened.SessionID},
	); err != nil {
		t.Fatalf("sessions.attach: %v", err)
	}

	run := runner.last()
	run.emit("hello")
	select {
	case value := <-binary:
		if value != string(opened.SessionID)+":hello" {
			t.Fatalf("binary = %q", value)
		}
	case <-time.After(time.Second):
		t.Fatal("binary output not received")
	}

	if err := client.SendBinary(string(opened.SessionID), []byte("input")); err != nil {
		t.Fatalf("SendBinary: %v", err)
	}
	eventually(t, func() bool {
		run.mu.Lock()
		defer run.mu.Unlock()
		return run.written.String() == "input"
	})

	if _, err := ipc.CallTyped[contracts.SessionsResizeParams, struct{}](
		ctx, client, contracts.MethodSessionsResize,
		contracts.SessionsResizeParams{SessionID: opened.SessionID, Cols: 90, Rows: 28},
	); err != nil {
		t.Fatalf("sessions.resize: %v", err)
	}
	run.mu.Lock()
	if run.cols != 90 || run.rows != 28 {
		t.Fatalf("run size = %dx%d", run.cols, run.rows)
	}
	run.mu.Unlock()

	if _, err := ipc.CallTyped[contracts.SessionsSignalParams, struct{}](
		ctx, client, contracts.MethodSessionsSignal,
		contracts.SessionsSignalParams{SessionID: opened.SessionID, Signal: "INT"},
	); err != nil {
		t.Fatalf("sessions.signal: %v", err)
	}
	run.mu.Lock()
	if len(run.signals) != 1 || run.signals[0] != "INT" {
		t.Fatalf("signals = %v", run.signals)
	}
	run.mu.Unlock()

	if _, err := ipc.CallTyped[contracts.SessionsCloseParams, struct{}](
		ctx, client, contracts.MethodSessionsClose,
		contracts.SessionsCloseParams{SessionID: opened.SessionID, Intent: contracts.CloseView},
	); err != nil {
		t.Fatalf("sessions.close: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case status := <-events:
			if status.Phase == contracts.PhaseClosed {
				return
			}
		case <-deadline:
			t.Fatal("closed event not received")
		}
	}
}

func TestServiceInventoryKeePassImport(t *testing.T) {
	db, key := keepassFixture(t)
	inventory := NewInventory(InventoryConfig{
		SSHConfigPath: writeSSHConfig(t),
		Keepass:       KeePassConfig{Database: db, KeyFile: key},
	})
	service := NewService(NewBackend(&fakeRunner{}, Options{}), inventory)
	socket := startServiceServer(t, service)

	client, err := ipc.Dial(socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hosts, err := ipc.CallTyped[contracts.HostsListParams, contracts.HostsListResult](
		ctx, client, contracts.MethodHostsList, contracts.HostsListParams{},
	)
	if err != nil {
		t.Fatalf("hosts.list: %v", err)
	}
	if len(hosts.Hosts) < 3 {
		t.Fatalf("hosts = %+v", hosts.Hosts)
	}
	if len(hosts.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v", hosts.Conflicts)
	}

	status, err := ipc.CallTyped[struct{}, contracts.VaultStatusResult](
		ctx, client, contracts.MethodVaultStatus, struct{}{},
	)
	if err != nil {
		t.Fatalf("vault.status: %v", err)
	}
	if status.Provider != "keepassxc" || status.Locked || status.Profiles != 3 {
		t.Fatalf("vault status = %+v", status)
	}

	unlocked, err := ipc.CallTyped[contracts.VaultUnlockParams, contracts.VaultStatusResult](
		ctx, client, contracts.MethodVaultUnlock,
		contracts.VaultUnlockParams{Provider: "keepassxc", Reason: "test"},
	)
	if err != nil {
		t.Fatalf("vault.unlockRequest: %v", err)
	}
	if unlocked.Locked || unlocked.Profiles != 3 {
		t.Fatalf("unlocked = %+v", unlocked)
	}

	plan, err := ipc.CallTyped[contracts.RoutesPlanParams, contracts.RoutesPlanResult](
		ctx, client, contracts.MethodRoutesPlan, contracts.RoutesPlanParams{HostProfileID: "lab-arm64"},
	)
	if err != nil {
		t.Fatalf("routes.plan: %v", err)
	}
	if plan.Selected != "proxy_jump:lab-arm64" || len(plan.Routes) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Routes[1].Via) != 1 || plan.Routes[1].Via[0] != "bastion.example.net" {
		t.Fatalf("via = %+v", plan.Routes[1].Via)
	}

	imported, err := ipc.CallTyped[contracts.HostsImportParams, contracts.HostsImportResult](
		ctx, client, contracts.MethodHostsImport, contracts.HostsImportParams{Source: "keepass"},
	)
	if err != nil {
		t.Fatalf("hosts.import: %v", err)
	}
	if imported.Imported < 3 {
		t.Fatalf("imported = %+v", imported)
	}
}

func TestServiceWritesAuditRecords(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "state", "audit.log")
	logger, err := audit.New(logPath, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(NewBackend(&fakeRunner{}, Options{}), nil, WithAudit(logger))
	socket := startServiceServer(t, service)

	client, err := ipc.Dial(socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opened, err := ipc.CallTyped[contracts.SessionsOpenParams, contracts.SessionsOpenResult](
		ctx, client, contracts.MethodSessionsOpen, contracts.SessionsOpenParams{HostProfileID: "host-1"},
	)
	if err != nil {
		t.Fatalf("sessions.open: %v", err)
	}
	if _, err := ipc.CallTyped[contracts.SessionsCloseParams, struct{}](
		ctx, client, contracts.MethodSessionsClose,
		contracts.SessionsCloseParams{SessionID: opened.SessionID, Intent: contracts.CloseView},
	); err != nil {
		t.Fatalf("sessions.close: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(logPath)
		if err == nil && strings.Contains(string(data), "session.open") && strings.Contains(string(data), "session.close") {
			if strings.Contains(string(data), "password") || strings.Contains(string(data), "token") {
				t.Fatalf("secret-like text in audit: %s", data)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("audit events missing: %s (%v)", data, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServiceHostsAndRoutes(t *testing.T) {
	backend := NewBackend(&fakeRunner{}, Options{})
	service := NewService(backend, nil)
	socket := startServiceServer(t, service)

	client, err := ipc.Dial(socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hosts, err := ipc.CallTyped[contracts.HostsListParams, contracts.HostsListResult](
		ctx, client, contracts.MethodHostsList, contracts.HostsListParams{},
	)
	if err != nil {
		t.Fatalf("hosts.list: %v", err)
	}
	if len(hosts.Hosts) != 0 {
		t.Fatalf("hosts = %+v", hosts.Hosts)
	}

	plan, err := ipc.CallTyped[contracts.RoutesPlanParams, contracts.RoutesPlanResult](
		ctx, client, contracts.MethodRoutesPlan, contracts.RoutesPlanParams{HostProfileID: "host-1"},
	)
	if err != nil {
		t.Fatalf("routes.plan: %v", err)
	}
	if plan.Selected != "direct:host-1" || len(plan.Routes) != 1 {
		t.Fatalf("plan = %+v", plan)
	}

	_, err = client.Call(ctx, contracts.MethodWorkspacesSave, contracts.WorkspacesSaveParams{})
	var rpcError *contracts.RPCError
	if !errors.As(err, &rpcError) || rpcError.Code != ipc.CodeInvalidRequest {
		t.Fatalf("workspaces.save error = %v", err)
	}
}

func TestServiceProbeRouteReachableAndRefused(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_config")
	content := "Host probe\n    HostName 127.0.0.1\n    Port " + strconv.Itoa(port) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(NewBackend(&fakeRunner{}, Options{}), NewInventory(InventoryConfig{SSHConfigPath: path}))
	ctx := context.Background()

	result := callProbe(t, ctx, service, "probe", "direct:probe")
	if result.State != "reachable" || result.Address != "127.0.0.1:"+strconv.Itoa(port) {
		t.Fatalf("reachable probe = %+v", result)
	}
	_ = listener.Close()

	result = callProbe(t, ctx, service, "probe", "direct:probe")
	if result.State != "refused" && result.State != "unknown" {
		t.Fatalf("closed port probe = %+v", result)
	}
}

func TestClassifyProbeErrorDoesNotTreatTimeoutAsClosed(t *testing.T) {
	if got := classifyProbeError(probeTimeout{}); got != "timeout" {
		t.Fatalf("timeout = %q", got)
	}
	if got := classifyProbeError(syscall.ECONNREFUSED); got != "refused" {
		t.Fatalf("refused = %q", got)
	}
	if got := classifyProbeError(&net.DNSError{Err: "no such host", IsNotFound: true}); got != "dns_failure" {
		t.Fatalf("dns = %q", got)
	}
	if got := classifyProbeError(errors.New("boom")); got != "unknown" {
		t.Fatalf("unknown = %q", got)
	}
}

func TestSplitJump(t *testing.T) {
	cases := map[string][2]string{
		"bastion":            {"bastion", "22"},
		"user@bastion":       {"bastion", "22"},
		"user@bastion:2222":  {"bastion", "2222"},
		"[2001:db8::1]:2200": {"2001:db8::1", "2200"},
		"2001:db8::1":        {"2001:db8::1", "22"},
	}
	for input, want := range cases {
		host, port := splitJump(input)
		if host != want[0] || port != want[1] {
			t.Fatalf("splitJump(%q) = %q/%q", input, host, port)
		}
	}
}

func callProbe(t *testing.T, ctx context.Context, service *Service, host, route string) contracts.RoutesProbeResult {
	t.Helper()
	payload, err := json.Marshal(contracts.RoutesProbeParams{HostProfileID: host, RouteID: route})
	if err != nil {
		t.Fatal(err)
	}
	raw, rpcErr := service.probeRoute(ctx, contracts.RPCRequest{Params: payload})
	if rpcErr != nil {
		t.Fatalf("probeRoute: %v", rpcErr)
	}
	var result contracts.RoutesProbeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

type probeTimeout struct{}

func (probeTimeout) Error() string   { return "i/o timeout" }
func (probeTimeout) Timeout() bool   { return true }
func (probeTimeout) Temporary() bool { return true }

func TestClosingSessionKeepsFRPTunnel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX frpc stub")
	}
	dir := t.TempDir()
	frpc := filepath.Join(dir, "frpc")
	script := "#!/bin/sh\nif [ \"$1\" = \"verify\" ]; then exit 0; fi\nexec sleep 30\n"
	if err := os.WriteFile(frpc, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	manager, err := frpmanager.NewManager(frpmanager.Options{
		ConfigPath: filepath.Join(dir, "frp.json"), KeyPath: filepath.Join(dir, "frp.key"),
		WorkDir: filepath.Join(dir, "work"), FrpcBin: frpc,
		EndpointAddress: "127.0.0.1", EndpointPort: 7000,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	service := NewService(NewBackend(&fakeRunner{}, Options{}), nil, WithFRP(manager))
	socket := startServiceServer(t, service)

	client, err := ipc.Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	applied, err := ipc.CallTyped[contracts.FrpProxiesApplyParams, contracts.FrpProxiesApplyResult](
		ctx, client, contracts.MethodFrpProxiesApply,
		contracts.FrpProxiesApplyParams{Proxy: contracts.FrpProxy{
			Name: "shared", Type: contracts.FrpTCP, ServicePort: 22, RemotePort: 22022,
		}},
	)
	if err != nil {
		t.Fatalf("frp apply: %v", err)
	}
	opened, err := ipc.CallTyped[contracts.SessionsOpenParams, contracts.SessionsOpenResult](
		ctx, client, contracts.MethodSessionsOpen, contracts.SessionsOpenParams{HostProfileID: "host-1"},
	)
	if err != nil {
		t.Fatalf("sessions.open: %v", err)
	}
	if _, err := ipc.CallTyped[contracts.SessionsCloseParams, struct{}](
		ctx, client, contracts.MethodSessionsClose,
		contracts.SessionsCloseParams{SessionID: opened.SessionID, Intent: contracts.CloseView},
	); err != nil {
		t.Fatalf("sessions.close: %v", err)
	}

	list, err := ipc.CallTyped[struct{}, contracts.FrpInstancesListResult](
		ctx, client, contracts.MethodFrpInstancesList, struct{}{},
	)
	if err != nil {
		t.Fatalf("frp list: %v", err)
	}
	if len(list.Proxies) != 1 || list.Proxies[0].ID != applied.Proxy.ID {
		t.Fatalf("proxies = %+v", list.Proxies)
	}
	if list.Proxies[0].State != contracts.FrpOnline {
		t.Fatalf("tunnel stopped after closing the tab: %+v", list.Proxies[0])
	}
}
