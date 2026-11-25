package nmap

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
)

//go:embed nmap-mac-prefixes
var macPrefixData string

var (
	ouiOnce sync.Once
	ouiMap  map[string]string
)

// Vendor resolves a MAC address to its OUI vendor using the embedded Nmap
// prefixes database. It returns an empty string when the prefix is unknown.
func Vendor(mac string) string {
	ouiOnce.Do(func() {
		ouiMap = make(map[string]string, 50000)
		scanner := bufio.NewScanner(strings.NewReader(macPrefixData))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			ouiMap[strings.ToUpper(fields[0])] = strings.Join(fields[1:], " ")
		}
	})
	normalized := normalizeMAC(mac)
	if len(normalized) < 6 {
		return ""
	}
	return ouiMap[normalized[:6]]
}

func normalizeMAC(mac string) string {
	var builder strings.Builder
	for _, char := range strings.ToUpper(mac) {
		if (char >= '0' && char <= '9') || (char >= 'A' && char <= 'F') {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}
