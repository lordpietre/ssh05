package contracts

import (
	"encoding/json"
	"fmt"
	"time"
)

// ProtocolVersion is bumped whenever the JSON envelope or method semantics
// change incompatibly.
const ProtocolVersion = 1

// RequestID correlates a request with its response and cancellation.
type RequestID string

// Method is an IPC command name from SDD Annex B.
type Method string

const (
	MethodHostsList          Method = "hosts.list"
	MethodHostsImport        Method = "hosts.import"
	MethodHostsResolve       Method = "hosts.resolve"
	MethodHostsOverridesSet  Method = "hosts.overrides.set"
	MethodHostsCreate        Method = "hosts.create"
	MethodSessionsOpen       Method = "sessions.open"
	MethodSessionsWrite      Method = "sessions.write"
	MethodSessionsResize     Method = "sessions.resize"
	MethodSessionsClose      Method = "sessions.close"
	MethodSessionsDetachTmux Method = "sessions.detachTmux"
	MethodSessionsAttachTmux Method = "sessions.attachTmux"
	MethodSessionsSignal     Method = "sessions.signal"
	MethodSessionsList       Method = "sessions.list"
	MethodSessionsAttach     Method = "sessions.attach"
	MethodRoutesPlan         Method = "routes.plan"
	MethodRoutesConnect      Method = "routes.connect"
	MethodRoutesProbe        Method = "routes.probe"
	MethodFrpInstancesList   Method = "frp.instances.list"
	MethodFrpProxiesApply    Method = "frp.proxies.apply"
	MethodFrpProxiesStop     Method = "frp.proxies.stop"
	MethodFrpProxiesRemove   Method = "frp.proxies.remove"
	MethodFrpRotate          Method = "frp.tokens.rotate"
	MethodFrpServerStart     Method = "frp.servers.start"
	MethodFrpServerStop      Method = "frp.servers.stop"
	MethodTransfersStart     Method = "transfers.start"
	MethodTransfersList      Method = "transfers.list"
	MethodTransfersCancel    Method = "transfers.cancel"
	MethodVaultStatus        Method = "vault.status"
	MethodVaultUnlock        Method = "vault.unlockRequest"
	MethodDiagnosticsReport  Method = "diagnostics.report"
	MethodDiagnosticsHost    Method = "diagnostics.host"
	MethodDiagnosticsNmap    Method = "diagnostics.nmap"
	MethodDiagnosticsNetwork Method = "diagnostics.network"
	MethodWorkspacesSave     Method = "workspaces.save"
	MethodWorkspacesRestore  Method = "workspaces.restore"
)

var validMethods = map[Method]struct{}{
	MethodHostsList:          {},
	MethodHostsImport:        {},
	MethodHostsResolve:       {},
	MethodHostsOverridesSet:  {},
	MethodHostsCreate:        {},
	MethodSessionsOpen:       {},
	MethodSessionsWrite:      {},
	MethodSessionsResize:     {},
	MethodSessionsClose:      {},
	MethodSessionsDetachTmux: {},
	MethodSessionsAttachTmux: {},
	MethodSessionsSignal:     {},
	MethodSessionsList:       {},
	MethodSessionsAttach:     {},
	MethodRoutesPlan:         {},
	MethodRoutesConnect:      {},
	MethodRoutesProbe:        {},
	MethodFrpInstancesList:   {},
	MethodFrpProxiesApply:    {},
	MethodFrpProxiesStop:     {},
	MethodFrpProxiesRemove:   {},
	MethodFrpRotate:          {},
	MethodFrpServerStart:     {},
	MethodFrpServerStop:      {},
	MethodTransfersStart:     {},
	MethodTransfersList:      {},
	MethodTransfersCancel:    {},
	MethodVaultStatus:        {},
	MethodVaultUnlock:        {},
	MethodDiagnosticsReport:  {},
	MethodDiagnosticsHost:    {},
	MethodDiagnosticsNmap:    {},
	MethodDiagnosticsNetwork: {},
	MethodWorkspacesSave:     {},
	MethodWorkspacesRestore:  {},
}

