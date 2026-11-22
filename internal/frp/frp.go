// Package frp supervises local frpc processes and encrypted proxy definitions.
package frp

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lordpietre/ssh05/contracts"
)

const keySize = 32

var randRead = rand.Read

type endpoint struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Address        string             `json:"address"`
	BindAddr       string             `json:"bindAddr,omitempty"`
	Port           int                `json:"port"`
	Role           string             `json:"role"`
	Token          string             `json:"-"`
	TokenEnc       string             `json:"token_enc,omitempty"`
	Managed        bool               `json:"managed"`
	State          contracts.FrpState `json:"state"`
	LastErrorCode  string             `json:"lastErrorCode,omitempty"`
	LastErrorLabel string             `json:"lastErrorLabel,omitempty"`
}

type proxy struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	EndpointID     string             `json:"endpointId"`
	HostProfileID  string             `json:"hostProfileId,omitempty"`
	Type           string             `json:"type"`
	ServicePort    int                `json:"servicePort"`
	RemotePort     int                `json:"remotePort"`
	BindPort       int                `json:"bindPort,omitempty"`
	Exposure       string             `json:"exposure"`
	Secret         string             `json:"-"`
	SecretEnc      string             `json:"secret_enc,omitempty"`
	State          contracts.FrpState `json:"state"`
	LastErrorCode  string             `json:"lastErrorCode,omitempty"`
	LastErrorLabel string             `json:"lastErrorLabel,omitempty"`
}

type storeFile struct {
	Endpoints []*endpoint `json:"endpoints"`
	Proxies   []*proxy    `json:"proxies"`
}

// Options configures the manager.
type Options struct {
	ConfigPath         string
	KeyPath            string
	WorkDir            string
	FrpcBin            string
	FrpcSHA256         string
	FrpsBin            string
	FrpsSHA256         string
	EndpointName       string
	EndpointAddress    string
	EndpointListen     string
	EndpointPort       int
	EndpointToken      string
	EndpointTokenForce bool
	// SkipListenProbe is for tests that use a stub frps without a socket.
	SkipListenProbe bool
}

// Manager owns local frpc processes and their definitions.
type Manager struct {
	mu        sync.Mutex
	opts      Options
	key       []byte
	endpoints []*endpoint
	proxies   []*proxy
	procs     map[string]*exec.Cmd
	servers   map[string]*exec.Cmd
	started   map[string]time.Time
	next      int
}

