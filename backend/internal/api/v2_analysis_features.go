package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	analysiskey "github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/dj"
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
	AnalyzedAt             *int64   `json:"analyzedAt,omitempty"`
	BPM                    *float64 `json:"bpm,omitempty"`
	BPMConfidence          *float64 `json:"bpmConfidence,omitempty"`
	BPMSource              string   `json:"bpmSource"`
	SyncAllowed            bool     `json:"syncAllowed"`
	Key                    *string  `json:"key,omitempty"`
	CamelotKey             *string  `json:"camelotKey,omitempty"`
	OpenKey                *string  `json:"openKey,omitempty"`
	KeyConfidence          *float64 `json:"keyConfidence,omitempty"`
	KeySource              string   `json:"keySource"`
	KeyTonic               *int     `json:"keyTonic,omitempty"`
	KeyMode                *string  `json:"keyMode,omitempty"`
	MeasuredKeyTonic       *int     `json:"measuredKeyTonic,omitempty"`
	MeasuredKeyMode        *string  `json:"measuredKeyMode,omitempty"`
	EnergyLevel            *int     `json:"energyLevel,omitempty"`
	EnergyLevelConfidence  *float64 `json:"energyLevelConfidence,omitempty"`
	EnergyAlgorithmVersion *string  `json:"energyAlgorithmVersion,omitempty"`
	StructureAvailable     bool     `json:"structureAvailable"`
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
	SongID       string `json:"songId"`
	LoudnessKind string `json:"loudnessKind"`
	PeakKind     string `json:"peakKind"`
	ChannelScope string `json:"channelScope"`
	Standard     string `json:"standard"`
	// Deprecated numeric compatibility aliases. Consult the metadata above for
	// the actual unweighted RMS and sample-plus-midpoint proxy semantics.
	IntegratedLUFS       float64                  `json:"integratedLufs"`
	TruePeakDBFS         float64                  `json:"truePeakDbfs"`
	IntegratedLUFSBS1770 *float64                 `json:"integratedLufsBs1770,omitempty"`
	TruePeakDBTP         *float64                 `json:"truePeakDbtp,omitempty"`
	LoudnessStandard     *string                  `json:"loudnessStandard,omitempty"`
	LoudnessAlgorithm    *string                  `json:"loudnessAlgorithmVersion,omitempty"`
	TruePeakAlgorithm    *string                  `json:"truePeakAlgorithmVersion,omitempty"`
	LoudnessLayout       *string                  `json:"loudnessChannelLayout,omitempty"`
	LoudnessWeighting    *string                  `json:"loudnessChannelWeighting,omitempty"`
	LoudnessStatus       *string                  `json:"loudnessStatus,omitempty"`
	TruePeakStatus       *string                  `json:"truePeakStatus,omitempty"`
	Energy               []features.EnergyPoint   `json:"energy"`
	Sections             []features.Section       `json:"sections"`
	CueSuggestions       []features.CueSuggestion `json:"cueSuggestions"`
	AlgorithmVersion     string                   `json:"algorithmVersion"`
}

// TransitionRecommendationResponse is an explicitly explainable, local
// candidate for the track currently leaving a deck.  It is advisory only;
// callers retain full control over cue and track selection.
type TransitionRecommendationResponse struct {
	SongID         string                         `json:"songId"`
	Title          string                         `json:"title"`
	Artist         string                         `json:"artist"`
	Score          float64                        `json:"score"`
	Intent         features.TransitionIntent      `json:"intent"`
	Vector         features.TransitionVector      `json:"vector"`
	Components     []features.TransitionComponent `json:"components"`
	FilterEvidence TransitionCandidateEvidence    `json:"filterEvidence"`
}

// TransitionCandidateEvidence echoes the resolved measurements used by the
// optional Mix Next filters. Stem availability means a registered ready set.
type TransitionCandidateEvidence struct {
	BPM            *float64 `json:"bpm,omitempty"`
	EnergyLevel    *int     `json:"energyLevel,omitempty"`
	StemsAvailable *bool    `json:"stemsAvailable,omitempty"`
	LastPlayed     *int64   `json:"lastPlayed,omitempty"`
}

