// Defines v2 stem library job functionality for package api.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/stems"
	"github.com/google/uuid"
)

// PCM32 fallback is evaluated once per source/layout, not once per package.
// Bound the layouts a configured root can trigger; exact full-file hashes are
// indexed and checked for every validated candidate regardless of this cap.
const maxStemLibraryIdentityGeometries = 16

func (a *API) queueStartupStemScan() {
	locations, err := a.db.ListStemLocations()
	if err != nil {
		logger.API("Unable to load startup Stem Libraries: %v", err)
		return
	}
	roots := make([]string, 0, len(locations))
	for _, location := range locations {
		if location.Enabled {
			roots = append(roots, location.Path)
		}
	}
	// Validation happens in the worker so startup never waits on stem I/O.
	raw, _ := json.Marshal(map[string][]string{"locations": roots})
	_, err = a.db.QueueStemLibraryScan(db.Job{ID: uuid.NewString(), Parameters: raw, Priority: 10, Message: "Queued startup Stem Library scan"})
	if err != nil {
		logger.API("Unable to queue startup Stem Library scan: %v", err)
		return
	}
	a.wakeJobScheduler()
}

func (a *API) updateStemScanProgress(jobID string, current, total int64, message string) bool {
	if err := a.db.UpdateJobProgress(jobID, current, total, message); err != nil {
		logger.API("Stem Library scan %s progress failed: %v", jobID, err)
		_ = a.db.FailJob(jobID, "stem_library_progress_failed", "Unable to save Stem Library scan progress. Please retry the scan.")
		return false
	}
	return true
}

func (a *API) stemScanThrottled(job db.Job) bool {
	if !a.analysisThrottled(job.Priority) {
		return false
	}
	// A manual request can promote an already running startup scan.
	if current, err := a.db.GetJob(job.ID); err == nil && current.Priority >= analysisForegroundPriority {
		return false
	}
	return true
}

type stemAudioGeometry struct {
	sampleRate int
	channels   int
}

type stemAudioIdentityKey struct {
	geometry stemAudioGeometry
	hash     string
}

// annotateStemCandidateIdentities checks exact source hashes first, then
// evaluates the PCM32 fallback once per source geometry. Geometries past the
// cap are explicitly marked checked-but-unmatched so registry refresh cannot
// fall back to decoding once per package.
func annotateStemCandidateIdentities(ctx context.Context, sourcePath, sourceHash string, candidates []stems.PackageCandidate) (int, error) {
	geometriesSet := make(map[stemAudioGeometry]struct{})
	for i := range candidates {
		manifest := candidates[i].Validation.Manifest
		if strings.EqualFold(sourceHash, manifest.Source.SHA256) {
			candidates[i].SourceIdentityChecked = true
			candidates[i].SourceIdentityMatches = true
			continue
		}
		if manifest.Source.AudioSHA256 != "" && manifest.Audio.SampleRate > 0 && manifest.Audio.Channels > 0 {
			geometriesSet[stemAudioGeometry{sampleRate: manifest.Audio.SampleRate, channels: manifest.Audio.Channels}] = struct{}{}
		}
	}
	geometries := make([]stemAudioGeometry, 0, len(geometriesSet))
	for geometry := range geometriesSet {
		geometries = append(geometries, geometry)
	}
	sort.Slice(geometries, func(i, j int) bool {
		if geometries[i].sampleRate != geometries[j].sampleRate {
			return geometries[i].sampleRate < geometries[j].sampleRate
		}
		return geometries[i].channels < geometries[j].channels
	})
	unchecked := 0
	if len(geometries) > maxStemLibraryIdentityGeometries {
		unchecked = len(geometries) - maxStemLibraryIdentityGeometries
		geometries = geometries[:maxStemLibraryIdentityGeometries]
	}
	matchedByGeometry := make(map[stemAudioGeometry]string, len(geometries))
	resolved := analysis.ResolvedSource{Name: filepath.Base(sourcePath), Path: sourcePath}
	for _, geometry := range geometries {
		if err := ctx.Err(); err != nil {
			return unchecked, err
		}
		hash, err := stemSourceAudioHashes.SHA256(ctx, decoderRegistry(), resolved, geometry.sampleRate, geometry.channels)
		if err != nil {
			if ctx.Err() != nil {
				return unchecked, ctx.Err()
			}
			// Match the previous per-candidate fallback behavior: an unsupported
			// geometry is simply not an identity match.
			continue
		}
		matchedByGeometry[geometry] = strings.ToLower(hash)
	}
	for i := range candidates {
		if candidates[i].SourceIdentityChecked {
			continue
		}
		manifest := candidates[i].Validation.Manifest
		geometry := stemAudioGeometry{sampleRate: manifest.Audio.SampleRate, channels: manifest.Audio.Channels}
		candidates[i].SourceIdentityChecked = true
		candidates[i].SourceIdentityMatches = strings.EqualFold(matchedByGeometry[geometry], manifest.Source.AudioSHA256) && manifest.Source.AudioSHA256 != ""
	}
	return unchecked, nil
}

