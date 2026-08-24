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

const gdmSearchRequest = "M-SEARCH * HTTP/1.0\r\n\r\n"

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

func interfaceBroadcasts() []net.IP {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []net.IP{net.IPv4bcast}
	}
	seen := map[string]struct{}{}
	result := make([]net.IP, 0, len(interfaces)+1)
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
			ip := ipNet.IP.To4()
			mask := ipNet.Mask
			if ip == nil || len(mask) != net.IPv4len || !ip.IsPrivate() {
				continue
			}
			broadcast := make(net.IP, net.IPv4len)
			for i := 0; i < net.IPv4len; i++ {
				broadcast[i] = ip[i] | ^mask[i]
			}
			if key := broadcast.String(); key != "" {
				if _, exists := seen[key]; !exists {
					seen[key] = struct{}{}
					result = append(result, broadcast)
				}
			}
		}
	}
	if _, exists := seen[net.IPv4bcast.String()]; !exists {
		result = append(result, net.IPv4bcast)
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

// Discover broadcasts a bounded Plex GDM search. It never runs continuously;
// callers choose the timeout and typically invoke it only from explicit UI actions.
func Discover(ctx context.Context, timeout time.Duration) ([]Server, error) {
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 1500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("start Plex discovery: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	request := []byte(gdmSearchRequest)
	var writeErr error
	for _, broadcast := range interfaceBroadcasts() {
		if _, err := conn.WriteToUDP(request, &net.UDPAddr{IP: broadcast, Port: GDMPort}); err != nil {
			writeErr = err
		}
	}

	servers := make([]Server, 0, 4)
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			break
		}
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				break
			}
			if ctx.Err() != nil {
				break
			}
			continue
		}
		server, err := ParseGDMResponse(buffer[:n], addr.IP)
		if err != nil {
			continue
		}
		servers = append(servers, server)
	}

	servers = DeduplicateServers(servers)
	if len(servers) == 0 && writeErr != nil {
		return []Server{}, fmt.Errorf("Plex discovery broadcast failed: %w", writeErr)
	}
	return servers, nil
}
