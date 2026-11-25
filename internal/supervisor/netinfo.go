package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/lordpietre/ssh05/contracts"
	"github.com/lordpietre/ssh05/internal/audit"
	"github.com/lordpietre/ssh05/internal/ipc"
	nmapbin "github.com/lordpietre/ssh05/internal/nmap"
)

type nmapRun struct {
	Hosts []nmapXMLHost `xml:"host"`
}

type nmapXMLHost struct {
	Status struct {
		State string `xml:"state,attr"`
	} `xml:"status"`
	Addresses []struct {
		Addr     string `xml:"addr,attr"`
		AddrType string `xml:"addrtype,attr"`
		Vendor   string `xml:"vendor,attr"`
	} `xml:"address"`
	Hostnames []struct {
		Name string `xml:"name,attr"`
		Type string `xml:"type,attr"`
	} `xml:"hostnames>hostname"`
	Ports []struct {
		Protocol string `xml:"protocol,attr"`
		PortID   int    `xml:"portid,attr"`
		State    struct {
			State string `xml:"state,attr"`
		} `xml:"state"`
		Service struct {
			Name      string   `xml:"name,attr"`
			Product   string   `xml:"product,attr"`
			Version   string   `xml:"version,attr"`
			ExtraInfo string   `xml:"extrainfo,attr"`
			CPE       []string `xml:"cpe"`
		} `xml:"service"`
	} `xml:"ports>port"`
	OS struct {
		Matches []struct {
			Name     string `xml:"name,attr"`
			Accuracy int    `xml:"accuracy,attr"`
			Classes  []struct {
				Type     string `xml:"type,attr"`
				Vendor   string `xml:"vendor,attr"`
				OSFamily string `xml:"osfamily,attr"`
				OSGen    string `xml:"osgen,attr"`
				Accuracy int    `xml:"accuracy,attr"`
			} `xml:"osclass"`
		} `xml:"osmatch"`
	} `xml:"os"`
	Uptime struct {
		Seconds  int64  `xml:"seconds,attr"`
		LastBoot string `xml:"lastboot,attr"`
	} `xml:"uptime"`
	Distance struct {
		Value int `xml:"value,attr"`
	} `xml:"distance"`
	Trace struct {
		Hops []struct {
			IPAddr string `xml:"ipaddr,attr"`
			TTL    int    `xml:"ttl,attr"`
		} `xml:"hop"`
	} `xml:"trace"`
	Scripts []struct {
		ID     string `xml:"id,attr"`
		Output string `xml:"output,attr"`
	} `xml:"hostscript>script"`
}