// NewManager loads the encrypted store and ensures the optional default endpoint.
func NewManager(options Options) (*Manager, error) {
	if options.ConfigPath == "" {
		options.ConfigPath = defaultPath("TRSS_FRP_CONFIG", ".config", "trss", "frp.json")
	}
	if options.KeyPath == "" {
		options.KeyPath = defaultPath("", ".config", "trss", "frp.key")
	}
	if options.WorkDir == "" {
		options.WorkDir = defaultStatePath("frp")
	}
	if options.FrpcBin == "" {
		options.FrpcBin = "frpc"
	}
	if options.FrpcSHA256 != "" {
		if err := verifyBinaryHash(options.FrpcBin, options.FrpcSHA256); err != nil {
			return nil, err
		}
	}
	if options.FrpsBin == "" {
		options.FrpsBin = "frps"
	}
	if options.FrpsSHA256 != "" {
		if err := verifyBinaryHash(options.FrpsBin, options.FrpsSHA256); err != nil {
			return nil, err
		}
	}
	key, err := loadOrCreateKey(options.KeyPath)
	if err != nil {
		return nil, err
	}
	m := &Manager{
		opts: options, key: key,
		procs: make(map[string]*exec.Cmd), servers: make(map[string]*exec.Cmd),
		started: make(map[string]time.Time),
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(options.WorkDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(options.WorkDir, 0o700); err != nil {
		return nil, err
	}
	if options.EndpointAddress != "" || options.EndpointListen != "" {
		if err := m.ensureEndpoint(); err != nil {
			return nil, err
		}
	}
	if options.EndpointListen != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := m.StartEndpoint(ctx, "endpoint-"+endpointName(options)); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func defaultPath(env, sub, dir, file string) string {
	if env != "" {
		if value := os.Getenv(env); value != "" {
			return value
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, sub, dir, file)
}

func defaultStatePath(dir string) string {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "trss", dir)
}

// verifyBinaryHash refuses a binary whose SHA-256 does not match the pinned
// digest. It is used before starting or updating frpc.
func verifyBinaryHash(binary, expected string) error {
	path, err := exec.LookPath(binary)
	if err != nil {
		return fmt.Errorf("frpc binary %q: %w", binary, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read frpc binary: %w", err)
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	want := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(expected, "sha256:")))
	if actual != want {
		return fmt.Errorf("frpc binary hash mismatch: got %s", actual)
	}
	return nil
}

// BinarySHA256 returns the local frpc digest, for display and update checks.
func (m *Manager) BinarySHA256() (string, error) {
	path, err := exec.LookPath(m.opts.FrpcBin)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		if len(data) != keySize {
			return nil, fmt.Errorf("invalid FRP key length %d", len(data))
		}
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	key := make([]byte, keySize)
	if _, err := randRead(key); err != nil {
		return nil, fmt.Errorf("FRP key: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func seal(key []byte, value string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := randRead(nonce); err != nil {
		return "", err
	}
	return "v1:" + hex.EncodeToString(gcm.Seal(nonce, nonce, []byte(value), nil)), nil
}

func open(key []byte, value string) (string, error) {
	if !strings.HasPrefix(value, "v1:") {
		return "", fmt.Errorf("unsupported FRP secret format")
	}
	data, err := hex.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("FRP secret too short")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.opts.ConfigPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var file storeFile
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	m.endpoints = file.Endpoints
	m.proxies = file.Proxies
	for _, endpoint := range m.endpoints {
		if endpoint.TokenEnc != "" {
			endpoint.Token, err = open(m.key, endpoint.TokenEnc)
			if err != nil {
				return err
			}
		}
		if endpoint.State != contracts.FrpStopped && endpoint.State != contracts.FrpError {
			endpoint.State = contracts.FrpStopped
		}
	}
	for _, proxy := range m.proxies {
		if proxy.SecretEnc != "" {
			proxy.Secret, err = open(m.key, proxy.SecretEnc)
			if err != nil {
				return err
			}
		}
		if proxy.State != contracts.FrpStopped && proxy.State != contracts.FrpError {
			proxy.State = contracts.FrpStopped
		}
	}
	return nil
}

func (m *Manager) saveLocked() error {
	for _, endpoint := range m.endpoints {
		if endpoint.Token != "" {
			sealed, err := seal(m.key, endpoint.Token)
			if err != nil {
				return err
			}
			endpoint.TokenEnc = sealed
		}
	}
	for _, proxy := range m.proxies {
		if proxy.Secret != "" {
			sealed, err := seal(m.key, proxy.Secret)
			if err != nil {
				return err
			}
			proxy.SecretEnc = sealed
		}
	}
	data, err := json.MarshalIndent(storeFile{Endpoints: m.endpoints, Proxies: m.proxies}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(m.opts.ConfigPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	tmp := m.opts.ConfigPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.opts.ConfigPath)
}

func endpointName(options Options) string {
	if options.EndpointName != "" {
		return options.EndpointName
	}
	return "default"
}

func parseListen(value string) (bindAddr, clientAddr string, port int, err error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		if _, convErr := fmt.Sscanf(strings.TrimSpace(value), "%d", &port); convErr == nil {
			host, portText = "", strings.TrimSpace(value)
		} else {
			return "", "", 0, fmt.Errorf("invalid frps listen address %q: %w", value, err)
		}
	}
	if portText == "" {
		port = 7000
	} else if _, scanErr := fmt.Sscanf(portText, "%d", &port); scanErr != nil || port <= 0 || port > 65535 {
		return "", "", 0, fmt.Errorf("invalid frps listen port %q", portText)
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "0.0.0.0"
	}
	clientAddr = host
	if host == "0.0.0.0" || host == "::" || host == "[::]" {
		clientAddr = "127.0.0.1"
	}
	return host, clientAddr, port, nil
}

func (m *Manager) ensureEndpoint() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := endpointName(m.opts)
	id := "endpoint-" + name
	listenBind, listenClient, listenPort := "", "", 0
	if m.opts.EndpointListen != "" {
		host, client, port, err := parseListen(m.opts.EndpointListen)
		if err != nil {
			return err
		}
		listenBind, listenClient, listenPort = host, client, port
	}
	for _, endpoint := range m.endpoints {
		match := endpoint.Address == m.opts.EndpointAddress && endpoint.Port == m.opts.EndpointPort
		if m.opts.EndpointListen != "" {
			match = endpoint.ID == id || (endpoint.Managed && endpoint.BindAddr == listenBind && endpoint.Port == listenPort)
		}
		if !match {
			continue
		}
		if endpoint.Token == "" || m.opts.EndpointTokenForce {
			if m.opts.EndpointToken != "" {
				endpoint.Token = m.opts.EndpointToken
			}
		}
		if m.opts.EndpointListen != "" {
			endpoint.BindAddr = listenBind
			endpoint.Port = listenPort
			endpoint.Managed = true
			if m.opts.EndpointAddress == "" {
				endpoint.Address = listenClient
			}
		}
		return m.saveLocked()
	}
	address := m.opts.EndpointAddress
	port := m.opts.EndpointPort
	if listenPort > 0 {
		address = listenClient
		port = listenPort
		if m.opts.EndpointAddress != "" {
			address = m.opts.EndpointAddress
		}
	}
	if port == 0 {
		port = 7000
	}
	m.endpoints = append(m.endpoints, &endpoint{
		ID: id, Name: name, Address: address, BindAddr: listenBind,
		Port: port, Role: "frps", Token: m.opts.EndpointToken,
		Managed: listenPort > 0, State: contracts.FrpStopped,
	})
	return m.saveLocked()
}

func (m *Manager) List() contracts.FrpInstancesListResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := contracts.FrpInstancesListResult{
		Endpoints: []contracts.FrpEndpoint{},
		Proxies:   []contracts.FrpProxy{},
	}
	for _, endpoint := range m.endpoints {
		pid, uptime := m.statsLocked("server", endpoint.ID, m.servers[endpoint.ID])
		result.Endpoints = append(result.Endpoints, contracts.FrpEndpoint{
			ID: endpoint.ID, Role: endpoint.Role, Address: endpoint.Address,
			Port: endpoint.Port, Managed: endpoint.Managed, State: endpoint.State,
			PID: pid, UptimeSeconds: uptime,
			LastErrorCode: endpoint.LastErrorCode, LastErrorLabel: endpoint.LastErrorLabel,
		})
	}
	for _, proxy := range m.proxies {
		item := toContract(proxy)
		item.PID, item.UptimeSeconds = m.statsLocked("proxy", proxy.ID, m.procs[proxy.ID])
		result.Proxies = append(result.Proxies, item)
	}
	return result
}

func (m *Manager) statsLocked(kind, id string, cmd *exec.Cmd) (int, int64) {
	if cmd == nil || cmd.Process == nil {
		return 0, 0
	}
	started, ok := m.started[kind+":"+id]
	if !ok {
		return cmd.Process.Pid, 0
	}
	seconds := int64(time.Since(started).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return cmd.Process.Pid, seconds
}

func toContract(proxy *proxy) contracts.FrpProxy {
	return contracts.FrpProxy{
		ID: proxy.ID, Name: proxy.Name, Type: contracts.FrpProxyType(proxy.Type),
		EndpointID: proxy.EndpointID, HostProfileID: proxy.HostProfileID,
		ServicePort: proxy.ServicePort, RemotePort: proxy.RemotePort, BindPort: proxy.BindPort,
		Exposure: proxy.Exposure, State: proxy.State,
		LastErrorCode: proxy.LastErrorCode, LastErrorLabel: proxy.LastErrorLabel,
	}
}

// Apply validates, verifies and starts a local frpc process.
func (m *Manager) Apply(ctx context.Context, requested contracts.FrpProxy) (contracts.FrpProxy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(requested.Name) == "" {
		return requested, fmt.Errorf("proxy name is required")
	}
	proxyType := normalizeType(string(requested.Type))
	if proxyType == "" {
		return requested, fmt.Errorf("unsupported proxy type %q", requested.Type)
	}
	endpoint := m.findEndpointLocked(requested.EndpointID)
	if endpoint == nil {
		return requested, fmt.Errorf("FRP endpoint not found")
	}
	if requested.ServicePort <= 0 || requested.ServicePort > 65535 {
		return requested, fmt.Errorf("invalid service port")
	}
	visitor := (proxyType == "stcp" || proxyType == "xtcp") && requested.BindPort > 0
	if (proxyType == "tcp" || proxyType == "udp") &&
		(requested.RemotePort <= 0 || requested.RemotePort > 65535) {
		return requested, fmt.Errorf("invalid remote port")
	}
	if visitor && (requested.BindPort <= 0 || requested.BindPort > 65535) {
		return requested, fmt.Errorf("invalid bind port")
	}

	existing := m.findProxyLocked(requested.ID)
	if existing == nil {
		id := requested.ID
		if id == "" {
			for {
				m.next++
				id = fmt.Sprintf("frp-%d", m.next)
				if m.findProxyLocked(id) == nil {
					break
				}
			}
		}
		existing = &proxy{ID: id}
		m.proxies = append(m.proxies, existing)
	}
	existing.Name = requested.Name
	existing.EndpointID = endpoint.ID
	existing.HostProfileID = requested.HostProfileID
	existing.Type = proxyType
	existing.ServicePort = requested.ServicePort
	existing.RemotePort = requested.RemotePort
	existing.BindPort = requested.BindPort
	existing.Exposure = requested.Exposure
	if existing.Exposure == "" {
		existing.Exposure = "private"
		if proxyType == "tcp" || proxyType == "udp" {
			existing.Exposure = "public"
		}
	}
	if endpoint.Token == "" {
		token, err := generateSecret()
		if err != nil {
			return requested, err
		}
		endpoint.Token = token
	}
	if endpoint.Managed && endpoint.State != contracts.FrpOnline {
		if err := m.startEndpointLocked(ctx, endpoint); err != nil {
			existing.State = contracts.FrpError
			existing.LastErrorCode = "frps_start_failed"
			existing.LastErrorLabel = err.Error()
			_ = m.saveLocked()
			return requested, err
		}
	}
	if (proxyType == "stcp" || proxyType == "xtcp") && existing.Secret == "" {
		secret, err := generateSecret()
		if err != nil {
			return requested, err
		}
		existing.Secret = secret
	}
	if err := m.startLocked(ctx, existing, endpoint); err != nil {
		existing.State = contracts.FrpError
		existing.LastErrorCode = "apply_failed"
		existing.LastErrorLabel = err.Error()
		_ = m.saveLocked()
		return requested, err
	}
	existing.State = contracts.FrpOnline
	existing.LastErrorCode = ""
	existing.LastErrorLabel = ""
	if err := m.saveLocked(); err != nil {
		return requested, err
	}
	return toContract(existing), nil
}

func (m *Manager) findEndpointLocked(id string) *endpoint {
	if id == "" && len(m.endpoints) > 0 {
		return m.endpoints[0]
	}
	for _, endpoint := range m.endpoints {
		if endpoint.ID == id || endpoint.Name == id {
			return endpoint
		}
	}
	return nil
}

func (m *Manager) findProxyLocked(id string) *proxy {
	for _, proxy := range m.proxies {
		if proxy.ID == id {
			return proxy
		}
	}
	return nil
}

func (m *Manager) startLocked(ctx context.Context, proxy *proxy, endpoint *endpoint) error {
	m.stopLocked(proxy.ID)
	path := filepath.Join(m.opts.WorkDir, proxy.ID+".toml")
	if err := os.WriteFile(path, []byte(renderConfig(endpoint, proxy)), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	verify := exec.CommandContext(ctx, m.opts.FrpcBin, "verify", "-c", path)
	if output, err := verify.CombinedOutput(); err != nil {
		return fmt.Errorf("frpc verify: %v: %s", err, strings.TrimSpace(string(output)))
	}
	logPath := filepath.Join(m.opts.WorkDir, proxy.ID+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(m.opts.FrpcBin, "-c", path)
	configureProcess(cmd)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	m.procs[proxy.ID] = cmd
	m.started["proxy:"+proxy.ID] = time.Now()
	go m.wait(proxy.ID, cmd)
	return nil
}

func (m *Manager) wait(id string, cmd *exec.Cmd) {
	err := cmd.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.procs[id] != cmd {
		return
	}
	delete(m.procs, id)
	delete(m.started, "proxy:"+id)
	proxy := m.findProxyLocked(id)
	if proxy != nil && proxy.State == contracts.FrpOnline {
		proxy.State = contracts.FrpError
		proxy.LastErrorCode = "process_exited"
		if err != nil {
			proxy.LastErrorLabel = err.Error()
		} else {
			proxy.LastErrorLabel = "frpc exited"
		}
	}
	_ = m.saveLocked()
}

// Stop terminates one local frpc process.
func (m *Manager) Stop(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	proxy := m.findProxyLocked(id)
	if proxy == nil {
		return fmt.Errorf("FRP proxy not found")
	}
	m.stopLocked(id)
	proxy.State = contracts.FrpStopped
	proxy.LastErrorCode = ""
	proxy.LastErrorLabel = ""
	return m.saveLocked()
}

// Remove stops a proxy and deletes its persisted definition and local files.
func (m *Manager) Remove(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	proxy := m.findProxyLocked(id)
	if proxy == nil {
		return fmt.Errorf("FRP proxy not found")
	}
	m.stopLocked(id)
	for index, item := range m.proxies {
		if item.ID == id {
			m.proxies = append(m.proxies[:index], m.proxies[index+1:]...)
			break
		}
	}
	_ = os.Remove(filepath.Join(m.opts.WorkDir, id+".toml"))
	_ = os.Remove(filepath.Join(m.opts.WorkDir, id+".log"))
	return m.saveLocked()
}

func (m *Manager) stopLocked(id string) {
	if cmd := m.procs[id]; cmd != nil {
		killProcess(cmd)
		delete(m.procs, id)
		delete(m.started, "proxy:"+id)
	}
}

// RotateEndpoint generates a new token and restarts the endpoint proxies.
func (m *Manager) RotateEndpoint(ctx context.Context, endpointID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	endpoint := m.findEndpointLocked(endpointID)
	if endpoint == nil {
		return fmt.Errorf("FRP endpoint not found")
	}
	if !endpoint.Managed {
		return fmt.Errorf("FRP endpoint %q is remote: update its token out of band before rotating", endpoint.Name)
	}
	token, err := generateSecret()
	if err != nil {
		return err
	}
	endpoint.Token = token
	// The managed frps must accept the new token before clients reconnect.
	m.stopServerLocked(endpoint.ID)
	endpoint.State = contracts.FrpStopped
	if err := m.startEndpointLocked(ctx, endpoint); err != nil {
		endpoint.State = contracts.FrpError
		endpoint.LastErrorCode = "rotate_failed"
		endpoint.LastErrorLabel = err.Error()
		for _, proxy := range m.proxies {
			if proxy.EndpointID == endpoint.ID {
				m.stopLocked(proxy.ID)
				proxy.State = contracts.FrpError
				proxy.LastErrorCode = "rotate_failed"
				proxy.LastErrorLabel = "frps no aceptó el token nuevo"
			}
		}
		_ = m.saveLocked()
		return err
	}
	for _, proxy := range m.proxies {
		if proxy.EndpointID != endpoint.ID {
			continue
		}
		proxy.State = contracts.FrpRestarting
		if err := m.startLocked(ctx, proxy, endpoint); err != nil {
			proxy.State = contracts.FrpError
			proxy.LastErrorCode = "rotate_failed"
			proxy.LastErrorLabel = err.Error()
			continue
		}
		proxy.State = contracts.FrpOnline
	}
	return m.saveLocked()
}

// RouteForHost returns the local visitor port for a host, if online.
func (m *Manager) RouteForHost(hostProfileID string) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, proxy := range m.proxies {
		if proxy.HostProfileID == hostProfileID && proxy.BindPort > 0 && proxy.State == contracts.FrpOnline {
			return proxy.BindPort, true
		}
	}
	return 0, false
}

// Close stops all local processes.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.procs {
		m.stopLocked(id)
	}
	for id := range m.servers {
		m.stopServerLocked(id)
	}
}

// StartEndpoint starts a managed local frps.
func (m *Manager) StartEndpoint(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep := m.findEndpointLocked(id)
	if ep == nil {
		return fmt.Errorf("FRP endpoint not found")
	}
	return m.startEndpointLocked(ctx, ep)
}

func (m *Manager) startEndpointLocked(ctx context.Context, ep *endpoint) error {
	if !ep.Managed {
		return fmt.Errorf("FRP endpoint %q is not managed locally", ep.Name)
	}
	if m.servers[ep.ID] != nil {
		ep.State = contracts.FrpOnline
		return nil
	}
	if ep.Token == "" {
		token, err := generateSecret()
		if err != nil {
			return err
		}
		ep.Token = token
	}
	path := filepath.Join(m.opts.WorkDir, ep.ID+"-frps.toml")
	if err := os.WriteFile(path, []byte(renderServerConfig(ep)), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	verify := exec.CommandContext(ctx, m.opts.FrpsBin, "verify", "-c", path)
	if output, err := verify.CombinedOutput(); err != nil {
		return fmt.Errorf("frps verify: %v: %s", err, strings.TrimSpace(string(output)))
	}
	logPath := filepath.Join(m.opts.WorkDir, ep.ID+"-frps.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(m.opts.FrpsBin, "-c", path)
	configureProcess(cmd)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	ep.State = contracts.FrpStarting
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		ep.State = contracts.FrpError
		ep.LastErrorCode = "start_failed"
		ep.LastErrorLabel = err.Error()
		_ = m.saveLocked()
		return err
	}
	m.servers[ep.ID] = cmd
	m.started["server:"+ep.ID] = time.Now()
	go m.waitServer(ep.ID, cmd)
	if !m.opts.SkipListenProbe {
		if err := waitEndpointReady(ep); err != nil {
			m.stopServerLocked(ep.ID)
			ep.State = contracts.FrpError
			ep.LastErrorCode = "listen_failed"
			ep.LastErrorLabel = err.Error()
			_ = m.saveLocked()
			return err
		}
	}
	ep.State = contracts.FrpOnline
	ep.LastErrorCode = ""
	ep.LastErrorLabel = ""
	return m.saveLocked()
}

func waitEndpointReady(ep *endpoint) error {
	host := ep.BindAddr
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::", "[::]":
		host = "::1"
	}
	address := net.JoinHostPort(host, fmt.Sprintf("%d", ep.Port))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("frps did not listen on %s", address)
}

func (m *Manager) waitServer(id string, cmd *exec.Cmd) {
	err := cmd.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.servers[id] != cmd {
		return
	}
	delete(m.servers, id)
	delete(m.started, "server:"+id)
	ep := m.findEndpointLocked(id)
	if ep != nil && ep.State == contracts.FrpOnline {
		ep.State = contracts.FrpError
		ep.LastErrorCode = "process_exited"
		if err != nil {
			ep.LastErrorLabel = err.Error()
		} else {
			ep.LastErrorLabel = "frps exited"
		}
	}
	_ = m.saveLocked()
}

// StopEndpoint stops a managed frps and the local proxies that depend on it.
func (m *Manager) StopEndpoint(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep := m.findEndpointLocked(id)
	if ep == nil {
		return fmt.Errorf("FRP endpoint not found")
	}
	m.stopServerLocked(ep.ID)
	ep.State = contracts.FrpStopped
	ep.LastErrorCode = ""
	ep.LastErrorLabel = ""
	for _, proxy := range m.proxies {
		if proxy.EndpointID != ep.ID {
			continue
		}
		m.stopLocked(proxy.ID)
		proxy.State = contracts.FrpStopped
		proxy.LastErrorCode = ""
		proxy.LastErrorLabel = ""
	}
	return m.saveLocked()
}

func (m *Manager) stopServerLocked(id string) {
	if cmd := m.servers[id]; cmd != nil {
		killProcess(cmd)
		delete(m.servers, id)
		delete(m.started, "server:"+id)
	}
}

func renderServerConfig(endpoint *endpoint) string {
	bindAddr := endpoint.BindAddr
	if bindAddr == "" {
		bindAddr = "0.0.0.0"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "bindAddr = %q\nbindPort = %d\n\nauth.method = \"token\"\nauth.token = %q\n",
		bindAddr, endpoint.Port, endpoint.Token)
	return b.String()
}

func renderConfig(endpoint *endpoint, proxy *proxy) string {
	var b strings.Builder
	fmt.Fprintf(&b, "serverAddr = %q\nserverPort = %d\n\nauth.method = \"token\"\nauth.token = %q\n\n",
		endpoint.Address, endpoint.Port, endpoint.Token)
	if proxy.Type == "stcp" || proxy.Type == "xtcp" {
		fmt.Fprintf(&b, "[[proxies]]\nname = %q\ntype = %q\nlocalIP = \"127.0.0.1\"\nlocalPort = %d\nsecretKey = %q\n",
			proxy.Name, proxy.Type, proxy.ServicePort, proxy.Secret)
		if proxy.BindPort > 0 {
			fmt.Fprintf(&b, "\n[[visitors]]\nname = %q\ntype = %q\nserverName = %q\nsecretKey = %q\nbindAddr = \"127.0.0.1\"\nbindPort = %d\n",
				proxy.Name+"-visitor", proxy.Type, proxy.Name, proxy.Secret, proxy.BindPort)
		}
	} else {
		fmt.Fprintf(&b, "[[proxies]]\nname = %q\ntype = %q\nlocalIP = \"127.0.0.1\"\nlocalPort = %d\nremotePort = %d\n",
			proxy.Name, proxy.Type, proxy.ServicePort, proxy.RemotePort)
	}
	return b.String()
}

func normalizeType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "tcp", "udp", "stcp", "xtcp":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func generateSecret() (string, error) {
	buffer := make([]byte, 32)
	if _, err := randRead(buffer); err != nil {
		return "", fmt.Errorf("secure FRP secret: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

// Version returns the local frpc version, if available.
func (m *Manager) Version(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, m.opts.FrpcBin, "-v").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
