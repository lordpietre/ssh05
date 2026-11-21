package contracts

import (
	"encoding/json"
	"testing"
	"time"
)

func TestProtocolVersion(t *testing.T) {
	if ProtocolVersion != 1 {
		t.Fatalf("ProtocolVersion = %d, want 1", ProtocolVersion)
	}
}

func TestSessionPhases(t *testing.T) {
	phases := []SessionPhase{
		PhaseCreated, PhaseResolving, PhaseConnecting, PhaseAuthenticating,
		PhaseConnected, PhaseDegraded, PhaseReconnecting, PhaseDisconnected,
		PhaseDetached, PhaseFailed, PhaseClosing, PhaseClosed,
	}
	for _, phase := range phases {
		if !phase.Valid() {
			t.Errorf("phase %q should be valid", phase)
		}
	}
	if SessionPhase("unknown").Valid() {
		t.Error("unknown phase must be invalid")
	}
}

func TestCloseIntents(t *testing.T) {
	for _, intent := range []CloseIntent{CloseView, DisconnectSSH, DetachRemoteTmux} {
		if !intent.Valid() {
			t.Errorf("intent %q should be valid", intent)
		}
	}
	if CloseIntent("kill-everything").Valid() {
		t.Error("unknown close intent must be invalid")
	}
}

func TestMethods(t *testing.T) {
	methods := []Method{
		MethodHostsList, MethodHostsImport, MethodHostsResolve,
		MethodSessionsOpen, MethodSessionsWrite, MethodSessionsResize,
		MethodSessionsClose, MethodSessionsDetachTmux, MethodSessionsAttachTmux,
		MethodSessionsSignal,
		MethodSessionsList, MethodSessionsAttach,
		MethodRoutesPlan, MethodRoutesConnect, MethodFrpInstancesList,
		MethodFrpProxiesApply, MethodFrpProxiesStop, MethodFrpProxiesRemove, MethodFrpRotate,
		MethodFrpServerStart, MethodFrpServerStop, MethodVaultStatus,
		MethodVaultUnlock, MethodDiagnosticsReport, MethodWorkspacesSave, MethodWorkspacesRestore,
	}
	seen := make(map[Method]struct{}, len(methods))
	for _, method := range methods {
		if !method.Valid() {
			t.Errorf("method %q should be valid", method)
		}
		if _, ok := seen[method]; ok {
			t.Errorf("duplicate method %q", method)
		}
		seen[method] = struct{}{}
	}
	if Method("shell.exec").Valid() {
		t.Error("arbitrary shell execution must not be a valid IPC method")
	}
}

func TestHostProfileJSONRoundTrip(t *testing.T) {
	original := HostProfile{
		ID:    "host-92",
		Alias: "laboratorio-arm64",
		Target: HostTarget{
			Hostname: "lab.example.net",
			Port:     2222,
			Username: "admin",
		},
		SourceRefs: []SourceReference{
			{Kind: SourceKeePass, EntryID: "uuid-externo", Fingerprint: "sha256:abc"},
			{Kind: SourceSSHConfig, EntryID: "~/.ssh/config#laboratorio-arm64"},
		},
		AuthRef:          &CredentialReference{Provider: "keepass", EntryID: "uuid-externo"},
		RoutePolicyID:    "rp-lab",
		Labels:           []string{"laboratorio", "arm64"},
		Color:            "#5878CC",
		VerifiedIdentity: true,
		Discovered:       false,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded HostProfile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ID != original.ID || decoded.Target != original.Target {
		t.Fatalf("round trip mismatch: %+v", decoded)
	}
	if decoded.AuthRef == nil || decoded.AuthRef.EntryID != "uuid-externo" {
		t.Fatalf("credential reference lost: %+v", decoded.AuthRef)
	}
	// A profile must never carry secret values. This guards accidental field
	// additions named like secrets.
	for _, forbidden := range []string{`"password"`, `"token"`, `"privateKey"`, `"secret"`} {
		if containsJSONKey(data, forbidden) {
			t.Fatalf("HostProfile leaked forbidden key %s: %s", forbidden, data)
		}
	}
}

func TestRPCEnvelopeJSON(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	request := RPCRequest{
		ProtocolVersion: ProtocolVersion,
		RequestID:       "req-41",
		Method:          MethodSessionsOpen,
		Params:          json.RawMessage(`{"hostProfileId":"host-92"}`),
		Timestamp:       now,
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded RPCRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Method != MethodSessionsOpen || decoded.RequestID != "req-41" {
		t.Fatalf("envelope mismatch: %+v", decoded)
	}
	if decoded.ProtocolVersion != ProtocolVersion {
		t.Fatalf("protocol version mismatch: %d", decoded.ProtocolVersion)
	}
}

func TestSessionStatusSequence(t *testing.T) {
	status := SessionStatus{
		ID:    "ses-1",
		Phase: PhaseConnected,
		Seq:   7,
		At:    time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC),
	}
	if !status.Phase.Valid() {
		t.Fatal("status phase must be valid")
	}
	if status.Seq == 0 {
		t.Fatal("status sequence must be monotonic and start above zero")
	}
}

func containsJSONKey(data []byte, quotedKey string) bool {
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return false
	}
	for key := range value {
		if `"`+key+`"` == quotedKey {
			return true
		}
	}
	return false
}
