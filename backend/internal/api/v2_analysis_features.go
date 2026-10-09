// Defines v2 analysis features functionality for package api.

package api

import (
	"context"
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
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	analysiskey "github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/dj"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/go-chi/chi/v5"
)

// TrackAnalysisFeatureResponse is the resolved, presentation-ready analysis
// snapshot for one song. It intentionally omits diagnostic and source-path
// fields, leaving the library UI with only values it can safely display.
type TrackAnalysisFeatureResponse struct {
	EffectiveFields          []db.EffectiveScalar              `json:"effectiveFields,omitempty"`
	ProviderScores           map[string]db.SpotifyScoreSummary `json:"providerScores,omitempty"`
	ProviderScoresUnverified bool                              `json:"providerScoresUnverified,omitempty"`
	ProviderScalars          *SongProviderScalars              `json:"providerScalars,omitempty"`
	MeasuredBPM              *float64                          `json:"measuredBpm,omitempty"`
	MeasuredBPMConfidence    *float64                          `json:"measuredBpmConfidence,omitempty"`
	MeasuredKeyConfidence    *float64                          `json:"measuredKeyConfidence,omitempty"`
	LocalAlgorithmVersion    *string                           `json:"localAlgorithmVersion,omitempty"`
	BPMAltCandidate          *float64                          `json:"bpmAltCandidate,omitempty"`
	TempoStability           *float64                          `json:"tempoStability,omitempty"`
	TempoKind                *string                           `json:"tempoKind,omitempty"`
	SongID                   string                            `json:"songId"`
	Status                   string                            `json:"status"`
	AnalyzedAt               *int64                            `json:"analyzedAt,omitempty"`
	BPM                      *float64                          `json:"bpm,omitempty"`
	BPMConfidence            *float64                          `json:"bpmConfidence,omitempty"`
	BPMSource                string                            `json:"bpmSource"`
	SyncAllowed              bool                              `json:"syncAllowed"`
	Key                      *string                           `json:"key,omitempty"`
	CamelotKey               *string                           `json:"camelotKey,omitempty"`
	OpenKey                  *string                           `json:"openKey,omitempty"`
	KeyConfidence            *float64                          `json:"keyConfidence,omitempty"`
	KeySource                string                            `json:"keySource"`
	KeyTonic                 *int                              `json:"keyTonic,omitempty"`
	KeyMode                  *string                           `json:"keyMode,omitempty"`
	MeasuredKeyTonic         *int                              `json:"measuredKeyTonic,omitempty"`
	MeasuredKeyMode          *string                           `json:"measuredKeyMode,omitempty"`
	MeasuredEnergyLevel      *int                              `json:"measuredEnergyLevel,omitempty"`
	EnergyLevelSource        string                            `json:"energyLevelSource,omitempty"`
	EnergyLevel              *int                              `json:"energyLevel,omitempty"`
	EnergyLevelConfidence    *float64                          `json:"energyLevelConfidence,omitempty"`
	EnergyAlgorithmVersion   *string                           `json:"energyAlgorithmVersion,omitempty"`
	StructureAvailable       bool                              `json:"structureAvailable"`
	IntegratedLUFSBS1770     *float64                          `json:"integratedLufsBs1770,omitempty"`
	TruePeakDBTP             *float64                          `json:"truePeakDbtp,omitempty"`
	SourceFingerprint        string                            `json:"sourceFingerprint,omitempty"`
}

// BeatGridResponse is a presentation-safe timing artifact.  Beat times stay
// in seconds so waveform and deck clients do not need to reproduce codec or
// tempo interpolation behavior.
type BeatGridResponse struct {
	SourceFingerprint string    `json:"sourceFingerprint"`
	Resolution        string    `json:"resolution"`
	Reason            string    `json:"reason,omitempty"`
	Source            string    `json:"source"`
	Provenance        string    `json:"provenance"`
	SongID            string    `json:"songId"`
	Beats             []float64 `json:"beats"`
	DownbeatIndices   []int     `json:"downbeatIndices"`
	Locked            bool      `json:"locked"`
	AlgorithmVersion  string    `json:"algorithmVersion"`
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
	SpotifyScore       *db.SpotifyScoreSummary `json:"spotifyScore,omitempty"`
	SpotifyScoreMetric string                  `json:"spotifyScoreMetric,omitempty"`
	BPM                *float64                `json:"bpm,omitempty"`
	EnergyLevel        *int                    `json:"energyLevel,omitempty"`
	StemsAvailable     *bool                   `json:"stemsAvailable,omitempty"`
	LastPlayed         *int64                  `json:"lastPlayed,omitempty"`
}

