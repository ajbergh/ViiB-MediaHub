//go:build spotify_research

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

type catalogSearchPageReport struct {
	Offset      int `json:"offset"`
	Tracks      int `json:"tracks"`
	Albums      int `json:"albums"`
	Artists     int `json:"artists"`
	Playlists   int `json:"playlists"`
	NextBuckets int `json:"nextBuckets"`
}

func runCatalogSearchResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer) error {
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
	client := catalog.New(catalog.Options{Client: httpClient})
	defer client.Close()
	pages := []catalogSearchPageReport{}
	for _, offset := range []int{0, 1} {
		result, err := client.Search(ctx, token, catalog.SearchQuery{Term: "music", Types: []string{"track", "album", "artist", "playlist"}, Limit: 1, Offset: offset})
		if err != nil {
			return &probeFailure{stage: "catalog_search", authenticated: true, renewed: renew, cause: err}
		}
		page := catalogSearchPageReport{Offset: offset, Tracks: availableSearchItems(result.Tracks.Items), Albums: availableSearchItems(result.Albums.Items), Artists: availableSearchItems(result.Artists.Items), Playlists: availableSearchItems(result.Playlists.Items)}
		for _, next := range []*string{result.Tracks.Next, result.Albums.Next, result.Artists.Next, result.Playlists.Next} {
			if next != nil {
				page.NextBuckets++
			}
		}
		pages = append(pages, page)
	}
	return json.NewEncoder(out).Encode(struct {
		AuthenticationVerified bool                      `json:"authenticationVerified"`
		RenewalVerified        bool                      `json:"renewalVerified"`
		SearchNormalized       bool                      `json:"searchNormalized"`
		Pages                  []catalogSearchPageReport `json:"pages"`
	}{true, renew, true, pages})
}

func availableSearchItems[T any](items []*T) int {
	count := 0
	for _, item := range items {
		if item != nil {
			count++
		}
	}
	return count
}
