// Maps Spotify download failures to sanitized HTTP responses for authentication, availability, and cooldown.
package api

import (
	"errors"
	"net/http"
	"time"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

// Preserve actionable Spotify failures without exposing upstream bodies to the renderer.
func respondSpotifyDownloadError(w http.ResponseWriter, err error, fallback int, message string) {
	if errors.Is(err, errNoDownloadableTracks) {
		respondError(w, http.StatusUnprocessableEntity, "No downloadable Spotify tracks are available in this selection.")
		return
	}
	if errors.Is(err, spotifyauth.ErrAuthenticationRequired) || errors.Is(err, spotifyauth.ErrDisabled) ||
		errors.Is(err, catalog.ErrInvalidQuery) {
		respondSpotifySessionError(w, err)
		return
	}
	var upstream *spotifyauth.WebPlayerHTTPError
	if errors.As(err, &upstream) {
		switch upstream.Status {
		case http.StatusTooManyRequests:
			respondSpotifySessionError(w, err)
			return
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			respondError(w, upstream.Status, message)
			return
		}
	}
	respondError(w, fallback, message)
}

func spotifyDownloadResponseError(response *http.Response) error {
	delay := time.Duration(0)
	if response.StatusCode == http.StatusTooManyRequests {
		delay = spotifyauth.RetryAfter(response.Header.Get("Retry-After"), time.Now())
	}
	return &spotifyauth.WebPlayerHTTPError{Status: response.StatusCode, RetryAfter: delay}
}
