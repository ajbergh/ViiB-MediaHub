//go:build spotify_research

// Probes fixed Web Player album operations and track pagination.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	"io"
	"net/http"
	"time"
)

func runCatalogAlbumResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	provider, err := auth.NewWebPlayerProvider(cookie, contract, opts)
	cookie = ""
	if err != nil {
		return err
	}
	defer provider.Disconnect()
	token, err := provider.Token(ctx)
	if err != nil {
		return &probeFailure{stage: "authentication", cause: err}
	}
	if renew {
		token, err = provider.Refresh(ctx, token)
		if err != nil {
			return &probeFailure{stage: "token_renewal", authenticated: true, cause: err}
		}
	}
	var schema []string
	transport := http.DefaultTransport
	if httpClient != nil && httpClient.Transport != nil {
		transport = httpClient.Transport
	}
	diagnosticClient := &http.Client{Transport: albumSchemaTransport{base: transport, schema: &schema}}
	client := catalog.New(catalog.Options{Client: diagnosticClient})
	defer client.Close()
	// Fixed public album, no arbitrary catalog operation or endpoint.
	counts := []int{}
	for _, offset := range []int{0, 1} {
		album, err := client.Album(ctx, token, catalog.AlbumQuery{ID: "4aawyAB9vmqN3uQ7FjRGTy", Limit: 1, Offset: offset})
		if err != nil {
			_ = json.NewEncoder(out).Encode(map[string]any{"albumSchema": schema})
			return &probeFailure{stage: "catalog_album", authenticated: true, renewed: renew, cause: err}
		}
		counts = append(counts, availableSearchItems(album.Tracks.Items))
	}
	return json.NewEncoder(out).Encode(struct {
		Authenticated bool  `json:"authenticationVerified"`
		Renewed       bool  `json:"renewalVerified"`
		Normalized    bool  `json:"albumNormalized"`
		Counts        []int `json:"availableTrackCounts"`
	}{true, renew, true, counts})
}

type albumSchemaTransport struct {
	base   http.RoundTripper
	schema *[]string
}

func (t albumSchemaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err != nil || r.URL.Host != "api-partner.spotify.com" || response.StatusCode != 200 {
		return response, err
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil {
		return response, err
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) == nil {
		*t.schema = pathfinderSchema(object["data"])
		if data, ok := object["data"].(map[string]any); ok {
			if me, ok := data["me"].(map[string]any); ok {
				if library, ok := me["libraryV3"].(map[string]any); ok {
					*t.schema = append(*t.schema, pathfinderSchema(library)...)
				}
			}
			if track, ok := data["trackUnion"].(map[string]any); ok {
				*t.schema = append(*t.schema, pathfinderSchema(track)...)
			}
			if playlist, ok := data["playlistV2"].(map[string]any); ok {
				*t.schema = append(*t.schema, pathfinderSchema(playlist["content"])...)
			}
			if artist, ok := data["artistUnion"].(map[string]any); ok {
				*t.schema = append(*t.schema, pathfinderSchema(map[string]any{"uri": artist["uri"], "id": artist["id"], "visuals": artist["visuals"], "__typename": artist["__typename"]})...)
				if discography, ok := artist["discography"].(map[string]any); ok {
					*t.schema = append(*t.schema, pathfinderSchema(discography["topTracks"])...)
				}
			}
			if album, ok := data["albumUnion"].(map[string]any); ok {
				*t.schema = append(*t.schema, pathfinderSchema(album["tracksV2"])...)
			}
		}
	}
	return response, nil
}