type TransitionRecommendationFilters struct {
	SpotifyScoreMetric     string   `json:"spotifyScoreMetric,omitempty"`
	MinSpotifyScore        *float64 `json:"minSpotifyScore,omitempty"`
	MaxSpotifyScore        *float64 `json:"maxSpotifyScore,omitempty"`
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
	analysis, err := a.trackAnalysisOrUnanalyzed(songID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "song not found")
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

	override.Fields, err = a.db.GetTrackMetadataOverrides(songID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "manual metadata unavailable")
		return
	}

	currentFingerprints, err := a.currentAnalysisSourceFingerprints([]string{songID})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fingerprint := currentFingerprints[songID]
	err = a.withSongProviderScalars(r.Context(), songID, fingerprint, func(providerScalars *SongProviderScalars, providerErr error) {
		if r.Context().Err() != nil {
			return
		}
		latest, sourceErr := a.currentAnalysisSourceFingerprints([]string{songID})
		if sourceErr != nil {
			respondError(w, http.StatusInternalServerError, "source unavailable")
			return
		}
		if latest[songID] != fingerprint {
			respondError(w, http.StatusPreconditionFailed, "song source changed while reading analysis; reload analysis details")
			return
		}
		if providerErr != nil && !errors.Is(providerErr, spotifyauth.ErrAuthenticationRequired) {
			if errors.Is(providerErr, context.Canceled) || errors.Is(providerErr, context.DeadlineExceeded) {
				return
			}
			logger.API("Optional Spotify scalar read failed for song %s; returning local/manual analysis: %v", songID, providerErr)
			providerScalars = nil
		}
		response := trackAnalysisFeatureResponseWithCurrentSource(analysis, override, fingerprint)
		response.ProviderScalars = providerScalars
		var admitted []db.SpotifyScalarField
		if providerScalars != nil && !providerScalars.Unverified {
			admitted = providerScalars.Fields
		}
		response.EffectiveFields = db.ResolveAnalysisScalarFields(analysis, override, fingerprint, admitted)
		applyEffectiveScalarCompatibilityFields(&response, response.EffectiveFields)
		if response.SourceFingerprint != "" {
			w.Header().Set("ETag", strconv.Quote(response.SourceFingerprint))
		}
		respondJSON(w, response)
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		respondError(w, http.StatusInternalServerError, "metadata unavailable")
	}
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
	manualFields, err := a.db.ListTrackMetadataOverrides()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "manual metadata unavailable")
		return
	}
	for song, fields := range manualFields {
		override := overrides[song]
		override.SongID = song
		override.Fields = fields
		overrides[song] = override
	}

	structureMetadata, err := a.db.ListTrackAnalysisArtifactMetadata(features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	loudnessMetadata, err := a.db.ListTrackAnalysisArtifactMetadata(features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	analysisBySongID := make(map[string]db.TrackAnalysis, len(analyses))
	allAnalysisIDs := make([]string, 0, len(analyses))
	for _, analysis := range analyses {
		analysisBySongID[analysis.SongID] = analysis
		allAnalysisIDs = append(allAnalysisIDs, analysis.SongID)
	}
	metadataBySongID := make(map[string]db.TrackAnalysisArtifactMetadata, len(structureMetadata))
	candidateIDs := make([]string, 0, len(structureMetadata))
	for _, metadata := range structureMetadata {
		analysis, exists := analysisBySongID[metadata.SongID]
		if !exists || !trackStructureSourceEligible(analysis) || metadata.SourceFingerprint == "" || metadata.SourceFingerprint != analysis.SourceFingerprint ||
			metadata.Kind != features.ArtifactKind || metadata.FormatVersion != features.FormatVersion || metadata.AlgorithmVersion != features.AlgorithmVersion ||
			metadata.Encoding != features.Encoding || metadata.Provenance != "measured" || metadata.PayloadBytes <= 0 || metadata.PayloadBytes > features.MaxStructureStatusArtifactBytes {
			continue
		}
		metadataBySongID[metadata.SongID] = metadata
		candidateIDs = append(candidateIDs, metadata.SongID)
	}
	loudnessMetadataBySongID := make(map[string]db.TrackAnalysisArtifactMetadata, len(loudnessMetadata))
	loudnessCandidateIDs := make([]string, 0, len(loudnessMetadata))
	for _, metadata := range loudnessMetadata {
		analysis, exists := analysisBySongID[metadata.SongID]
		if !exists || !trackStructureSourceEligible(analysis) || metadata.SourceFingerprint == "" || metadata.SourceFingerprint != analysis.SourceFingerprint ||
			metadata.Kind != features.BS1770ArtifactKind || metadata.FormatVersion != features.BS1770FormatVersion || metadata.AlgorithmVersion != features.BS1770AlgorithmVersion ||
			metadata.Encoding != features.BS1770Encoding || metadata.Provenance != "measured" || metadata.PayloadBytes <= 0 || metadata.PayloadBytes > features.MaxBS1770ArtifactBytes {
			continue
		}
		loudnessMetadataBySongID[metadata.SongID] = metadata
		loudnessCandidateIDs = append(loudnessCandidateIDs, metadata.SongID)
	}
	currentFingerprints, err := a.currentAnalysisSourceFingerprints(allAnalysisIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	readyStructureBySongID := make(map[string]bool, len(candidateIDs))
	const structureStatusBatchSize = 200
	for start := 0; start < len(candidateIDs); start += structureStatusBatchSize {
		end := start + structureStatusBatchSize
		if end > len(candidateIDs) {
			end = len(candidateIDs)
		}
		chunk := candidateIDs[start:end]
		if err := a.db.VisitTrackAnalysisArtifactPayloads(chunk, features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion,
			func(songID, payloadFingerprint string, data []byte) error {
				analysis, exists := analysisBySongID[songID]
				metadata, hasMetadata := metadataBySongID[songID]
				currentFingerprint := currentFingerprints[songID]
				if exists && hasMetadata && currentFingerprint != "" {
					readyStructureBySongID[songID] = trackStructureAvailable(analysis, currentFingerprint, metadata, payloadFingerprint, data)
				}
				return nil
			}); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	lufsBySongID := make(map[string]float64, len(loudnessCandidateIDs))
	truePeakBySongID := make(map[string]float64, len(loudnessCandidateIDs))
	for start := 0; start < len(loudnessCandidateIDs); start += structureStatusBatchSize {
		end := start + structureStatusBatchSize
		if end > len(loudnessCandidateIDs) {
			end = len(loudnessCandidateIDs)
		}
		chunk := loudnessCandidateIDs[start:end]
		if err := a.db.VisitTrackAnalysisArtifactPayloads(chunk, features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion,
			func(songID, payloadFingerprint string, data []byte) error {
				analysis, exists := analysisBySongID[songID]
				metadata, hasMetadata := loudnessMetadataBySongID[songID]
				if !exists || !hasMetadata || payloadFingerprint == "" || payloadFingerprint != metadata.SourceFingerprint {
					return nil
				}
				lufs, truePeak := trackBS1770ListMeasurements(analysis, currentFingerprints[songID], metadata, data)
				if lufs != nil {
					lufsBySongID[songID] = *lufs
				}
				if truePeak != nil {
					truePeakBySongID[songID] = *truePeak
				}
				return nil
			}); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	var providerFields map[string][]db.SpotifyScalarField
	scoresUnverified := false
	a.spotifyAuthMu.Lock()
	scoreRuntime := a.spotifyAuth
	a.spotifyAuthMu.Unlock()
	publish := func() error {
		// Source reads and durable-file verification can span replacement. Re-resolve
		// once as a batch before applying any captured observations to library rows.
		latestFingerprints, sourceErr := a.currentAnalysisSourceFingerprints(allAnalysisIDs)
		if sourceErr != nil {
			respondError(w, http.StatusInternalServerError, "source unavailable")
			return nil
		}
		for song, fp := range currentFingerprints {
			if latestFingerprints[song] != fp {
				delete(providerFields, song)
				delete(lufsBySongID, song)
				delete(truePeakBySongID, song)
				readyStructureBySongID[song] = false
			}
		}
		currentFingerprints = latestFingerprints
		providerFields, err = a.db.RevalidateSpotifyScalarCandidateBatch(currentFingerprints, providerFields)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "metadata unavailable")
			return nil
		}
		scoreSummaries := db.SpotifyScoreSummariesFromFields(providerFields)
		response := make([]TrackAnalysisFeatureResponse, 0, len(analyses))
		for _, analysis := range analyses {
			feature := trackAnalysisFeatureResponseWithCurrentSource(analysis, overrides[analysis.SongID], currentFingerprints[analysis.SongID])
			feature.EffectiveFields = db.ResolveAnalysisScalarFields(analysis, overrides[analysis.SongID], currentFingerprints[analysis.SongID], providerFields[analysis.SongID])
			applyEffectiveScalarCompatibilityFields(&feature, feature.EffectiveFields)
			feature.ProviderScores = scoreSummaries[analysis.SongID]
			feature.ProviderScoresUnverified = scoresUnverified && len(feature.ProviderScores) > 0
			feature.StructureAvailable = readyStructureBySongID[analysis.SongID]
			if value, exists := lufsBySongID[analysis.SongID]; exists {
				feature.IntegratedLUFSBS1770 = &value
			}
			if value, exists := truePeakBySongID[analysis.SongID]; exists {
				feature.TruePeakDBTP = &value
			}
			response = append(response, feature)
		}
		respondJSON(w, response)
		return nil
	}
	if scoreRuntime != nil {
		scoreCtx, cancel := scoreRuntime.requestContext(r.Context())
		err := scoreRuntime.withMetadataRead(scoreCtx, func(fence db.SpotifyMetadataReadFence) error {
			var err error
			providerFields, err = a.db.GetSpotifyScalarCandidateBatchForRuntime(fence, currentFingerprints, time.Now())
			scoresUnverified = fence.Pending
			if err != nil {
				return err
			}
			if err := scoreCtx.Err(); err != nil {
				return err
			}
			return publish()
		})
		cancel()
		if err == nil {
			return
		}
		providerFields = nil
	} else {
		providerFields, err = a.db.GetDownloadedSpotifyScalarCandidateBatch(currentFingerprints, time.Now())
		if err != nil {
			respondError(w, http.StatusInternalServerError, "metadata unavailable")
			return
		}
	}
	_ = publish()
}

// currentAnalysisSourceFingerprints resolves many current identities without
// issuing a database read per song or opening remote media streams. Local
// identity uses the same path/hash/size/mtime contract as ResolveLocalSource;
// Plex identity comes from one cached catalog/source metadata snapshot.
func (a *API) currentAnalysisSourceFingerprints(songIDs []string) (map[string]string, error) {
	current := make(map[string]string, len(songIDs))
	if len(songIDs) == 0 {
		return current, nil
	}
	plexSources, err := a.db.ListPlexTrackSources()
	if err != nil {
		return nil, err
	}
	const sourceBatchSize = 200
	for start := 0; start < len(songIDs); start += sourceBatchSize {
		end := start + sourceBatchSize
		if end > len(songIDs) {
			end = len(songIDs)
		}
		songs, err := a.db.GetSongsByIDs(songIDs[start:end])
		if err != nil {
			return nil, err
		}
		for _, song := range songs {
			if plexTrack, exists := plexSources[song.ID]; exists {
				if plexTrack.Available && strings.TrimSpace(plexTrack.MediaKey) != "" {
					current[song.ID] = plexAnalysisFingerprint(plexTrack)
				}
				continue
			}
			resolved, err := analysis.ResolveLocalSongSource(song)
			if err == nil {
				current[song.ID] = resolved.Fingerprint
			}
		}
	}
	return current, nil
}

func currentEnergyArtifactMatchesSource(analysis db.TrackAnalysis, currentFingerprint string, artifact db.TrackAnalysisArtifact) bool {
	return trackStructureSourceEligible(analysis) && currentFingerprint != "" && currentFingerprint == analysis.SourceFingerprint &&
		artifact.SongID == analysis.SongID && artifact.SourceFingerprint != "" && artifact.SourceFingerprint == analysis.SourceFingerprint &&
		artifact.Kind == features.ArtifactKind && artifact.FormatVersion == features.FormatVersion && artifact.AlgorithmVersion == features.AlgorithmVersion &&
		artifact.Encoding == features.Encoding && artifact.Provenance == "measured"
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
func trackStructureAvailable(analysis db.TrackAnalysis, currentFingerprint string, artifact db.TrackAnalysisArtifactMetadata, payloadFingerprint string, data []byte) bool {
	if !trackStructureSourceEligible(analysis) || artifact.SongID != analysis.SongID || artifact.SourceFingerprint == "" || artifact.SourceFingerprint != analysis.SourceFingerprint ||
		artifact.Kind != features.ArtifactKind || artifact.FormatVersion != features.FormatVersion || artifact.AlgorithmVersion != features.AlgorithmVersion || artifact.Encoding != features.Encoding || artifact.Provenance != "measured" ||
		currentFingerprint == "" || currentFingerprint != analysis.SourceFingerprint || payloadFingerprint == "" || payloadFingerprint != artifact.SourceFingerprint || len(data) == 0 {
		return false
	}
	result, err := features.DecodeBounded(data, features.MaxStructureStatusArtifactBytes)
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

func decodeCurrentBS1770Artifact(analysis db.TrackAnalysis, currentFingerprint string, artifact db.TrackAnalysisArtifact) (features.BS1770Result, bool) {
	if !trackStructureSourceEligible(analysis) || currentFingerprint == "" || currentFingerprint != analysis.SourceFingerprint ||
		artifact.SongID != analysis.SongID || artifact.SourceFingerprint == "" || artifact.SourceFingerprint != analysis.SourceFingerprint ||
		artifact.Kind != features.BS1770ArtifactKind || artifact.FormatVersion != features.BS1770FormatVersion || artifact.AlgorithmVersion != features.BS1770AlgorithmVersion ||
		artifact.Encoding != features.BS1770Encoding || artifact.Provenance != "measured" || len(artifact.Data) == 0 || len(artifact.Data) > features.MaxBS1770ArtifactBytes {
		return features.BS1770Result{}, false
	}
	result, err := features.DecodeBS1770(artifact.Data)
	if err != nil {
		return features.BS1770Result{}, false
	}
	return result, true
}

func trackBS1770ListMeasurements(analysis db.TrackAnalysis, currentFingerprint string, artifact db.TrackAnalysisArtifactMetadata, data []byte) (*float64, *float64) {
	if artifact.SongID != analysis.SongID || artifact.SourceFingerprint == "" || artifact.SourceFingerprint != analysis.SourceFingerprint ||
		artifact.Kind != features.BS1770ArtifactKind || artifact.FormatVersion != features.BS1770FormatVersion || artifact.AlgorithmVersion != features.BS1770AlgorithmVersion ||
		artifact.Encoding != features.BS1770Encoding || artifact.Provenance != "measured" || len(data) == 0 || len(data) > features.MaxBS1770ArtifactBytes {
		return nil, nil
	}
	result, err := features.DecodeBS1770(data)
	if err != nil {
		return nil, nil
	}
	if !trackStructureSourceEligible(analysis) || currentFingerprint == "" || currentFingerprint != analysis.SourceFingerprint {
		return nil, nil
	}
	var lufs, truePeak *float64
	if result.LoudnessStatus == "available" && result.IntegratedLUFS != nil && !math.IsNaN(*result.IntegratedLUFS) && !math.IsInf(*result.IntegratedLUFS, 0) {
		lufs = result.IntegratedLUFS
	}
	if result.TruePeakStatus == "available" && result.TruePeakDBTP != nil && !math.IsNaN(*result.TruePeakDBTP) && !math.IsInf(*result.TruePeakDBTP, 0) {
		truePeak = result.TruePeakDBTP
	}
	return lufs, truePeak
}

func trackAnalysisFeatureResponseWithCurrentSource(analysis db.TrackAnalysis, override db.TrackAnalysisOverride, currentFingerprint string) TrackAnalysisFeatureResponse {
	response := trackAnalysisFeatureResponseResolved(analysis, override, db.ResolveEffectiveBPMForSource(db.EffectiveBPMInputs{Override: &override, Analysis: &analysis}, currentFingerprint), db.ResolveEffectiveKeyForSource(db.EffectiveKeyInputs{Override: &override, Analysis: &analysis}, currentFingerprint))
	response.SourceFingerprint = currentFingerprint
	if currentFingerprint == "" || currentFingerprint != analysis.SourceFingerprint {
		response.MeasuredBPM = nil
		response.MeasuredBPMConfidence = nil
		response.MeasuredKeyTonic = nil
		response.MeasuredKeyMode = nil
		response.MeasuredKeyConfidence = nil
		response.LocalAlgorithmVersion = nil
		response.EnergyLevel = nil
		response.EnergyLevelConfidence = nil
		response.EnergyAlgorithmVersion = nil
	}
	// Resolve the measured alternative independently of manual precedence.
	measured := db.ResolveEffectiveScalar("local_energy_level", currentFingerprint, db.AnalysisScalarCandidates(analysis, db.TrackAnalysisOverride{}, currentFingerprint))
	if measured.Selected != nil {
		var level int
		if json.Unmarshal(measured.Selected.Value, &level) == nil {
			response.EnergyLevel = &level
			response.EnergyLevelConfidence = measured.Selected.Confidence
			version := measured.Selected.AdapterRevision
			response.EnergyAlgorithmVersion = &version
		}
	}
	response.MeasuredEnergyLevel = response.EnergyLevel
	if response.EnergyLevel != nil {
		response.EnergyLevelSource = "local"
	}
	// The compatibility value drives library filters and Mix Next. Resolve only
	// the local 1–10 metric; Spotify's native energy score is a different field.
	energy := db.ResolveEffectiveScalar("local_energy_level", currentFingerprint, db.AnalysisScalarCandidates(analysis, override, currentFingerprint))
	if energy.Selected != nil {
		var level int
		if json.Unmarshal(energy.Selected.Value, &level) == nil {
			response.EnergyLevel = &level
			response.EnergyLevelSource = energy.Selected.Source
			response.EnergyLevelConfidence = energy.Selected.Confidence
			if energy.Selected.Source == "manual" {
				response.EnergyAlgorithmVersion = nil
			} else {
				version := energy.Selected.AdapterRevision
				response.EnergyAlgorithmVersion = &version
			}
		}
	}
	return response
}

func (a *API) trackAnalysisFeatureResponseWithProviderCandidates(ctx context.Context, songID string, analysis db.TrackAnalysis, override db.TrackAnalysisOverride, fingerprint string) (TrackAnalysisFeatureResponse, error) {
	manualFields, err := a.db.GetTrackMetadataOverrides(songID)
	if err != nil {
		return TrackAnalysisFeatureResponse{}, err
	}
	override.Fields = manualFields
	providerScalars, err := a.songProviderScalarsContext(ctx, songID, fingerprint)
	if err != nil && !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return TrackAnalysisFeatureResponse{}, err
		}
		logger.API("Optional Spotify scalar read failed for song %s; returning local/manual analysis: %v", songID, err)
		providerScalars = nil
	}
	if err := ctx.Err(); err != nil {
		return TrackAnalysisFeatureResponse{}, err
	}
	var admitted []db.SpotifyScalarField
	if providerScalars != nil && !providerScalars.Unverified {
		admitted = providerScalars.Fields
	}
	response := trackAnalysisFeatureResponseWithCurrentSource(analysis, override, fingerprint)
	response.EffectiveFields = db.ResolveAnalysisScalarFields(analysis, override, fingerprint, admitted)
	applyEffectiveScalarCompatibilityFields(&response, response.EffectiveFields)
	return response, nil
}

func applyEffectiveScalarCompatibilityFields(response *TrackAnalysisFeatureResponse, effective []db.EffectiveScalar) {
	if response == nil {
		return
	}
	for _, field := range effective {
		switch field.Key {
		case "tempo_bpm":
			response.BPM = nil
			response.BPMConfidence = nil
			response.BPMSource = db.EffectiveBPMUnknown
			response.SyncAllowed = false
			response.BPMAltCandidate = nil
			response.TempoStability = nil
			response.TempoKind = nil
		case "key_mode":
			response.Key = nil
			response.CamelotKey = nil
			response.OpenKey = nil
			response.KeyTonic = nil
			response.KeyMode = nil
			response.KeyConfidence = nil
			response.KeySource = db.EffectiveKeyUnknown
		default:
			continue
		}
		if field.Selected == nil {
			continue
		}
		selected := field.Selected
		switch field.Key {
		case "tempo_bpm":
			var value float64
			if json.Unmarshal(selected.Value, &value) != nil {
				continue
			}
			response.BPM = &value
			response.BPMConfidence = selected.Confidence
			response.BPMSource = compatibilityScalarSource(selected.Source)
			response.SyncAllowed = selected.Source == "manual" || selected.Source == "local"
			if selected.Source != "local" {
				response.BPMAltCandidate = nil
				response.TempoStability = nil
				response.TempoKind = nil
			}
		case "key_mode":
			var value struct {
				Tonic int `json:"tonic"`
				Mode  int `json:"mode"`
			}
			if json.Unmarshal(selected.Value, &value) != nil || value.Tonic < 0 || value.Tonic > 11 || (value.Mode != 0 && value.Mode != 1) {
				continue
			}
			mode := "minor"
			if value.Mode == 1 {
				mode = "major"
			}
			response.KeyTonic = &value.Tonic
			response.KeyMode = &mode
			response.KeyConfidence = selected.Confidence
			response.KeySource = compatibilityScalarSource(selected.Source)
			keyName := analysiskey.FormatKey(value.Tonic, mode)
			camelot := analysiskey.Camelot(value.Tonic, mode)
			openKey := analysiskey.OpenKey(value.Tonic, mode)
			response.Key = &keyName
			response.CamelotKey = &camelot
			response.OpenKey = &openKey
		}
	}
}

func compatibilityScalarSource(source string) string {
	switch source {
	case "manual":
		return db.EffectiveBPMManual
	case "local":
		return db.EffectiveBPMMeasured
	case "spotify_private", "spotify_download_import":
		return db.EffectiveBPMSpotify
	default:
		return db.EffectiveBPMUnknown
	}
}
func trackAnalysisFeatureResponseResolved(analysis db.TrackAnalysis, override db.TrackAnalysisOverride, effectiveBPM db.EffectiveBPM, effectiveKey db.EffectiveKey) TrackAnalysisFeatureResponse {
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
	if effectiveBPM.Source == db.EffectiveBPMMeasured {
		response.BPMConfidence = analysis.BPMConfidence
		response.BPMAltCandidate = analysis.BPMAltCandidate
		response.TempoStability = analysis.TempoStability
		response.TempoKind = analysis.TempoKind
	}
	if effectiveKey.Source == db.EffectiveKeyMeasured {
		response.KeyConfidence = analysis.KeyConfidence
	}
	if local := db.CurrentLocalScalars(&analysis); local != nil {
		response.MeasuredBPM = local.BPM
		response.MeasuredBPMConfidence = local.BPMConfidence
		response.MeasuredKeyTonic = local.KeyTonic
		response.MeasuredKeyMode = local.KeyMode
		response.MeasuredKeyConfidence = local.KeyConfidence
		response.LocalAlgorithmVersion = &local.AlgorithmVersion
		if effectiveBPM.Source == db.EffectiveBPMMeasured {
			response.BPMConfidence = local.BPMConfidence
			response.BPMAltCandidate = local.BPMAltCandidate
			response.TempoStability = local.TempoStability
			response.TempoKind = local.TempoKind
		}
		if effectiveKey.Source == db.EffectiveKeyMeasured {
			response.KeyConfidence = local.KeyConfidence
		}
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

// TrackBPMUpdate stores a user-corrected scalar tempo independently of the beat grid.
type TrackBPMUpdate struct {
	BPM *float64 `json:"bpm"`
}

func (a *API) getTrackBPMV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	trackAnalysis, err := a.trackAnalysisOrUnanalyzed(songID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "song not found")
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
	currentFingerprints, err := a.currentAnalysisSourceFingerprints([]string{songID})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if current := currentFingerprints[songID]; current != "" {
		if err := a.db.RefreshTrackAnalysisSourceRevision(songID, current); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	manualFields, err := a.db.GetTrackMetadataOverrides(songID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "manual metadata unavailable")
		return
	}
	override.Fields = manualFields
	fingerprint := currentFingerprints[songID]
	err = a.withSongProviderScalars(r.Context(), songID, fingerprint, func(providerScalars *SongProviderScalars, providerErr error) {
		if r.Context().Err() != nil {
			return
		}
		latest, sourceErr := a.currentAnalysisSourceFingerprints([]string{songID})
		if sourceErr != nil {
			respondError(w, http.StatusInternalServerError, "source unavailable")
			return
		}
		if latest[songID] != fingerprint {
			respondError(w, http.StatusPreconditionFailed, "song source changed while reading analysis; reload analysis details")
			return
		}
		if providerErr != nil && !errors.Is(providerErr, spotifyauth.ErrAuthenticationRequired) {
			if errors.Is(providerErr, context.Canceled) || errors.Is(providerErr, context.DeadlineExceeded) {
				return
			}
			logger.API("Optional Spotify scalar read failed for song %s; returning local/manual analysis: %v", songID, providerErr)
			providerScalars = nil
		}
		var admitted []db.SpotifyScalarField
		if providerScalars != nil && !providerScalars.Unverified {
			admitted = providerScalars.Fields
		}
		response := trackAnalysisFeatureResponseWithCurrentSource(trackAnalysis, override, fingerprint)
		response.ProviderScalars = providerScalars
		response.EffectiveFields = db.ResolveAnalysisScalarFields(trackAnalysis, override, fingerprint, admitted)
		applyEffectiveScalarCompatibilityFields(&response, response.EffectiveFields)
		if response.SourceFingerprint != "" {
			w.Header().Set("ETag", strconv.Quote(response.SourceFingerprint))
		}
		respondJSON(w, response)
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		respondError(w, http.StatusInternalServerError, "metadata unavailable")
	}
}

func (a *API) putTrackBPMV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	var update TrackBPMUpdate
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&update); err != nil {
		respondError(w, http.StatusBadRequest, "invalid BPM update")
		return
	}
	if update.BPM == nil || math.IsNaN(*update.BPM) || math.IsInf(*update.BPM, 0) || *update.BPM <= 0 || *update.BPM > 1000 {
		respondError(w, http.StatusBadRequest, "BPM must be a finite number greater than 0 and at most 1000")
		return
	}
	trackAnalysis, err := a.trackAnalysisOrUnanalyzed(songID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "song not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	currentFingerprint, err := a.requireCurrentAnalysisSource(w, r, songID)
	if err != nil {
		return
	}
	updated, err := a.db.SetTrackAnalysisBPMOverrideIfSourceCurrent(songID, *update.BPM, currentFingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !updated {
		respondError(w, http.StatusPreconditionFailed, "song source changed while saving BPM; reload analysis details before editing")
		return
	}
	if err := a.revalidateAnalysisSourceAfterWrite(songID, currentFingerprint); err != nil {
		respondError(w, http.StatusPreconditionFailed, "song source changed while saving BPM; reload analysis details before editing")
		return
	}
	override, err := a.db.GetTrackAnalysisOverride(songID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response, err := a.trackAnalysisFeatureResponseWithProviderCandidates(r.Context(), songID, trackAnalysis, override, currentFingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "metadata unavailable")
		return
	}
	response.SourceFingerprint = currentFingerprint
	w.Header().Set("ETag", strconv.Quote(currentFingerprint))
	respondJSON(w, response)
}

func (a *API) resetTrackBPMV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	trackAnalysis, err := a.trackAnalysisOrUnanalyzed(songID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "song not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	currentFingerprint, err := a.requireCurrentAnalysisSource(w, r, songID)
	if err != nil {
		return
	}
	reset, err := a.db.ResetTrackAnalysisBPMOverrideIfSourceCurrent(songID, currentFingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !reset {
		respondError(w, http.StatusPreconditionFailed, "song source changed while resetting BPM; reload analysis details before editing")
		return
	}
	if err := a.revalidateAnalysisSourceAfterWrite(songID, currentFingerprint); err != nil {
		respondError(w, http.StatusPreconditionFailed, "song source changed while resetting BPM; reload analysis details before editing")
		return
	}
	override, err := a.db.GetTrackAnalysisOverride(songID)
	if errors.Is(err, sql.ErrNoRows) {
		override = db.TrackAnalysisOverride{SongID: songID}
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response, err := a.trackAnalysisFeatureResponseWithProviderCandidates(r.Context(), songID, trackAnalysis, override, currentFingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "metadata unavailable")
		return
	}
	response.SourceFingerprint = currentFingerprint
	w.Header().Set("ETag", strconv.Quote(currentFingerprint))
	respondJSON(w, response)
}

func (a *API) requireCurrentAnalysisSource(w http.ResponseWriter, r *http.Request, songID string) (string, error) {
	expected := strings.TrimSpace(r.Header.Get("If-Match"))
	if expected == "" {
		respondError(w, http.StatusPreconditionRequired, "current source fingerprint is required")
		return "", errors.New("source fingerprint precondition missing")
	}
	current, err := a.currentAnalysisSourceFingerprints([]string{songID})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return "", err
	}
	actual := current[songID]
	if actual != "" {
		if err := a.db.RefreshTrackAnalysisSourceRevision(songID, actual); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return "", err
		}
	}
	if actual == "" || strconv.Quote(actual) != expected {
		respondError(w, http.StatusPreconditionFailed, "song source changed or is unavailable; reload analysis details before editing")
		return "", errors.New("source fingerprint precondition failed")
	}
	return actual, nil
}

func (a *API) revalidateAnalysisSourceAfterWrite(songID, expected string) error {
	current, err := a.currentAnalysisSourceFingerprints([]string{songID})
	if err != nil {
		return err
	}
	actual := current[songID]
	if actual != "" {
		if err := a.db.RefreshTrackAnalysisSourceRevision(songID, actual); err != nil {
			return err
		}
	}
	if actual == "" || actual != expected {
		return errors.New("source changed during analysis write")
	}
	return nil
}

func (a *API) trackAnalysisOrUnanalyzed(songID string) (db.TrackAnalysis, error) {
	trackAnalysis, err := a.db.GetTrackAnalysis(songID)
	if err == nil {
		return trackAnalysis, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.TrackAnalysis{}, err
	}
	if _, err := a.db.GetSongByID(songID); err != nil {
		return db.TrackAnalysis{}, err
	}
	return db.TrackAnalysis{SongID: songID, Status: "not_analyzed"}, nil
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
	analysis, err := a.trackAnalysisOrUnanalyzed(songID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "song not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fingerprint, err := a.requireCurrentAnalysisSource(w, r, songID)
	if err != nil {
		return
	}
	saved, err := a.db.SetTrackAnalysisKeyOverrideIfSourceCurrent(songID, *update.Tonic, update.Mode, fingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !saved {
		respondError(w, http.StatusPreconditionFailed, "song source changed while saving key; reload analysis details")
		return
	}
	if err := a.revalidateAnalysisSourceAfterWrite(songID, fingerprint); err != nil {
		respondError(w, http.StatusPreconditionFailed, "song source changed while saving key; reload analysis details")
		return
	}
	override, err := a.db.GetTrackAnalysisOverride(songID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response, err := a.trackAnalysisFeatureResponseWithProviderCandidates(r.Context(), songID, analysis, override, fingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "metadata unavailable")
		return
	}
	w.Header().Set("ETag", strconv.Quote(fingerprint))
	respondJSON(w, response)
}

func (a *API) resetTrackKeyV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	analysis, err := a.trackAnalysisOrUnanalyzed(songID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "song not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fingerprint, err := a.requireCurrentAnalysisSource(w, r, songID)
	if err != nil {
		return
	}
	reset, err := a.db.ResetTrackAnalysisKeyOverrideIfSourceCurrent(songID, fingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !reset {
		respondError(w, http.StatusPreconditionFailed, "song source changed while resetting key; reload analysis details")
		return
	}
	if err := a.revalidateAnalysisSourceAfterWrite(songID, fingerprint); err != nil {
		respondError(w, http.StatusPreconditionFailed, "song source changed while resetting key; reload analysis details")
		return
	}
	override, err := a.db.GetTrackAnalysisOverride(songID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response, err := a.trackAnalysisFeatureResponseWithProviderCandidates(r.Context(), songID, analysis, override, fingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "metadata unavailable")
		return
	}
	w.Header().Set("ETag", strconv.Quote(fingerprint))
	respondJSON(w, response)
}

func (a *API) getBeatGridV2(w http.ResponseWriter, r *http.Request) {
	song := chi.URLParam(r, "songID")
	override, err := a.db.GetTrackAnalysisOverride(song)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		respondError(w, 500, err.Error())
		return
	}
	fingerprints, err := a.currentAnalysisSourceFingerprints([]string{song})
	if err != nil {
		respondError(w, 500, err.Error())
		return
	}
	fp := fingerprints[song]
	resolved, err := a.db.ResolveBeatGrid(song, fp)
	if err != nil {
		respondError(w, 500, err.Error())
		return
	}
	response := BeatGridResponse{SongID: song, Locked: override.BeatgridLocked, AlgorithmVersion: beatgrid.AlgorithmVersion, Source: "unknown", Provenance: "unknown", SourceFingerprint: fp, Resolution: "unavailable", Reason: resolved.Reason, Beats: []float64{}, DownbeatIndices: []int{}}
	if fp != "" {
		w.Header().Set("ETag", strconv.Quote(fp))
	}
	if resolved.Grid != nil {
		response.Resolution = "available"
		response.Reason = ""
		response.Beats = resolved.Grid.Beats
		response.DownbeatIndices = resolved.Grid.DownbeatIndices
		response.Source = string(resolved.Grid.Provenance)
		response.Provenance = response.Source
	}
	respondJSON(w, response)
}
func (a *API) putBeatGridV2(w http.ResponseWriter, r *http.Request) {
	song := chi.URLParam(r, "songID")
	if song == "" {
		respondError(w, 400, "song ID is required")
		return
	}
	var update BeatGridUpdate
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&update); err != nil {
		respondError(w, 400, "invalid beatgrid update")
		return
	}
	if update.BPM != nil && (math.IsNaN(*update.BPM) || math.IsInf(*update.BPM, 0) || *update.BPM <= 0 || *update.BPM > 1000) {
		respondError(w, 400, "invalid beatgrid BPM")
		return
	}
	grid := beatgrid.Grid{Beats: update.Beats, DownbeatIndices: update.DownbeatIndices, Provenance: beatgrid.ProvenanceManual}
	encoded, err := grid.Encode()
	if err != nil || len(grid.Beats) < 2 {
		respondError(w, 400, "invalid beatgrid timing")
		return
	}
	fp, err := a.requireCurrentAnalysisSource(w, r, song)
	if err != nil {
		return
	}
	artifact := db.TrackAnalysisArtifact{ID: song + ":" + beatgrid.AlgorithmVersion, SongID: song, Kind: beatgrid.ArtifactKind, FormatVersion: beatgrid.FormatVersion, AlgorithmVersion: beatgrid.AlgorithmVersion, Encoding: beatgrid.Encoding, Provenance: string(beatgrid.ProvenanceManual), SourceFingerprint: fp, Data: encoded}
	updated, err := a.db.SaveManualBeatGridIfSourceCurrent(artifact, update.Locked, update.BPM)
	if err != nil {
		respondError(w, 500, err.Error())
		return
	}
	if !updated || a.revalidateAnalysisSourceAfterWrite(song, fp) != nil {
		respondError(w, 412, "song source changed while saving beatgrid; reload before editing")
		return
	}
	w.Header().Set("ETag", strconv.Quote(fp))
	respondJSON(w, BeatGridResponse{SongID: song, Source: "manual", Provenance: "manual", Beats: grid.Beats, DownbeatIndices: grid.DownbeatIndices, Locked: update.Locked, AlgorithmVersion: beatgrid.AlgorithmVersion, SourceFingerprint: fp, Resolution: "available"})
}

// resetBeatGridV2 clears an explicit grid edit and its lock.  A later normal
// analysis pass can then write a new detected grid; BPM/key overrides are
// preserved exactly as the user set them.
func (a *API) resetBeatGridV2(w http.ResponseWriter, r *http.Request) {
	song := chi.URLParam(r, "songID")
	fp, err := a.requireCurrentAnalysisSource(w, r, song)
	if err != nil {
		return
	}
	updated, err := a.db.ResetBeatGridIfSourceCurrent(song, fp)
	if err != nil {
		respondError(w, 500, err.Error())
		return
	}
	if !updated || a.revalidateAnalysisSourceAfterWrite(song, fp) != nil {
		respondError(w, 412, "song source changed while resetting beatgrid; reload before editing")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getEnergyFeaturesV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	analysis, err := a.db.GetTrackAnalysis(songID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "analysis not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	artifact, err := a.db.GetTrackAnalysisArtifact(songID, features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "energy features not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	currentFingerprints, err := a.currentAnalysisSourceFingerprints([]string{songID})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !currentEnergyArtifactMatchesSource(analysis, currentFingerprints[songID], artifact) {
		respondError(w, http.StatusConflict, "energy features are stale or cannot be verified for the current source")
		return
	}
	result, err := features.DecodeBounded(artifact.Data, features.MaxStructureStatusArtifactBytes)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	energy := result.Energy
	if energy == nil {
		energy = []features.EnergyPoint{}
	}
	sections := result.Sections
	if sections == nil {
		sections = []features.Section{}
	}
	cueSuggestions := result.CueSuggestions
	if cueSuggestions == nil {
		cueSuggestions = []features.CueSuggestion{}
	}
	response := EnergyFeaturesResponse{SongID: songID, LoudnessKind: result.LoudnessKind, PeakKind: result.PeakKind, ChannelScope: result.ChannelScope, Standard: result.Standard, IntegratedLUFS: result.IntegratedLUFS, TruePeakDBFS: result.TruePeakDBFS, Energy: energy, Sections: sections, CueSuggestions: cueSuggestions, AlgorithmVersion: artifact.AlgorithmVersion}
	measurement, measurementErr := a.db.GetTrackAnalysisArtifact(songID, features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion)
	if measurementErr != nil && !errors.Is(measurementErr, sql.ErrNoRows) {
		respondError(w, http.StatusInternalServerError, measurementErr.Error())
		return
	}
	if measurementErr == nil {
		if standards, valid := decodeCurrentBS1770Artifact(analysis, currentFingerprints[songID], measurement); valid {
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
	artifactSongIDs := make([]string, 0, len(artifacts)+1)
	artifactSongIDs = append(artifactSongIDs, songID)
	for _, artifact := range artifacts {
		artifactSongIDs = append(artifactSongIDs, artifact.SongID)
	}
	currentFingerprints, err := a.currentAnalysisSourceFingerprints(artifactSongIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sourceAnalysis, hasSourceAnalysis := analysisByID[songID]
	if !hasSourceAnalysis || !currentEnergyArtifactMatchesSource(sourceAnalysis, currentFingerprints[songID], sourceArtifact) {
		respondError(w, http.StatusConflict, "energy features are stale or cannot be verified for the current source")
		return
	}
	source, err := features.DecodeBounded(sourceArtifact.Data, features.MaxStructureStatusArtifactBytes)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	overrides, err := a.db.ListTrackAnalysisOverrides()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	manualFields, err := a.db.ListTrackMetadataOverrides()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "manual metadata unavailable")
		return
	}
	for id, fields := range manualFields {
		override := overrides[id]
		override.Fields = fields
		overrides[id] = override
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
	assemble := func(providerFields map[string][]db.SpotifyScalarField) (TransitionRecommendationsResponse, int, string) {
		providerScores := db.SpotifyScoreSummariesFromFields(providerFields)
		metadataByID := make(map[string]features.TransitionMetadata, len(analysisByID))
		for id, record := range analysisByID {
			metadataByID[id] = resolvedTransitionMetadata(record, overrides[id], currentFingerprints[id], providerFields[id])
		}
		assemblyStatus := http.StatusOK
		assemblyMessage := ""
		recommendations := make([]TransitionRecommendationResponse, 0, len(artifacts))
		candidatesBeforeFilters := 0
		checkedSourceIDs := []string{songID}
		for _, artifact := range artifacts {
			if artifact.SongID == songID {
				continue
			}
			analysisRecord, hasAnalysis := analysisByID[artifact.SongID]
			if !hasAnalysis || !currentEnergyArtifactMatchesSource(analysisRecord, currentFingerprints[artifact.SongID], artifact) {
				continue
			}
			candidate, err := features.DecodeBounded(artifact.Data, features.MaxStructureStatusArtifactBytes)
			if err != nil {
				continue // a corrupt candidate must not make the deck unavailable
			}
			song, exists := songByID[artifact.SongID]
			if !exists {
				continue
			}
			candidatesBeforeFilters++
			checkedSourceIDs = append(checkedSourceIDs, artifact.SongID)
			var providerScore *db.SpotifyScoreSummary
			if filters.SpotifyScoreMetric != "" {
				summary, exists := providerScores[artifact.SongID]["spotify_"+filters.SpotifyScoreMetric+"_score"]
				if !exists || !transitionSpotifyScoreMatches(summary, filters, time.Now()) {
					continue
				}
				providerScore = &summary
			}
			transitionMetadata := metadataByID[artifact.SongID]
			stemAvailable := stemStatuses[artifact.SongID] == "ready"
			if !transitionCandidateMatchesFilters(transitionMetadata, stemAvailable, filters) || !transitionLibrarySongMatchesFilters(song, playlistSongIDs, filters) {
				continue
			}
			if filters.NotRecentlyPlayedHours != nil {
				if _, playedRecently := recentlyPlayedIDs[song.ID]; playedRecently {
					continue
				}
			}
			score, scoreErr := features.ScoreTransitionWithMetadata(source, candidate, metadataByID[songID], metadataByID[artifact.SongID], intent)
			if scoreErr != nil {
				return TransitionRecommendationsResponse{}, http.StatusBadRequest, scoreErr.Error()
			}
			if filters.CamelotCompatible != nil && *filters.CamelotCompatible {
				if !validTransitionMetadataKey(metadataByID[songID]) || !validTransitionMetadataKey(metadataByID[artifact.SongID]) || !transitionCamelotCompatible(score.Vector.CamelotRelation) {
					continue
				}
			}
			evidence := TransitionCandidateEvidence{BPM: transitionMetadata.BPM, EnergyLevel: transitionMetadata.EnergyLevel, SpotifyScore: providerScore, SpotifyScoreMetric: filters.SpotifyScoreMetric}
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
		stillCurrent, sourceErr := a.transitionRecommendationSourcesCurrent(currentFingerprints, checkedSourceIDs)
		if sourceErr != nil {
			return TransitionRecommendationsResponse{}, http.StatusInternalServerError, sourceErr.Error()
		}
		if !stillCurrent {
			return TransitionRecommendationsResponse{}, http.StatusConflict, "a recommendation source changed during assembly; refresh recommendations"
		}
		return TransitionRecommendationsResponse{SongID: songID, Intent: intent, AlgorithmVersion: features.TransitionAlgorithmVersion, Filters: filters, CandidatesBeforeFilters: candidatesBeforeFilters, CandidatesAfterFilters: candidatesAfterFilters, Recommendations: recommendations}, assemblyStatus, assemblyMessage
	}

	a.spotifyAuthMu.Lock()
	runtime := a.spotifyAuth
	a.spotifyAuthMu.Unlock()
	if runtime == nil {
		providerFields, err := a.db.GetDownloadedSpotifyScalarCandidateBatch(currentFingerprints, time.Now())
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			logger.API("Optional Spotify score read failed; returning local transition recommendations: %v", err)
			providerFields = map[string][]db.SpotifyScalarField{}
		}
		response, status, message := assemble(providerFields)
		if status != http.StatusOK {
			respondError(w, status, message)
		} else {
			respondJSON(w, response)
		}
		return
	}
	scoreCtx, cancel := runtime.requestContext(r.Context())
	var response TransitionRecommendationsResponse
	responseStatus := http.StatusOK
	responseMessage := ""
	readErr := runtime.withMetadataRead(scoreCtx, func(fence db.SpotifyMetadataReadFence) error {
		providerFields := map[string][]db.SpotifyScalarField{}
		if !fence.Pending {
			providerFields, err = a.db.GetSpotifyScalarCandidateBatchForRuntime(fence, currentFingerprints, time.Now())
			if err != nil {
				return err
			}
		}
		response, responseStatus, responseMessage = assemble(providerFields)
		// Retirement cancels the captured lifetime before waiting for this read
		// fence. Reject a response assembled after that cancellation.
		if err := scoreCtx.Err(); err != nil {
			return err
		}
		// Keep account ownership fenced through serialization so a concurrent
		// replacement cannot publish this account's score evidence afterward.
		if responseStatus != http.StatusOK {
			respondError(w, responseStatus, responseMessage)
		} else {
			respondJSON(w, response)
		}
		return nil
	})
	cancel()
	if readErr != nil {
		if r.Context().Err() != nil {
			return
		}
		logger.API("Optional Spotify score read/fence failed; returning local transition recommendations: %v", readErr)
		response, status, message := assemble(nil)
		if status != http.StatusOK {
			respondError(w, status, message)
		} else {
			respondJSON(w, response)
		}
	}
}

func (a *API) transitionRecommendationSourcesCurrent(expected map[string]string, songIDs []string) (bool, error) {
	current, err := a.currentAnalysisSourceFingerprints(songIDs)
	if err != nil {
		return false, err
	}
	return transitionRecommendationSourcesMatch(expected, current, songIDs), nil
}

func transitionRecommendationSourcesMatch(expected, current map[string]string, songIDs []string) bool {
	for _, songID := range songIDs {
		if expected[songID] == "" || current[songID] != expected[songID] {
			return false
		}
	}
	return true
}

func parseTransitionRecommendationFilters(values url.Values) (TransitionRecommendationFilters, error) {
	var filters TransitionRecommendationFilters
	if metric, present, err := singleQueryValue(values, "spotifyScoreMetric"); err != nil {
		return filters, err
	} else if present {
		switch metric {
		case "energy", "danceability", "acousticness", "instrumentalness", "liveness", "speechiness", "valence":
			filters.SpotifyScoreMetric = metric
		default:
			return filters, errors.New("unsupported Spotify score metric")
		}
	}
	for _, name := range []string{"minSpotifyScore", "maxSpotifyScore"} {
		value, present, err := singleQueryValue(values, name)
		if err != nil {
			return filters, err
		}
		if !present {
			continue
		}
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1 {
			return filters, fmt.Errorf("%s must be between 0 and 1", name)
		}
		if filters.SpotifyScoreMetric == "" {
			return filters, errors.New("Spotify score range requires spotifyScoreMetric")
		}
		if name == "minSpotifyScore" {
			filters.MinSpotifyScore = &n
		} else {
			filters.MaxSpotifyScore = &n
		}
	}
	if filters.MinSpotifyScore != nil && filters.MaxSpotifyScore != nil && *filters.MinSpotifyScore > *filters.MaxSpotifyScore {
		return filters, errors.New("minimum Spotify score must not exceed maximum")
	}
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

func validTransitionKey(analysis db.TrackAnalysis, override db.TrackAnalysisOverride, currentFingerprint string) bool {
	key := db.ResolveEffectiveKeyForSource(db.EffectiveKeyInputs{Override: &override, Analysis: &analysis}, currentFingerprint)
	if key.Tonic == nil || key.Mode == nil || *key.Tonic < 0 || *key.Tonic > 11 {
		return false
	}
	return *key.Mode == analysiskey.ModeMajor || *key.Mode == analysiskey.ModeMinor
}

func validTransitionMetadataKey(metadata features.TransitionMetadata) bool {
	return metadata.CamelotKey != nil && metadata.KeySource != db.EffectiveBPMUnknown
}

func resolvedTransitionMetadata(analysis db.TrackAnalysis, override db.TrackAnalysisOverride, currentFingerprint string, providerCandidates ...[]db.SpotifyScalarField) features.TransitionMetadata {
	resolved := trackAnalysisFeatureResponseWithCurrentSource(analysis, override, currentFingerprint)
	var provider []db.SpotifyScalarField
	if len(providerCandidates) > 0 {
		provider = providerCandidates[0]
	}
	effective := db.ResolveAnalysisScalarFields(analysis, override, currentFingerprint, provider)
	applyEffectiveScalarCompatibilityFields(&resolved, effective)
	return features.TransitionMetadata{
		BPM: resolved.BPM, BPMSource: resolved.BPMSource, BPMConfidence: resolved.BPMConfidence,
		CamelotKey: resolved.CamelotKey, KeySource: resolved.KeySource, KeyConfidence: resolved.KeyConfidence,
		EnergyLevel: resolved.EnergyLevel, EnergyLevelSource: resolved.EnergyLevelSource, EnergyLevelConfidence: resolved.EnergyLevelConfidence,
	}
}

// measuredEnergyForDJ returns the same persisted curve summary exposed to the
// UI.  Corrupt individual artifacts are ignored rather than making the AI DJ
// unavailable; that song falls back to its existing metadata score.
func (a *API) measuredEnergyForDJ() (map[string]float64, error) {
	analyses, err := a.db.ListTrackAnalysis()
	if err != nil {
		return nil, err
	}
	analysisBySongID := make(map[string]db.TrackAnalysis, len(analyses))
	analysisIDs := make([]string, 0, len(analyses))
	for _, analysis := range analyses {
		analysisBySongID[analysis.SongID] = analysis
		analysisIDs = append(analysisIDs, analysis.SongID)
	}
	currentFingerprints, err := a.currentAnalysisSourceFingerprints(analysisIDs)
	if err != nil {
		return nil, err
	}
	artifacts, err := a.db.ListTrackAnalysisArtifacts(features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		return nil, err
	}
	values := make(map[string]float64, len(artifacts))
	for _, artifact := range artifacts {
		analysis, exists := analysisBySongID[artifact.SongID]
		if !exists || !currentEnergyArtifactMatchesSource(analysis, currentFingerprints[artifact.SongID], artifact) {
			continue
		}
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

// Optional native score filtering never changes the local transition score or grid.
func transitionSpotifyScoreMatches(score db.SpotifyScoreSummary, filters TransitionRecommendationFilters, now time.Time) bool {
	if score.Stale || !now.Before(score.ExpiresAt) || math.IsNaN(score.Value) || math.IsInf(score.Value, 0) || score.Value < 0 || score.Value > 1 {
		return false
	}
	return (filters.MinSpotifyScore == nil || score.Value >= *filters.MinSpotifyScore) && (filters.MaxSpotifyScore == nil || score.Value <= *filters.MaxSpotifyScore)
}
