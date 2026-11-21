//go:build !linux && !windows

package ipc

import "net"

// On non-Linux Unix systems the 0700 directory and 0600 socket are the primary
// access control. Peer credential verification is added per platform as needed.
func peerIdentityFor(conn net.Conn) (peerIdentity, error) {
	return peerIdentity{Verified: false}, nil
}
