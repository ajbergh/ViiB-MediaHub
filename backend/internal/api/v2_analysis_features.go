package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
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

// BeatGridResponse is a presentation-safe timing artifact.  Beat times stay
// in seconds so waveform and deck clients do not need to reproduce codec or
// tempo interpolation behavior.
type BeatGridResponse struct {
	SongID           string    `json:"songId"`
	Beats            []float64 `json:"beats"`
	DownbeatIndices  []int     `json:"downbeatIndices"`
	Locked           bool      `json:"locked"`
	AlgorithmVersion string    `json:"algorithmVersion"`
}

// BeatGridUpdate accepts a complete validated replacement from the editor.
// Requiring a whole grid prevents a stale drag operation from applying a
// partial positional patch against a different dynamic grid.
type BeatGridUpdate struct {
	Beats           []float64 `json:"beats"`
	DownbeatIndices []int     `json:"downbeatIndices"`
	Locked          bool      `json:"locked"`
}

// EnergyFeaturesResponse exposes measurements and derived sections with the
// producing version, so clients can render an explainable curve without
// inferring energy from an LLM tag.
type EnergyFeaturesResponse struct {
	SongID           string                   `json:"songId"`
	IntegratedLUFS   float64                  `json:"integratedLufs"`
	TruePeakDBFS     float64                  `json:"truePeakDbfs"`
	Energy           []features.EnergyPoint   `json:"energy"`
	Sections         []features.Section       `json:"sections"`
	CueSuggestions   []features.CueSuggestion `json:"cueSuggestions"`
	AlgorithmVersion string                   `json:"algorithmVersion"`
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

func (a *API) getBeatGridV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	artifact, err := a.db.GetTrackAnalysisArtifact(songID, beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "beatgrid not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	grid, err := beatgrid.Decode(artifact.Data)
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
	respondJSON(w, BeatGridResponse{SongID: songID, Beats: grid.Beats, DownbeatIndices: grid.DownbeatIndices, Locked: override.BeatgridLocked, AlgorithmVersion: artifact.AlgorithmVersion})
}

func (a *API) putBeatGridV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	var update BeatGridUpdate
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	if err := decoder.Decode(&update); err != nil {
		respondError(w, http.StatusBadRequest, "invalid beatgrid update")
		return
	}
	grid := beatgrid.Grid{Beats: update.Beats, DownbeatIndices: update.DownbeatIndices}
	encoded, err := grid.Encode()
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	artifactID := songID + ":" + beatgrid.AlgorithmVersion
	if err := a.db.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: artifactID, SongID: songID, Kind: beatgrid.ArtifactKind, FormatVersion: beatgrid.FormatVersion, AlgorithmVersion: beatgrid.AlgorithmVersion, Encoding: beatgrid.Encoding, Data: encoded}); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	override, err := a.db.GetTrackAnalysisOverride(songID)
	if errors.Is(err, sql.ErrNoRows) {
		override = db.TrackAnalysisOverride{SongID: songID}
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	override.BeatgridArtifactID = &artifactID
	override.BeatgridLocked = update.Locked
	if err := a.db.UpsertTrackAnalysisOverride(override); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, BeatGridResponse{SongID: songID, Beats: grid.Beats, DownbeatIndices: grid.DownbeatIndices, Locked: update.Locked, AlgorithmVersion: beatgrid.AlgorithmVersion})
}

// resetBeatGridV2 clears an explicit grid edit and its lock.  A later normal
// analysis pass can then write a new detected grid; BPM/key overrides are
// preserved exactly as the user set them.
func (a *API) resetBeatGridV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	override, err := a.db.GetTrackAnalysisOverride(songID)
	if errors.Is(err, sql.ErrNoRows) {
		override = db.TrackAnalysisOverride{SongID: songID}
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	override.BeatgridArtifactID = nil
	override.BeatgridLocked = false
	if err := a.db.UpsertTrackAnalysisOverride(override); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.db.DeleteTrackAnalysisArtifact(songID, beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getEnergyFeaturesV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	artifact, err := a.db.GetTrackAnalysisArtifact(songID, features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "energy features not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := features.Decode(artifact.Data)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, EnergyFeaturesResponse{SongID: songID, IntegratedLUFS: result.IntegratedLUFS, TruePeakDBFS: result.TruePeakDBFS, Energy: result.Energy, Sections: result.Sections, CueSuggestions: result.CueSuggestions, AlgorithmVersion: artifact.AlgorithmVersion})
}

// measuredEnergyForDJ returns the same persisted curve summary exposed to the
// UI.  Corrupt individual artifacts are ignored rather than making the AI DJ
// unavailable; that song falls back to its existing metadata score.
func (a *API) measuredEnergyForDJ() (map[string]float64, error) {
	artifacts, err := a.db.ListTrackAnalysisArtifacts(features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		return nil, err
	}
	values := make(map[string]float64, len(artifacts))
	for _, artifact := range artifacts {
		result, err := features.Decode(artifact.Data)
		if err != nil || len(result.Energy) == 0 {
			continue
		}
		var total float64
		for _, point := range result.Energy {
			total += point.Value
		}
		values[artifact.SongID] = total / float64(len(result.Energy))
	}
	return values, nil
}