func (s *Service) nmapScan(ctx context.Context, req contracts.RPCRequest) (json.RawMessage, *contracts.RPCError) {
	params, rpcError := decodeParams[contracts.NmapParams](req)
	if rpcError != nil {
		return nil, rpcError
	}
	if !params.Confirm || !params.Authorized {
		return nil, &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: "nmap requires explicit confirmation and authorization"}
	}
	binary, dataDir, err := s.nmapRuntime()
	if err != nil {
		return nil, internalError(err)
	}
	dataArgs := []string{}
	if dataDir != "" {
		dataArgs = append(dataArgs, "--datadir", dataDir)
	}
	networks, err := parseNmapNetworks(params.Network)
	if err != nil {
		return nil, &contracts.RPCError{Code: ipc.CodeInvalidRequest, Message: err.Error()}
	}
	if len(networks) == 0 {
		networks, err = localScanNetworks()
		if err != nil {
			return nil, internalError(err)
		}
	}

	result := contracts.NmapResult{
		Networks:    networks,
		Hosts:       []contracts.NmapHost{},
		Warnings:    []string{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	discoveryArgs := append(append([]string{}, dataArgs...), "-sn", "--unprivileged", "-T4", "-oX", "-", "--")
	discoveryArgs = append(discoveryArgs, networks...)
	discovery, err := s.runNmap(ctx, binary, 90*time.Second, discoveryArgs)
	if err != nil {
		s.log(audit.Event{Action: "diagnostics.nmap", Result: "error", ErrorCode: "nmap_failed", Detail: strings.Join(networks, ",")})
		return nil, internalError(err)
	}
	hosts, parseErr := parseNmapHosts(discovery)
	if parseErr != nil {
		s.log(audit.Event{Action: "diagnostics.nmap", Result: "error", ErrorCode: "nmap_xml", Detail: strings.Join(networks, ",")})
		return nil, internalError(parseErr)
	}
	result.Hosts = hosts

	canRoot := os.Geteuid() == 0
	useSudo := false
	if !canRoot && params.Sudo && (params.OS || params.Trace) {
		if sudoAvailable(ctx) {
			useSudo = true
		} else {
			result.Warnings = append(result.Warnings, "sudo -n no está disponible sin contraseña; SO y trazado se omiten.")
		}
	}
	rootAllowed := canRoot || useSudo
	if params.OS && !rootAllowed {
		result.Warnings = append(result.Warnings, "La detección de SO requiere root; se ha omitido -O. Activa «sudo» o ejecuta ssh05d como root.")
	}
	if params.Trace && !rootAllowed {
		result.Warnings = append(result.Warnings, "El trazado de ruta requiere root; se ha omitido --traceroute.")
	}
	detailRequested := params.Services || params.Scripts || params.AllPorts || (params.OS && rootAllowed) || (params.Trace && rootAllowed)
	if detailRequested && len(result.Hosts) > 0 {
		addresses := make([]string, 0, len(result.Hosts))
		for _, host := range result.Hosts {
			if host.Address != "" {
				addresses = append(addresses, host.Address)
			}
		}
		detailArgs := append(append([]string{}, dataArgs...), "-sT", "-n", "-T4", "--max-retries", "1")
		if params.Services {
			detailArgs = append(detailArgs, "-sV", "--version-light")
		}
		if params.Scripts {
			detailArgs = append(detailArgs, "-sC", "--script-timeout", "30s")
		}
		if params.Trace && rootAllowed {
			detailArgs = append(detailArgs, "--traceroute")
		}
		if params.AllPorts {
			detailArgs = append(detailArgs, "-p-")
		} else {
			detailArgs = append(detailArgs, "--top-ports", "200")
		}
		if params.OS && rootAllowed {
			detailArgs = append(detailArgs, "-O", "--osscan-guess")
		}
		detailArgs = append(detailArgs, "-oX", "-", "--")
		detailArgs = append(detailArgs, addresses...)
		detailTimeout := 180 * time.Second
		if params.AllPorts {
			detailTimeout = 600 * time.Second
		}
		var detail []byte
		var detailErr error
		if useSudo {
			detail, detailErr = s.runNmapCommand(ctx, "sudo", append([]string{"-n", binary}, detailArgs...), detailTimeout)
		} else {
			detail, detailErr = s.runNmap(ctx, binary, detailTimeout, detailArgs)
		}
		detailedHosts, detailParseErr := parseNmapHosts(detail)
		if len(detailedHosts) > 0 {
			result.Hosts = mergeNmapHosts(result.Hosts, detailedHosts)
		}
		if detailParseErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("XML detallado: %v", detailParseErr))
		}
		if detailErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("análisis detallado: %v", detailErr))
		}
	}
	enrichARP(result.Hosts)
	classifyNmapHosts(result.Hosts)

	sortNmapHosts(result.Hosts)
	s.log(audit.Event{Action: "diagnostics.nmap", Result: "ok", Detail: strings.Join(networks, ",")})
	return marshal(result)
}

func (s *Service) nmapRuntime() (string, string, error) {
	if path := strings.TrimSpace(s.nmapPath); path != "" && path != "embedded" {
		return path, "", nil
	}
	path, err := nmapbin.Binary()
	if err != nil {
		return "", "", fmt.Errorf("nmap embebido no disponible: %w; usa --nmap <ruta>", err)
	}
	dataDir, err := nmapbin.DataDir()
	if err != nil {
		return "", "", fmt.Errorf("nmap datos embebidos no disponibles: %w", err)
	}
	return path, dataDir, nil
}

func (s *Service) runNmap(ctx context.Context, binary string, timeout time.Duration, args []string) ([]byte, error) {
	return s.runNmapCommand(ctx, binary, args, timeout)
}

func (s *Service) runNmapCommand(ctx context.Context, command string, args []string, timeout time.Duration) ([]byte, error) {
	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(scanCtx, command, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return output, fmt.Errorf("nmap: %s", message)
	}
	return output, nil
}

func sudoAvailable(ctx context.Context) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return exec.CommandContext(checkCtx, "sudo", "-n", "true").Run() == nil
}

