package supervisor

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/lordpietre/ssh05/contracts"
	"github.com/lordpietre/ssh05/internal/audit"
	"github.com/lordpietre/ssh05/internal/ipc"
)

type nmapRun struct {
	Hosts []struct {
		Ports []struct {
			Protocol string `xml:"protocol,attr"`
			PortID   int    `xml:"portid,attr"`
			State    struct {
				State string `xml:"state,attr"`
			} `xml:"state"`
			Service struct {
				Name    string `xml:"name,attr"`
				Product string `xml:"product,attr"`
				Version string `xml:"version,attr"`
			} `xml:"service"`
		} `xml:"ports>port"`
	} `xml:"host"`
}

func (s *Service) nmapScan(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.NmapParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if !params.Confirm || !params.Authorized {
		return nil, &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: "nmap requires explicit confirmation and authorization"}
	}
	if strings.TrimSpace(s.nmapPath) == "" {
		return nil, internalError(fmt.Errorf("nmap is not configured; start ssh05d with --nmap"))
	}
	ports := strings.TrimSpace(params.Ports)
	if ports == "" {
		ports = "22,80,443"
	}
	if len(ports) > 64 || strings.TrimLeft(ports, "0123456789,-") != "" {
		return nil, &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: "invalid nmap port list"}
	}
	routes, _ := s.routeCandidates(ctx, params.HostProfileID)
	route, ok := selectRoute(routes, params.RouteID)
	if !ok {
		return nil, internalError(fmt.Errorf("no authorized route for %q", params.HostProfileID))
	}
	address, err := s.routeAddress(ctx, route)
	if err != nil {
		return nil, internalError(err)
	}
	target := address
	if host, _, splitErr := net.SplitHostPort(address); splitErr == nil {
		target = host
	}
	if target == "" || strings.HasPrefix(target, "-") || strings.ContainsAny(target, " \t\r\n") {
		return nil, &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: "invalid nmap target"}
	}
	scanCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(scanCtx, s.nmapPath, "-sT", "-Pn", "-T4", "-oX", "-", "-p", ports, "--", target).Output()
	if err != nil {
		s.log(audit.Event{Action: "diagnostics.nmap", HostProfileID: params.HostProfileID, Result: "error", ErrorCode: "nmap_failed"})
		return nil, internalError(fmt.Errorf("nmap: %w", err))
	}
	var run nmapRun
	if err := xml.Unmarshal(output, &run); err != nil {
		return nil, internalError(fmt.Errorf("nmap XML: %w", err))
	}
	result := contracts.NmapResult{Target: target, Ports: []contracts.NmapPort{}, GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, host := range run.Hosts {
		for _, port := range host.Ports {
			result.Ports = append(result.Ports, contracts.NmapPort{
				Port: port.PortID, Protocol: port.Protocol, State: port.State.State,
				Service: port.Service.Name, Product: port.Service.Product, Version: port.Service.Version,
			})
		}
	}
	sort.Slice(result.Ports, func(a, b int) bool { return result.Ports[a].Port < result.Ports[b].Port })
	s.log(audit.Event{Action: "diagnostics.nmap", HostProfileID: params.HostProfileID, Result: "ok"})
	return marshal(result)
}

