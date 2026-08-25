package api

import "net/http"

// ServeLocalAudio exposes the existing filesystem audio handler to the top-level
// server router. It is intentionally a thin wrapper so local playback behavior
// remains unchanged when the server selects the local-media fast path.
func (a *API) ServeLocalAudio(w http.ResponseWriter, r *http.Request) {
	a.serveAudio(w, r)
}

// ServeLocalCover exposes the existing filesystem/cache artwork handler for the
// same reason as ServeLocalAudio.
func (a *API) ServeLocalCover(w http.ResponseWriter, r *http.Request) {
	a.serveCover(w, r)
}
