//go:build linux && amd64

package nmap

import _ "embed"

//go:embed bin/nmap_linux_amd64.gz
var nmapLinuxAMD64 []byte

func embeddedBinary() ([]byte, string, string) {
	return nmapLinuxAMD64, "linux-amd64", "af9ca522f69a2d58e0161992be9d94b2c25fafaacdccfa6e9b731b158f92bbd3"
}
