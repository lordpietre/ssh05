package tssh

import "sync"

// configMu serializes access to the process-wide SSH config state. A future
// refactor should make config per-connection; until then this prevents two
// supervisors from resetting userConfig concurrently.
var configMu sync.Mutex

// HostEntry is the non-secret host inventory exposed to the supervisor.
type HostEntry struct {
	Alias  string
	Host   string
	Port   string
	User   string
	System bool
}

// ListHostEntries loads ~/.ssh/config (or configPath) and returns the visible
// host aliases. It never returns credentials.
func ListHostEntries(configPath string) ([]HostEntry, error) {
	configMu.Lock()
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
			Alias:  host.Alias,
			Host:   host.Host,
			Port:   host.Port,
			User:   host.User,
			System: host.System,
		})
	}
	return entries, nil
}