func parseNmapHosts(data []byte) ([]contracts.NmapHost, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var run nmapRun
	if err := xml.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("nmap XML: %w", err)
	}
	hosts := make([]contracts.NmapHost, 0, len(run.Hosts))
	for _, raw := range run.Hosts {
		if raw.Status.State != "" && raw.Status.State != "up" {
			continue
		}
		host := contracts.NmapHost{State: raw.Status.State}
		if host.State == "" {
			host.State = "up"
		}
		for _, address := range raw.Addresses {
			switch address.AddrType {
			case "ipv4", "ipv6":
				if host.Address == "" {
					host.Address = address.Addr
				}
			case "mac":
				host.MAC = address.Addr
				host.Vendor = address.Vendor
			}
		}
		for _, name := range raw.Hostnames {
			if name.Name == "" {
				continue
			}
			if host.Hostname == "" || name.Type == "PTR" {
				host.Hostname = name.Name
			}
		}
		host.UptimeSeconds = raw.Uptime.Seconds
		host.LastBoot = raw.Uptime.LastBoot
		host.Distance = raw.Distance.Value
		for _, hop := range raw.Trace.Hops {
			if hop.IPAddr != "" {
				host.Trace = append(host.Trace, hop.IPAddr)
			}
		}
		for _, match := range raw.OS.Matches {
			entry := contracts.NmapOSMatch{Name: match.Name, Accuracy: match.Accuracy}
			for _, class := range match.Classes {
				entry.Classes = append(entry.Classes, contracts.NmapOSClass{
					Type: class.Type, Vendor: class.Vendor, OSFamily: class.OSFamily,
					OSGen: class.OSGen, Accuracy: class.Accuracy,
				})
				if host.DeviceType == "" && class.Type != "" {
					host.DeviceType = class.Type
				}
			}
			host.OSMatches = append(host.OSMatches, entry)
		}
		for _, port := range raw.Ports {
			host.Ports = append(host.Ports, contracts.NmapPort{
				Port: port.PortID, Protocol: port.Protocol, State: port.State.State,
				Service: port.Service.Name, Product: port.Service.Product,
				Version: port.Service.Version, ExtraInfo: port.Service.ExtraInfo,
				CPE: port.Service.CPE,
			})
		}
		sort.Slice(host.Ports, func(a, b int) bool { return host.Ports[a].Port < host.Ports[b].Port })
		for _, script := range raw.Scripts {
			host.Scripts = append(host.Scripts, contracts.NmapScript{ID: script.ID, Output: script.Output})
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func enrichARP(hosts []contracts.NmapHost) {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return
	}
	byIP := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[3] == "00:00:00:00:00:00" {
			continue
		}
		byIP[fields[0]] = fields[3]
	}
	for index := range hosts {
		if hosts[index].MAC != "" {
			continue
		}
		mac := byIP[hosts[index].Address]
		if mac == "" {
			continue
		}
		hosts[index].MAC = mac
		if hosts[index].Vendor == "" {
			hosts[index].Vendor = nmapbin.Vendor(mac)
		}
	}
}

func classifyNmapHosts(hosts []contracts.NmapHost) {
	for index := range hosts {
		host := &hosts[index]
		if host.DeviceType != "" {
			host.DeviceType = translateDeviceType(host.DeviceType)
			continue
		}
		services := make(map[string]bool, len(host.Ports))
		for _, port := range host.Ports {
			services[strings.ToLower(port.Service)] = true
			if port.Port == 3389 {
				services["rdp"] = true
			}
			if port.Port == 631 || port.Port == 9100 {
				services["printer"] = true
			}
		}
		vendor := strings.ToLower(host.Vendor)
		switch {
		case services["microsoft-ds"] || services["netbios-ssn"] || services["msrpc"] || services["rdp"]:
			host.DeviceType = "PC Windows (estimado)"
		case services["printer"]:
			host.DeviceType = "impresora (estimado)"
		case strings.Contains(vendor, "arcadyan") ||
			strings.Contains(vendor, "sercomm") ||
			strings.Contains(vendor, "huawei") ||
			strings.Contains(vendor, "tp-link") ||
			strings.Contains(vendor, "cisco") ||
			strings.Contains(vendor, "mikrotik") ||
			strings.Contains(vendor, "ubiquiti") ||
			strings.Contains(vendor, "router"):
			host.DeviceType = "router / dispositivo de red (estimado)"
		case services["ssh"]:
			host.DeviceType = "equipo Unix (estimado)"
		case host.Vendor != "":
			host.DeviceType = "dispositivo de red (estimado)"
		}
	}
}

func translateDeviceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "general purpose":
		return "equipo general"
	case "router":
		return "router"
	case "switch":
		return "switch"
	case "firewall":
		return "firewall"
	case "printer":
		return "impresora"
	case "phone":
		return "teléfono"
	case "media device":
		return "dispositivo multimedia"
	case "game console":
		return "consola de juegos"
	case "webcam":
		return "cámara"
	case "storage-misc":
		return "almacenamiento"
	case "specialized":
		return "dispositivo especializado"
	default:
		return value
	}
}