type TransitionRecommendationFilters struct {
	MinBPM                 *float64 `json:"minBpm,omitempty"`
	MaxBPM                 *float64 `json:"maxBpm,omitempty"`
	MinEnergyLevel         *int     `json:"minEnergyLevel,omitempty"`
	MaxEnergyLevel         *int     `json:"maxEnergyLevel,omitempty"`
	StemsAvailable         *bool    `json:"stemsAvailable,omitempty"`
	CamelotCompatible      *bool    `json:"camelotCompatible,omitempty"`
	PlaylistID             *string  `json:"playlistId,omitempty"`
	PlaylistIDs            []string `json:"playlistIds,omitempty"`
	Genre                  *string  `json:"genre,omitempty"`
	NotRecentlyPlayedHours *int     `json:"notRecentlyPlayedHours,omitempty"`
}

type TransitionRecommendationsResponse struct {
	SongID                  string                             `json:"songId"`
	Intent                  features.TransitionIntent          `json:"intent"`
	AlgorithmVersion        string                             `json:"algorithmVersion"`
	Filters                 TransitionRecommendationFilters    `json:"filters"`
	CandidatesBeforeFilters int                                `json:"candidatesBeforeFilters"`
	CandidatesAfterFilters  int                                `json:"candidatesAfterFilters"`
	Recommendations         []TransitionRecommendationResponse `json:"recommendations"`
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
	structureMetadata, err := a.db.ListTrackAnalysisArtifactMetadata(features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	analysisBySongID := make(map[string]db.TrackAnalysis, len(analyses))
	for _, analysis := range analyses {
		analysisBySongID[analysis.SongID] = analysis
	}
	metadataBySongID := make(map[string]db.TrackAnalysisArtifactMetadata, len(structureMetadata))
	eligiblePayloadIDs := make([]string, 0, len(structureMetadata))
	for _, metadata := range structureMetadata {
		analysis, exists := analysisBySongID[metadata.SongID]
		if !exists || !trackStructureSourceEligible(analysis) || metadata.SourceFingerprint == "" || metadata.SourceFingerprint != analysis.SourceFingerprint ||
			metadata.Kind != features.ArtifactKind || metadata.FormatVersion != features.FormatVersion || metadata.AlgorithmVersion != features.AlgorithmVersion ||
			metadata.Encoding != features.Encoding || metadata.Provenance != "measured" {
			continue
		}
		metadataBySongID[metadata.SongID] = metadata
		eligiblePayloadIDs = append(eligiblePayloadIDs, metadata.SongID)
	}
	structurePayloads, err := a.db.GetTrackAnalysisArtifactPayloads(eligiblePayloadIDs, features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := make([]TrackAnalysisFeatureResponse, 0, len(analyses))
	for _, analysis := range analyses {
		feature := trackAnalysisFeatureResponse(analysis, overrides[analysis.SongID])
		metadata, exists := metadataBySongID[analysis.SongID]
		if exists {
			feature.StructureAvailable = trackStructureAvailable(analysis, metadata, structurePayloads[analysis.SongID])
		}
		response = append(response, feature)
	}
	respondJSON(w, response)
}

func trackStructureSourceEligible(analysis db.TrackAnalysis) bool {
	if analysis.SourceFingerprint == "" {
		return false
	}
	switch analysis.Status {
	case db.TrackAnalysisComplete, db.TrackAnalysisPartial, db.TrackAnalysisFailed:
		return true
	default:
		return false
	}
}

// trackStructureAvailable only exposes a valid artifact bound to the exact
// source fingerprint of a settled analysis. Legacy artifacts without a source
// identity remain unknown rather than being treated as current.
func trackStructureAvailable(analysis db.TrackAnalysis, artifact db.TrackAnalysisArtifactMetadata, payload db.TrackAnalysisArtifactPayload) bool {
	if !trackStructureSourceEligible(analysis) || artifact.SongID != analysis.SongID || artifact.SourceFingerprint == "" || artifact.SourceFingerprint != analysis.SourceFingerprint ||
		artifact.Kind != features.ArtifactKind || artifact.FormatVersion != features.FormatVersion || artifact.AlgorithmVersion != features.AlgorithmVersion || artifact.Encoding != features.Encoding || artifact.Provenance != "measured" ||
		payload.SourceFingerprint == "" || payload.SourceFingerprint != artifact.SourceFingerprint || len(payload.Data) == 0 {
		return false
	}
	result, err := features.Decode(payload.Data)
	if err != nil || len(result.Sections) == 0 {
		return false
	}
	for _, section := range result.Sections {
		if math.IsNaN(section.Start) || math.IsInf(section.Start, 0) || section.Start < 0 ||
			math.IsNaN(section.End) || math.IsInf(section.End, 0) || section.End <= section.Start ||
			math.IsNaN(section.Energy) || math.IsInf(section.Energy, 0) || section.Energy < 0 || section.Energy > 1 ||
			math.IsNaN(section.Confidence) || math.IsInf(section.Confidence, 0) || section.Confidence < 0 || section.Confidence > 1 {
			return false
		}
		switch section.Label {
		case features.StructureIntro, features.StructureBuild, features.StructureDrop, features.StructureBreakdown, features.StructureOutro, features.StructureUnknown:
		default:
			return false
		}
	}
	return true
}

func trackAnalysisFeatureResponse(analysis db.TrackAnalysis, override db.TrackAnalysisOverride) TrackAnalysisFeatureResponse {
	effectiveBPM := db.ResolveEffectiveBPM(db.EffectiveBPMInputs{Override: &override, Analysis: &analysis})
	effectiveKey := db.ResolveEffectiveKey(db.EffectiveKeyInputs{Override: &override, Analysis: &analysis})
	response := TrackAnalysisFeatureResponse{
		SongID:      analysis.SongID,
		Status:      analysis.Status,
		AnalyzedAt:  analysis.AnalyzedAt,
		BPM:         effectiveBPM.Value,
		BPMSource:   effectiveBPM.Source,
		SyncAllowed: effectiveBPM.SyncAllowed,
		KeySource:   effectiveKey.Source,
		KeyTonic:    effectiveKey.Tonic,
		KeyMode:     effectiveKey.Mode,
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
	measuredKey := db.ResolveEffectiveKey(db.EffectiveKeyInputs{Analysis: &analysis})
	if measuredKey.Tonic != nil && measuredKey.Mode != nil {
		response.MeasuredKeyTonic = measuredKey.Tonic
		response.MeasuredKeyMode = measuredKey.Mode
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

// TrackKeyUpdate stores an explicitly verified tonic and mode.
type TrackKeyUpdate struct {
	Tonic *int   `json:"tonic"`
	Mode  string `json:"mode"`
}

func (a *API) putTrackKeyV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	var update TrackKeyUpdate
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&update); err != nil {
		respondError(w, http.StatusBadRequest, "invalid key update")
		return
	}
	if update.Tonic == nil || *update.Tonic < 0 || *update.Tonic > 11 || (update.Mode != "major" && update.Mode != "minor") {
		respondError(w, http.StatusBadRequest, "key tonic must be 0 through 11 and mode must be major or minor")
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
		override = db.TrackAnalysisOverride{SongID: songID}
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	override.KeyTonic = update.Tonic
	override.KeyMode = &update.Mode
	override.KeyLocked = true
	if err := a.db.UpsertTrackAnalysisOverride(override); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, trackAnalysisFeatureResponse(analysis, override))
}

func (a *API) resetTrackKeyV2(w http.ResponseWriter, r *http.Request) {
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
		override = db.TrackAnalysisOverride{SongID: songID}
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	override.KeyTonic = nil
	override.KeyMode = nil
	override.KeyLocked = false
	if err := a.db.UpsertTrackAnalysisOverride(override); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, trackAnalysisFeatureResponse(analysis, override))
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
	response := EnergyFeaturesResponse{SongID: songID, LoudnessKind: result.LoudnessKind, PeakKind: result.PeakKind, ChannelScope: result.ChannelScope, Standard: result.Standard, IntegratedLUFS: result.IntegratedLUFS, TruePeakDBFS: result.TruePeakDBFS, Energy: result.Energy, Sections: result.Sections, CueSuggestions: result.CueSuggestions, AlgorithmVersion: artifact.AlgorithmVersion}
	measurement, measurementErr := a.db.GetTrackAnalysisArtifact(songID, features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion)
	if measurementErr != nil && !errors.Is(measurementErr, sql.ErrNoRows) {
		respondError(w, http.StatusInternalServerError, measurementErr.Error())
		return
	}
	if measurementErr == nil {
		standards, decodeErr := features.DecodeBS1770(measurement.Data)
		if decodeErr != nil {
			respondError(w, http.StatusInternalServerError, decodeErr.Error())
			return
		}
		response.IntegratedLUFSBS1770 = standards.IntegratedLUFS
		response.TruePeakDBTP = standards.TruePeakDBTP
		response.LoudnessStandard = &standards.Standard
		response.LoudnessAlgorithm = &standards.LoudnessAlgorithm
		response.TruePeakAlgorithm = &standards.TruePeakAlgorithm
		response.LoudnessLayout = &standards.Layout
		response.LoudnessWeighting = &standards.Weighting
		response.LoudnessStatus = &standards.LoudnessStatus
		response.TruePeakStatus = &standards.TruePeakStatus
	}
	respondJSON(w, response)
}

// getTransitionRecommendationsV2 ranks only locally analyzed tracks.  It
// never fabricates features for an unmeasured candidate and returns every
// score component so the UI can present a useful reason rather than a black
// box number.
func (a *API) getTransitionRecommendationsV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	filters, err := parseTransitionRecommendationFilters(r.URL.Query())
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
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
	var playlistSongIDs map[string]struct{}
	if filters.PlaylistID != nil || len(filters.PlaylistIDs) > 0 {
		playlists, playlistErr := a.db.GetAllPlaylists()
		if playlistErr != nil {
			respondError(w, http.StatusInternalServerError, playlistErr.Error())
			return
		}
		playlistSongIDs = make(map[string]struct{})
		selectedPlaylistIDs := make(map[string]struct{}, len(filters.PlaylistIDs)+1)
		if filters.PlaylistID != nil {
			selectedPlaylistIDs[*filters.PlaylistID] = struct{}{}
		}
		for _, id := range filters.PlaylistIDs {
			selectedPlaylistIDs[id] = struct{}{}
		}
		for _, playlist := range playlists {
			if _, selected := selectedPlaylistIDs[playlist.ID]; selected {
				for _, id := range playlist.SongIDs {
					playlistSongIDs[id] = struct{}{}
				}
			}
		}
	}
	recentlyPlayedIDs := map[string]struct{}{}
	if filters.NotRecentlyPlayedHours != nil {
		ids, playedErr := a.db.GetRecentlyPlayedSongIDs(*filters.NotRecentlyPlayedHours)
		if playedErr != nil {
			respondError(w, http.StatusInternalServerError, playedErr.Error())
			return
		}
		for _, id := range ids {
			recentlyPlayedIDs[id] = struct{}{}
		}
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
	stemStatuses := map[string]string{}
	if filters.StemsAvailable != nil {
		candidateIDs := make([]string, 0, len(artifacts))
		for _, artifact := range artifacts {
			if artifact.SongID != songID {
				if _, exists := songByID[artifact.SongID]; exists {
					candidateIDs = append(candidateIDs, artifact.SongID)
				}
			}
		}
		stemStatuses, err = a.db.ListStemStatuses(candidateIDs)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	recommendations := make([]TransitionRecommendationResponse, 0, len(artifacts))
	candidatesBeforeFilters := 0
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
		candidatesBeforeFilters++
		metadata := metadataByID[artifact.SongID]
		stemAvailable := stemStatuses[artifact.SongID] == "ready"
		if !transitionCandidateMatchesFilters(metadata, stemAvailable, filters) || !transitionLibrarySongMatchesFilters(song, playlistSongIDs, filters) {
			continue
		}
		if filters.NotRecentlyPlayedHours != nil {
			if _, playedRecently := recentlyPlayedIDs[song.ID]; playedRecently {
				continue
			}
		}
		score, scoreErr := features.ScoreTransitionWithMetadata(source, candidate, metadataByID[songID], metadataByID[artifact.SongID], intent)
		if scoreErr != nil {
			respondError(w, http.StatusBadRequest, scoreErr.Error())
			return
		}
		if filters.CamelotCompatible != nil && *filters.CamelotCompatible {
			if !validTransitionKey(analysisByID[songID], overrides[songID]) || !validTransitionKey(analysisByID[artifact.SongID], overrides[artifact.SongID]) || !transitionCamelotCompatible(score.Vector.CamelotRelation) {
				continue
			}
		}
		evidence := TransitionCandidateEvidence{BPM: metadata.BPM, EnergyLevel: metadata.EnergyLevel}
		if filters.StemsAvailable != nil {
			evidence.StemsAvailable = &stemAvailable
		}
		if filters.NotRecentlyPlayedHours != nil {
			lastPlayed := song.LastPlayed
			evidence.LastPlayed = &lastPlayed
		}
		recommendations = append(recommendations, TransitionRecommendationResponse{SongID: song.ID, Title: song.Title, Artist: song.Artist, Score: score.Score, Intent: intent, Vector: score.Vector, Components: score.Components, FilterEvidence: evidence})
	}
	sort.Slice(recommendations, func(i, j int) bool {
		if recommendations[i].Score == recommendations[j].Score {
			return recommendations[i].SongID < recommendations[j].SongID
		}
		return recommendations[i].Score > recommendations[j].Score
	})
	candidatesAfterFilters := len(recommendations)
	limit := parseBoundedInt(r.URL.Query().Get("limit"), 10, 50)
	if len(recommendations) > limit {
		recommendations = recommendations[:limit]
	}
	respondJSON(w, TransitionRecommendationsResponse{SongID: songID, Intent: intent, AlgorithmVersion: features.TransitionAlgorithmVersion, Filters: filters, CandidatesBeforeFilters: candidatesBeforeFilters, CandidatesAfterFilters: candidatesAfterFilters, Recommendations: recommendations})
}

func parseTransitionRecommendationFilters(values url.Values) (TransitionRecommendationFilters, error) {
	var filters TransitionRecommendationFilters
	parseFloat := func(name string) (*float64, error) {
		value, present, err := singleQueryValue(values, name)
		if err != nil || !present {
			return nil, err
		}
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < float64(dj.MinValidBPM) || n > float64(dj.MaxValidBPM) {
			return nil, fmt.Errorf("%s must be a number between %d and %d", name, dj.MinValidBPM, dj.MaxValidBPM)
		}
		return &n, nil
	}
	parseEnergy := func(name string) (*int, error) {
		value, present, err := singleQueryValue(values, name)
		if err != nil || !present {
			return nil, err
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 10 {
			return nil, fmt.Errorf("%s must be an integer between 1 and 10", name)
		}
		return &n, nil
	}
	var err error
	if filters.MinBPM, err = parseFloat("minBpm"); err != nil {
		return filters, err
	}
	if filters.MaxBPM, err = parseFloat("maxBpm"); err != nil {
		return filters, err
	}
	if filters.MinEnergyLevel, err = parseEnergy("minEnergyLevel"); err != nil {
		return filters, err
	}
	if filters.MaxEnergyLevel, err = parseEnergy("maxEnergyLevel"); err != nil {
		return filters, err
	}
	if filters.MinBPM != nil && filters.MaxBPM != nil && *filters.MinBPM > *filters.MaxBPM {
		return filters, errors.New("minBpm must be less than or equal to maxBpm")
	}
	if filters.MinEnergyLevel != nil && filters.MaxEnergyLevel != nil && *filters.MinEnergyLevel > *filters.MaxEnergyLevel {
		return filters, errors.New("minEnergyLevel must be less than or equal to maxEnergyLevel")
	}
	if value, present, err := singleQueryValue(values, "stemsAvailable"); err != nil {
		return filters, err
	} else if present {
		var parsed bool
		switch value {
		case "true":
			parsed = true
		case "false":
			parsed = false
		default:
			return filters, errors.New("stemsAvailable must be true or false")
		}
		filters.StemsAvailable = &parsed
	}
	if value, present, err := singleQueryValue(values, "camelotCompatible"); err != nil {
		return filters, err
	} else if present {
		var parsed bool
		switch value {
		case "true":
			parsed = true
		case "false":
			parsed = false
		default:
			return filters, errors.New("camelotCompatible must be true or false")
		}
		filters.CamelotCompatible = &parsed
	}
	if value, present, err := singleQueryValue(values, "playlistId"); err != nil {
		return filters, err
	} else if present {
		filters.PlaylistID = &value
	}
	if playlistIDs, present := values["playlistIds"]; present {
		if _, singularPresent := values["playlistId"]; singularPresent {
			return filters, errors.New("playlistId and playlistIds cannot be combined")
		}
		if len(playlistIDs) == 0 || len(playlistIDs) > 20 {
			return filters, errors.New("playlistIds must contain between 1 and 20 playlist IDs")
		}
		seen := make(map[string]struct{}, len(playlistIDs))
		filters.PlaylistIDs = make([]string, 0, len(playlistIDs))
		for _, rawID := range playlistIDs {
			id := strings.TrimSpace(rawID)
			if id == "" {
				return filters, errors.New("playlistIds cannot contain an empty ID")
			}
			if _, duplicate := seen[id]; duplicate {
				return filters, errors.New("playlistIds cannot contain duplicate IDs")
			}
			seen[id] = struct{}{}
			filters.PlaylistIDs = append(filters.PlaylistIDs, id)
		}
	}
	if value, present, err := singleQueryValue(values, "genre"); err != nil {
		return filters, err
	} else if present {
		// Echo a canonical form and compare normalized genre entries below.
		genre := db.NormalizeGenre(value)
		if genre == "" {
			return filters, errors.New("genre must contain a value")
		}
		filters.Genre = &genre
	}
	if value, present, err := singleQueryValue(values, "notRecentlyPlayedHours"); err != nil {
		return filters, err
	} else if present {
		hours, parseErr := strconv.Atoi(value)
		if parseErr != nil || hours < 1 || hours > 168 {
			return filters, errors.New("notRecentlyPlayedHours must be an integer between 1 and 168")
		}
		filters.NotRecentlyPlayedHours = &hours
	}
	return filters, nil
}

func singleQueryValue(values url.Values, key string) (string, bool, error) {
	items, present := values[key]
	if !present {
		return "", false, nil
	}
	if len(items) != 1 || strings.TrimSpace(items[0]) == "" {
		return "", true, fmt.Errorf("%s must be supplied once with a value", key)
	}
	return strings.TrimSpace(items[0]), true, nil
}

func transitionCandidateMatchesFilters(metadata features.TransitionMetadata, stemsAvailable bool, filters TransitionRecommendationFilters) bool {
	if filters.MinBPM != nil && (metadata.BPM == nil || *metadata.BPM < *filters.MinBPM) {
		return false
	}
	if filters.MaxBPM != nil && (metadata.BPM == nil || *metadata.BPM > *filters.MaxBPM) {
		return false
	}
	if filters.MinEnergyLevel != nil && (metadata.EnergyLevel == nil || *metadata.EnergyLevel < *filters.MinEnergyLevel) {
		return false
	}
	if filters.MaxEnergyLevel != nil && (metadata.EnergyLevel == nil || *metadata.EnergyLevel > *filters.MaxEnergyLevel) {
		return false
	}
	if filters.StemsAvailable != nil && stemsAvailable != *filters.StemsAvailable {
		return false
	}
	return true
}

func transitionLibrarySongMatchesFilters(song db.Song, playlistSongIDs map[string]struct{}, filters TransitionRecommendationFilters) bool {
	if filters.PlaylistID != nil || len(filters.PlaylistIDs) > 0 {
		if _, ok := playlistSongIDs[song.ID]; !ok {
			return false
		}
	}
	if filters.Genre != nil {
		wanted := strings.ToLower(*filters.Genre)
		matched := false
		for _, genre := range song.Genre {
			if strings.ToLower(db.NormalizeGenre(genre)) == wanted {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func transitionCamelotCompatible(relation string) bool {
	switch relation {
	case "same", "adjacent", "relative":
		return true
	default:
		return false
	}
}

func validTransitionKey(analysis db.TrackAnalysis, override db.TrackAnalysisOverride) bool {
	key := db.ResolveEffectiveKey(db.EffectiveKeyInputs{Override: &override, Analysis: &analysis})
	if key.Tonic == nil || key.Mode == nil || *key.Tonic < 0 || *key.Tonic > 11 {
		return false
	}
	return *key.Mode == analysiskey.ModeMajor || *key.Mode == analysiskey.ModeMinor
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
