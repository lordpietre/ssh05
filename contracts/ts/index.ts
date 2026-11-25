/**
 * ssh05 shared contracts — TypeScript side.
 *
 * This file mirrors the Go package `contracts/`. It is deliberately free of
 * runtime dependencies so the UI and the supervisor can version it together.
 * PTY bulk data does NOT travel through these JSON events: it uses a separate
 * length-prefixed binary channel over the authenticated local socket.
 */

export const PROTOCOL_VERSION = 1 as const
export type ProtocolVersion = typeof PROTOCOL_VERSION

export type SessionId = string
export type TabId = string
export type WorkspaceId = string

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

export const SESSION_PHASES = [
  "created",
  "resolving",
  "connecting",
  "authenticating",
  "connected",
  "degraded",
  "reconnecting",
  "disconnected",
  "detached",
  "failed",
  "closing",
  "closed",
] as const

export type SessionPhase = (typeof SESSION_PHASES)[number]

export const CLOSE_INTENTS = ["close-view", "disconnect-ssh", "detach-remote-tmux"] as const
export type CloseIntent = (typeof CLOSE_INTENTS)[number]

export interface Capabilities {
  supportsDetach: boolean
  supportsReattach: boolean
  supportsRemoteTmux: boolean
  supportsPersistentProcess: boolean
  supportsBinaryStreaming: boolean
}

export interface SessionStatus {
  sessionId: SessionId
  phase: SessionPhase
  seq: number
  at: string
  errorCode?: string
  message?: string
  tmuxSession: string
}

export interface SessionOptions {
  tmuxSession?: string
  tmuxCreate?: boolean
  destination?: string
}

export type DataHandler = (data: Uint8Array) => void
export type StatusHandler = (status: SessionStatus) => void

/**
 * Stable backend contract from SDD §17.3. Implementations must preserve order
 * per session and apply bounded queues; they must not buffer PTY output in the
 * UI process without limits.
 */
export interface SessionBackend {
  open(profileId: string, routeId?: string, options?: SessionOptions): Promise<SessionId>
  write(id: SessionId, bytes: Uint8Array): Promise<void>
  resize(id: SessionId, cols: number, rows: number): Promise<void>
  requestClose(id: SessionId, intent: CloseIntent): Promise<void>
  attach(id: SessionId, cursor?: bigint): Promise<void>
  signal(id: SessionId, signal: string): Promise<void>
  onData(id: SessionId, cb: DataHandler): () => void
  onStatus(id: SessionId, cb: StatusHandler): () => void
  capabilities(): Promise<Capabilities>
}

// ---------------------------------------------------------------------------
// Tabs / workspaces
// ---------------------------------------------------------------------------

export interface TmuxBinding {
  hostProfileId: string
  sessionName: string
  verified: boolean
}

export interface TabState {
  id: TabId
  sessionId: SessionId
  title: string
  phase: SessionPhase
  color?: string
  tmux?: TmuxBinding
}

export type TabStoreEvent =
  | { type: "added"; tabId: TabId; index: number }
  | { type: "removed"; tabId: TabId }
  | { type: "activated"; tabId: TabId }
  | { type: "moved"; tabId: TabId; index: number }
  | { type: "updated"; tabId: TabId }

/**
 * Renderer-independent tab state. Closing a tab removes its view only; whether
 * the session survives is a SessionBackend capability and an explicit intent.
 */
export interface TabStore {
  list(): readonly TabState[]
  active(): TabState | undefined
  add(tab: TabState, index?: number): void
  remove(id: TabId): void
  activate(id: TabId): void
  move(id: TabId, index: number): void
  update(id: TabId, patch: Partial<Omit<TabState, "id">>): void
  subscribe(listener: (event: TabStoreEvent) => void): () => void
}

export interface TabRecord {
  id: TabId
  title: string
  hostProfileId: string
  sessionId?: SessionId
  tmux?: TmuxBinding
  color?: string
  order: number
}

