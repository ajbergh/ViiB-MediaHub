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

func TestGDMServerDiscoveryUsesMulticast(t *testing.T) {
	ip := net.ParseIP(gdmMulticastAddress)
	if ip == nil || !ip.IsMulticast() {
		t.Fatalf("expected GDM server endpoint to be multicast, got %q", gdmMulticastAddress)
	}
	if gdmMulticastAddress != "239.0.0.250" || GDMPort != 32414 {
		t.Fatalf("unexpected Plex GDM server endpoint %s:%d", gdmMulticastAddress, GDMPort)
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
