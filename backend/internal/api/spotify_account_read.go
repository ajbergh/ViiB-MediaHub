// Exposes account views and reports which require an OAuth connection.
package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
)

var errSpotifyAccountRequiresOAuth = errors.New("this Spotify view requires an OAuth connection")

var spotifyAccountPaths = map[string]string{
	"liked": "me/tracks", "top-tracks": "me/top/tracks", "top-artists": "me/top/artists",
	"devices": "me/player/devices", "queue": "me/player/queue",
}

// spotifyAccountRequiresOAuth identifies account routes without a Web Player adapter.
func spotifyAccountRequiresOAuth(path string) bool {
	for view, resource := range spotifyAccountPaths {
		if view != "liked" && path == "/v1/"+resource {
			return true
		}
	}
	return false
}

// spotifyAccountCapabilities reports whether the active OAuth session can request
// OAuth-only views. Spotify still decides access to individual endpoints.
func (a *API) spotifyAccountCapabilities(w http.ResponseWriter, _ *http.Request) {
	status := a.spotifyTokens().status()
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, map[string]bool{
		"oauthViews": status.Provider == "oauth" && status.Connected && !status.AuthRequired,
	})
}

// spotifyAccountRead exposes specific account reads under the existing Spotify session.
func (a *API) spotifyAccountRead(w http.ResponseWriter, r *http.Request) {
	view := chi.URLParam(r, "view")
	path, ok := spotifyAccountPaths[view]
	if !ok {
		respondError(w, http.StatusNotFound, "Unknown Spotify account view")
		return
	}
	input := r.URL.Query()
	query := url.Values{"path": {path}}
	if view == "liked" || view == "top-tracks" || view == "top-artists" {
		offset := 0
		if raw := input.Get("offset"); raw != "" {
			var err error
			offset, err = strconv.Atoi(raw)
			if err != nil || offset < 0 || offset > 10000 {
				respondError(w, http.StatusBadRequest, "Invalid Spotify account page")
				return
			}
		}
		query.Set("limit", "20")
		query.Set("offset", strconv.Itoa(offset))
	}
	if view == "top-tracks" || view == "top-artists" {
		window := input.Get("time_range")
		switch window {
		case "", "medium_term":
			query.Set("time_range", "medium_term")
		case "short_term", "long_term":
			query.Set("time_range", window)
		default:
			respondError(w, http.StatusBadRequest, "Invalid Spotify ranking range")
			return
		}
	}
	request := r.Clone(r.Context())
	request.URL.RawQuery = query.Encode()
	a.spotifyProxy(w, request)
}
