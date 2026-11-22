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

	"github.com/trzsz/trzsz-ssh/contracts"
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

func TestServiceNmapRequiresConfirmationAndParsesXML(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "nmap.log")
	script := filepath.Join(dir, "nmap")
	body := "#!/bin/sh\necho \"$*\" > " + log + "\ncat <<'XML'\n" +
		`<nmaprun><host><ports><port protocol="tcp" portid="22"><state state="open"/><service name="ssh" product="OpenSSH" version="9.6"/></port>` +
		`<port protocol="tcp" portid="80"><state state="closed"/><service name="http"/></port></ports></host></nmaprun>` +
		"\nXML\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "ssh_config")
	if err := os.WriteFile(config, []byte("Host lab\n    HostName 127.0.0.1\n    Port 2222\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(NewBackend(&fakeRunner{}, Options{}), NewInventory(InventoryConfig{SSHConfigPath: config}), WithNmap(script))

	params := contracts.NmapParams{HostProfileID: "lab", Ports: "22,80", Confirm: false}
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
	if len(result.Ports) != 2 || result.Ports[0].Port != 22 || result.Ports[0].State != "open" || result.Ports[0].Version != "9.6" {
		t.Fatalf("result = %+v", result)
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "-p 22,80") || !strings.Contains(string(data), "-- 127.0.0.1") {
		t.Fatalf("nmap args = %s", data)
	}

	params.Ports = "22; rm -rf /"
	payload, _ = json.Marshal(params)
	if _, rpcErr := service.nmapScan(context.Background(), contracts.RPCRequest{Params: payload}); rpcErr == nil {
		t.Fatal("invalid ports must fail")
	}
}
