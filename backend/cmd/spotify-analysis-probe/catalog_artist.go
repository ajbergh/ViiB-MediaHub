//go:build spotify_research

// Probes fixed Web Player artist and top-track operations.
package main

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	"io"
	"net/http"
	"time"
)

func runCatalogArtistResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer) error {
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
	artist, tracks, err := client.ArtistOverview(ctx, token, "0TnOYISbd1XYRBk9myaseg", true)
	if err != nil {
		_ = json.NewEncoder(out).Encode(map[string]any{"artistSchema": schema})
		return &probeFailure{stage: "catalog_artist", authenticated: true, renewed: renew, cause: err}
	}
	return json.NewEncoder(out).Encode(struct {
		Authenticated bool `json:"authenticationVerified"`
		Renewed       bool `json:"renewalVerified"`
		Normalized    bool `json:"artistNormalized"`
		Images        int  `json:"imageCount"`
		Followers     bool `json:"followersAvailable"`
		Tracks        int  `json:"availableTopTracks"`
	}{true, renew, true, len(artist.Images), artist.Followers != nil, len(tracks.Tracks)})
}
