package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/stems"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var stemSourceHashes = stems.NewSourceHashCache()

type stemStatusResponse struct {
	SongID      string            `json:"songId"`
	Status      string            `json:"status"`
	ActiveSetID string            `json:"activeSetId,omitempty"`
	StemSets    []stemSetResponse `json:"stemSets"`
}
type stemSetResponse struct {
	ID               string                 `json:"id"`
	Status           string                 `json:"status"`
	SourceAudioHash  string                 `json:"sourceAudioHash,omitempty"`
	ModelName        string                 `json:"modelName,omitempty"`
	ModelVersion     string                 `json:"modelVersion,omitempty"`
	GeneratorName    string                 `json:"generatorName,omitempty"`
	GeneratorVersion string                 `json:"generatorVersion,omitempty"`
	Layout           string                 `json:"stemLayout,omitempty"`
	SampleRate       int                    `json:"sampleRate,omitempty"`
	Channels         int                    `json:"channels,omitempty"`
	Frames           int64                  `json:"frames,omitempty"`
	DurationSeconds  float64                `json:"durationSeconds,omitempty"`
	ExplicitlyLinked bool                   `json:"explicitlyLinked"`
	DiscoverySource  string                 `json:"discoverySource,omitempty"`
	ErrorCode        string                 `json:"validationErrorCode,omitempty"`
	ErrorMessage     string                 `json:"validationErrorMessage,omitempty"`
	Stems            []stemArtifactResponse `json:"stems,omitempty"`
}
type stemArtifactResponse struct {
	Name       string `json:"name"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"sizeBytes"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
	Frames     int64  `json:"frames"`
	Encoding   string `json:"encoding"`
}

// V2StemRoutes exposes package registry management. File paths are accepted for
// configuring/linking local packages, but are not returned in per-track status.
func (a *API) V2StemRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/locations", a.listStemLocationsV2)
	r.Put("/locations", a.putStemLocationsV2)
	r.Post("/scan", a.scanStemLibrariesV2)
	r.Get("/{songID}/{stemSetID}/frames", a.getStemFramesV2)
	r.Get("/{songID}/{stemSetID}/{stemName}", a.getStemPreviewV2)
	r.Get("/{songID}", a.getStemStatusV2)
	r.Post("/{songID}/link", a.linkStemPackageV2)
	r.Post("/{songID}/refresh", a.refreshStemStatusV2)
	r.Delete("/{songID}/{stemSetID}", a.unlinkStemPackageV2)
	return r
}

func (a *API) listStemLocationsV2(w http.ResponseWriter, r *http.Request) {
	locations, err := a.db.ListStemLocations()
	if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_registry_unavailable", "Unable to load Stem Library locations", true)
		return
	}
	respondV2JSON(w, http.StatusOK, map[string]any{"locations": locations})
}
func (a *API) putStemLocationsV2(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Locations []string `json:"locations"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req) != nil {
		stemError(w, r, http.StatusBadRequest, "invalid_request", "Stem Library locations are not valid JSON", false)
		return
	}
	if len(req.Locations) > 128 {
		stemError(w, r, http.StatusBadRequest, "too_many_locations", "At most 128 Stem Library locations are allowed", false)
		return
	}
	locations := make([]db.StemLocation, 0, len(req.Locations))
	seen := map[string]bool{}
	for _, raw := range req.Locations {
		p := strings.TrimSpace(raw)
		if p == "" {
			stemError(w, r, http.StatusBadRequest, "invalid_location", "A Stem Library path is empty", false)
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			stemError(w, r, http.StatusBadRequest, "invalid_location", "A Stem Library path is invalid", false)
			return
		}
		abs = filepath.Clean(abs)
		key := strings.ToLower(abs)
		if seen[key] {
			continue
		}
		seen[key] = true
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			stemError(w, r, http.StatusBadRequest, "invalid_location", "Every Stem Library location must be an existing directory", false)
			return
		}
		locations = append(locations, db.StemLocation{ID: uuid.NewString(), Path: abs, Enabled: true})
	}
	musicFolders, err := a.db.GetScanFolders()
	if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_location_update_failed", "Unable to check Music Folders", true)
		return
	}
	stemPaths := make([]string, 0, len(locations))
	for _, location := range locations {
		stemPaths = append(stemPaths, location.Path)
	}
	if err = validateStemLibraryRootsAgainstMusic(musicFolders, stemPaths); err != nil {
		stemError(w, r, http.StatusBadRequest, "stem_music_root_overlap", err.Error(), false)
		return
	}
	if err := a.db.SetStemLocations(locations); err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_location_update_failed", "Unable to save Stem Library locations", true)
		return
	}
	respondV2JSON(w, http.StatusOK, map[string]any{"locations": locations})
}

