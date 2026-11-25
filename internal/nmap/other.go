//go:build !linux || (!amd64 && !arm64)

package nmap

func embeddedBinary() ([]byte, string, string) {
	return nil, "", ""
}
