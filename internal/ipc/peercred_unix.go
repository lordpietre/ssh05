//go:build !windows

package ipc

import (
	"fmt"
	"net"
	"os"
)

func checkPeer(conn net.Conn) error {
	identity, err := peerIdentityFor(conn)
	if err != nil {
		return fmt.Errorf("ipc: peer credentials: %w", err)
	}
	if identity.Verified && identity.UID != uint32(os.Getuid()) {
		return ErrUnauthorized
	}
	return nil
}
