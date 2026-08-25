package plex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	gdmSearchRequest        = "M-SEARCH * HTTP/1.0\r\n\r\n"
	gdmMulticastAddress    = "239.0.0.250"
	gdmDefaultTimeout      = 3500 * time.Millisecond
	gdmResponseWindow      = 1500 * time.Millisecond
	gdmRepeatDelay         = 120 * time.Millisecond
	plexProbeDialTimeout   = 140 * time.Millisecond
	plexProbeRequestTimeout = 300 * time.Millisecond
	plexProbeMaxHosts      = 512
	plexProbeConcurrency   = 64
)

// ParseGDMResponse parses Plex GDM's HTTP-header-like UDP response. Malformed
// individual lines are ignored so one noisy device cannot abort discovery.
func ParseGDMResponse(payload []byte, remoteIP net.IP) (Server, error) {
	server := Server{Scheme: "http", Port: DefaultPort}
	scanner := bufio.NewScanner(strings.NewReader(string(payload)))
	if !scanner.Scan() {
		return server, errors.New("empty GDM response")
	}
	first := strings.TrimSpace(scanner.Text())
	if !strings.Contains(strings.ToUpper(first), "200 OK") {
		return server, fmt.Errorf("unexpected GDM response status: %q", first)
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch key {
		case "name":
			server.Name = value
		case "port":
			if port, err := strconv.Atoi(value); err == nil && port > 0 && port <= 65535 {
				server.Port = port
			}
		case "resource-identifier":
			server.MachineIdentifier = value
		case "version":
			server.Version = value
		}
	}
	if remoteIP == nil || remoteIP.IsUnspecified() {
		return server, errors.New("GDM response has no usable source address")
	}
	server.Host = remoteIP.String()
	server.URL = fmt.Sprintf("http://%s", net.JoinHostPort(server.Host, strconv.Itoa(server.Port)))
	if server.Name == "" {
		server.Name = server.Host
	}
	return server, nil
}

func usableGDMIPv4(ip net.IP) net.IP {
	ipv4 := ip.To4()
	if ipv4 == nil || ipv4.IsUnspecified() || ipv4.IsLoopback() || ipv4.IsMulticast() {
		return nil
	}
	return ipv4
}

type gdmInterfaceBinding struct {
	IP      net.IP
	Network *net.IPNet
}

// interfaceIPv4Bindings returns every usable IPv4 address and its subnet on an
// active non-loopback interface. GDM is link-local discovery, so do not assume
// RFC1918 addressing: corporate, VPN, link-local, and unusual lab networks can
// all have valid interfaces that are not net.IP.IsPrivate().
func interfaceIPv4Bindings() []gdmInterfaceBinding {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []gdmInterfaceBinding{{IP: net.IPv4zero}}
	}
	seen := map[string]struct{}{}
	result := make([]gdmInterfaceBinding, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := usableGDMIPv4(ipNet.IP)
			if ip == nil {
				continue
			}
			key := ip.String()
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			copyIP := append(net.IP(nil), ip...)
			copyNet := &net.IPNet{IP: append(net.IP(nil), ipNet.IP.To4()...), Mask: append(net.IPMask(nil), ipNet.Mask...)}
			result = append(result, gdmInterfaceBinding{IP: copyIP, Network: copyNet})
		}
	}
	if len(result) == 0 {
		// Let the OS select the egress interface as a last-resort GDM fallback.
		return []gdmInterfaceBinding{{IP: net.IPv4zero}}
	}
	return result
}

// interfaceIPv4Addresses is retained as a small compatibility/test helper.
func interfaceIPv4Addresses() []net.IP {
	bindings := interfaceIPv4Bindings()
	result := make([]net.IP, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, append(net.IP(nil), binding.IP...))
	}
	return result
}

func directedBroadcast(network *net.IPNet) net.IP {
	if network == nil {
		return nil
	}
	ip := network.IP.To4()
	if ip == nil || len(network.Mask) != net.IPv4len {
		return nil
	}
	broadcast := make(net.IP, net.IPv4len)
	for i := 0; i < net.IPv4len; i++ {
		broadcast[i] = ip[i] | ^network.Mask[i]
	}
	return broadcast
}

