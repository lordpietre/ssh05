package supervisor

import (
	"context"
	"strconv"

	"github.com/trzsz/trzsz-ssh/contracts"
	"github.com/trzsz/trzsz-ssh/tssh"
)

// SSHHosts exposes the read-only host inventory from ssh_config.
type SSHHosts struct {
	ConfigPath string
}

// List implements HostProvider. Imported hosts remain discovered/unverified.
func (h SSHHosts) List(ctx context.Context) ([]contracts.HostProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := tssh.ListHostEntries(h.ConfigPath)
	if err != nil {
		return nil, err
	}
	profiles := make([]contracts.HostProfile, 0, len(entries))
	for _, entry := range entries {
		port := 22
		if parsed, err := strconv.Atoi(entry.Port); err == nil && parsed > 0 && parsed <= 65535 {
			port = parsed
		}
		hostname := entry.Host
		if hostname == "" {
			hostname = entry.Alias
		}
		profiles = append(profiles, contracts.HostProfile{
			ID:    entry.Alias,
			Alias: entry.Alias,
			Target: contracts.HostTarget{
				Hostname: hostname,
				Port:     port,
				Username: entry.User,
			},
			SourceRefs: []contracts.SourceReference{{
				Kind:    contracts.SourceSSHConfig,
				EntryID: entry.Alias,
			}},
			Discovered:       true,
			VerifiedIdentity: false,
		})
	}
	return profiles, nil
}