// Valid reports whether m is part of ProtocolVersion.
func (m Method) Valid() bool {
	_, ok := validMethods[m]
	return ok
}

// RPCRequest is the JSON control envelope. Large PTY payloads travel through
// the separate framed binary channel, never as unbounded JSON events.
type RPCRequest struct {
	ProtocolVersion int             `json:"protocolVersion"`
	RequestID       RequestID       `json:"requestId"`
	Method          Method          `json:"method"`
	Params          json.RawMessage `json:"params,omitempty"`
	SessionID       SessionID       `json:"sessionId,omitempty"`
	Timestamp       time.Time       `json:"timestamp"`
}

// RPCResponse is the reply to one request.
type RPCResponse struct {
	ProtocolVersion int             `json:"protocolVersion"`
	RequestID       RequestID       `json:"requestId"`
	Result          json.RawMessage `json:"result,omitempty"`
	Error           *RPCError       `json:"error,omitempty"`
}

// RPCError is a stable, non-sensitive error shape.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error lets RPCError be returned as a normal Go error.
func (e *RPCError) Error() string {
	if e == nil {
		return "rpc error"
	}
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// Event is an ordered, typed notification from supervisor to UI.
type Event struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Event           string          `json:"event"`
	SessionID       SessionID       `json:"sessionId,omitempty"`
	Seq             uint64          `json:"seq"`
	Timestamp       time.Time       `json:"timestamp"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

// --- hosts ---

type HostsListParams struct {
	Query  string `json:"query,omitempty"`
	Source string `json:"source,omitempty"`
}

type HostConflict struct {
	HostProfileID string   `json:"hostProfileId"`
	Field         string   `json:"field"`
	LocalValue    string   `json:"localValue"`
	RemoteValue   string   `json:"remoteValue"`
	Sources       []string `json:"sources"`
}

type HostsListResult struct {
	Hosts     []HostProfile  `json:"hosts"`
	Conflicts []HostConflict `json:"conflicts,omitempty"`
}

type HostsImportParams struct {
	Source string `json:"source"` // "ssh_config" or "keepass"
	Path   string `json:"path,omitempty"`
}

type HostsImportResult struct {
	Imported  int            `json:"imported"`
	Skipped   int            `json:"skipped"`
	Conflicts []HostConflict `json:"conflicts,omitempty"`
}

type HostCreateParams struct {
	Alias        string `json:"alias"`
	Hostname     string `json:"hostname"`
	Port         int    `json:"port,omitempty"`
	Username     string `json:"username,omitempty"`
	IdentityFile string `json:"identityFile,omitempty"`
	Password     string `json:"password,omitempty"`
}

type HostOverridePatch struct {
	HostProfileID string    `json:"hostProfileId"`
	Labels        *[]string `json:"labels,omitempty"`
	Favorite      *bool     `json:"favorite,omitempty"`
	Color         *string   `json:"color,omitempty"`
}

// --- sessions ---

type SessionsOpenParams struct {
	HostProfileID string `json:"hostProfileId"`
	RoutePolicyID string `json:"routePolicyId,omitempty"`
	RouteID       string `json:"routeId,omitempty"`
	TmuxSession   string `json:"tmuxSession,omitempty"`
	TmuxCreate    bool   `json:"tmuxCreate,omitempty"`
}

type SessionsOpenResult struct {
	SessionID   SessionID `json:"sessionId"`
	TmuxSession string    `json:"tmuxSession,omitempty"`
}

type SessionsWriteParams struct {
	SessionID SessionID `json:"sessionId"`
	// Data is only for small control writes. Bulk PTY bytes use the binary channel.
	Data []byte `json:"data,omitempty"`
}

type SessionsResizeParams struct {
	SessionID SessionID `json:"sessionId"`
	Cols      int       `json:"cols"`
	Rows      int       `json:"rows"`
}

type SessionsCloseParams struct {
	SessionID SessionID   `json:"sessionId"`
	Intent    CloseIntent `json:"intent"`
}

type SessionsTmuxParams struct {
	SessionID SessionID `json:"sessionId"`
	Name      string    `json:"name,omitempty"`
}

type SessionsSignalParams struct {
	SessionID SessionID `json:"sessionId"`
	Signal    string    `json:"signal"`
}

type SessionsListResult struct {
	Sessions []SessionInfo `json:"sessions"`
}

type SessionsAttachParams struct {
	SessionID SessionID `json:"sessionId"`
}

// --- routes ---

type RoutesPlanParams struct {
	HostProfileID string `json:"hostProfileId"`
	RoutePolicyID string `json:"routePolicyId,omitempty"`
}

type RoutesPlanResult struct {
	Routes   []ConnectionRoute `json:"routes"`
	Selected string            `json:"selected,omitempty"`
	Reason   string            `json:"reason,omitempty"`
}

type RoutesProbeParams struct {
	HostProfileID string `json:"hostProfileId"`
	RouteID       string `json:"routeId,omitempty"`
}

type RoutesProbeResult struct {
	RouteID   string    `json:"routeId"`
	Kind      RouteKind `json:"kind"`
	Address   string    `json:"address"`
	State     string    `json:"state"`
	LatencyMS int64     `json:"latencyMs"`
	Message   string    `json:"message,omitempty"`
}

type RoutesConnectParams struct {
	HostProfileID string `json:"hostProfileId"`
	RouteID       string `json:"routeId"`
}

// --- FRP ---

type FrpInstancesListResult struct {
	Endpoints []FrpEndpoint `json:"endpoints"`
	Proxies   []FrpProxy    `json:"proxies"`
}

type FrpProxiesApplyParams struct {
	Proxy FrpProxy `json:"proxy"`
}

type FrpProxiesApplyResult struct {
	Proxy FrpProxy `json:"proxy"`
}

type FrpProxiesStopParams struct {
	ProxyID string `json:"proxyId"`
}

type FrpProxiesRemoveParams struct {
	ProxyID string `json:"proxyId"`
}

type FrpRotateParams struct {
	EndpointID string `json:"endpointId"`
}

type FrpServerParams struct {
	EndpointID string `json:"endpointId"`
}

// --- transfers ---

type TransferInfo struct {
	ID         string    `json:"transferId"`
	Protocol   string    `json:"protocol"`
	Direction  string    `json:"direction"`
	Target     string    `json:"target"`
	LocalPath  string    `json:"localPath"`
	RemotePath string    `json:"remotePath"`
	Recursive  bool      `json:"recursive"`
	State      string    `json:"state"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	ErrorCode  string    `json:"errorCode,omitempty"`
	ErrorLabel string    `json:"errorLabel,omitempty"`
}

