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
		if err == nil {
			s.setOwnerRetryIdentity(provider, s.metadataContext)
		}
		return err
	}
	newContext := uuid.NewString()
	if s.pendingOwner.AccountID == "" && s.pendingOwner.ContextKey != "" {
		newContext = s.pendingOwner.ContextKey
	}
	contextKey, err := s.database.ConfirmSpotifyMetadataOwner(s.metadataEpoch, provider, id, newContext)
	if err != nil {
		return err
	}
	changed := contextKey != s.metadataContext
	s.metadataContext = contextKey
	s.pendingOwner = nil
	s.setOwnerRetryIdentity(provider, contextKey)
	if changed {
		if err := s.database.ClearProviderThreeBandCapabilityStatuses("", ""); err != nil {
			return err
		}
		s.endLifetime()
		s.lifetime, s.endLifetime = newSpotifyAccountLifetime()
		return errSpotifyAccountChanged
	}
	return nil
}
