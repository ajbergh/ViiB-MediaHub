package api

import (
	"context"
	"errors"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

// Only fixed classifications cross the diagnostic boundary. Error strings,
// resource IDs, query terms, provider bodies and credentials are excluded.
func spotifyCatalogDiagnostic(err error) (code, stage string, status int) {
	code, stage = "unknown", "catalog"
	var upstream *catalog.HTTPError
	if errors.As(err, &upstream) {
		code, status = "upstream_http", upstream.Status
		switch upstream.Stage {
		case "client_token", "profile", "search", "album", "artist", "playlist", "library", "track":
			stage = upstream.Stage
		}
		return
	}
	switch {
	case errors.Is(err, context.Canceled):
		code = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		code = "deadline"
	case errors.Is(err, catalog.ErrSchema):
		code = "schema_incompatible"
	case errors.Is(err, catalog.ErrInvalidQuery):
		code = "invalid_query"
	case errors.Is(err, spotifyauth.ErrAuthenticationRequired):
		code = "authentication_required"
	case errors.Is(err, spotifyauth.ErrProviderChanged):
		code = "provider_changed"
	case errors.Is(err, spotifyauth.ErrTemporarilyUnavailable):
		code = "temporarily_unavailable"
	}
	return
}