func gdmSearchTargets(binding gdmInterfaceBinding) []*net.UDPAddr {
	targets := []*net.UDPAddr{{IP: net.ParseIP(gdmMulticastAddress).To4(), Port: GDMPort}}
	if broadcast := directedBroadcast(binding.Network); broadcast != nil && !broadcast.Equal(binding.IP) {
		targets = append(targets, &net.UDPAddr{IP: broadcast, Port: GDMPort})
	}
	return targets
}

// DeduplicateServers removes duplicate GDM/probe responses. A
// machineIdentifier is authoritative when present; otherwise host:port is used.
func DeduplicateServers(in []Server) []Server {
	seen := make(map[string]struct{}, len(in))
	out := make([]Server, 0, len(in))
	for _, server := range in {
		key := strings.TrimSpace(server.MachineIdentifier)
		if key == "" {
			key = net.JoinHostPort(server.Host, strconv.Itoa(server.Port))
		}
		if key == ":0" || key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, server)
	}
	return out
}

type gdmInterfaceResult struct {
	servers []Server
	err     error
}

func discoverGDMOnBinding(ctx context.Context, binding gdmInterfaceBinding, deadline time.Time) ([]Server, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: binding.IP, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("bind Plex GDM discovery on %s: %w", binding.IP, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set Plex GDM discovery deadline: %w", err)
	}

	targets := gdmSearchTargets(binding)
	if len(targets) == 0 || targets[0].IP == nil {
		return nil, errors.New("invalid Plex GDM multicast address")
	}

	// A single multicast datagram is easy to lose and some Plex/server/network
	// combinations advertise via subnet broadcast rather than multicast. Send
	// two small search waves to both the documented multicast group and the
	// interface's directed broadcast address. UDPConn supports concurrent reads
	// and writes, but sending the second wave before reading also works because
	// kernel receive buffers retain any immediate response.
	successfulSends := 0
	var lastSendErr error
	for wave := 0; wave < 2; wave++ {
		for _, target := range targets {
			if _, err := conn.WriteToUDP([]byte(gdmSearchRequest), target); err != nil {
				lastSendErr = err
				continue
			}
			successfulSends++
		}
		if wave == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(gdmRepeatDelay):
			}
		}
	}
	if successfulSends == 0 {
		return nil, fmt.Errorf("send Plex GDM discovery on %s: %w", binding.IP, lastSendErr)
	}

	servers := make([]Server, 0, 2)
	buffer := make([]byte, 64*1024)
	for {
		if ctx.Err() != nil {
			break
		}
		n, addr, readErr := conn.ReadFromUDP(buffer)
		if readErr != nil {
			var netErr net.Error
			if errors.As(readErr, &netErr) && netErr.Timeout() {
				break
			}
			if ctx.Err() != nil {
				break
			}
			// A malformed/noisy datagram must not abort discovery on this interface.
			continue
		}
		server, parseErr := ParseGDMResponse(buffer[:n], addr.IP)
		if parseErr != nil {
			continue
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func isSafeStandardPortProbeIP(ip net.IP) bool {
	ipv4 := ip.To4()
	return ipv4 != nil && (ipv4.IsPrivate() || ipv4.IsLinkLocalUnicast())
}

func subnetProbeCandidates(bindings []gdmInterfaceBinding, maximum int) []net.IP {
	if maximum <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]net.IP, 0, maximum)
	add := func(ip net.IP) {
		if len(result) >= maximum || ip == nil {
			return
		}
		ipv4 := ip.To4()
		if ipv4 == nil {
			return
		}
		key := ipv4.String()
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, append(net.IP(nil), ipv4...))
	}

	// Always probe the current machine. This fixes the common case where PMS is
	// local but multicast loopback/firewall behavior prevents its GDM reply.
	add(net.IPv4(127, 0, 0, 1))
	for _, binding := range bindings {
		if !binding.IP.IsUnspecified() {
			add(binding.IP)
		}
		if !isSafeStandardPortProbeIP(binding.IP) || binding.Network == nil {
			continue
		}
		maskSize, bits := binding.Network.Mask.Size()
		if bits != 32 || maskSize < 0 {
			continue
		}
		// Never sweep a broad corporate/VPN network. For masks broader than /24,
		// limit the fallback probe to the /24 containing this host. GDM remains the
		// primary discovery mechanism across the actual broadcast domain.
		mask := binding.Network.Mask
		if maskSize < 24 {
			mask = net.CIDRMask(24, 32)
		}
		networkIP := binding.IP.Mask(mask).To4()
		if networkIP == nil {
			continue
		}
		broadcast := directedBroadcast(&net.IPNet{IP: networkIP, Mask: mask})
		for host := 1; host < 255 && len(result) < maximum; host++ {
			candidate := net.IPv4(networkIP[0], networkIP[1], networkIP[2], byte(host)).To4()
			if candidate.Equal(binding.IP) || (broadcast != nil && candidate.Equal(broadcast)) {
				continue
			}
			add(candidate)
		}
	}
	return result
}