export interface Workspace {
  id: WorkspaceId
  name: string
  tabs: TabRecord[]
  preferences?: Record<string, string>
}

// ---------------------------------------------------------------------------
// Hosts, routes and FRP
// ---------------------------------------------------------------------------

export type SourceKind = "manual" | "ssh_config" | "keepass" | "docker"

export interface SourceReference {
  kind: SourceKind
  entryId: string
  fingerprint?: string
}

export interface CredentialReference {
  provider: string
  entryId: string
  fingerprint?: string
}

export interface HostTarget {
  hostname: string
  port: number
  username?: string
}

export interface HostProfile {
  id: string
  alias: string
  target: HostTarget
  sourceRefs?: SourceReference[]
  authRef?: CredentialReference
  routePolicyId?: string
  proxyJump?: string[]
  labels?: string[]
  color?: string
  favorite?: boolean
  verifiedIdentity: boolean
  discovered: boolean
}

export type RouteKind = "direct" | "proxy_jump" | "frp"

export interface ConnectionRoute {
  id: string
  kind: RouteKind
  hostProfileId: string
  via?: string[]
  frpProxyId?: string
  authorized: boolean
}

export interface RoutePolicy {
  id: string
  name: string
  preferred?: RouteKind[]
  allowPublicExposure: boolean
}

export type FrpProxyType = "tcp" | "udp" | "stcp" | "xtcp"

export type FrpState =
  | "stopped"
  | "starting"
  | "online"
  | "degraded"
  | "restarting"
  | "error"
  | "stopping"

export interface FrpEndpoint {
  id: string
  role: "frps" | "frpc"
  address: string
  port: number
  managed: boolean
  state?: FrpState
  pid?: number
  uptimeSeconds?: number
  lastErrorCode?: string
  lastErrorLabel?: string
}

export interface FrpProxy {
  id: string
  name: string
  type: FrpProxyType
  endpointId: string
  hostProfileId?: string
  servicePort: number
  remotePort: number
  bindPort?: number
  exposure: "private" | "public"
  state: FrpState
  pid?: number
  uptimeSeconds?: number
  credentialRef?: CredentialReference
  lastErrorCode?: string
  lastErrorLabel?: string
}

// ---------------------------------------------------------------------------
// IPC envelopes and method contracts (SDD Annex B)
// ---------------------------------------------------------------------------

export type RequestId = string

export interface RpcError {
  code: number
  message: string
  data?: unknown
}

/** Typed IPC error codes shared with internal/ipc/errors.go. */
export const RPC_CODE_HOST_KEY_UNKNOWN = -32010
export const RPC_CODE_HOST_KEY_CHANGED = -32011

export interface RpcRequest<P = unknown> {
  protocolVersion: ProtocolVersion
  requestId: RequestId
  method: Method
  params?: P
  sessionId?: SessionId
  timestamp: string
}

export interface RpcResponse<R = unknown> {
  protocolVersion: ProtocolVersion
  requestId: RequestId
  result?: R
  error?: RpcError
}

export interface IpcEvent<P = unknown> {
  protocolVersion: ProtocolVersion
  event: string
  sessionId?: SessionId
  seq: number
  timestamp: string
  payload?: P
}

export interface HostsListParams {
  query?: string
  source?: string
}

export interface HostConflict {
  hostProfileId: string
  field: string
  localValue: string
  remoteValue: string
  sources: string[]
}

export interface HostsListResult {
  hosts: HostProfile[]
  conflicts?: HostConflict[]
}

export interface HostsImportParams {
  source: "ssh_config" | "keepass"
  path?: string
}

export interface HostsImportResult {
  imported: number
  skipped: number
  conflicts?: HostConflict[]
}

export interface HostCreateParams {
  alias: string
  hostname: string
  port?: number
  username?: string
  identityFile?: string
  password?: string
}

export interface HostUpdateParams {
  alias: string
  previousAlias?: string
  hostname: string
  port?: number
  username?: string
  identityFile?: string
  password?: string
}

