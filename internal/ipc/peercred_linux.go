//go:build linux

package ipc

import (
	"net"
	"syscall"
)

func peerIdentityFor(conn net.Conn) (peerIdentity, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return peerIdentity{}, nil
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return peerIdentity{}, err
	}

	var credential *syscall.Ucred
	var credentialErr error
	if err := raw.Control(func(fd uintptr) {
		credential, credentialErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return peerIdentity{}, err
	}
	if credentialErr != nil {
		return peerIdentity{}, credentialErr
	}
	return peerIdentity{UID: credential.Uid, Verified: true}, nil
}
