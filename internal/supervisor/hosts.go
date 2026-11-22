package supervisor

import (
	"context"

	"github.com/lordpietre/ssh05/contracts"
	"github.com/lordpietre/ssh05/ssh05"
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
	entries, err := ssh05.ListHostEntries(h.ConfigPath)
	if err != nil {
		return nil, err
	}
	profiles := make([]contracts.HostProfile, 0, len(entries))
	for _, entry := range entries {
		profiles = append(profiles, sshProfile(entry))
	}
	return profiles, nil
}
