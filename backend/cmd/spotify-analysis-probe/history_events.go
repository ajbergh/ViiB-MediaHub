//go:build spotify_research

// Probes experimental listening-history event shapes without guessing fallback operations.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

// Experimental endpoint lead from Spotify Community topic 5181981 (2022-01-13).
// This is not a verified protocol contract or an alternative production history.
// One fixed read, no cursor guesses, retries, redirects or account values in output.
func probeHistoryEvents(ctx context.Context, client *http.Client, token auth.Token, clientToken, version string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://spclient.wg.spotify.com/listening-history/v2", nil)
	if err != nil {
		return nil, catalog.ErrSchema
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+token.Bearer())
	request.Header.Set("Client-Token", clientToken)
	request.Header.Set("App-Platform", "WebPlayer")
	request.Header.Set("Spotify-App-Version", version)
	request.Header.Set("Origin", "https://open.spotify.com")
	request.Header.Set("Referer", "https://open.spotify.com/")
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, auth.ErrTemporarilyUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, &catalog.HTTPError{Stage: "history_events", Status: response.StatusCode, RetryAfter: auth.RetryAfter(response.Header.Get("Retry-After"), time.Now())}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	defer clear(raw)
	if err != nil || len(raw) > 1<<20 {
		return nil, catalog.ErrSchema
	}
	var object any
	if json.Unmarshal(raw, &object) != nil {
		return nil, catalog.ErrSchema
	}
	report := map[string]any{"httpStatus": 200, "responseBytes": len(raw)}
	switch root := object.(type) {
	case map[string]any:
		report["rootType"] = "object"
		counts := map[string]int{}
		// Fixed names only: arbitrary map keys can contain private account values.
		for _, key := range []string{"items", "events", "tracks", "history", "entries"} {
			if rows, ok := root[key].([]any); ok {
				counts[key] = len(rows)
			}
		}
		report["containerCounts"] = counts
	case []any:
		report["rootType"] = "array"
		report["records"] = len(root)
	default:
		return nil, catalog.ErrSchema
	}
	return report, nil
}