func (a *API) scanStemLibrariesV2(w http.ResponseWriter, r *http.Request) {
	locations, err := a.db.ListStemLocations()
	if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_registry_unavailable", "Unable to load Stem Library locations", true)
		return
	}
	roots := make([]string, 0, len(locations))
	for _, location := range locations {
		if location.Enabled {
			roots = append(roots, location.Path)
		}
	}
	if len(roots) == 0 {
		stemError(w, r, http.StatusConflict, "stem_library_empty", "Add a Stem Library location before scanning", false)
		return
	}
	roots, err = a.validateStemLibraryScanRoots(roots)
	if err != nil {
		stemError(w, r, http.StatusConflict, "stem_library_configuration_changed", err.Error(), false)
		return
	}
	raw, err := json.Marshal(map[string][]string{"locations": roots})
	if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_job_create_failed", "Unable to prepare Stem Library scan", true)
		return
	}
	job := db.Job{ID: uuid.NewString(), Type: "stem_library_scan", Parameters: raw, Priority: 10, Message: "Queued Stem Library scan"}
	if err = a.db.CreateJob(job); err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_job_create_failed", "Unable to queue Stem Library scan", true)
		return
	}
	a.wakeJobScheduler()
	respondV2JSON(w, http.StatusAccepted, map[string]string{"jobId": job.ID, "status": "accepted"})
}

func (a *API) getStemStatusV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		stemError(w, r, http.StatusBadRequest, "song_id_required", "Song ID is required", false)
		return
	}
	if song, err := a.db.GetSongByID(songID); errors.Is(err, sql.ErrNoRows) || song == nil {
		stemError(w, r, http.StatusNotFound, "song_not_found", "Song was not found", false)
		return
	} else if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_registry_unavailable", "Unable to load track", true)
		return
	}
	a.writeStemStatus(w, r, songID)
}
func (a *API) refreshStemStatusV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	if songID == "" {
		stemError(w, r, http.StatusBadRequest, "song_id_required", "Song ID is required", false)
		return
	}
	if _, err := a.db.GetSongByID(songID); errors.Is(err, sql.ErrNoRows) {
		stemError(w, r, http.StatusNotFound, "song_not_found", "Song was not found", false)
		return
	} else if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_registry_unavailable", "Unable to load track", true)
		return
	}
	a.enqueueStemJob(w, r, "stem_registry_refresh", map[string]string{"songId": songID})
}
func (a *API) writeStemStatus(w http.ResponseWriter, r *http.Request, songID string) {
	sets, err := a.db.ListStemSets(songID)
	if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_registry_unavailable", "Unable to load stem package status", true)
		return
	}
	out := stemStatusResponse{SongID: songID, Status: "none", StemSets: []stemSetResponse{}}
	song, songErr := a.db.GetSongByID(songID)
	sourceUnavailable := songErr == nil && (song.Source == "plex" || strings.TrimSpace(song.FilePath) == "")
	if songErr == nil && !sourceUnavailable {
		if _, err := os.Stat(song.FilePath); err != nil {
			sourceUnavailable = true
		}
	}
	if sourceUnavailable {
		out.Status = "unavailable"
	}
	for _, s := range sets {
		v := stemSetResponse{ID: s.ID, Status: s.Status, SourceAudioHash: s.SourceAudioHash, ModelName: s.ModelName, ModelVersion: s.ModelVersion, GeneratorName: s.GeneratorName, GeneratorVersion: s.GeneratorVersion, Layout: s.Layout, SampleRate: s.SampleRate, Channels: s.Channels, Frames: s.Frames, DurationSeconds: s.DurationSeconds, ExplicitlyLinked: s.ExplicitlyLinked, DiscoverySource: s.DiscoverySource, ErrorCode: s.ValidationErrorCode, ErrorMessage: s.ValidationErrorMessage}
		for _, art := range s.Stems {
			v.Stems = append(v.Stems, stemArtifactResponse{Name: art.Name, SHA256: art.SHA256, SizeBytes: art.SizeBytes, SampleRate: art.SampleRate, Channels: art.Channels, Frames: art.Frames, Encoding: art.Encoding})
		}
		out.StemSets = append(out.StemSets, v)
		if out.ActiveSetID == "" && (s.ExplicitlyLinked || s.Status == "ready") {
			out.ActiveSetID = s.ID
		}
		if !sourceUnavailable && (out.Status == "none" || s.Status == "ready") {
			out.Status = s.Status
		}
	}
	respondV2JSON(w, http.StatusOK, out)
}

