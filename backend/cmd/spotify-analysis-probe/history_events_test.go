//go:build spotify_research

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHistoryEventsDiagnosticFixedBoundedRedacted(t *testing.T) {
	for _, status := range []int{200, 401, 403, 404, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.String() != "https://spclient.wg.spotify.com/listening-history/v2" ||
					r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer fixture-secret-bearer" ||
					r.Header.Get("Client-Token") != "fixture-client-token" {
					t.Fatal("unsafe event request")
				}
				body := `{"items":[{"uri":"private-track","played_at":"private-time"}],"private-user":"private-value"}`
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			report, err := probeHistoryEvents(context.Background(), client, webTestToken(), "fixture-client-token", "test-version")
			if calls != 1 {
				t.Fatal("unexpected retries")
			}
			if status != 200 {
				if err == nil {
					t.Fatal("rejection accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(report)
			for _, private := range []string{"private-track", "private-time", "private-user", "private-value", "fixture-secret-bearer", "fixture-client-token"} {
				if strings.Contains(string(raw), private) {
					t.Fatal("diagnostic leaked account data")
				}
			}
			if report["containerCounts"].(map[string]int)["items"] != 1 {
				t.Fatal("missing container count")
			}
		})
	}
}
func TestHistoryEventsDiagnosticRejectsMalformed(t *testing.T) {
	for _, body := range []string{"null", "true", "invalid", strings.Repeat("x", (1<<20)+1)} {
		client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		if _, err := probeHistoryEvents(context.Background(), client, webTestToken(), "fixture-client-token", "test-version"); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
}