func mergeNmapHosts(base, detail []contracts.NmapHost) []contracts.NmapHost {
	byAddress := make(map[string]contracts.NmapHost, len(base))
	for _, host := range base {
		byAddress[host.Address] = host
	}
	for _, extra := range detail {
		current, ok := byAddress[extra.Address]
		if !ok {
			byAddress[extra.Address] = extra
			continue
		}
		if extra.MAC != "" {
			current.MAC = extra.MAC
		}
		if extra.Vendor != "" {
			current.Vendor = extra.Vendor
		}
		if extra.Hostname != "" {
			current.Hostname = extra.Hostname
		}
		if extra.DeviceType != "" {
			current.DeviceType = extra.DeviceType
		}
		if len(extra.OSMatches) > 0 {
			current.OSMatches = extra.OSMatches
		}
		if len(extra.Ports) > 0 {
			current.Ports = extra.Ports
		}
		if len(extra.Scripts) > 0 {
			current.Scripts = extra.Scripts
		}
		if extra.UptimeSeconds > 0 {
			current.UptimeSeconds = extra.UptimeSeconds
			current.LastBoot = extra.LastBoot
		}
		if extra.Distance > 0 {
			current.Distance = extra.Distance
		}
		if len(extra.Trace) > 0 {
			current.Trace = extra.Trace
		}
		if extra.State != "" {
			current.State = extra.State
		}
		byAddress[extra.Address] = current
	}
	hosts := make([]contracts.NmapHost, 0, len(byAddress))
	for _, host := range byAddress {
		hosts = append(hosts, host)
	}
	return hosts
}

func sortNmapHosts(hosts []contracts.NmapHost) {
	sort.Slice(hosts, func(i, j int) bool {
		a, b := net.ParseIP(hosts[i].Address), net.ParseIP(hosts[j].Address)
		if a != nil && b != nil {
			return bytes.Compare(a.To4(), b.To4()) < 0
		}
		return hosts[i].Address < hosts[j].Address
	})
}

func parseNmapNetworks(requested string) ([]string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(requested, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	seen := make(map[string]struct{}, len(parts))
	networks := make([]string, 0, len(parts))
	for _, part := range parts {
		network, err := parsePrivateNetwork(part)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[network]; ok {
			continue
		}
		seen[network] = struct{}{}
		networks = append(networks, network)
	}
	return networks, nil
}

func parsePrivateNetwork(value string) (string, error) {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("red inválida %q", value)
	}
	if ip.To4() == nil {
		return "", fmt.Errorf("solo se admiten redes IPv4")
	}
	if !isPrivateNetworkIP(ip) {
		return "", fmt.Errorf("solo se admiten redes privadas o locales")
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones < 20 {
		return "", fmt.Errorf("red demasiado grande; máximo /20")
	}
	return network.String(), nil
}

func isPrivateNetworkIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

func localScanNetworks() ([]string, error) {
	primary := primaryLocalIP()
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("interfaces: %w", err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || !isPrivateNetworkIP(ip) {
				continue
			}
			if primary != nil && !ip.Equal(primary) {
				continue
			}
			if network := normalizeLocalNetwork(ip, ipnet); network != "" {
				return []string{network}, nil
			}
		}
	}
	// No primary route match: use the first private, non-virtual interface.
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || isVirtualInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || !isPrivateNetworkIP(ip) {
				continue
			}
			if network := normalizeLocalNetwork(ip, ipnet); network != "" {
				return []string{network}, nil
			}
		}
	}
	return nil, fmt.Errorf("no se detectó una red local privada")
}

func primaryLocalIP() net.IP {
	conn, err := net.DialTimeout("udp4", "1.1.1.1:53", time.Second)
	if err != nil {
		return nil
	}
	defer func() { _ = conn.Close() }()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.To4()
	}
	return nil
}

func isVirtualInterface(name string) bool {
	prefixes := []string{"lo", "docker", "br-", "veth", "virbr", "tun", "tap", "lxc", "podman", "cni", "vboxnet"}
	lower := strings.ToLower(name)
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func normalizeLocalNetwork(ip net.IP, network *net.IPNet) string {
	ones, bits := network.Mask.Size()
	if bits != 32 {
		return ""
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	mask := network.Mask
	if ones < 24 {
		mask = net.CIDRMask(24, 32)
	}
	return (&net.IPNet{IP: ip4.Mask(mask), Mask: mask}).String()
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
