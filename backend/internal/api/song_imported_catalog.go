package api

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

// This inspection endpoint never consults the current provider account or performs
// provider I/O. Retained catalog evidence is not live membership/playability.
func (a *API) getSongImportedCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := chi.URLParam(r, "songID")
	sources, err := a.currentAnalysisSourceFingerprints([]string{id})
	if err != nil {
		respondError(w, 500, "source unavailable")
		return
	}
	fp := sources[id]
	if fp == "" {
		respondError(w, 404, "current catalog source unavailable")
		return
	}
	catalog, err := a.db.GetDownloadedSpotifyCatalog(r.Context(), id, fp)
	if err != nil {
		respondError(w, 500, "retained catalog unavailable")
		return
	}
	if catalog == nil {
		respondError(w, 404, "no retained catalog for current file")
		return
	}
	w.Header().Set("ETag", strconv.Quote(fp))
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, catalog)
}
