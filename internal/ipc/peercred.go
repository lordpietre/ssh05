package ipc

// peerIdentity is the verified local identity of a socket peer.
type peerIdentity struct {
	UID      uint32
	Verified bool
}
