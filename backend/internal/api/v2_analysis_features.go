package api

import (
	"database/sql"
	"errors"
	"net/http"

	analysiskey "github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
)

// TrackAnalysisFeatureResponse is the resolved, presentation-ready analysis
// snapshot for one song. It intentionally omits diagnostic and source-path
// fields, leaving the library UI with only values it can safely display.
type TrackAnalysisFeatureResponse struct {
	SongID        string   `json:"songId"`
	Status        string   `json:"status"`
	BPM           *float64 `json:"bpm,omitempty"`
	BPMConfidence *float64 `json:"bpmConfidence,omitempty"`
	BPMSource     string   `json:"bpmSource"`
	SyncAllowed   bool     `json:"syncAllowed"`
	Key           *string  `json:"key,omitempty"`
	CamelotKey    *string  `json:"camelotKey,omitempty"`
	OpenKey       *string  `json:"openKey,omitempty"`
	KeyConfidence *float64 `json:"keyConfidence,omitempty"`
	KeySource     string   `json:"keySource"`
}

func (a *API) getTrackAnalysisFeatureV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	analysis, err := a.db.GetTrackAnalysis(songID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "analysis not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	override, err := a.db.GetTrackAnalysisOverride(songID)
	if errors.Is(err, sql.ErrNoRows) {
		override = db.TrackAnalysisOverride{}
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, trackAnalysisFeatureResponse(analysis, override))
}

func (a *API) listTrackAnalysisFeaturesV2(w http.ResponseWriter, r *http.Request) {
	analyses, err := a.db.ListTrackAnalysis()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	overrides, err := a.db.ListTrackAnalysisOverrides()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := make([]TrackAnalysisFeatureResponse, 0, len(analyses))
	for _, analysis := range analyses {
		response = append(response, trackAnalysisFeatureResponse(analysis, overrides[analysis.SongID]))
	}
	respondJSON(w, response)
}

func trackAnalysisFeatureResponse(analysis db.TrackAnalysis, override db.TrackAnalysisOverride) TrackAnalysisFeatureResponse {
	effectiveBPM := db.ResolveEffectiveBPM(db.EffectiveBPMInputs{Override: &override, Analysis: &analysis})
	effectiveKey := db.ResolveEffectiveKey(db.EffectiveKeyInputs{Override: &override, Analysis: &analysis})
	response := TrackAnalysisFeatureResponse{
		SongID:      analysis.SongID,
		Status:      analysis.Status,
		BPM:         effectiveBPM.Value,
		BPMSource:   effectiveBPM.Source,
		SyncAllowed: effectiveBPM.SyncAllowed,
		KeySource:   effectiveKey.Source,
	}
	if effectiveBPM.Source == db.EffectiveBPMMeasured {
		response.BPMConfidence = analysis.BPMConfidence
	}
	if effectiveKey.Source == db.EffectiveKeyMeasured {
		response.KeyConfidence = analysis.KeyConfidence
	}
	if effectiveKey.Tonic != nil && effectiveKey.Mode != nil {
		keyName := analysiskey.FormatKey(*effectiveKey.Tonic, *effectiveKey.Mode)
		camelot := analysiskey.Camelot(*effectiveKey.Tonic, *effectiveKey.Mode)
		openKey := analysiskey.OpenKey(*effectiveKey.Tonic, *effectiveKey.Mode)
		response.Key = &keyName
		response.CamelotKey = &camelot
		response.OpenKey = &openKey
	}
	return response
}