export interface HostDeleteParams {
  alias: string
}

/** Returned when a server host key is not in known_hosts. */
export interface HostKeyChallenge {
  host: string
  keyType: string
  fingerprint: string
  knownHostsPath?: string
  /** False when StrictHostKeyChecking=yes forbids adding the key. */
  canTrust: boolean
}

/** Pins the exact fingerprint the user approved. */
export interface HostsTrustKeyParams {
  hostProfileId: string
  fingerprint: string
}

export interface HostsTrustKeyResult {
  host: string
  keyType: string
  fingerprint: string
  knownHostsPath?: string
}

export interface HostOverridePatch {
  hostProfileId: string
  labels?: string[]
  favorite?: boolean
  color?: string
}

export interface SessionsOpenParams {
  hostProfileId: string
  routePolicyId?: string
  routeId?: string
  tmuxSession?: string
  tmuxCreate?: boolean
}

export interface SessionsOpenResult {
  sessionId: SessionId
  tmuxSession?: string
}

export interface SessionsWriteParams {
  sessionId: SessionId
  /** Small control writes only. Bulk PTY bytes use the binary channel. */
  data?: Uint8Array
}

export interface SessionsResizeParams {
  sessionId: SessionId
  cols: number
  rows: number
}

export interface SessionsCloseParams {
  sessionId: SessionId
  intent: CloseIntent
}

export interface SessionsTmuxParams {
  sessionId: SessionId
  name?: string
}

export interface SessionsSignalParams {
  sessionId: SessionId
  signal: string
}

export interface SessionInfo {
  sessionId: SessionId
  profileId: string
  phase: SessionPhase
  tmuxSession?: string
}

export interface SessionsListResult {
  sessions: SessionInfo[]
}

export interface SessionsAttachParams {
  sessionId: SessionId
}

export interface RoutesPlanParams {
  hostProfileId: string
  routePolicyId?: string
}

export interface RoutesPlanResult {
  routes: ConnectionRoute[]
  selected?: string
  reason?: string
}

export interface RoutesProbeParams {
  hostProfileId: string
  routeId?: string
}

export interface RoutesProbeResult {
  routeId: string
  kind: RouteKind
  address: string
  state: "reachable" | "refused" | "timeout" | "dns_failure" | "unknown"
  latencyMs: number
  message?: string
}

export interface RoutesConnectParams {
  hostProfileId: string
  routeId: string
}

export interface FrpInstancesListResult {
  endpoints: FrpEndpoint[]
  proxies: FrpProxy[]
}

export interface FrpProxiesApplyParams {
  proxy: FrpProxy
}

export interface FrpProxiesApplyResult {
  proxy: FrpProxy
}

export interface FrpProxiesStopParams {
  proxyId: string
}

export interface FrpProxiesRemoveParams {
  proxyId: string
}

export interface FrpRotateParams {
  endpointId: string
}

export interface FrpServerParams {
  endpointId: string
}

export interface TransferInfo {
  transferId: string
  hostProfileId?: string
  protocol: string
  direction: "upload" | "download"
  target: string
  localPath: string
  remotePath: string
  recursive: boolean
  state: "running" | "completed" | "failed" | "cancelled"
  startedAt: string
  finishedAt?: string
  errorCode?: string
  errorLabel?: string
}

export interface TransfersStartParams {
  hostProfileId: string
  routeId?: string
  protocol: "scp" | "sftp"
  direction: "upload" | "download"
  localPath: string
  remotePath: string
  recursive?: boolean
}

export interface TransfersListResult {
  transfers: TransferInfo[]
}

export interface TransfersCancelParams {
  transferId: string
}

export interface VaultStatusResult {
  provider: string
  locked: boolean
  profiles: number
  agentProvider?: string
  agentSocket?: string
  agentIdentities?: number
  agentMessage?: string
}

export interface VaultUnlockParams {
  provider: string
  reason: string
}