func (a *API) runStemLibraryScanJob(job db.Job) {
	// Every exit must settle or requeue the job, including unexpected persistence
	// errors. Otherwise Settings would keep displaying a running job forever.
	defer func() {
		current, err := a.db.GetJob(job.ID)
		if err == nil && current.Status == db.JobStatusRunning {
			_ = a.db.FailJob(job.ID, "stem_library_scan_incomplete", "Stem Library scan stopped before completion. Please retry.")
		} else if err == nil && current.Status == db.JobStatusCanceling {
			a.cancelStemLibraryJob(job.ID, "Stem Library scan canceled")
		}
	}()
	var params struct {
		Locations []string `json:"locations"`
	}
	if err := json.Unmarshal(job.Parameters, &params); err != nil {
		_ = a.db.FailJob(job.ID, "invalid_stem_library_scan", "Stem Library scan locations are missing")
		return
	}
	locations, err := a.validateStemLibraryScanRoots(params.Locations)
	if err != nil {
		_ = a.db.FailJob(job.ID, "stem_library_roots_invalid", err.Error())
		return
	}
	ctx, stopWatchingCancellation := a.stemLibraryScanContext(job.ID)
	defer stopWatchingCancellation()
	if a.jobCancellationRequested(job.ID) {
		a.cancelStemLibraryJob(job.ID, "Stem Library scan canceled before package discovery")
		return
	}
	if a.stemScanThrottled(job) {
		a.deferStemLibraryScan(job.ID)
		return
	}

	if !a.updateStemScanProgress(job.ID, 0, 0, "Discovering packages in Stem Libraries") {
		return
	}
	rootDiscovery, err := stems.DiscoverLibraryPackagesWithProgressContext(ctx, locations, func(path string, checked int) error {
		if !a.updateStemScanProgress(job.ID, 0, 0, fmt.Sprintf("Validating stem package %d: %s", checked+1, filepath.Base(path))) {
			return fmt.Errorf("unable to save discovery progress")
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			a.cancelStemLibraryJob(job.ID, "Stem Library scan canceled during package discovery")
			return
		}
		_ = a.db.FailJob(job.ID, "stem_library_discovery_failed", "Unable to discover Stem Library packages")
		return
	}
	bySourceHash := make(map[string][]stems.PackageCandidate)
	byAudioIdentity := make(map[stemAudioIdentityKey][]stems.PackageCandidate)
	geometriesSet := make(map[stemAudioGeometry]struct{})
	for _, candidate := range rootDiscovery.Candidates {
		manifest := candidate.Validation.Manifest
		if manifest.Source.SHA256 != "" {
			key := strings.ToLower(manifest.Source.SHA256)
			bySourceHash[key] = append(bySourceHash[key], candidate)
		}
		if manifest.Source.AudioSHA256 != "" && manifest.Audio.SampleRate > 0 && manifest.Audio.Channels > 0 {
			geometry := stemAudioGeometry{sampleRate: manifest.Audio.SampleRate, channels: manifest.Audio.Channels}
			key := stemAudioIdentityKey{geometry: geometry, hash: strings.ToLower(manifest.Source.AudioSHA256)}
			byAudioIdentity[key] = append(byAudioIdentity[key], candidate)
			geometriesSet[geometry] = struct{}{}
		}
	}
	geometries := make([]stemAudioGeometry, 0, len(geometriesSet))
	for geometry := range geometriesSet {
		geometries = append(geometries, geometry)
	}
	sort.Slice(geometries, func(i, j int) bool {
		if geometries[i].sampleRate != geometries[j].sampleRate {
			return geometries[i].sampleRate < geometries[j].sampleRate
		}
		return geometries[i].channels < geometries[j].channels
	})
	geometryLimitReached := len(geometries) > maxStemLibraryIdentityGeometries
	uncheckedGeometryCount := 0
	if geometryLimitReached {
		uncheckedGeometryCount = len(geometries) - maxStemLibraryIdentityGeometries
		geometries = geometries[:maxStemLibraryIdentityGeometries]
	}

	songs, err := a.db.GetAllSongs()
	if err != nil {
		_ = a.db.FailJob(job.ID, "stem_library_read_failed", "Unable to load local music tracks")
		return
	}
	localSongs := make([]db.Song, 0, len(songs))
	for _, song := range songs {
		if song.Source == "plex" || strings.TrimSpace(song.FilePath) == "" {
			continue
		}
		if _, statErr := os.Stat(song.FilePath); statErr == nil {
			localSongs = append(localSongs, song)
		}
	}

	if !a.updateStemScanProgress(job.ID, 0, int64(len(localSongs)), fmt.Sprintf("Matching stems for %d local tracks", len(localSongs))) {
		return
	}
	var refreshed, failed int
	lastProgress := time.Now()
	for index, song := range localSongs {
		if ctx.Err() != nil || a.jobCancellationRequested(job.ID) {
			_ = a.db.CancelJob(job.ID, fmt.Sprintf("Canceled after %d of %d local tracks", index, len(localSongs)))
			return
		}
		if a.stemScanThrottled(job) {
			a.deferStemLibraryScan(job.ID)
			return
		}
		if time.Since(lastProgress) >= 300*time.Millisecond || index == 0 {
			if !a.updateStemScanProgress(job.ID, int64(index), int64(len(localSongs)), fmt.Sprintf("Checking track %d of %d: %s", index+1, len(localSongs), song.Title)) {
				return
			}
			lastProgress = time.Now()
		}

		existingSets, listErr := a.db.ListStemSets(song.ID)
		if listErr != nil {
			failed++
			continue
		}
		adjacent, adjacentErr := stems.DiscoverPackagesContext(ctx, song.FilePath, nil)
		if adjacentErr != nil {
			if ctx.Err() != nil {
				a.cancelStemLibraryJob(job.ID, fmt.Sprintf("Stem Library scan canceled while validating track %d of %d", index+1, len(localSongs)))
				return
			}
			failed++
			continue
		}
		// No discovered packages means there is nothing to match. Registry
		// refresh still validates explicit links and removes deleted packages.
		var sourceHash string
		var hashErr error
		if len(rootDiscovery.Candidates) > 0 || len(adjacent.Candidates) > 0 {
			sourceHash, hashErr = stemSourceHashes.SHA256Context(ctx, song.FilePath)
		}
		if hashErr != nil {
			if ctx.Err() != nil {
				a.cancelStemLibraryJob(job.ID, fmt.Sprintf("Stem Library scan canceled while hashing track %d of %d", index+1, len(localSongs)))
				return
			}
			failed++
			if time.Since(lastProgress) >= 300*time.Millisecond || index+1 == len(localSongs) {
				message := fmt.Sprintf("Scanned Stem Libraries for %d of %d tracks", index+1, len(localSongs))
				if !a.updateStemScanProgress(job.ID, int64(index+1), int64(len(localSongs)), message) {
					return
				}
				lastProgress = time.Now()
			}
			continue
		}
		matches := make([]stems.PackageCandidate, 0)
		for _, candidate := range bySourceHash[strings.ToLower(sourceHash)] {
			candidate.SourceIdentityChecked = true
			candidate.SourceIdentityMatches = true
			matches = append(matches, candidate)
		}
		registeredPaths := make(map[string]bool, len(existingSets))
		for _, existing := range existingSets {
			registeredPaths[pathKeyForOS(existing.PackagePath)] = true
		}

		// Adjacent packages retain priority and are checked once for their
		// corresponding track. The shared matcher preserves the retagged-source
		// PCM32 fallback only for matching decoder geometry.
		for _, candidate := range adjacent.Candidates {
			matched, matchErr := stemSourceMatchesContext(ctx, &song, candidate.Validation.Manifest, sourceHash)
			if ctx.Err() != nil {
				a.cancelStemLibraryJob(job.ID, fmt.Sprintf("Stem Library scan canceled while checking adjacent package for track %d of %d", index+1, len(localSongs)))
				return
			}
			candidate.SourceIdentityChecked = true
			candidate.SourceIdentityMatches = matchErr == nil && matched
			if matched || registeredPaths[pathKeyForOS(candidate.Path)] {
				candidate.Source = stems.CandidateAdjacent
				matches = append(matches, candidate)
			}
		}

		needsPCMIdentityScan := len(matches) == 0
		if !needsPCMIdentityScan {
			for _, existing := range existingSets {
				if existing.ExplicitlyLinked || !pathIsWithinAnyRoot(existing.PackagePath, locations) {
					continue
				}
				for _, candidate := range rootDiscovery.Candidates {
					if pathKeyForOS(candidate.Path) != pathKeyForOS(existing.PackagePath) {
						continue
					}
					manifest := candidate.Validation.Manifest
					if strings.EqualFold(manifest.Source.SHA256, sourceHash) || manifest.Source.AudioSHA256 == "" {
						continue
					}
					geometry := stemAudioGeometry{sampleRate: manifest.Audio.SampleRate, channels: manifest.Audio.Channels}
					for _, checked := range geometries {
						if checked == geometry {
							needsPCMIdentityScan = true
							break
						}
					}
					if needsPCMIdentityScan {
						break
					}
				}
				if needsPCMIdentityScan {
					break
				}
			}
		}
		if needsPCMIdentityScan {
			resolved := analysis.ResolvedSource{Name: filepath.Base(song.FilePath), Path: song.FilePath}
			for _, geometry := range geometries {
				pcmHash, pcmErr := stemSourceAudioHashes.SHA256(ctx, decoderRegistry(), resolved, geometry.sampleRate, geometry.channels)
				if ctx.Err() != nil {
					a.cancelStemLibraryJob(job.ID, fmt.Sprintf("Stem Library scan canceled while checking track identity %d of %d", index+1, len(localSongs)))
					return
				}
				if pcmErr != nil {
					continue
				}
				key := stemAudioIdentityKey{geometry: geometry, hash: strings.ToLower(pcmHash)}
				for _, candidate := range byAudioIdentity[key] {
					candidate.SourceIdentityChecked = true
					candidate.SourceIdentityMatches = true
					matches = append(matches, candidate)
				}
			}
		}
		matches = dedupeAndPrioritizeStemCandidates(matches)
		// Include only already-registered packages from the configured roots when
		// they no longer match this source identity. This lets the registry mark a
		// valid package stale (or an invalid package invalid) without attaching
		// unrelated packages to every local track.
		for _, existing := range existingSets {
			if existing.ExplicitlyLinked || !pathIsWithinAnyRoot(existing.PackagePath, locations) {
				continue
			}
			key := pathKeyForOS(existing.PackagePath)
			for _, candidate := range rootDiscovery.Candidates {
				if pathKeyForOS(candidate.Path) == key {
					candidate.SourceIdentityChecked = true
					candidate.SourceIdentityMatches = false
					matches = append(matches, candidate)
				}
			}
			for _, rejected := range rootDiscovery.Rejected {
				if pathKeyForOS(rejected.Path) == key {
					adjacent.Rejected = append(adjacent.Rejected, rejected)
				}
			}
		}
		matches = dedupeAndPrioritizeStemCandidates(matches)
		discovery := stems.DiscoveryResult{Candidates: matches, Rejected: adjacent.Rejected}
		if err := a.refreshStemRegistryWithDiscoveryContext(ctx, song.ID, &discovery, nil); err != nil {
			if ctx.Err() != nil {
				a.cancelStemLibraryJob(job.ID, fmt.Sprintf("Stem Library scan canceled while refreshing track %d of %d", index+1, len(localSongs)))
				return
			}
			failed++
		} else {
			refreshed++
		}
		if time.Since(lastProgress) >= 300*time.Millisecond || index+1 == len(localSongs) {
			message := fmt.Sprintf("Scanned Stem Libraries for %d of %d tracks", index+1, len(localSongs))
			if !a.updateStemScanProgress(job.ID, int64(index+1), int64(len(localSongs)), message) {
				return
			}
			lastProgress = time.Now()
		}
	}

	if ctx.Err() != nil || a.jobCancellationRequested(job.ID) {
		a.cancelStemLibraryJob(job.ID, "Stem Library scan canceled before completion")
		return
	}
	result := map[string]any{"total": len(localSongs), "refreshed": refreshed, "failed": failed, "packages": len(rootDiscovery.Candidates), "discoveryIssues": len(rootDiscovery.Rejected)}
	if geometryLimitReached {
		result["audioIdentityGeometriesUnchecked"] = uncheckedGeometryCount
	}
	message := fmt.Sprintf("Stem Library scan complete: %d tracks checked, %d failed, %d packages, %d discovery issues", refreshed, failed, len(rootDiscovery.Candidates), len(rootDiscovery.Rejected))
	if geometryLimitReached {
		message += fmt.Sprintf(". PCM32 fallback skipped %d additional package audio geometries; retagged tracks that match only those layouts may remain unmatched", uncheckedGeometryCount)
	}
	completed, completeErr := a.db.CompleteJobIfRunning(job.ID, result, message)
	if completeErr == nil && !completed && a.jobCancellationRequested(job.ID) {
		a.cancelStemLibraryJob(job.ID, "Stem Library scan canceled before completion")
	}
}

func (a *API) cancelStemLibraryJob(jobID, message string) {
	_ = a.db.CancelJob(jobID, message)
}

func (a *API) deferStemLibraryScan(jobID string) {
	message := "Waiting for DJ playback to finish before scanning Stem Libraries"
	if a.jobCancellationRequested(jobID) {
		a.cancelStemLibraryJob(jobID, "Stem Library scan canceled while waiting for DJ playback")
		return
	}
	requeued, _ := a.db.RequeueJob(jobID, message, analysisDeferBackoff)
	if !requeued && a.jobCancellationRequested(jobID) {
		a.cancelStemLibraryJob(jobID, "Stem Library scan canceled while waiting for DJ playback")
	}
}

func (a *API) stemLibraryScanContext(jobID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if a.jobCancellationRequested(jobID) {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, func() {
		close(stop)
		<-done
		cancel()
	}
}

func pathIsWithinAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if localRootsOverlap(root, path) && rootPathContainsMust(root, path) {
			return true
		}
	}
	return false
}

func rootPathContainsMust(parent, child string) bool {
	p, err1 := normalizedLocalRoot(parent)
	c, err2 := normalizedLocalRoot(child)
	return err1 == nil && err2 == nil && rootPathContains(p, c)
}

func dedupeAndPrioritizeStemCandidates(candidates []stems.PackageCandidate) []stems.PackageCandidate {
	seen := make(map[string]bool, len(candidates))
	out := make([]stems.PackageCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := strings.ToLower(candidate.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source == stems.CandidateAdjacent
		}
		a, b := strings.ToLower(out[i].Path), strings.ToLower(out[j].Path)
		if a == b {
			return out[i].Path < out[j].Path
		}
		return a < b
	})
	return out
}
