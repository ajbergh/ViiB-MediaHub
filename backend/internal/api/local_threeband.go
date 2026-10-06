package api

import (
	"database/sql"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

func (a *API) getLocalThreeBand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "songID")
	current, err := a.currentAnalysisSourceFingerprints([]string{id})
	if err != nil {
		respondError(w, 500, "source unavailable")
		return
	}
	fp := current[id]
	if fp == "" {
		respondError(w, 404, "local waveform source unavailable")
		return
	}
	artifact, err := a.db.GetTrackAnalysisArtifact(id, threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, 404, "local three-band waveform not prepared")
		return
	}
	if err != nil {
		respondError(w, 500, "local waveform unavailable")
		return
	}
	if artifact.SourceFingerprint != fp || artifact.Provenance != "measured" || artifact.Encoding != threeband.Encoding {
		respondError(w, 404, "local three-band waveform not current")
		return
	}
	o, err := threeband.Decode(artifact.Data)
	if err != nil {
		respondError(w, 500, "invalid local waveform artifact")
		return
	}
	final, err := a.currentAnalysisSourceFingerprints([]string{id})
	if err != nil || final[id] != fp {
		respondError(w, 412, "local source changed")
		return
	}
	w.Header().Set("ETag", strconv.Quote(fp))
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, struct {
		SongID            string             `json:"songId"`
		SourceFingerprint string             `json:"sourceFingerprint"`
		Representation    string             `json:"representation"`
		AlgorithmVersion  string             `json:"algorithmVersion"`
		Overview          threeband.Overview `json:"overview"`
	}{id, fp, threeband.Kind, threeband.AlgorithmVersion, o})
}
