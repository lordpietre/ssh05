package ssh05

import "sync"

// The SSH config layer is process-wide. Initialization takes the write lock,
// while connections hold the read lock so sessions that share the same config
// can connect concurrently without a reset. A different config file waits for
// in-flight connections before replacing the global configuration.
var (
	configMu      sync.RWMutex
	configInitMu  sync.Mutex
	configReady   bool
	configInitArg string
)

// acquireConfigRead initializes the global config if needed and returns a
// release function. Same-config callers share the read lock; initialization is
// serialized and never overlaps active readers.
func acquireConfigRead(configFile string) (func(), error) {
	configInitMu.Lock()
	configMu.RLock()
	if configReady && configInitArg == configFile {
		configInitMu.Unlock()
		return configMu.RUnlock, nil
	}
	configMu.RUnlock()

	configMu.Lock()
	if !configReady || configInitArg != configFile {
		if err := initUserConfig(configFile); err != nil {
			configMu.Unlock()
			configInitMu.Unlock()
			return nil, err
		}
	}
	configMu.Unlock()
	configMu.RLock()
	configInitMu.Unlock()
	return configMu.RUnlock, nil
}

// HostEntry is the non-secret host inventory exposed to the supervisor.
type HostEntry struct {
	Alias       string
	Host        string
	Port        string
	User        string
	ProxyJump   string
	GroupLabels string
	Color       string
	Favorite    bool
	System      bool
	IsDocker    bool
}

// ListHostEntries loads ~/.ssh/config (or configPath) and returns the visible
// host aliases. It never returns credentials.
func ListHostEntries(configPath string) ([]HostEntry, error) {
	configInitMu.Lock()
	configMu.Lock()
	defer configInitMu.Unlock()
	defer configMu.Unlock()

	if err := initUserConfig(configPath); err != nil {
		return nil, err
	}
	hosts := getAllHosts()
	entries := make([]HostEntry, 0, len(hosts))
	for _, host := range hosts {
		if host == nil || host.Alias == "" {
			continue
		}
		entries = append(entries, HostEntry{
			Alias:       host.Alias,
			Host:        host.Host,
			Port:        host.Port,
			User:        host.User,
			ProxyJump:   host.ProxyJump,
			GroupLabels: host.GroupLabels,
			Color:       host.Color,
			Favorite:    host.Favorite,
			System:      host.System,
			IsDocker:    host.IsDocker,
		})
	}
	return entries, nil
}
