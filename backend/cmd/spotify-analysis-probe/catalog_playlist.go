//go:build spotify_research

package main

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

func runCatalogPlaylistResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer) error {
	return runPlaylistResearch(ctx, cookie, contract, renew, opts, httpClient, out, false)
}

func runPlaylistResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer, saved bool) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	p, err := auth.NewWebPlayerProvider(cookie, contract, opts)
	cookie = ""
	if err != nil {
		return err
	}
	defer p.Disconnect()
	token, err := p.Token(ctx)
	if err != nil {
		return &probeFailure{stage: "authentication", cause: err}
	}
	if renew {
		token, err = p.Refresh(ctx, token)
		if err != nil {
			return &probeFailure{stage: "token_renewal", authenticated: true, cause: err}
		}
	}
	var schema []string
	transport := http.DefaultTransport
	if httpClient != nil && httpClient.Transport != nil {
		transport = httpClient.Transport
	}
	var shape map[string]int
	client := catalog.New(catalog.Options{Client: &http.Client{Transport: playlistShapeTransport{base: albumSchemaTransport{base: transport, schema: &schema}, report: &shape}}})
	defer client.Close()
	counts := []int{}
	queries := []catalog.PlaylistQuery{{ID: "37i9dQZF1DXcBWIGoYBM5M", Limit: 1, Offset: 0}, {ID: "37i9dQZF1DXcBWIGoYBM5M", Limit: 1, Offset: 1}, {ID: "37i9dQZF1DXcBWIGoYBM5M", Limit: 100, Offset: 0}}
	if saved {
		target := os.Getenv("VIIB_SPOTIFY_RESEARCH_PLAYLIST_ID")
		_ = os.Unsetenv("VIIB_SPOTIFY_RESEARCH_PLAYLIST_ID")
		if target != "" {
			query, err := catalog.ParsePlaylistQuery(target, url.Values{})
			if err != nil {
				return err
			}
			queries = []catalog.PlaylistQuery{query}
		} else {
			page, err := client.Library(ctx, token, catalog.LibraryQuery{Kind: "playlist", Limit: 20, Offset: 20})
			if err != nil {
				return &probeFailure{stage: "saved_playlist_selection", authenticated: true, renewed: renew, cause: err}
			}
			playlists, ok := page.(catalog.Page[catalog.Playlist])
			if !ok {
				return &probeFailure{stage: "saved_playlist_selection", authenticated: true, renewed: renew, cause: catalog.ErrSchema}
			}
			queries = nil
			for _, playlist := range playlists.Items {
				if playlist != nil && playlist.Tracks.Total > 1 && playlist.Tracks.Total <= 12 {
					queries = []catalog.PlaylistQuery{{ID: playlist.ID, Limit: 100, Offset: 0}}
					break
				}
			}
			if len(queries) == 0 {
				return &probeFailure{stage: "saved_playlist_selection", authenticated: true, renewed: renew, cause: catalog.ErrSchema}
			}
		}
	}
	for _, query := range queries {
		playlist, err := client.Playlist(ctx, token, query)
		if err != nil {
			_ = json.NewEncoder(out).Encode(map[string]any{"playlistSchema": schema, "playlistShape": shape})
			return &probeFailure{stage: "catalog_playlist", authenticated: true, renewed: renew, cause: err}
		}
		count := 0
		for _, item := range playlist.Tracks.Items {
			if item != nil && item.Track != nil {
				count++
			}
		}
		counts = append(counts, count)
	}
	return json.NewEncoder(out).Encode(struct {
		Authenticated bool  `json:"authenticationVerified"`
		Renewed       bool  `json:"renewalVerified"`
		Normalized    bool  `json:"playlistNormalized"`
		Counts        []int `json:"availableTrackCounts"`
	}{true, renew, true, counts})
}