func (s *Service) networkInfo(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.NetworkParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	routes, _ := s.routeCandidates(ctx, params.HostProfileID)
	route, ok := selectRoute(routes, params.RouteID)
	if !ok {
		return nil, internalError(fmt.Errorf("no authorized route for %q", params.HostProfileID))
	}
	address, err := s.routeAddress(ctx, route)
	if err != nil {
		return nil, internalError(err)
	}
	host := address
	if name, _, splitErr := net.SplitHostPort(address); splitErr == nil {
		host = name
	}
	result := contracts.NetworkInfoResult{
		Host: host, Addresses: []string{},
		GeoPrecision: "país del registro regional; no es geolocalización precisa",
		Provenance: []string{
			"ASN: Team Cymru DNS",
			"RDAP: rdap.org / registro regional",
			"GeoIP: país del registro (orientativo)",
		},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	dnsCtx, cancelDNS := context.WithTimeout(ctx, 3*time.Second)
	addresses, dnsErr := net.DefaultResolver.LookupHost(dnsCtx, host)
	cancelDNS()
	if dnsErr != nil {
		result.ASN = contracts.ASNInfo{Error: dnsErr.Error()}
		result.RDAP = contracts.RDAPInfo{Error: dnsErr.Error()}
		return marshal(result)
	}
	sort.Strings(addresses)
	result.Addresses = addresses
	ip := net.ParseIP(addresses[0])
	if ip == nil {
		result.ASN = contracts.ASNInfo{Error: "unparseable IP"}
		result.RDAP = contracts.RDAPInfo{Error: "unparseable IP"}
		return marshal(result)
	}
	if !isPublicAddress(ip) {
		message := "dirección privada o local; no se consulta el registro público"
		result.ASN = contracts.ASNInfo{Error: message}
		result.RDAP = contracts.RDAPInfo{Error: message}
		s.log(audit.Event{Action: "diagnostics.network", HostProfileID: params.HostProfileID, Result: "ok"})
		return marshal(result)
	}
	result.ASN = lookupASN(ctx, ip)
	result.RDAP = lookupRDAP(ctx, ip)
	if result.RDAP.Country != "" {
		result.GeoCountry = result.RDAP.Country
	} else if result.ASN.Country != "" {
		result.GeoCountry = result.ASN.Country
	}
	s.log(audit.Event{Action: "diagnostics.network", HostProfileID: params.HostProfileID, Result: "ok"})
	return marshal(result)
}

func isPublicAddress(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback()
}

func selectRoute(routes []contracts.ConnectionRoute, id string) (contracts.ConnectionRoute, bool) {
	if len(routes) == 0 {
		return contracts.ConnectionRoute{}, false
	}
	for _, route := range routes {
		if route.ID == id {
			return route, true
		}
	}
	return routes[len(routes)-1], true
}

func asnQuery(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d.origin.asn.cymru.com", v4[3], v4[2], v4[1], v4[0])
	}
	v6 := ip.To16()
	if v6 == nil {
		return ""
	}
	parts := make([]string, 0, 32)
	for index := len(v6) - 1; index >= 0; index-- {
		parts = append(parts, fmt.Sprintf("%x", v6[index]&0x0f), fmt.Sprintf("%x", v6[index]>>4))
	}
	return strings.Join(parts, ".") + ".origin6.asn.cymru.com"
}

func lookupASN(ctx context.Context, ip net.IP) contracts.ASNInfo {
	query := asnQuery(ip)
	if query == "" {
		return contracts.ASNInfo{Error: "unparseable IP"}
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	records, err := net.DefaultResolver.LookupTXT(lookupCtx, query)
	if err != nil || len(records) == 0 {
		if err == nil {
			err = fmt.Errorf("sin registro ASN")
		}
		return contracts.ASNInfo{Error: err.Error()}
	}
	return parseASNRecord(records[0])
}

func parseASNRecord(record string) contracts.ASNInfo {
	parts := strings.Split(record, "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(strings.Trim(parts[index], `"`))
	}
	info := contracts.ASNInfo{}
	if len(parts) > 0 {
		info.ASN = parts[0]
	}
	if len(parts) > 1 {
		info.Prefix = parts[1]
	}
	if len(parts) > 2 {
		info.Country = parts[2]
	}
	if len(parts) > 3 {
		info.Registry = parts[3]
	}
	if len(parts) > 4 {
		info.Allocated = parts[4]
	}
	return info
}

type rdapResponse struct {
	Name         string `json:"name"`
	Handle       string `json:"handle"`
	Country      string `json:"country"`
	StartAddress string `json:"startAddress"`
	EndAddress   string `json:"endAddress"`
	Type         string `json:"type"`
}

func lookupRDAP(ctx context.Context, ip net.IP) contracts.RDAPInfo {
	rdapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url := "https://rdap.org/ip/" + ip.String()
	request, err := http.NewRequestWithContext(rdapCtx, http.MethodGet, url, nil)
	if err != nil {
		return contracts.RDAPInfo{Error: err.Error()}
	}
	request.Header.Set("User-Agent", "ssh05/1")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return contracts.RDAPInfo{Error: err.Error()}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return contracts.RDAPInfo{Error: fmt.Sprintf("RDAP HTTP %d", response.StatusCode)}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return contracts.RDAPInfo{Error: err.Error()}
	}
	var parsed rdapResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return contracts.RDAPInfo{Error: err.Error()}
	}
	return contracts.RDAPInfo{
		Name: parsed.Name, Handle: parsed.Handle, Country: parsed.Country,
		StartAddress: parsed.StartAddress, EndAddress: parsed.EndAddress, Type: parsed.Type,
	}
}
