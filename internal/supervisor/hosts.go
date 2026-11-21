package supervisor

import (
	"context"

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
		profiles = append(profiles, sshProfile(entry))
	}
	return profiles, nil
}