export interface DiagnosticsReport {
  generatedAt: string
  protocolVersion: number
  hostsCount: number
  conflicts: number
  sessions: SessionInfo[]
  vault: VaultStatusResult
  frp: FrpInstancesListResult
}

export interface HostDiagnosticsParams {
  hostProfileId: string
  routeId?: string
}

export interface DNSCheck {
  host: string
  addresses: string[]
  durationMs: number
  error?: string
}

export interface TCPCheck {
  address: string
  state: "reachable" | "refused" | "timeout" | "dns_failure" | "unknown"
  latencyMs: number
  error?: string
}

export interface BannerCheck {
  received?: string
  durationMs: number
  error?: string
}

export interface TLSCheck {
  applicable: boolean
  subject?: string
  issuer?: string
  notAfter?: string
  error?: string
  message?: string
}

export interface HostDiagnosticsResult {
  hostProfileId: string
  generatedAt: string
  routeId: string
  routeKind: RouteKind
  address: string
  dns: DNSCheck
  tcp: TCPCheck
  banner: BannerCheck
  tls: TLSCheck
}

export interface NmapParams {
  /** Optional private IPv4 CIDR. Empty means auto-detect the local network. */
  network?: string
  services?: boolean
  scripts?: boolean
  os?: boolean
  trace?: boolean
  allPorts?: boolean
  sudo?: boolean
  confirm: boolean
  authorized: boolean
  /** @deprecated Host port scans are no longer performed. */
  hostProfileId?: string
  routeId?: string
  ports?: string
}

export interface NmapOSClass {
  type?: string
  vendor?: string
  osFamily?: string
  osGen?: string
  accuracy?: number
}

export interface NmapOSMatch {
  name: string
  accuracy?: number
  classes?: NmapOSClass[]
}

export interface NmapScript {
  id: string
  output?: string
}

/** One host discovered by nmap. */
export interface NmapHost {
  address: string
  mac?: string
  vendor?: string
  hostname?: string
  state: string
  deviceType?: string
  osMatches?: NmapOSMatch[]
  ports?: NmapPort[]
  scripts?: NmapScript[]
  uptimeSeconds?: number
  lastBoot?: string
  distance?: number
  trace?: string[]
}

export interface NmapPort {
  port: number
  protocol: string
  state: string
  service?: string
  product?: string
  version?: string
  extraInfo?: string
  cpe?: string[]
}

export interface NmapResult {
  networks: string[]
  hosts: NmapHost[]
  warnings?: string[]
  generatedAt: string
  /** @deprecated */
  target?: string
  /** @deprecated */
  ports?: NmapPort[]
}

export interface NetworkParams {
  hostProfileId: string
  routeId?: string
}

export interface ASNInfo {
  asn?: string
  prefix?: string
  country?: string
  registry?: string
  allocated?: string
  error?: string
}

export interface RDAPInfo {
  name?: string
  handle?: string
  country?: string
  startAddress?: string
  endAddress?: string
  type?: string
  error?: string
}

export interface NetworkInfoResult {
  host: string
  addresses: string[]
  asn: ASNInfo
  rdap: RDAPInfo
  geoCountry?: string
  geoPrecision: string
  provenance: string[]
  generatedAt: string
}

export interface WorkspacesSaveParams {
  workspace: Workspace
}

export interface WorkspacesRestoreParams {
  workspaceId: WorkspaceId
}

export interface WorkspacesRestoreResult {
  workspace: Workspace
  plan: RoutesPlanResult
}

