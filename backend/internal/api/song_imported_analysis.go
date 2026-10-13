package api

import (
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"time"
)

// Explicit retained-array reads do not dispatch provider work or alter playback timing.
func (a *API) getSongImportedAnalysis(w http.ResponseWriter, r *http.Request) {
	a.getSongDetailedAnalysis(w, r, false)
}
func (a *API) getSongProviderAnalysis(w http.ResponseWriter, r *http.Request) {
	a.getSongDetailedAnalysis(w, r, true)
}
func (a *API) getSongDetailedAnalysis(w http.ResponseWriter, r *http.Request, allowPrivate bool) {
	id := chi.URLParam(r, "songID")
	kind := chi.URLParam(r, "kind")
	switch kind {
	case "bars", "beats", "tatums", "sections", "segments":
	default:
		respondError(w, 400, "Unsupported detailed array")
		return
	}
	offset, limit := 0, 100
	for name, destination := range map[string]*int{"offset": &offset, "limit": &limit} {
		if raw := r.URL.Query().Get(name); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				respondError(w, 400, "Invalid detailed array range")
				return
			}
			*destination = parsed
		}
	}
	if offset < 0 || limit < 1 || limit > 1000 {
		respondError(w, 400, "Invalid detailed array range")
		return
	}
	current, err := a.currentAnalysisSourceFingerprints([]string{id})
	fp := current[id]
	if err != nil || fp == "" {
		respondError(w, 404, "Local source unavailable")
		return
	}
	artifact, err := a.db.GetDownloadedSpotifyAudioArtifact(r.Context(), id, fp, "audio_analysis", kind)
	if err != nil {
		respondError(w, 503, "Imported analysis unavailable")
		return
	}
	serve := func(artifact *db.SpotifyAudioArtifact, provenance string, pending bool) {
		var object map[string][]json.RawMessage
		if err = json.Unmarshal(artifact.Payload, &object); err != nil {
			respondError(w, 503, "Invalid retained array")
			return
		}
		items, ok := object[kind]
		if !ok {
			respondError(w, 503, "Retained array missing")
			return
		}
		if offset > len(items) {
			respondError(w, 416, "Detailed array offset outside range")
			return
		}
		end := min(len(items), offset+limit)
		final, err := a.currentAnalysisSourceFingerprints([]string{id})
		link, linkErr := a.db.GetSpotifyRecording(id, fp)
		if err != nil || final[id] != fp || linkErr != nil || link == nil || link.ExternalID != artifact.TrackID {
			respondError(w, 412, "Source or recording changed")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		respondJSON(w, map[string]any{"songId": id, "sourceFingerprint": fp, "recordingId": artifact.TrackID, "kind": kind, "provenance": provenance, "unverified": pending, "readOnly": true, "retrievedAt": artifact.RetrievedAt, "stale": !time.Now().Before(artifact.ExpiresAt), "items": items[offset:end], "totalItems": len(items), "offset": offset, "limit": limit})
	}
	if artifact != nil {
		serve(artifact, "spotify_durable_import", false)
		return
	}
	if !allowPrivate {
		respondError(w, 404, "Detailed array not retained for this file")
		return
	}
	link, err := a.db.GetSpotifyRecording(id, fp)
	if err != nil || link == nil {
		respondError(w, 404, "Current recording link unavailable")
		return
	}
	runtime := a.spotifyTokens()
	ctx, cancel := runtime.requestContext(r.Context())
	defer cancel()
	err = runtime.withMetadataRead(ctx, func(fence db.SpotifyMetadataReadFence) error {
		var artifact *db.SpotifyAudioArtifact
		var e error
		if fence.Pending {
			artifact, e = a.db.GetSpotifyAudioArtifactForRuntime(fence, link.ExternalID, "audio_analysis", kind)
		} else {
			artifact, e = a.db.GetSpotifyActiveAudioArtifactForRuntime(db.SpotifyMetadataFence{Epoch: fence.Epoch, ContextKey: fence.ContextKey}, link.ExternalID, "audio_analysis", kind)
		}
		if e != nil {
			return e
		}
		if artifact == nil {
			respondError(w, 404, "Detailed array not cached for this file")
			return nil
		}
		serve(artifact, "spotify_private_cache", fence.Pending)
		return nil
	})
	if err != nil {
		respondError(w, 503, "Cached detailed analysis unavailable")
	}
}
