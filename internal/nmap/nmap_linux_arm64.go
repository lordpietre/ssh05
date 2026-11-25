//go:build linux && arm64

package nmap

import _ "embed"

//go:embed bin/nmap_linux_arm64.gz
var nmapLinuxARM64 []byte

func embeddedBinary() ([]byte, string, string) {
	return nmapLinuxARM64, "linux-arm64", "d4aa0f8115b71738533941abb5dbff873215af65875b9ac84933273e100fd456"
}