type TransfersStartParams struct {
	HostProfileID string `json:"hostProfileId"`
	RouteID       string `json:"routeId,omitempty"`
	Protocol      string `json:"protocol"`
	Direction     string `json:"direction"`
	LocalPath     string `json:"localPath"`
	RemotePath    string `json:"remotePath"`
	Recursive     bool   `json:"recursive,omitempty"`
}

type TransfersListResult struct {
	Transfers []TransferInfo `json:"transfers"`
}

type TransfersCancelParams struct {
	TransferID string `json:"transferId"`
}

// --- vault ---

type VaultStatusResult struct {
	Provider        string `json:"provider"`
	Locked          bool   `json:"locked"`
	Profiles        int    `json:"profiles"`
	AgentProvider   string `json:"agentProvider,omitempty"`
	AgentSocket     string `json:"agentSocket,omitempty"`
	AgentIdentities int    `json:"agentIdentities,omitempty"`
	AgentMessage    string `json:"agentMessage,omitempty"`
}

type VaultUnlockParams struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
}

// --- diagnostics ---

// DiagnosticsReport is a sanitized snapshot safe to persist or share. It never
// contains credentials, tokens, private keys, environment values or PTY data.
type DiagnosticsReport struct {
	GeneratedAt     string                 `json:"generatedAt"`
	ProtocolVersion int                    `json:"protocolVersion"`
	HostsCount      int                    `json:"hostsCount"`
	Conflicts       int                    `json:"conflicts"`
	Sessions        []SessionInfo          `json:"sessions"`
	Vault           VaultStatusResult      `json:"vault"`
	FRP             FrpInstancesListResult `json:"frp"`
}

