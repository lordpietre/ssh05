//go:build !windows

package supervisor

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lordpietre/ssh05/contracts"
)

func TestParseASNRecord(t *testing.T) {
	info := parseASNRecord(`"13335 | 104.16.0.0/13 | US | arin | 2014-03-28"`)
	if info.ASN != "13335" || info.Prefix != "104.16.0.0/13" || info.Country != "US" || info.Registry != "arin" {
		t.Fatalf("info = %+v", info)
	}
}

func TestPublicAddressPolicy(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "::1", "fe80::1"} {
		if isPublicAddress(net.ParseIP(value)) {
			t.Fatalf("%s must not be considered public", value)
		}
	}
	if !isPublicAddress(net.ParseIP("1.1.1.1")) {
		t.Fatal("1.1.1.1 must be public")
	}
}

func TestASNQueryIPv6(t *testing.T) {
	query := asnQuery(net.ParseIP("2001:db8::1"))
	prefix := strings.TrimSuffix(query, ".origin6.asn.cymru.com")
	parts := strings.Split(prefix, ".")
	if len(parts) != 32 || parts[0] != "1" || parts[1] != "0" {
		t.Fatalf("query = %q", query)
	}
	if got := asnQuery(net.ParseIP("1.2.3.4")); got != "4.3.2.1.origin.asn.cymru.com" {
		t.Fatalf("ipv4 query = %q", got)
	}
}

func TestRDAPResponseParsing(t *testing.T) {
	var parsed rdapResponse
	if err := json.Unmarshal([]byte(`{"name":"CLOUDFLARENET","handle":"AS13335","country":"US","startAddress":"104.16.0.0","endAddress":"104.31.255.255","type":"DIRECT ALLOCATION"}`), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "CLOUDFLARENET" || parsed.Country != "US" || parsed.StartAddress != "104.16.0.0" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestServiceNmapNetworkDiscoveryAndDetailedAnalysis(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "nmap.log")
	script := filepath.Join(dir, "nmap")
	discovery := `<nmaprun>` +
		`<host><status state="up"/><address addr="192.168.1.1" addrtype="ipv4"/><hostnames><hostname name="router.lan" type="PTR"/></hostnames></host>` +
		`<host><status state="up"/><address addr="192.168.1.10" addrtype="ipv4"/><hostnames><hostname name="laptop.lan" type="PTR"/></hostnames></host>` +
		`</nmaprun>`
	detail := `<nmaprun>` +
		`<host><status state="up"/><address addr="192.168.1.1" addrtype="ipv4"/><address addr="AA:BB:CC:DD:EE:01" addrtype="mac" vendor="RouterVendor"/>` +
		`<hostnames><hostname name="router.lan" type="PTR"/></hostnames>` +
		`<os><osmatch name="Linux 5.4" accuracy="98"><osclass type="router" vendor="Linux" osfamily="Linux" osgen="5.X" accuracy="98"/></osmatch></os>` +
		`<ports><port protocol="tcp" portid="22"><state state="open"/><service name="ssh" product="OpenSSH" version="9.6" extrainfo="Ubuntu"><cpe>cpe:/a:openbsd:openssh:9.6</cpe></service></port></ports>` +
		`<uptime seconds="12345" lastboot="Tue Nov 25 10:00:00 2026"/><distance value="1"/><trace><hop ipaddr="192.168.1.1" ttl="1"/></trace>` +
		`<hostscript><script id="http-title" output="Router"/></hostscript></host>` +
		`<host><status state="up"/><address addr="192.168.1.10" addrtype="ipv4"/>` +
		`<os><osmatch name="Windows 11" accuracy="95"><osclass type="general purpose" vendor="Microsoft" osfamily="Windows" osgen="11" accuracy="95"/></osmatch></os>` +
		`<ports><port protocol="tcp" portid="445"><state state="open"/><service name="microsoft-ds" product="Windows SMB"/></port></ports></host>` +
		`</nmaprun>`
	body := "#!/bin/sh\necho \"$*\" >> " + log + "\ncase \"$*\" in\n  *-sn*) cat <<'XML'\n" + discovery +
		"\nXML\n  ;;\n  *) cat <<'XML'\n" + detail + "\nXML\n  ;;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	service := NewService(NewBackend(&fakeRunner{}, Options{}), nil, WithNmap(script))

	params := contracts.NmapParams{Network: "192.168.1.0/24", Services: true, Scripts: true, Trace: true, OS: true, Confirm: false}
	payload, _ := json.Marshal(params)
	if _, rpcErr := service.nmapScan(context.Background(), contracts.RPCRequest{Params: payload}); rpcErr == nil {
		t.Fatal("nmap must require confirmation")
	}
	params.Confirm = true
	params.Authorized = true
	payload, _ = json.Marshal(params)
	raw, rpcErr := service.nmapScan(context.Background(), contracts.RPCRequest{Params: payload})
	if rpcErr != nil {
		t.Fatalf("nmapScan: %v", rpcErr)
	}
	var result contracts.NmapResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Networks) != 1 || result.Networks[0] != "192.168.1.0/24" {
		t.Fatalf("networks = %+v", result.Networks)
	}
	if len(result.Hosts) != 2 {
		t.Fatalf("hosts = %+v", result.Hosts)
	}
	router := result.Hosts[0]
	if router.Address != "192.168.1.1" || router.Hostname != "router.lan" || router.Vendor != "RouterVendor" {
		t.Fatalf("router = %+v", router)
	}
	if router.DeviceType != "router" || len(router.OSMatches) == 0 || router.OSMatches[0].Name != "Linux 5.4" {
		t.Fatalf("os = %+v", router)
	}
	if len(router.Ports) != 1 || router.Ports[0].Service != "ssh" || router.Ports[0].Version != "9.6" || len(router.Ports[0].CPE) != 1 {
		t.Fatalf("ports = %+v", router.Ports)
	}
	if router.UptimeSeconds != 12345 || router.Distance != 1 || len(router.Trace) != 1 {
		t.Fatalf("details = %+v", router)
	}
	if len(router.Scripts) != 1 || router.Scripts[0].ID != "http-title" {
		t.Fatalf("scripts = %+v", router.Scripts)
	}
	if result.Hosts[1].DeviceType != "equipo general" || len(result.Hosts[1].Ports) != 1 {
		t.Fatalf("laptop = %+v", result.Hosts[1])
	}
	data, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("nmap runs = %q", data)
	}
	if !strings.Contains(lines[0], "-sn") || !strings.Contains(lines[0], "--unprivileged") || !strings.Contains(lines[0], "192.168.1.0/24") {
		t.Fatalf("discovery args = %s", lines[0])
	}
	if !strings.Contains(lines[1], "-sV") || !strings.Contains(lines[1], "-sC") || !strings.Contains(lines[1], "--top-ports 200") {
		t.Fatalf("detail args = %s", lines[1])
	}
	if os.Geteuid() != 0 {
		if strings.Contains(lines[1], "--traceroute") || strings.Contains(lines[1], "-O") {
			t.Fatalf("root-only args in unprivileged scan: %s", lines[1])
		}
		if len(result.Warnings) < 2 {
			t.Fatalf("warnings = %+v", result.Warnings)
		}
	}

	params.Network = "8.8.8.0/24"
	payload, _ = json.Marshal(params)
	if _, rpcErr := service.nmapScan(context.Background(), contracts.RPCRequest{Params: payload}); rpcErr == nil {
		t.Fatal("public networks must fail")
	}
	params.Network = "10.0.0.0/8"
	payload, _ = json.Marshal(params)
	if _, rpcErr := service.nmapScan(context.Background(), contracts.RPCRequest{Params: payload}); rpcErr == nil {
		t.Fatal("too large networks must fail")
	}
}

