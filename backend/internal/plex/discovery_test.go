package plex

import (
	"net"
	"testing"
)

func TestParseGDMResponse(t *testing.T) {
	payload := []byte("HTTP/1.0 200 OK\r\nName: Studio Plex\r\nPort: 32400\r\nResource-Identifier: machine-1\r\nVersion: 1.43.2\r\nMalformed line\r\n\r\n")
	server, err := ParseGDMResponse(payload, net.ParseIP("192.168.1.25"))
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if server.Name != "Studio Plex" || server.Port != 32400 || server.MachineIdentifier != "machine-1" || server.Version != "1.43.2" {
		t.Fatalf("unexpected server: %#v", server)
	}
	if server.URL != "http://192.168.1.25:32400" {
		t.Fatalf("unexpected URL: %s", server.URL)
	}
}

func TestParseGDMResponseRejectsMalformedStatus(t *testing.T) {
	if _, err := ParseGDMResponse([]byte("garbage\r\nName: bad\r\n"), net.ParseIP("192.168.1.2")); err == nil {
		t.Fatal("expected malformed response error")
	}
}

func TestDeduplicateServers(t *testing.T) {
	input := []Server{
		{MachineIdentifier: "same", Host: "192.168.1.2", Port: 32400},
		{MachineIdentifier: "same", Host: "192.168.1.3", Port: 32400},
		{Host: "192.168.1.4", Port: 32400},
		{Host: "192.168.1.4", Port: 32400},
	}
	result := DeduplicateServers(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 unique servers, got %d: %#v", len(result), result)
	}
}

func TestGDMServerDiscoveryUsesMulticastAndDirectedBroadcast(t *testing.T) {
	ip := net.ParseIP(gdmMulticastAddress)
	if ip == nil || !ip.IsMulticast() {
		t.Fatalf("expected GDM server endpoint to be multicast, got %q", gdmMulticastAddress)
	}
	if gdmMulticastAddress != "239.0.0.250" || GDMPort != 32414 {
		t.Fatalf("unexpected Plex GDM server endpoint %s:%d", gdmMulticastAddress, GDMPort)
	}

	binding := gdmInterfaceBinding{
		IP: net.ParseIP("192.168.50.23").To4(),
		Network: &net.IPNet{IP: net.ParseIP("192.168.50.23").To4(), Mask: net.CIDRMask(24, 32)},
	}
	targets := gdmSearchTargets(binding)
	if len(targets) != 2 {
		t.Fatalf("expected multicast and directed broadcast targets, got %#v", targets)
	}
	if got := targets[1].IP.String(); got != "192.168.50.255" {
		t.Fatalf("directed broadcast=%s want 192.168.50.255", got)
	}
}

func TestDirectedBroadcastHonorsNon24Mask(t *testing.T) {
	network := &net.IPNet{IP: net.ParseIP("192.168.10.70").To4(), Mask: net.CIDRMask(26, 32)}
	if got := directedBroadcast(network).String(); got != "192.168.10.127" {
		t.Fatalf("broadcast=%s want 192.168.10.127", got)
	}
}

func TestSubnetProbeCandidatesStayInsideBoundedNetwork(t *testing.T) {
	binding := gdmInterfaceBinding{
		IP: net.ParseIP("192.168.10.70").To4(),
		Network: &net.IPNet{IP: net.ParseIP("192.168.10.70").To4(), Mask: net.CIDRMask(26, 32)},
	}
	candidates := subnetProbeCandidates([]gdmInterfaceBinding{binding}, 512)
	seen := map[string]bool{}
	for _, candidate := range candidates {
		seen[candidate.String()] = true
		if candidate.IsLoopback() {
			continue
		}
		last := candidate.To4()[3]
		if last < 65 || last > 126 {
			t.Fatalf("candidate escaped /26: %s", candidate)
		}
	}
	if !seen["127.0.0.1"] || !seen["192.168.10.70"] || !seen["192.168.10.65"] || !seen["192.168.10.126"] {
		t.Fatalf("missing expected fallback candidates: %#v", seen)
	}
	if seen["192.168.10.64"] || seen["192.168.10.127"] {
		t.Fatalf("network/broadcast address should not be probed: %#v", seen)
	}
}

func TestBroadPrivateSubnetProbeIsLimitedToLocal24(t *testing.T) {
	binding := gdmInterfaceBinding{
		IP: net.ParseIP("10.44.12.99").To4(),
		Network: &net.IPNet{IP: net.ParseIP("10.44.12.99").To4(), Mask: net.CIDRMask(16, 32)},
	}
	candidates := subnetProbeCandidates([]gdmInterfaceBinding{binding}, 512)
	for _, candidate := range candidates {
		if candidate.IsLoopback() {
			continue
		}
		ipv4 := candidate.To4()
		if ipv4[0] != 10 || ipv4[1] != 44 || ipv4[2] != 12 {
			t.Fatalf("broad-subnet fallback escaped local /24: %s", candidate)
		}
	}
}

func TestStandardPortFallbackDoesNotSweepPublicNetworks(t *testing.T) {
	binding := gdmInterfaceBinding{
		IP: net.ParseIP("203.0.113.20").To4(),
		Network: &net.IPNet{IP: net.ParseIP("203.0.113.20").To4(), Mask: net.CIDRMask(24, 32)},
	}
	candidates := subnetProbeCandidates([]gdmInterfaceBinding{binding}, 512)
	if len(candidates) != 2 || candidates[0].String() != "127.0.0.1" || candidates[1].String() != "203.0.113.20" {
		t.Fatalf("public network should only probe loopback/current host, got %#v", candidates)
	}
}

func TestInterfaceIPv4AddressesAreIPv4Values(t *testing.T) {
	for _, address := range interfaceIPv4Addresses() {
		if address.To4() == nil {
			t.Fatalf("expected discovery helper to return IPv4 values, got %s", address)
		}
	}
}

func TestUsableGDMIPv4DoesNotRequirePrivateAddressing(t *testing.T) {
	accepted := []string{
		"192.168.1.20", // RFC1918 LAN
		"169.254.10.20", // IPv4 link-local
		"203.0.113.20", // non-RFC1918 address used here to prove no IsPrivate gate
	}
	for _, value := range accepted {
		if got := usableGDMIPv4(net.ParseIP(value)); got == nil {
			t.Fatalf("expected %s to be usable for interface-scoped GDM multicast", value)
		}
	}

	rejected := []string{"127.0.0.1", "0.0.0.0", "239.0.0.250", "2001:db8::1"}
	for _, value := range rejected {
		if got := usableGDMIPv4(net.ParseIP(value)); got != nil {
			t.Fatalf("expected %s to be rejected, got %s", value, got)
		}
	}
}