func (a *API) linkStemPackageV2(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	var req struct {
		PackagePath string `json:"packagePath"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req) != nil || strings.TrimSpace(req.PackagePath) == "" {
		stemError(w, r, http.StatusBadRequest, "invalid_request", "A packagePath is required", false)
		return
	}
	song, err := a.db.GetSongByID(songID)
	if err != nil || song == nil {
		stemError(w, r, http.StatusNotFound, "song_not_found", "Song was not found", false)
		return
	}
	if song.Source == "plex" || song.FilePath == "" {
		stemError(w, r, http.StatusConflict, "stem_source_unavailable", "Stems require a local source audio file", false)
		return
	}
	if _, err := os.Stat(song.FilePath); err != nil {
		stemError(w, r, http.StatusConflict, "stem_source_unavailable", "The local source audio file is unavailable", false)
		return
	}
	abs, err := filepath.Abs(req.PackagePath)
	if err != nil {
		stemError(w, r, http.StatusBadRequest, "invalid_package_path", "Package path is invalid", false)
		return
	}
	if !strings.HasSuffix(strings.ToLower(abs), ".viibstems") {
		stemError(w, r, http.StatusBadRequest, "invalid_package_path", "Package directory must end in .viibstems", false)
		return
	}
	a.enqueueStemJob(w, r, "stem_package_link", map[string]string{"songId": songID, "packagePath": filepath.Clean(abs)})
}
func (a *API) unlinkStemPackageV2(w http.ResponseWriter, r *http.Request) {
	songID, id := chi.URLParam(r, "songID"), chi.URLParam(r, "stemSetID")
	sets, err := a.db.ListStemSets(songID)
	if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_registry_unavailable", "Unable to load stem package status", true)
		return
	}
	for _, s := range sets {
		if s.ID == id && s.ExplicitlyLinked {
			_ = a.db.ClearExplicitStemLinks(songID)
		}
	}
	err = a.db.DeleteStemSet(songID, id)
	if errors.Is(err, sql.ErrNoRows) {
		stemError(w, r, http.StatusNotFound, "stem_set_not_found", "Stem package was not found", false)
		return
	}
	if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_unlink_failed", "Unable to unlink stem package", true)
		return
	}
	a.writeStemStatus(w, r, songID)
}

func (a *API) refreshStemRegistry(songID string) error {
	return a.refreshStemRegistryContext(context.Background(), songID)
}

func (a *API) refreshStemRegistryContext(ctx context.Context, songID string) error {
	locations, err := a.db.ListStemLocations()
	if err != nil {
		return err
	}
	dirs := make([]string, 0, len(locations))
	for _, location := range locations {
		if location.Enabled {
			dirs = append(dirs, location.Path)
		}
	}
	return a.refreshStemRegistryWithDiscoveryContext(ctx, songID, nil, dirs)
}

func (a *API) refreshStemRegistryWithDiscoveryContext(ctx context.Context, songID string, knownDiscovery *stems.DiscoveryResult, libraryDirs []string) error {
	song, err := a.db.GetSongByID(songID)
	if err != nil {
		return err
	}
	if song == nil {
		return sql.ErrNoRows
	}
	if song.Source == "plex" || song.FilePath == "" {
		return nil
	}
	existing, err := a.db.ListStemSets(songID)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(song.FilePath); statErr != nil {
		for _, old := range existing {
			old.Status = "unavailable"
			if err = a.db.UpsertStemSet(old); err != nil {
				return err
			}
		}
		return nil
	}
	hash, hashErr := stemSourceHashes.SHA256Context(ctx, song.FilePath)
	if hashErr != nil {
		return hashErr
	}
	var discovery stems.DiscoveryResult
	if knownDiscovery != nil {
		discovery = *knownDiscovery
	} else {
		var discoveryErr error
		discovery, discoveryErr = stems.DiscoverPackagesContext(ctx, song.FilePath, libraryDirs)
		if discoveryErr != nil {
			return discoveryErr
		}
	}
	existingByPath := map[string]db.StemSet{}
	for _, s := range existing {
		existingByPath[strings.ToLower(filepath.Clean(s.PackagePath))] = s
	}
	seen := map[string]bool{}
	storeCandidate := func(candidate stems.PackageCandidate) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		path, source, validation := candidate.Path, string(candidate.Source), candidate.Validation
		key := strings.ToLower(filepath.Clean(path))
		seen[key] = true
		m := validation.Manifest
		state := "stale"
		if candidate.SourceIdentityChecked {
			if candidate.SourceIdentityMatches {
				state = "ready"
			}
		} else if strings.EqualFold(m.Source.SHA256, hash) || stemAudioIdentityMatches(song, m) {
			state = "ready"
		}
		old := existingByPath[key]
		set := stemSetFromValidation(songID, path, source, m, validation.Files, old.ExplicitlyLinked, state, "")
		if old.ID != "" {
			set.ID = old.ID
		}
		return a.db.UpsertStemSet(set)
	}
	// An explicitly linked package remains the top-priority candidate even when
	// it lives outside the configured Stem Libraries.
	for _, old := range existing {
		if err = ctx.Err(); err != nil {
			return err
		}
		if !old.ExplicitlyLinked {
			continue
		}
		var validation stems.Validation
		var validationErr error
		validationReused := false
		var explicitCandidate stems.PackageCandidate
		for _, existingCandidate := range discovery.Candidates {
			if strings.EqualFold(filepath.Clean(existingCandidate.Path), filepath.Clean(old.PackagePath)) {
				explicitCandidate = existingCandidate
				validation = existingCandidate.Validation
				validationReused = true
				break
			}
		}
		if !validationReused {
			validation, validationErr = stems.ValidatePlayablePackageContext(ctx, old.PackagePath)
			explicitCandidate = stems.PackageCandidate{Path: old.PackagePath, Source: stems.CandidateAdjacent, Validation: validation}
		}
		if validationErr == nil {
			explicitCandidate.Path = old.PackagePath
			explicitCandidate.Source = stems.CandidateAdjacent
			if !explicitCandidate.SourceIdentityChecked {
				matched, matchErr := stemSourceMatchesContext(ctx, song, validation.Manifest, hash)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				explicitCandidate.SourceIdentityChecked = true
				explicitCandidate.SourceIdentityMatches = matchErr == nil && matched
			}
			if err = storeCandidate(explicitCandidate); err != nil {
				return err
			}
		} else {
			_ = a.db.UpsertStemSet(db.StemSet{ID: old.ID, SongID: songID, PackagePath: old.PackagePath, DiscoverySource: "explicit", Status: "invalid", ExplicitlyLinked: true, ValidationErrorCode: "package_validation_failed", ValidationErrorMessage: "Package validation failed"})
			seen[strings.ToLower(filepath.Clean(old.PackagePath))] = true
		}
	}
	for _, candidate := range discovery.Candidates {
		if err = ctx.Err(); err != nil {
			return err
		}
		if seen[strings.ToLower(filepath.Clean(candidate.Path))] {
			continue
		}
		if err = storeCandidate(candidate); err != nil {
			return err
		}
	}
	for _, rejected := range discovery.Rejected {
		if err = ctx.Err(); err != nil {
			return err
		}
		if !strings.HasSuffix(strings.ToLower(rejected.Path), ".viibstems") {
			continue
		}
		if _, statErr := os.Stat(rejected.Path); statErr != nil {
			continue
		}
		key := strings.ToLower(filepath.Clean(rejected.Path))
		seen[key] = true
		old := existingByPath[key]
		set := db.StemSet{ID: old.ID, SongID: songID, PackagePath: rejected.Path, DiscoverySource: string(rejected.Source), Status: "invalid", ValidationErrorCode: "package_validation_failed", ValidationErrorMessage: "Package validation failed"}
		if old.ExplicitlyLinked {
			set.ExplicitlyLinked = true
		}
		if err = a.db.UpsertStemSet(set); err != nil {
			return err
		}
	}
	for key, old := range existingByPath {
		if err = ctx.Err(); err != nil {
			return err
		}
		if !seen[key] {
			old.Status = "unavailable"
			if err = a.db.UpsertStemSet(old); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *API) runStemRegistryJob(job db.Job) {
	if a.analysisThrottled(job.Priority) {
		_, _ = a.db.RequeueJob(job.ID, "Waiting for DJ playback to finish before validating stem packages", analysisDeferBackoff)
		return
	}
	var params struct {
		SongID      string `json:"songId"`
		PackagePath string `json:"packagePath"`
	}
	if json.Unmarshal(job.Parameters, &params) != nil || params.SongID == "" {
		_ = a.db.FailJob(job.ID, "invalid_stem_job", "Stem operation parameters are invalid")
		return
	}
	if job.Type == "stem_package_link" {
		_ = a.db.UpdateJobProgress(job.ID, 0, 1, "Validating linked stem package")
		song, err := a.db.GetSongByID(params.SongID)
		if err != nil || song == nil {
			_ = a.db.FailJob(job.ID, "song_not_found", "Song was not found")
			return
		}
		candidate, err := stems.ValidatePlayablePackage(params.PackagePath)
		if err != nil {
			_ = a.db.FailJob(job.ID, "invalid_stem_package", "Stem package validation failed")
			return
		}
		if !stemSourceMatches(song, candidate.Manifest) {
			_ = a.db.FailJob(job.ID, "stem_source_mismatch", "Package source SHA-256 does not match this track")
			return
		}
		if err = a.db.ClearExplicitStemLinks(params.SongID); err != nil {
			_ = a.db.FailJob(job.ID, "stem_link_failed", "Unable to select linked package")
			return
		}
		set := stemSetFromValidation(params.SongID, params.PackagePath, "explicit", candidate.Manifest, candidate.Files, true, "ready", "")
		if err = a.db.UpsertStemSet(set); err != nil {
			_ = a.db.FailJob(job.ID, "stem_link_failed", "Unable to persist linked package")
			return
		}
		_ = a.db.CompleteJob(job.ID, map[string]string{"songId": params.SongID, "stemSetId": set.ID}, "Stem package validated and linked")
		return
	}
	_ = a.db.UpdateJobProgress(job.ID, 0, 1, "Discovering and validating stem packages")
	ctx := context.Background()
	song, err := a.db.GetSongByID(params.SongID)
	if err != nil || song == nil {
		_ = a.db.FailJob(job.ID, "song_not_found", "Song was not found")
		return
	}
	locations, err := a.db.ListStemLocations()
	if err != nil {
		_ = a.db.FailJob(job.ID, "stem_refresh_failed", "Stem package discovery failed")
		return
	}
	dirs := make([]string, 0, len(locations))
	for _, location := range locations {
		if location.Enabled {
			dirs = append(dirs, location.Path)
		}
	}
	var uncheckedGeometries int
	if song.Source != "plex" && song.FilePath != "" {
		sourceHash, hashErr := stemSourceHashes.SHA256Context(ctx, song.FilePath)
		if hashErr != nil {
			_ = a.db.FailJob(job.ID, "stem_refresh_failed", "Stem package discovery failed")
			return
		}
		discovery, discoveryErr := stems.DiscoverPackagesContext(ctx, song.FilePath, dirs)
		if discoveryErr != nil {
			_ = a.db.FailJob(job.ID, "stem_refresh_failed", "Stem package discovery failed")
			return
		}
		uncheckedGeometries, err = annotateStemCandidateIdentities(ctx, song.FilePath, sourceHash, discovery.Candidates)
		if err == nil {
			err = a.refreshStemRegistryWithDiscoveryContext(ctx, params.SongID, &discovery, dirs)
		}
	} else {
		err = a.refreshStemRegistryWithDiscoveryContext(ctx, params.SongID, nil, dirs)
	}
	if err != nil {
		_ = a.db.FailJob(job.ID, "stem_refresh_failed", "Stem package discovery failed")
		return
	}
	message := "Stem package registry refreshed"
	if uncheckedGeometries > 0 {
		message = fmt.Sprintf("%s; PCM32 fallback skipped %d additional package audio geometries, so packages matching only those layouts may remain unmatched or stale", message, uncheckedGeometries)
	}
	_ = a.db.CompleteJob(job.ID, map[string]string{"songId": params.SongID, "audioIdentityGeometriesUnchecked": fmt.Sprint(uncheckedGeometries)}, message)
}

func stemSetFromValidation(songID, path, source string, m stems.Manifest, files map[stems.StemName]string, linked bool, status, message string) db.StemSet {
	set := db.StemSet{ID: uuid.NewString(), SongID: songID, SourceAudioHash: m.Source.SHA256, AudioSHA256: m.Source.AudioSHA256, ModelName: m.Model.Name, ModelVersion: m.Model.Version, GeneratorName: m.Generator.Name, GeneratorVersion: m.Generator.Version, Layout: string(m.StemLayout), Status: status, SampleRate: m.Audio.SampleRate, Channels: m.Audio.Channels, Frames: m.Audio.Frames, DurationSeconds: m.Source.Duration, DecoderDelayFrames: m.Timing.DecoderDelayFrames, StartTrimFrames: m.Timing.StartTrimFrames, PackagePath: path, DiscoverySource: source, ManifestSchemaVersion: m.SchemaVersion, GeneratedAt: "", ExplicitlyLinked: linked, ValidationErrorMessage: message}
	for name, artifact := range m.Stems {
		set.Stems = append(set.Stems, db.StemArtifact{Name: string(name), RelativePath: artifact.Path, SHA256: artifact.SHA256, SizeBytes: artifact.SizeBytes, SampleRate: artifact.SampleRate, Channels: artifact.Channels, Frames: artifact.Frames, Encoding: artifact.Encoding})
		_ = files[name]
	}
	return set
}

func stemError(w http.ResponseWriter, r *http.Request, status int, code, message string, retryable bool) {
	respondV2Error(w, r, status, code, message, retryable, nil)
}

func (a *API) enqueueStemJob(w http.ResponseWriter, r *http.Request, kind string, params map[string]string) {
	raw, _ := json.Marshal(params)
	job := db.Job{ID: uuid.NewString(), Type: kind, Parameters: raw, Priority: 10, Message: "Queued stem package validation"}
	if err := a.db.CreateJob(job); err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_job_create_failed", "Unable to queue stem package work", true)
		return
	}
	a.wakeJobScheduler()
	respondV2JSON(w, http.StatusAccepted, map[string]string{"jobId": job.ID, "status": "accepted"})
}