func TestParsePrivateNetwork(t *testing.T) {
	for _, value := range []string{"192.168.1.0/24", "10.20.0.0/24", "172.16.5.0/24", "127.0.0.0/24"} {
		if _, err := parsePrivateNetwork(value); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
	for _, value := range []string{"8.8.8.0/24", "10.0.0.0/8", "2001:db8::/64", "not-a-network"} {
		if _, err := parsePrivateNetwork(value); err == nil {
			t.Fatalf("%s must be rejected", value)
		}
	}
}

func TestNormalizeLocalNetwork(t *testing.T) {
	ip := net.ParseIP("192.168.1.17")
	_, wide, _ := net.ParseCIDR("192.168.1.17/16")
	if got := normalizeLocalNetwork(ip, wide); got != "192.168.1.0/24" {
		t.Fatalf("wide = %q", got)
	}
	_, narrow, _ := net.ParseCIDR("192.168.1.17/24")
	if got := normalizeLocalNetwork(ip, narrow); got != "192.168.1.0/24" {
		t.Fatalf("narrow = %q", got)
	}
}

func TestClassifyNmapHosts(t *testing.T) {
	hosts := []contracts.NmapHost{
		{Address: "192.168.1.20", Vendor: "Arcadyan"},
		{Address: "192.168.1.21", Ports: []contracts.NmapPort{{Port: 445, Service: "microsoft-ds"}}},
		{Address: "192.168.1.22", DeviceType: "general purpose"},
	}
	classifyNmapHosts(hosts)
	if hosts[0].DeviceType != "router / dispositivo de red (estimado)" {
		t.Fatalf("router = %q", hosts[0].DeviceType)
	}
	if hosts[1].DeviceType != "PC Windows (estimado)" {
		t.Fatalf("windows = %q", hosts[1].DeviceType)
	}
	if hosts[2].DeviceType != "equipo general" {
		t.Fatalf("translated = %q", hosts[2].DeviceType)
	}
}
