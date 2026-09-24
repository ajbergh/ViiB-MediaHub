package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"

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
	BPMAltCandidate        *float64 `json:"bpmAltCandidate,omitempty"`
	TempoStability         *float64 `json:"tempoStability,omitempty"`
	TempoKind              *string  `json:"tempoKind,omitempty"`
	SongID                 string   `json:"songId"`
	Status                 string   `json:"status"`
	BPM                    *float64 `json:"bpm,omitempty"`
	BPMConfidence          *float64 `json:"bpmConfidence,omitempty"`
	BPMSource              string   `json:"bpmSource"`
	SyncAllowed            bool     `json:"syncAllowed"`
	Key                    *string  `json:"key,omitempty"`
	CamelotKey             *string  `json:"camelotKey,omitempty"`
	OpenKey                *string  `json:"openKey,omitempty"`
	KeyConfidence          *float64 `json:"keyConfidence,omitempty"`
	KeySource              string   `json:"keySource"`
	EnergyLevel            *int     `json:"energyLevel,omitempty"`
	EnergyLevelConfidence  *float64 `json:"energyLevelConfidence,omitempty"`
	EnergyAlgorithmVersion *string  `json:"energyAlgorithmVersion,omitempty"`
}

// BeatGridResponse is a presentation-safe timing artifact.  Beat times stay
// in seconds so waveform and deck clients do not need to reproduce codec or
// tempo interpolation behavior.
type BeatGridResponse struct {
	Source           string    `json:"source"`
	Provenance       string    `json:"provenance"`
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
	BPM             *float64  `json:"bpm,omitempty"`
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

// TransitionRecommendationResponse is an explicitly explainable, local
// candidate for the track currently leaving a deck.  It is advisory only;
// callers retain full control over cue and track selection.
type TransitionRecommendationResponse struct {
	SongID     string                         `json:"songId"`
	Title      string                         `json:"title"`
	Artist     string                         `json:"artist"`
	Score      float64                        `json:"score"`
	Intent     features.TransitionIntent      `json:"intent"`
	Vector     features.TransitionVector      `json:"vector"`
	Components []features.TransitionComponent `json:"components"`
}

type TransitionRecommendationsResponse struct {
	SongID           string                             `json:"songId"`
	Intent           features.TransitionIntent          `json:"intent"`
	AlgorithmVersion string                             `json:"algorithmVersion"`
	Recommendations  []TransitionRecommendationResponse `json:"recommendations"`
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
	// Scores from a running row may refer to an older source fingerprint. Only
	// expose a settled score whose own algorithm provenance is present.
	switch analysis.Status {
	case db.TrackAnalysisComplete, db.TrackAnalysisPartial, db.TrackAnalysisFailed:
		if analysis.EnergyLevel != nil && analysis.EnergyLevelConfidence != nil && analysis.EnergyAlgorithmVersion != nil && *analysis.EnergyAlgorithmVersion == features.EnergyLevelAlgorithmVersion {
			response.EnergyLevel = analysis.EnergyLevel
			response.EnergyLevelConfidence = analysis.EnergyLevelConfidence
			response.EnergyAlgorithmVersion = analysis.EnergyAlgorithmVersion
		}
	}
	if effectiveBPM.Source == db.EffectiveBPMMeasured {
		response.BPMConfidence = analysis.BPMConfidence
		response.BPMAltCandidate = analysis.BPMAltCandidate
		response.TempoStability = analysis.TempoStability
		response.TempoKind = analysis.TempoKind
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
	provenance := beatgrid.Provenance(artifact.Provenance)
	if artifact.Provenance == "" {
		provenance = grid.EffectiveProvenance()
	} else if !provenance.Valid() {
		provenance = beatgrid.ProvenanceUnknown
	}
	// Preserve the legacy source property while making its value truthful for
	// existing clients; provenance is the preferred, explicit field.
	respondJSON(w, BeatGridResponse{Source: string(provenance), Provenance: string(provenance), SongID: songID, Beats: grid.Beats, DownbeatIndices: grid.DownbeatIndices, Locked: override.BeatgridLocked, AlgorithmVersion: artifact.AlgorithmVersion})
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
	if update.BPM != nil && (math.IsNaN(*update.BPM) || math.IsInf(*update.BPM, 0) || *update.BPM <= 0 || *update.BPM > 1000) {
		respondError(w, http.StatusBadRequest, "invalid beatgrid BPM")
		return
	}
	grid := beatgrid.Grid{Beats: update.Beats, DownbeatIndices: update.DownbeatIndices}
	encoded, err := grid.Encode()
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	artifactID := songID + ":" + beatgrid.AlgorithmVersion
	if err := a.db.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: artifactID, SongID: songID, Kind: beatgrid.ArtifactKind, FormatVersion: beatgrid.FormatVersion, AlgorithmVersion: beatgrid.AlgorithmVersion, Encoding: beatgrid.Encoding, Provenance: string(beatgrid.ProvenanceManual), Data: encoded}); err != nil {
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
	if update.BPM != nil {
		override.BPM = update.BPM
		override.BPMLocked = true
	}
	if err := a.db.UpsertTrackAnalysisOverride(override); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, BeatGridResponse{Source: string(beatgrid.ProvenanceManual), Provenance: string(beatgrid.ProvenanceManual), SongID: songID, Beats: grid.Beats, DownbeatIndices: grid.DownbeatIndices, Locked: update.Locked, AlgorithmVersion: beatgrid.AlgorithmVersion})
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

// getTransitionRecommendationsV2 ranks only locally analyzed tracks.  It
// never fabricates features for an unmeasured candidate and returns every
// score component so the UI can present a useful reason rather than a black
// box number.
func (a *API) getTransitionRecommendationsV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	intent := features.TransitionIntent(r.URL.Query().Get("intent"))
	if intent == "" {
		intent = features.TransitionIntentHold
	}
	if intent != features.TransitionIntentHold && intent != features.TransitionIntentLift && intent != features.TransitionIntentReset && intent != features.TransitionIntentHarmonic {
		respondError(w, http.StatusBadRequest, "intent must be one of hold, lift, reset, or harmonic; surprise and vocal-safe require evidence not yet available")
		return
	}
	sourceArtifact, err := a.db.GetTrackAnalysisArtifact(songID, features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "energy features not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	source, err := features.Decode(sourceArtifact.Data)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	artifacts, err := a.db.ListTrackAnalysisArtifacts(features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	songs, err := a.db.GetAllSongs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	songByID := make(map[string]db.Song, len(songs))
	for _, song := range songs {
		songByID[song.ID] = song
	}
	analyses, err := a.db.ListTrackAnalysis()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	analysisByID := make(map[string]db.TrackAnalysis, len(analyses))
	for _, record := range analyses {
		analysisByID[record.SongID] = record
	}
	overrides, err := a.db.ListTrackAnalysisOverrides()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	metadataByID := make(map[string]features.TransitionMetadata, len(analysisByID))
	for id, record := range analysisByID {
		metadataByID[id] = resolvedTransitionMetadata(record, overrides[id])
	}
	recommendations := make([]TransitionRecommendationResponse, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.SongID == songID {
			continue
		}
		candidate, err := features.Decode(artifact.Data)
		if err != nil {
			continue // a corrupt candidate must not make the deck unavailable
		}
		song, exists := songByID[artifact.SongID]
		if !exists {
			continue
		}
		score, scoreErr := features.ScoreTransitionWithMetadata(source, candidate, metadataByID[songID], metadataByID[artifact.SongID], intent)
		if scoreErr != nil {
			respondError(w, http.StatusBadRequest, scoreErr.Error())
			return
		}
		recommendations = append(recommendations, TransitionRecommendationResponse{SongID: song.ID, Title: song.Title, Artist: song.Artist, Score: score.Score, Intent: intent, Vector: score.Vector, Components: score.Components})
	}
	sort.Slice(recommendations, func(i, j int) bool {
		if recommendations[i].Score == recommendations[j].Score {
			return recommendations[i].SongID < recommendations[j].SongID
		}
		return recommendations[i].Score > recommendations[j].Score
	})
	limit := parseBoundedInt(r.URL.Query().Get("limit"), 10, 50)
	if len(recommendations) > limit {
		recommendations = recommendations[:limit]
	}
	respondJSON(w, TransitionRecommendationsResponse{SongID: songID, Intent: intent, AlgorithmVersion: features.TransitionAlgorithmVersion, Recommendations: recommendations})
}

func resolvedTransitionMetadata(analysis db.TrackAnalysis, override db.TrackAnalysisOverride) features.TransitionMetadata {
	resolved := trackAnalysisFeatureResponse(analysis, override)
	return features.TransitionMetadata{
		BPM: resolved.BPM, BPMSource: resolved.BPMSource, BPMConfidence: resolved.BPMConfidence,
		CamelotKey: resolved.CamelotKey, KeySource: resolved.KeySource, KeyConfidence: resolved.KeyConfidence,
		EnergyLevel: resolved.EnergyLevel, EnergyLevelConfidence: resolved.EnergyLevelConfidence,
	}
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