type HostDiagnosticsParams struct {
	HostProfileID string `json:"hostProfileId"`
	RouteID       string `json:"routeId,omitempty"`
}

type DNSCheck struct {
	Host       string   `json:"host"`
	Addresses  []string `json:"addresses"`
	DurationMS int64    `json:"durationMs"`
	Error      string   `json:"error,omitempty"`
}

type TCPCheck struct {
	Address   string `json:"address"`
	State     string `json:"state"`
	LatencyMS int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

type BannerCheck struct {
	Received   string `json:"received,omitempty"`
	DurationMS int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

type TLSCheck struct {
	Applicable bool   `json:"applicable"`
	Subject    string `json:"subject,omitempty"`
	Issuer     string `json:"issuer,omitempty"`
	NotAfter   string `json:"notAfter,omitempty"`
	Error      string `json:"error,omitempty"`
	Message    string `json:"message,omitempty"`
}

type HostDiagnosticsResult struct {
	HostProfileID string      `json:"hostProfileId"`
	GeneratedAt   string      `json:"generatedAt"`
	RouteID       string      `json:"routeId"`
	RouteKind     RouteKind   `json:"routeKind"`
	Address       string      `json:"address"`
	DNS           DNSCheck    `json:"dns"`
	TCP           TCPCheck    `json:"tcp"`
	Banner        BannerCheck `json:"banner"`
	TLS           TLSCheck    `json:"tls"`
}

type NmapParams struct {
	HostProfileID string `json:"hostProfileId"`
	RouteID       string `json:"routeId,omitempty"`
	Ports         string `json:"ports,omitempty"`
	Confirm       bool   `json:"confirm"`
	Authorized    bool   `json:"authorized"`
}

type NmapPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	State    string `json:"state"`
	Service  string `json:"service,omitempty"`
	Product  string `json:"product,omitempty"`
	Version  string `json:"version,omitempty"`
}

type NmapResult struct {
	Target      string     `json:"target"`
	Ports       []NmapPort `json:"ports"`
	GeneratedAt string     `json:"generatedAt"`
}

type NetworkParams struct {
	HostProfileID string `json:"hostProfileId"`
	RouteID       string `json:"routeId,omitempty"`
}

type ASNInfo struct {
	ASN       string `json:"asn,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Country   string `json:"country,omitempty"`
	Registry  string `json:"registry,omitempty"`
	Allocated string `json:"allocated,omitempty"`
	Error     string `json:"error,omitempty"`
}

type RDAPInfo struct {
	Name         string `json:"name,omitempty"`
	Handle       string `json:"handle,omitempty"`
	Country      string `json:"country,omitempty"`
	StartAddress string `json:"startAddress,omitempty"`
	EndAddress   string `json:"endAddress,omitempty"`
	Type         string `json:"type,omitempty"`
	Error        string `json:"error,omitempty"`
}

type NetworkInfoResult struct {
	Host         string   `json:"host"`
	Addresses    []string `json:"addresses"`
	ASN          ASNInfo  `json:"asn"`
	RDAP         RDAPInfo `json:"rdap"`
	GeoCountry   string   `json:"geoCountry,omitempty"`
	GeoPrecision string   `json:"geoPrecision"`
	Provenance   []string `json:"provenance"`
	GeneratedAt  string   `json:"generatedAt"`
}

// --- workspaces ---

type WorkspacesSaveParams struct {
	Workspace Workspace `json:"workspace"`
}

type WorkspacesRestoreParams struct {
	WorkspaceID WorkspaceID `json:"workspaceId"`
}

type WorkspacesRestoreResult struct {
	Workspace Workspace        `json:"workspace"`
	Plan      RoutesPlanResult `json:"plan"`
}
