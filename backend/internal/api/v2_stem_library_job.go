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
	"github.com/ajbergh/viib-mediahub/internal/stems"
)

// PCM32 fallback is evaluated once per source/layout, not once per package.
// Bound the layouts a configured root can trigger; exact full-file hashes are
// indexed and checked for every validated candidate regardless of this cap.
const maxStemLibraryIdentityGeometries = 16

type stemAudioGeometry struct {
	sampleRate int
	channels   int
}

type stemAudioIdentityKey struct {
	geometry stemAudioGeometry
	hash     string
}

func (a *API) runStemLibraryScanJob(job db.Job) {
	var params struct {
		Locations []string `json:"locations"`
	}
	if err := json.Unmarshal(job.Parameters, &params); err != nil || len(params.Locations) == 0 {
		_ = a.db.FailJob(job.ID, "invalid_stem_library_scan", "Stem Library scan locations are missing")
		return
	}
	if a.analysisThrottled(job.Priority) {
		_, _ = a.db.RequeueJob(job.ID, "Waiting for DJ playback to finish before scanning Stem Libraries", analysisDeferBackoff)
		return
	}

	_ = a.db.UpdateJobProgress(job.ID, 0, 0, "Discovering packages in Stem Libraries")
	rootDiscovery := stems.DiscoverLibraryPackages(params.Locations)
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

	_ = a.db.UpdateJobProgress(job.ID, 0, int64(len(localSongs)), fmt.Sprintf("Validating packages for %d local tracks", len(localSongs)))
	var refreshed, failed int
	lastProgress := time.Now()
	for index, song := range localSongs {
		if a.jobCancellationRequested(job.ID) {
			_ = a.db.CancelJob(job.ID, fmt.Sprintf("Canceled after %d of %d local tracks", index, len(localSongs)))
			return
		}
		if a.analysisThrottled(job.Priority) {
			_, _ = a.db.RequeueJob(job.ID, "Waiting for DJ playback to finish before scanning Stem Libraries", analysisDeferBackoff)
			return
		}

		sourceHash, hashErr := stemSourceHashes.SHA256(song.FilePath)
		if hashErr != nil {
			failed++
			if time.Since(lastProgress) >= 300*time.Millisecond || index+1 == len(localSongs) {
				message := fmt.Sprintf("Scanned Stem Libraries for %d of %d tracks", index+1, len(localSongs))
				_ = a.db.UpdateJobProgress(job.ID, int64(index+1), int64(len(localSongs)), message)
				lastProgress = time.Now()
			}
			continue
		}
		matches := make([]stems.PackageCandidate, 0)
		matches = append(matches, bySourceHash[strings.ToLower(sourceHash)]...)

		// Adjacent packages retain priority and are checked once for their
		// corresponding track. The shared matcher preserves the retagged-source
		// PCM32 fallback only for matching decoder geometry.
		adjacent := stems.DiscoverPackages(song.FilePath, nil)
		for _, candidate := range adjacent.Candidates {
			if stemSourceMatches(&song, candidate.Validation.Manifest) {
				candidate.Source = stems.CandidateAdjacent
				matches = append(matches, candidate)
			}
		}

		if len(matches) == 0 {
			resolved := analysis.ResolvedSource{Name: filepath.Base(song.FilePath), Path: song.FilePath}
			for _, geometry := range geometries {
				pcmHash, pcmErr := stemSourceAudioHashes.SHA256(context.Background(), decoderRegistry(), resolved, geometry.sampleRate, geometry.channels)
				if pcmErr != nil {
					continue
				}
				key := stemAudioIdentityKey{geometry: geometry, hash: strings.ToLower(pcmHash)}
				matches = append(matches, byAudioIdentity[key]...)
			}
		}
		matches = dedupeAndPrioritizeStemCandidates(matches)
		discovery := stems.DiscoveryResult{Candidates: matches, Rejected: adjacent.Rejected}
		if err := a.refreshStemRegistryWithDiscovery(song.ID, &discovery, nil); err != nil {
			failed++
		} else {
			refreshed++
		}
		if time.Since(lastProgress) >= 300*time.Millisecond || index+1 == len(localSongs) {
			message := fmt.Sprintf("Scanned Stem Libraries for %d of %d tracks", index+1, len(localSongs))
			if err := a.db.UpdateJobProgress(job.ID, int64(index+1), int64(len(localSongs)), message); err != nil {
				return
			}
			lastProgress = time.Now()
		}
	}

	result := map[string]any{"total": len(localSongs), "refreshed": refreshed, "failed": failed, "packages": len(rootDiscovery.Candidates), "discoveryIssues": len(rootDiscovery.Rejected)}
	if geometryLimitReached {
		result["audioIdentityGeometriesUnchecked"] = uncheckedGeometryCount
	}
	message := fmt.Sprintf("Stem Library scan complete: %d tracks checked, %d failed, %d packages, %d discovery issues", refreshed, failed, len(rootDiscovery.Candidates), len(rootDiscovery.Rejected))
	if geometryLimitReached {
		message += fmt.Sprintf(". PCM32 fallback skipped %d additional package audio geometries; retagged tracks that match only those layouts may remain unmatched", uncheckedGeometryCount)
	}
	_ = a.db.CompleteJob(job.ID, result, message)
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
