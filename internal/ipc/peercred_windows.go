//go:build windows

package ipc

import "net"

func checkPeer(conn net.Conn) error {
	return nil
}
