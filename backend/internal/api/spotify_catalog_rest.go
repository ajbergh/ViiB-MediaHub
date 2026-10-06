package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	"github.com/ajbergh/viib-mediahub/internal/spotify/metadata"
)

type catalogReplayError struct{ err error }

func (r *catalogReplayError) Read([]byte) (int, error) {
	if r.err == nil {
		return 0, io.EOF
	}
	err := r.err
	r.err = nil
	return 0, err
}

type catalogReplayBody struct {
	io.Reader
	io.Closer
}

// Capture a bounded prefix without changing the response delivered to consumers.
// Oversized or incompatible responses remain usable but are not persisted.
func (a *API) captureOAuthCatalogResponse(ctx context.Context, runtime *spotifyAuthRuntime, target *url.URL, response *http.Response) {
	if target.Scheme != "https" || target.Host != "api.spotify.com" || target.User != nil {
		return
	}
	parts := strings.Split(strings.Trim(target.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "v1" {
		return
	}
	switch parts[1] {
	case "tracks", "albums", "artists", "playlists", "search":
	case "me":
		if len(parts) != 3 || (parts[2] != "tracks" && parts[2] != "albums" && parts[2] != "playlists") {
			return
		}
	default:
		return
	}
	original := response.Body
	raw, err := io.ReadAll(io.LimitReader(original, metadata.CatalogLimit+1))
	var replay io.Reader = io.MultiReader(bytes.NewReader(raw), original)
	if err != nil {
		replay = io.MultiReader(bytes.NewReader(raw), &catalogReplayError{err: err}, original)
	}
	response.Body = &catalogReplayBody{Reader: replay, Closer: original}
	if err != nil || len(raw) > metadata.CatalogLimit || ctx.Err() != nil {
		return
	}
	entities, err := catalog.CaptureREST(target, raw)
	if err != nil {
		logger.Scan("spotify_catalog_snapshot status=failed reason=invalid_rest_domain")
		return
	}
	runtime.persistCatalogDomain(ctx, entities)
}
