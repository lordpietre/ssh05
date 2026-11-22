//go:build !windows

package identity

import "os"

func userID() int { return os.Getuid() }
