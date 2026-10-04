//go:build spotify_research

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

func runCatalogTrackResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer) error {
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
	client := catalog.New(catalog.Options{Client: &http.Client{Transport: albumSchemaTransport{base: transport, schema: &schema}}})
	defer client.Close()
	id := "5r9W9MJLvHk83fcZSPQ8SE"
	track, err := client.Track(ctx, token, id)
	if err != nil {
		_ = json.NewEncoder(out).Encode(map[string]any{"trackSchema": schema})
		return &probeFailure{stage: "catalog_track", authenticated: true, renewed: renew, cause: err}
	}
	batch, err := client.Tracks(ctx, token, []string{id, "11dFghVXANMlKmJXsNCbNl", id})
	if err != nil {
		_ = json.NewEncoder(out).Encode(map[string]any{"trackSchema": schema})
		return &probeFailure{stage: "catalog_track_batch", authenticated: true, renewed: renew, cause: err}
	}
	source, err := client.Playlist(ctx, token, catalog.PlaylistQuery{ID: "37i9dQZF1DXcBWIGoYBM5M", Limit: 50})
	if err != nil {
		return &probeFailure{stage: "catalog_track_source", authenticated: true, renewed: renew, cause: err}
	}
	ids := []string{}
	for _, item := range source.Tracks.Items {
		if item != nil && item.Track != nil {
			ids = append(ids, item.Track.ID)
		}
	}
	fullBatch, err := client.Tracks(ctx, token, ids)
	if err != nil {
		_ = json.NewEncoder(out).Encode(map[string]any{"trackSchema": schema})
		return &probeFailure{stage: "catalog_track_batch_50", authenticated: true, renewed: renew, cause: err}
	}
	return json.NewEncoder(out).Encode(struct {
		Authenticated        bool `json:"authenticationVerified"`
		Renewed              bool `json:"renewalVerified"`
		TrackNormalized      bool `json:"trackNormalized"`
		ArtworkAvailable     bool `json:"artworkAvailable"`
		ReleaseDateAvailable bool `json:"releaseDateAvailable"`
		BatchAvailable       int  `json:"batchAvailable"`
		FullBatchRequested   int  `json:"fullBatchRequested"`
		FullBatchAvailable   int  `json:"fullBatchAvailable"`
		DuplicatePreserved   bool `json:"duplicatePreserved"`
	}{true, renew, track != nil, len(track.Album.Images) > 0, track.Album.ReleaseDate != nil, availableSearchItems(batch.Tracks), len(ids), availableSearchItems(fullBatch.Tracks), len(batch.Tracks) == 3 && batch.Tracks[0] != nil && batch.Tracks[2] != nil && batch.Tracks[0].ID == batch.Tracks[2].ID})
}
