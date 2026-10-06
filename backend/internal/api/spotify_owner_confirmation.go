package api

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/google/uuid"
)

type spotifyOwnerBootstrapKey struct{}

func (s *spotifyAuthRuntime) confirmProfileOwner(ctx context.Context, id string) error {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || ctx.Value(spotifyAccountContextKey{}) != s.lifetime || s.closed || s.lifetime.Err() != nil || s.invalid.Load() {
		return spotifyauth.ErrAuthenticationRequired
	}
	provider := "oauth"
	if s.cookieMode {
		provider = "webplayer"
	}
	if s.pendingOwner == nil {
		err := s.database.BindSpotifyMetadataOwner(s.metadataEpoch, provider, id, s.metadataContext)
		if err == db.ErrSpotifyMetadataRuntimeSuperseded {
			s.invalid.Store(true)
		}
		return err
	}
	contextKey, err := s.database.ConfirmSpotifyMetadataOwner(s.metadataEpoch, provider, id, uuid.NewString())
	if err != nil {
		return err
	}
	changed := contextKey != s.metadataContext
	s.metadataContext = contextKey
	s.pendingOwner = nil
	if changed {
		s.endLifetime()
		s.lifetime, s.endLifetime = context.WithCancel(context.Background())
		return errSpotifyAccountChanged
	}
	return nil
}
