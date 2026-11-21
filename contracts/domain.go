// Package contracts contains the stable, transport-agnostic vocabulary shared
// by the TRSS UI (TypeScript) and the supervisor (Go).
//
// This package is intentionally free of dependencies. It represents the SDD
// domain and IPC envelope, not an implementation. Changes that break JSON or
// protocol compatibility require a new ProtocolVersion and an ADR.
package contracts

// SourceKind identifies where a profile value came from.
type SourceKind string

const (
	SourceManual    SourceKind = "manual"
	SourceSSHConfig SourceKind = "ssh_config"
	SourceKeePass   SourceKind = "keepass"
)

// HostProfile is the canonical host record. It never contains passwords or
// private keys: those live behind a CredentialReference.
type HostProfile struct {
	ID               string               `json:"id"`
	Alias            string               `json:"alias"`
	Target           HostTarget           `json:"target"`
	SourceRefs       []SourceReference    `json:"sourceRefs,omitempty"`
	AuthRef          *CredentialReference `json:"authRef,omitempty"`
	RoutePolicyID    string               `json:"routePolicyId,omitempty"`
	ProxyJump        []string             `json:"proxyJump,omitempty"`
	Labels           []string             `json:"labels,omitempty"`
	Color            string               `json:"color,omitempty"`
	Favorite         bool                 `json:"favorite,omitempty"`
	VerifiedIdentity bool                 `json:"verifiedIdentity"`
	Discovered       bool                 `json:"discovered"`
}

// HostTarget is the network destination of a profile.
type HostTarget struct {
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
}

// SourceReference identifies the external record behind a profile value.
type SourceReference struct {
	Kind        SourceKind `json:"kind"`
	EntryID     string     `json:"entryId"`
	Fingerprint string     `json:"fingerprint,omitempty"`
}

// CredentialReference identifies a secret provider entry. It never contains
// the secret value itself.
type CredentialReference struct {
	Provider    string `json:"provider"`
	EntryID     string `json:"entryId"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// RouteKind enumerates the authorized ways to reach a host.
type RouteKind string

const (
	RouteDirect    RouteKind = "direct"
	RouteProxyJump RouteKind = "proxy_jump"
	RouteFRP       RouteKind = "frp"
)

// ConnectionRoute is one candidate route produced by the resolver.
type ConnectionRoute struct {
	ID            string    `json:"id"`
	Kind          RouteKind `json:"kind"`
	HostProfileID string    `json:"hostProfileId"`
	Via           []string  `json:"via,omitempty"`
	FrpProxyID    string    `json:"frpProxyId,omitempty"`
	Authorized    bool      `json:"authorized"`
}

// RoutePolicy constrains which routes may be used.
type RoutePolicy struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	Preferred           []RouteKind `json:"preferred,omitempty"`
	AllowPublicExposure bool        `json:"allowPublicExposure"`
}

// TmuxBinding records an optional remote tmux session. Its existence is
// verified on the server; it is never assumed from local state.
type TmuxBinding struct {
	HostProfileID string `json:"hostProfileId"`
	SessionName   string `json:"sessionName"`
	Verified      bool   `json:"verified"`
}

// TabRecord is the persisted, non-secret description of a tab.
type TabRecord struct {
	ID            TabID        `json:"id"`
	Title         string       `json:"title"`
	HostProfileID string       `json:"hostProfileId"`
	SessionID     SessionID    `json:"sessionId,omitempty"`
	Tmux          *TmuxBinding `json:"tmux,omitempty"`
	Color         string       `json:"color,omitempty"`
	Order         int          `json:"order"`
}

// Workspace is a restorable set of tabs and preferences. It never serializes
// PTY bytes, credentials or terminal buffers.
type Workspace struct {
	ID          WorkspaceID       `json:"id"`
	Name        string            `json:"name"`
	Tabs        []TabRecord       `json:"tabs"`
	Preferences map[string]string `json:"preferences,omitempty"`
}

// FrpProxyType is the proxy kind supported by the installed FRP version.
type FrpProxyType string

const (
	FrpTCP  FrpProxyType = "tcp"
	FrpUDP  FrpProxyType = "udp"
	FrpSTCP FrpProxyType = "stcp"
	FrpXTCP FrpProxyType = "xtcp"
)

// FrpState is the observable lifecycle of a proxy.
type FrpState string

const (
	FrpStopped    FrpState = "stopped"
	FrpStarting   FrpState = "starting"
	FrpOnline     FrpState = "online"
	FrpDegraded   FrpState = "degraded"
	FrpRestarting FrpState = "restarting"
	FrpError      FrpState = "error"
	FrpStopping   FrpState = "stopping"
)

// FrpEndpoint is an authorized frps/frpc installation.
type FrpEndpoint struct {
	ID             string   `json:"id"`
	Role           string   `json:"role"` // "frps" or "frpc"
	Address        string   `json:"address"`
	Port           int      `json:"port"`
	Managed        bool     `json:"managed"`
	State          FrpState `json:"state,omitempty"`
	PID            int      `json:"pid,omitempty"`
	UptimeSeconds  int64    `json:"uptimeSeconds,omitempty"`
	LastErrorCode  string   `json:"lastErrorCode,omitempty"`
	LastErrorLabel string   `json:"lastErrorLabel,omitempty"`
}

// FrpProxy is a proxy definition plus its observed state.
type FrpProxy struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Type           FrpProxyType         `json:"type"`
	EndpointID     string               `json:"endpointId"`
	HostProfileID  string               `json:"hostProfileId,omitempty"`
	ServicePort    int                  `json:"servicePort"`
	RemotePort     int                  `json:"remotePort"`
	BindPort       int                  `json:"bindPort,omitempty"`
	Exposure       string               `json:"exposure"` // "private" or "public"
	State          FrpState             `json:"state"`
	PID            int                  `json:"pid,omitempty"`
	UptimeSeconds  int64                `json:"uptimeSeconds,omitempty"`
	CredentialRef  *CredentialReference `json:"credentialRef,omitempty"`
	LastErrorCode  string               `json:"lastErrorCode,omitempty"`
	LastErrorLabel string               `json:"lastErrorLabel,omitempty"`
}
