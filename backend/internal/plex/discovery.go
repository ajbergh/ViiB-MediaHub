package plex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	gdmSearchRequest     = "M-SEARCH * HTTP/1.0\r\n\r\n"
	gdmMulticastAddress = "239.0.0.250"
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

// interfaceIPv4Addresses returns every usable IPv4 address on an active
// non-loopback interface. GDM is link-local multicast, so do not assume RFC1918
// addressing: corporate, VPN, link-local, and unusual lab networks can all have
// valid local interfaces that are not net.IP.IsPrivate().
func interfaceIPv4Addresses() []net.IP {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []net.IP{net.IPv4zero}
	}
	seen := map[string]struct{}{}
	result := make([]net.IP, 0, len(interfaces))
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
			result = append(result, copyIP)
		}
	}
	if len(result) == 0 {
		// Let the OS select the egress interface as a last-resort fallback.
		return []net.IP{net.IPv4zero}
	}
	return result
}

// DeduplicateServers removes duplicate GDM responses. A machineIdentifier is
// authoritative when present; otherwise host:port is used.
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

func discoverGDMOnAddress(ctx context.Context, localIP net.IP, deadline time.Time) ([]Server, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: localIP, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("bind Plex GDM discovery on %s: %w", localIP, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set Plex GDM discovery deadline: %w", err)
	}

	multicastIP := net.ParseIP(gdmMulticastAddress).To4()
	if multicastIP == nil {
		return nil, errors.New("invalid Plex GDM multicast address")
	}
	if _, err := conn.WriteToUDP([]byte(gdmSearchRequest), &net.UDPAddr{IP: multicastIP, Port: GDMPort}); err != nil {
		return nil, fmt.Errorf("send Plex GDM multicast on %s: %w", localIP, err)
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

// Discover performs Plex server GDM discovery using the standard local multicast
// group (239.0.0.250:32414). A request is sent from every usable IPv4 interface
// in parallel so multi-NIC/VPN systems are not limited to the OS default route.
// Discovery is bounded and only runs when explicitly invoked by the caller.
func Discover(ctx context.Context, timeout time.Duration) ([]Server, error) {
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 1500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}

	addresses := interfaceIPv4Addresses()
	results := make(chan gdmInterfaceResult, len(addresses))
	for _, localIP := range addresses {
		ip := append(net.IP(nil), localIP...)
		go func() {
			servers, err := discoverGDMOnAddress(ctx, ip, deadline)
			results <- gdmInterfaceResult{servers: servers, err: err}
		}()
	}

	servers := make([]Server, 0, 4)
	var lastErr error
	successfulInterfaces := 0
	for range addresses {
		result := <-results
		if result.err != nil {
			lastErr = result.err
			continue
		}
		successfulInterfaces++
		servers = append(servers, result.servers...)
	}

	servers = DeduplicateServers(servers)
	if successfulInterfaces == 0 && lastErr != nil {
		return []Server{}, fmt.Errorf("Plex GDM discovery failed on all IPv4 interfaces: %w", lastErr)
	}
	return servers, nil
}
