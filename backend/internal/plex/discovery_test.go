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
