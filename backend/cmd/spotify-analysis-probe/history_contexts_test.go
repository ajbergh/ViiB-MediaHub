//go:build spotify_research

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHistoryContextProbeBoundedRedactedAndStops(t *testing.T) {
	for _, status := range []int{200, 401, 403, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "spclient.wg.spotify.com" || r.Method != "GET" || r.URL.Path != "/recently-played/v3/user/private-user/recently-played" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer fixture-secret-bearer" || r.Header.Get("Client-Token") != "private-client-token" {
					t.Fatal("unsafe history request")
				}
				q := r.URL.Query()
				if q.Get("limit") != "50" || q.Get("filter") != "default,track" || q.Get("format") != "json" || (calls == 1 && q.Get("offset") != "0") || (calls == 2 && q.Get("offset") != "50") {
					t.Fatal("unbounded request")
				}
				items := []any{}
				count := 50
				if calls == 2 {
					count = 1
				}
				for range count {
					items = append(items, map[string]any{"uri": "spotify:album:" + webTestID, "lastPlayedTrackUri": "spotify:track:" + webTestID, "lastPlayedTime": 1700000000000})
				}
				return fixtureWebResponse(r, status, map[string]any{"playContexts": items}), nil
			})}
			pages, err := probeHistoryContexts(context.Background(), client, webTestToken(), "private-client-token", "test-version", "private-user")
			if status != 200 {
				if err == nil || calls != 1 {
					t.Fatal("continued after rejection")
				}
				return
			}
			if err != nil || calls != 2 || len(pages) != 2 || pages[0].Records != 50 || pages[0].OtherContexts != 50 || pages[0].TrackContexts != 0 || pages[0].TrackReferences != 50 || pages[1].RepeatedContexts != 1 || pages[1].RepeatedTracks != 1 {
				t.Fatal("history diagnostics mismatch")
			}
			var output bytes.Buffer
			if json.NewEncoder(&output).Encode(pages) != nil {
				t.Fatal("encode failed")
			}
			for _, secret := range []string{"private-user", "private-client-token", "fixture-secret-bearer", webTestID, "1700000000000"} {
				if strings.Contains(output.String(), secret) {
					t.Fatal("scalar value leaked")
				}
			}
		})
	}
}
func TestHistoryContextProbeRejectsMalformedAndMissingIdentity(t *testing.T) {
	for _, body := range []string{"{}", "{\"playContexts\":null}", "{\"playContexts\":[{\"lastPlayedTime\":\"bad\"}]}", strings.Repeat("x", (1<<20)+1)} {
		calls := 0
		client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		if _, err := probeHistoryContexts(context.Background(), client, webTestToken(), "client", "version", "private-user"); err == nil || calls != 1 {
			t.Fatal("bad response accepted")
		}
	}
	for _, username := range []string{"", ".", "..", "user\nheader", strings.Repeat("x", 257)} {
		client := &http.Client{Transport: webTestTransport(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid identity reached network")
			return nil, nil
		})}
		if _, err := probeHistoryContexts(context.Background(), client, webTestToken(), "client", "version", username); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}
