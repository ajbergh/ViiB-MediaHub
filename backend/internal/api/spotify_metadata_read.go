package api

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

// Keep captured ownership stable through response assembly without provider I/O.
func (s *spotifyAuthRuntime) withMetadataRead(ctx context.Context, read func(db.SpotifyMetadataReadFence) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ctx.Err() != nil || s.closed || s.lifetime.Err() != nil || ctx.Value(spotifyAccountContextKey{}) != s.lifetime || s.invalid.Load() {
		return spotifyauth.ErrAuthenticationRequired
	}
	provider := "oauth"
	if s.cookieMode {
		provider = "webplayer"
	}
	pending := s.pendingOwner != nil
	if pending && (s.pendingOwner.ContextKey != s.metadataContext || s.pendingOwner.Provider != provider) {
		return spotifyauth.ErrAuthenticationRequired
	}
	return read(db.SpotifyMetadataReadFence{Epoch: s.metadataEpoch, ContextKey: s.metadataContext, Provider: provider, Pending: pending})
}