/** Every IPC method and its typed params/result. */
export interface MethodContracts {
  "hosts.list": { params: HostsListParams; result: HostsListResult }
  "hosts.import": { params: HostsImportParams; result: HostsImportResult }
  "hosts.overrides.set": { params: HostOverridePatch; result: HostProfile }
  "hosts.create": { params: HostCreateParams; result: HostProfile }
  "hosts.update": { params: HostUpdateParams; result: HostProfile }
  "hosts.delete": { params: HostDeleteParams; result: void }
  "hosts.trustKey": { params: HostsTrustKeyParams; result: HostsTrustKeyResult }
  "hosts.resolve": { params: HostsListParams; result: RoutesPlanResult }
  "sessions.open": { params: SessionsOpenParams; result: SessionsOpenResult }
  "sessions.write": { params: SessionsWriteParams; result: void }
  "sessions.resize": { params: SessionsResizeParams; result: void }
  "sessions.close": { params: SessionsCloseParams; result: void }
  "sessions.detachTmux": { params: SessionsTmuxParams; result: void }
  "sessions.attachTmux": { params: SessionsTmuxParams; result: void }
  "sessions.signal": { params: SessionsSignalParams; result: void }
  "sessions.list": { params: Record<string, never>; result: SessionsListResult }
  "sessions.attach": { params: SessionsAttachParams; result: void }
  "routes.plan": { params: RoutesPlanParams; result: RoutesPlanResult }
  "routes.probe": { params: RoutesProbeParams; result: RoutesProbeResult }
  "routes.connect": { params: RoutesConnectParams; result: SessionsOpenResult }
  "frp.instances.list": { params: Record<string, never>; result: FrpInstancesListResult }
  "frp.proxies.apply": { params: FrpProxiesApplyParams; result: FrpProxiesApplyResult }
  "frp.proxies.stop": { params: FrpProxiesStopParams; result: void }
  "frp.proxies.remove": { params: FrpProxiesRemoveParams; result: void }
  "frp.tokens.rotate": { params: FrpRotateParams; result: void }
  "frp.servers.start": { params: FrpServerParams; result: FrpEndpoint }
  "frp.servers.stop": { params: FrpServerParams; result: void }
  "transfers.start": { params: TransfersStartParams; result: TransferInfo }
  "transfers.list": { params: Record<string, never>; result: TransfersListResult }
  "transfers.cancel": { params: TransfersCancelParams; result: void }
  "vault.status": { params: Record<string, never>; result: VaultStatusResult }
  "vault.unlockRequest": { params: VaultUnlockParams; result: VaultStatusResult }
  "diagnostics.report": { params: Record<string, never>; result: DiagnosticsReport }
  "diagnostics.host": { params: HostDiagnosticsParams; result: HostDiagnosticsResult }
  "diagnostics.nmap": { params: NmapParams; result: NmapResult }
  "diagnostics.network": { params: NetworkParams; result: NetworkInfoResult }
  "workspaces.save": { params: WorkspacesSaveParams; result: void }
  "workspaces.restore": { params: WorkspacesRestoreParams; result: WorkspacesRestoreResult }
}

export type Method = keyof MethodContracts
export type ParamsOf<M extends Method> = MethodContracts[M]["params"]
export type ResultOf<M extends Method> = MethodContracts[M]["result"]

export const IPC_METHODS = [
  "hosts.list",
  "hosts.import",
  "hosts.resolve",
  "hosts.overrides.set",
  "hosts.create",
  "hosts.update",
  "hosts.delete",
  "hosts.trustKey",
  "sessions.open",
  "sessions.write",
  "sessions.resize",
  "sessions.close",
  "sessions.detachTmux",
  "sessions.attachTmux",
  "sessions.signal",
  "sessions.list",
  "sessions.attach",
  "routes.plan",
  "routes.probe",
  "routes.connect",
  "frp.instances.list",
  "frp.proxies.apply",
  "frp.proxies.stop",
  "frp.proxies.remove",
  "frp.tokens.rotate",
  "frp.servers.start",
  "frp.servers.stop",
  "transfers.start",
  "transfers.list",
  "transfers.cancel",
  "vault.status",
  "vault.unlockRequest",
  "diagnostics.report",
  "diagnostics.host",
  "diagnostics.nmap",
  "diagnostics.network",
  "workspaces.save",
  "workspaces.restore",
] as const satisfies readonly Method[]
