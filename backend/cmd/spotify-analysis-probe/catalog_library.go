//go:build spotify_research

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	"io"
	"net/http"
	"time"
)

type libraryPageReport struct {
	Kind      string `json:"kind"`
	Offset    int    `json:"offset"`
	Limit     int    `json:"limit"`
	Available int    `json:"availableItems"`
	Total     int    `json:"total"`
	Next      bool   `json:"hasNext"`
}

func runCatalogLibraryResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer) error {
	return runCatalogLibraryTraversal(ctx, cookie, contract, renew, opts, httpClient, out, false)
}
func runCatalogLibraryTraversal(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer, full bool) error {
	timeout := 90 * time.Second
	if full {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
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
	var shape map[string]int
	transport := http.DefaultTransport
	if httpClient != nil && httpClient.Transport != nil {
		transport = httpClient.Transport
	}
	client := catalog.New(catalog.Options{Client: &http.Client{Transport: libraryShapeTransport{base: playlistShapeTransport{base: albumSchemaTransport{base: transport, schema: &schema}, report: &shape}, report: &shape}}})
	defer client.Close()
	reports := []libraryPageReport{}
	for _, kind := range []string{"album", "playlist"} {
		queries := []catalog.LibraryQuery{{Kind: kind, Limit: 1, Offset: 0}, {Kind: kind, Limit: 1, Offset: 1}, {Kind: kind, Limit: 20, Offset: 0}}
		if full {
			queries = []catalog.LibraryQuery{{Kind: kind, Limit: 50, Offset: 0}}
		}
		count, total := 0, -1
		for position := 0; position < len(queries); position++ {
			query := queries[position]
			result, err := client.Library(ctx, token, query)
			if err != nil {
				_ = json.NewEncoder(out).Encode(map[string]any{"librarySchema": schema, "shape": shape, "kind": kind, "offset": query.Offset})
				return &probeFailure{stage: "catalog_library", authenticated: true, renewed: renew, cause: err}
			}
			report := libraryPageReport{Kind: kind, Offset: query.Offset, Limit: query.Limit}
			switch page := result.(type) {
			case catalog.Page[catalog.SavedAlbum]:
				report.Available = availableSearchItems(page.Items)
				report.Total = page.Total
				report.Next = page.Next != nil
			case catalog.Page[catalog.Playlist]:
				report.Available = availableSearchItems(page.Items)
				report.Total = page.Total
				report.Next = page.Next != nil
			default:
				return catalog.ErrSchema
			}
			reports = append(reports, report)
			if full {
				if total < 0 {
					total = report.Total
				}
				if total != report.Total {
					return fmt.Errorf("library changed during traversal")
				}
				count += min(query.Limit, max(report.Total-query.Offset, 0))
				if report.Next {
					if len(queries) >= 200 {
						return fmt.Errorf("library traversal bound reached")
					}
					queries = append(queries, catalog.LibraryQuery{Kind: kind, Limit: 50, Offset: query.Offset + query.Limit})
				} else if count != total {
					return fmt.Errorf("library traversal incomplete")
				}
			}
			if !report.Next {
				break
			}
		}
	}
	return json.NewEncoder(out).Encode(struct {
		Authenticated bool                `json:"authenticationVerified"`
		Renewed       bool                `json:"renewalVerified"`
		Normalized    bool                `json:"libraryNormalized"`
		FullTraversal bool                `json:"fullTraversal"`
		Pages         []libraryPageReport `json:"pages"`
	}{true, renew, true, full, reports})
}
