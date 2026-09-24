package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	analysiscues "github.com/ajbergh/viib-mediahub/internal/analysis/cues"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
)

var errStaleAnalysisCueSource = errors.New("cue analysis is stale for the current audio source")

type AnalysisCueListResponse struct {
	SongID              string                   `json:"songId"`
	GeneratorVersion    string                   `json:"generatorVersion"`
	SourceFingerprint   string                   `json:"sourceFingerprint"`
	DefaultApplyMode    string                   `json:"defaultApplyMode"`
	HotCues             []HotCue                 `json:"hotCues"`
	GeneratedCandidates []analysiscues.Cue       `json:"generatedCandidates"`
	Suppressions        []db.DJHotCueSuppression `json:"suppressions"`
}

type AnalysisCueApplyRequest struct {
	Mode          string `json:"mode"`
	SelectedSlots []int  `json:"selectedSlots,omitempty"`
}

type AnalysisCueApplyResponse struct {
	AnalysisCueListResponse
	AppliedSlots []int `json:"appliedSlots"`
	BlockedSlots []int `json:"blockedSlots"`
}

func (a *API) listAnalysisCuesV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	response, err := a.analysisCueList(songID)
	if errors.Is(err, errStaleAnalysisCueSource) {
		respondError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "cue analysis not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, response)
}

func (a *API) applyAnalysisCuesV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		respondError(w, http.StatusBadRequest, "song ID is required")
		return
	}
	var request AnalysisCueApplyRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "invalid cue apply request")
		return
	}
	mode := db.GeneratedCueMode(request.Mode)
	if mode != db.GeneratedCueFillEmpty && mode != db.GeneratedCueReplaceGenerated && mode != db.GeneratedCueSelectedOnly {
		respondError(w, http.StatusBadRequest, "mode must be fill-empty, replace-generated, or selected-only")
		return
	}

	response, err := a.analysisCueList(songID)
	if errors.Is(err, errStaleAnalysisCueSource) {
		respondError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "cue analysis not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	selected := make(map[int]struct{}, len(request.SelectedSlots))
	for _, slot := range request.SelectedSlots {
		if slot < 1 || slot > 8 {
			respondError(w, http.StatusBadRequest, "selected cue slots must be between 1 and 8")
			return
		}
		if _, exists := selected[slot]; exists {
			respondError(w, http.StatusBadRequest, "selected cue slots must be unique")
			return
		}
		selected[slot] = struct{}{}
	}
	if mode == db.GeneratedCueSelectedOnly && len(selected) == 0 {
		respondError(w, http.StatusBadRequest, "selected-only mode requires selectedSlots")
		return
	}

	candidates := make([]analysiscues.Cue, 0, len(response.GeneratedCandidates))
	for _, candidate := range response.GeneratedCandidates {
		if mode == db.GeneratedCueSelectedOnly {
			if _, include := selected[candidate.Slot]; !include {
				continue
			}
		}
		candidates = append(candidates, candidate)
	}
	dbCues := make([]db.DJHotCue, 0, len(candidates))
	for _, candidate := range candidates {
		confidence := candidate.Confidence
		dbCues = append(dbCues, db.DJHotCue{
			Slot: candidate.Slot, Position: candidate.Position, Label: candidate.Label, Color: candidate.Color,
			Origin: candidate.Origin, GeneratorVersion: candidate.GeneratorVersion, Confidence: &confidence,
			Kind: candidate.Kind, Locked: candidate.Locked, Rationale: candidate.Rationale,
			SourceFingerprint: candidate.SourceFingerprint, DownbeatAligned: candidate.DownbeatAligned,
		})
	}
	if err := a.db.ApplyGeneratedDJHotCues(songID, dbCues, mode); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	response, err = a.analysisCueList(songID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := AnalysisCueApplyResponse{AnalysisCueListResponse: response, AppliedSlots: []int{}, BlockedSlots: []int{}}
	bySlot := make(map[int]HotCue, len(response.HotCues))
	for _, cue := range response.HotCues {
		bySlot[cue.Slot] = cue
	}
	for _, candidate := range candidates {
		stored, exists := bySlot[candidate.Slot]
		if exists && stored.Origin == "analysis" && stored.GeneratorVersion == candidate.GeneratorVersion && stored.SourceFingerprint == candidate.SourceFingerprint && stored.Position == candidate.Position {
			result.AppliedSlots = append(result.AppliedSlots, candidate.Slot)
		} else {
			result.BlockedSlots = append(result.BlockedSlots, candidate.Slot)
		}
	}
	respondJSON(w, result)
}

func (a *API) analysisCueList(songID string) (AnalysisCueListResponse, error) {
	song, err := a.db.GetSongByID(songID)
	if err != nil {
		return AnalysisCueListResponse{}, err
	}
	if song == nil {
		return AnalysisCueListResponse{}, sql.ErrNoRows
	}
	trackAnalysis, err := a.db.GetTrackAnalysis(songID)
	if err != nil {
		return AnalysisCueListResponse{}, err
	}
	resolved, err := analysis.ResolveLocalSource(a.db, songID)
	if err != nil || resolved.Fingerprint != trackAnalysis.SourceFingerprint {
		return AnalysisCueListResponse{}, errStaleAnalysisCueSource
	}
	energyArtifact, err := a.db.GetTrackAnalysisArtifact(songID, features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		return AnalysisCueListResponse{}, err
	}
	structure, err := features.Decode(energyArtifact.Data)
	if err != nil {
		return AnalysisCueListResponse{}, err
	}
	var grid *beatgrid.Grid
	gridArtifact, err := a.db.GetTrackAnalysisArtifact(songID, beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if err == nil {
		decoded, decodeErr := beatgrid.Decode(gridArtifact.Data)
		if decodeErr != nil {
			return AnalysisCueListResponse{}, decodeErr
		}
		decoded.Provenance = beatgrid.Provenance(gridArtifact.Provenance)
		grid = &decoded
	} else if !errors.Is(err, sql.ErrNoRows) {
		return AnalysisCueListResponse{}, err
	}
	candidates, err := analysiscues.Generate(song.Duration, grid, structure, trackAnalysis.SourceFingerprint)
	if err != nil {
		return AnalysisCueListResponse{}, err
	}
	persisted, err := a.db.GetDJHotCues(songID)
	if err != nil {
		return AnalysisCueListResponse{}, err
	}
	response := AnalysisCueListResponse{
		SongID: songID, GeneratorVersion: analysiscues.GeneratorVersion,
		SourceFingerprint: trackAnalysis.SourceFingerprint, DefaultApplyMode: string(db.GeneratedCueFillEmpty),
		HotCues: make([]HotCue, 0, len(persisted)), GeneratedCandidates: candidates,
		Suppressions: []db.DJHotCueSuppression{},
	}
	for _, cue := range persisted {
		response.HotCues = append(response.HotCues, HotCue{
			Slot: cue.Slot, Position: cue.Position, Label: cue.Label, Color: cue.Color,
			Origin: cue.Origin, GeneratorVersion: cue.GeneratorVersion, Confidence: cue.Confidence,
			Kind: cue.Kind, Locked: cue.Locked, Rationale: cue.Rationale,
			SourceFingerprint: cue.SourceFingerprint, DownbeatAligned: cue.DownbeatAligned, UpdatedAt: cue.UpdatedAt,
		})
	}
	response.Suppressions, err = a.db.GetDJHotCueSuppressions(songID)
	return response, err
}
