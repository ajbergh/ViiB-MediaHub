// Translates supported REST resource requests into fixed, account-scoped Web Player catalog operations.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

func (s *spotifyAuthRuntime) catalogClient(ctx context.Context, httpClient *http.Client) (*catalog.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || ctx.Value(spotifyAccountContextKey{}) != s.lifetime || s.closed || s.lifetime.Err() != nil || s.invalid.Load() {
		return nil, spotifyauth.ErrAuthenticationRequired
	}
	if s.catalog == nil {
		s.catalog = catalog.New(catalog.Options{
			Client:        httpClient,
			BeforeRequest: s.checkWebAPICooldown,
			OnRateLimit: func(value string) time.Duration {
				_ = s.recordWebAPICooldown(value)
				return s.webAPICooldownRemaining()
			},
		})
	}
	return s.catalog, nil
}

// Called while a bounded catalog transaction slot is held. Synthetic JSON retains the
// account lifetime until its response body closes.
func (a *API) cookieSpotifyCatalog(ctx context.Context, runtime *spotifyAuthRuntime, token spotifyauth.Token, target *url.URL) (*http.Response, error) {
	client, err := runtime.catalogClient(ctx, a.spotifyHTTPClient)
	if err != nil {
		return nil, err
	}
	var request func(spotifyauth.Token) (any, error)
	if target.Path == "/v1/me" {
		request = func(token spotifyauth.Token) (any, error) { return client.Profile(ctx, token) }
	} else if id, batch, ok := catalog.TrackPath(target.Path); ok {
		ids, err := catalog.ParseTrackQuery(id, batch, target.Query())
		if err != nil {
			return nil, err
		}
		request = func(token spotifyauth.Token) (any, error) {
			if batch {
				return client.Tracks(ctx, token, ids)
			}
			return client.Track(ctx, token, ids[0])
		}
	} else if id, tracks, ok := catalog.AlbumPath(target.Path); ok {
		query, err := catalog.ParseAlbumQuery(id, target.Query())
		if err != nil {
			return nil, err
		}
		request = func(token spotifyauth.Token) (any, error) {
			album, err := client.Album(ctx, token, query)
			if tracks {
				return album.Tracks, err
			}
			return album, err
		}
	} else if id, ok := catalog.ArtistAlbumsPath(target.Path); ok {
		if err := catalog.ValidateArtistAlbumsQuery(id, target.Query()); err != nil {
			return nil, err
		}
		request = func(token spotifyauth.Token) (any, error) { return client.ArtistAlbums(ctx, token, id) }
	} else if id, top, ok := catalog.ArtistPath(target.Path); ok {
		if err := catalog.ValidateArtistQuery(id, target.Query()); err != nil {
			return nil, err
		}
		request = func(token spotifyauth.Token) (any, error) {
			artist, tracks, err := client.ArtistOverview(ctx, token, id, top)
			if top {
				return tracks, err
			}
			return artist, err
		}
	} else if id, tracks, ok := catalog.PlaylistPath(target.Path); ok {
		query, err := catalog.ParsePlaylistQuery(id, target.Query())
		if err != nil {
			return nil, err
		}
		request = func(token spotifyauth.Token) (any, error) {
			playlist, err := client.Playlist(ctx, token, query)
			if tracks {
				return playlist.Tracks, err
			}
			return playlist, err
		}
	} else if kind, ok := catalog.LibraryPath(target.Path); ok {
		query, err := catalog.ParseLibraryQuery(kind, target.Query())
		if err != nil {
			return nil, err
		}
		request = func(token spotifyauth.Token) (any, error) { return client.Library(ctx, token, query) }
	} else {
		query, err := catalog.ParseSearchQuery(target.Query())
		if err != nil {
			return nil, err
		}
		request = func(token spotifyauth.Token) (any, error) { return client.Search(ctx, token, query) }
	}
	result, err := request(token)
	var upstream *catalog.HTTPError
	if errors.As(err, &upstream) && upstream.Status == 401 && (upstream.Stage == "profile" || upstream.Stage == "search" || upstream.Stage == "album" || upstream.Stage == "artist" || upstream.Stage == "playlist" || upstream.Stage == "library" || upstream.Stage == "track") {
		client.InvalidateClientToken()
		token, err = runtime.Refresh(ctx, spotifyauth.WebAPI, token)
		if err == nil {
			result, err = request(token)
		}
		if errors.As(err, &upstream) && upstream.Status == 401 && (upstream.Stage == "profile" || upstream.Stage == "search" || upstream.Stage == "album" || upstream.Stage == "artist" || upstream.Stage == "playlist" || upstream.Stage == "library" || upstream.Stage == "track") {
			runtime.rejectWebAPISession(ctx)
			return nil, spotifyauth.ErrAuthenticationRequired
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		code, stage, status := spotifyCatalogDiagnostic(err)
		logger.API("Spotify catalog failure: code=%s stage=%s upstream_status=%d", code, stage, status)
		if errors.As(err, &upstream) {
			if upstream.Status == 403 || upstream.Status == 404 {
				return catalogJSONResponse(upstream.Status, map[string]string{"error": "Spotify catalog request denied"})
			}
		}
		return nil, err
	}
	return catalogJSONResponse(http.StatusOK, result)
}
func catalogJSONResponse(status int, value any) (*http.Response, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, catalog.ErrSchema
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw))}, nil
}
