//go:build spotify_research

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBackendSessionMode(t *testing.T) {
	const id = "1OwGYy9S2R0lzhoGn7vv6S"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/spotify/analysis/"+id+"/refresh" || r.URL.Query().Get("endpoint") != "audio_features" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("CLI sent credentials")
		}
		fmt.Fprintf(w, `{"fromCache":true,"cache":{"observation":{"trackId":%q,"sourceEndpoint":"audio_features","bpm":131.331}}}`, id)
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--backend-url", server.URL, id}, &out, io.Discard, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"fromCache": true`) || !strings.Contains(out.String(), "131.331") {
		t.Fatalf("lost cache provenance: %s", out.String())
	}
}

func TestBackendURLBoundary(t *testing.T) {
	for _, base := range []string{"http://example.com", "http://127.0.0.1@evil.example", "http://127.0.0.1/api", "http://127.0.0.1?x=1"} {
		if err := fetchFromBackend(context.Background(), base, "1OwGYy9S2R0lzhoGn7vv6S", io.Discard); err == nil {
			t.Fatalf("accepted %s", base)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com", http.StatusFound)
	}))
	defer server.Close()
	if err := fetchFromBackend(context.Background(), server.URL, "1OwGYy9S2R0lzhoGn7vv6S", io.Discard); err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("redirect was not rejected: %v", err)
	}
}