func probePlexStandardPort(ctx context.Context, ip net.IP) (Server, bool) {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   plexProbeDialTimeout,
			KeepAlive: -1,
		}).DialContext,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: plexProbeRequestTimeout,
	}
	httpClient := &http.Client{Transport: transport, Timeout: plexProbeRequestTimeout}
	baseURL := "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(DefaultPort))
	client, err := NewClientWithHTTP(baseURL, "", "viib-discovery", httpClient)
	if err != nil {
		return Server{}, false
	}
	server, err := client.ValidateServer(ctx)
	if err != nil || strings.TrimSpace(server.MachineIdentifier) == "" {
		return Server{}, false
	}
	return server, true
}

func discoverStandardPort(ctx context.Context, bindings []gdmInterfaceBinding) []Server {
	candidates := subnetProbeCandidates(bindings, plexProbeMaxHosts)
	if len(candidates) == 0 {
		return nil
	}
	workerCount := plexProbeConcurrency
	if len(candidates) < workerCount {
		workerCount = len(candidates)
	}
	jobs := make(chan net.IP)
	found := make(chan Server, len(candidates))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer workers.Done()
			for ip := range jobs {
				if ctx.Err() != nil {
					return
				}
				if server, ok := probePlexStandardPort(ctx, ip); ok {
					found <- server
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, ip := range candidates {
			select {
			case <-ctx.Done():
				return
			case jobs <- ip:
			}
		}
	}()
	go func() {
		workers.Wait()
		close(found)
	}()

	servers := make([]Server, 0, 2)
	for server := range found {
		servers = append(servers, server)
	}
	return DeduplicateServers(servers)
}

// Discover performs Plex server discovery in two bounded phases. It first uses
// Plex GDM on every usable IPv4 interface, sending both multicast and directed
// broadcast search packets. If GDM returns nothing, it performs a conservative
// standard-port /identity probe on the local machine and at most two local /24
// neighborhoods. This second phase makes first-run discovery work when Windows
// firewall/multicast policy drops GDM while normal TCP access to PMS is allowed.
func Discover(ctx context.Context, timeout time.Duration) ([]Server, error) {
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = gdmDefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bindings := interfaceIPv4Bindings()
	gdmWindow := gdmResponseWindow
	if timeout < 2*gdmResponseWindow {
		gdmWindow = timeout / 2
	}
	if gdmWindow < 500*time.Millisecond {
		gdmWindow = timeout
	}
	deadline := time.Now().Add(gdmWindow)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}

	results := make(chan gdmInterfaceResult, len(bindings))
	for _, sourceBinding := range bindings {
		binding := gdmInterfaceBinding{IP: append(net.IP(nil), sourceBinding.IP...), Network: sourceBinding.Network}
		go func() {
			servers, err := discoverGDMOnBinding(ctx, binding, deadline)
			results <- gdmInterfaceResult{servers: servers, err: err}
		}()
	}

	servers := make([]Server, 0, 4)
	var lastErr error
	successfulInterfaces := 0
	for range bindings {
		result := <-results
		if result.err != nil {
			lastErr = result.err
			continue
		}
		successfulInterfaces++
		servers = append(servers, result.servers...)
	}
	servers = DeduplicateServers(servers)
	if len(servers) > 0 {
		return servers, nil
	}

	// GDM can be disabled in PMS or filtered by host/network firewalls. A
	// user-triggered, tightly bounded TCP fallback on the standard PMS port is
	// substantially more reliable on Windows and typical home LANs.
	if ctx.Err() == nil {
		servers = discoverStandardPort(ctx, bindings)
		if len(servers) > 0 {
			return servers, nil
		}
	}

	if successfulInterfaces == 0 && lastErr != nil {
		return []Server{}, fmt.Errorf("plex discovery failed on all IPv4 interfaces: %w", lastErr)
	}
	return []Server{}, nil
}
