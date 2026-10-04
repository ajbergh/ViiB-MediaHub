// Exports normalized cached references from a read-only database with recording and file provenance.
package analysisbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

type SpotifyReferenceExportOptions struct {
	Endpoint    string
	License     string
	LabelSource string
	Now         time.Time
}

type SpotifyReferenceExportCoverage struct {
	Tracks            int `json:"tracks"`
	Exported          int `json:"exported"`
	Unconfirmed       int `json:"unconfirmed"`
	MissingSong       int `json:"missingSong"`
	Ambiguous         int `json:"ambiguous"`
	UnavailableSource int `json:"unavailableSource"`
	StaleIdentity     int `json:"staleIdentity"`
	ChangedAudio      int `json:"changedAudio"`
	MissingCache      int `json:"missingCache"`
	StaleCache        int `json:"staleCache"`
	NewerFailure      int `json:"newerFailure"`
}

// ExportSpotifyReference reads an already-open read-only DB. A manifest's
// explicit version/confirmation is never inferred from a DB path or title.
// AudioSHA256 means the full container bytes, not decoded PCM or Song.FileHash.
func ExportSpotifyReference(ctx context.Context, database *db.DB, manifest CorpusManifest, options SpotifyReferenceExportOptions) (*SpotifyReferenceSnapshot, SpotifyReferenceExportCoverage, error) {
	var coverage SpotifyReferenceExportCoverage
	if err := manifest.Validate(); err != nil {
		return nil, coverage, err
	}
	if options.Endpoint != "audio_features" && options.Endpoint != "audio_analysis" {
		return nil, coverage, fmt.Errorf("explicit reference endpoint is required")
	}
	if strings.TrimSpace(options.License) == "" || strings.TrimSpace(options.LabelSource) == "" || options.Now.IsZero() {
		return nil, coverage, fmt.Errorf("reference license, label source and export time are required")
	}
	songs, err := database.GetAllSongs()
	if err != nil {
		return nil, coverage, fmt.Errorf("read reference library: %w", err)
	}
	paths := map[string][]db.Song{}
	for _, song := range songs {
		if strings.TrimSpace(song.FilePath) == "" {
			continue
		}
		path, err := referencePath(song.FilePath)
		if err == nil {
			paths[path] = append(paths[path], song)
		}
	}
	manifestPaths := map[string]int{}
	for _, track := range manifest.Tracks {
		path, err := referencePath(track.Path)
		if err != nil {
			return nil, coverage, fmt.Errorf("invalid corpus path")
		}
		manifestPaths[path]++
	}
	snapshot := SpotifyReferenceSnapshot{Version: SpotifyReferenceSnapshotVersion, EvidenceClass: manifest.EvidenceClass, License: options.License, LabelSource: options.LabelSource, Entries: []SpotifyReferenceEntry{}}
	for _, track := range manifest.Tracks {
		if err := ctx.Err(); err != nil {
			return nil, coverage, err
		}
		coverage.Tracks++
		if track.SpotifyRecording == nil {
			coverage.Unconfirmed++
			continue
		}
		path, _ := referencePath(track.Path)
		candidates := paths[path]
		if len(candidates) == 0 {
			coverage.MissingSong++
			continue
		}
		if len(candidates) != 1 || manifestPaths[path] != 1 {
			coverage.Ambiguous++
			continue
		}
		source, err := analysis.ResolveLocalSongSource(candidates[0])
		if err != nil {
			coverage.UnavailableSource++
			continue
		}
		link, err := database.GetSpotifyRecording(source.SongID, source.Fingerprint)
		if err != nil {
			return nil, coverage, fmt.Errorf("read confirmed identity: %w", err)
		}
		if link == nil {
			coverage.StaleIdentity++
			continue
		}
		if link.LinkOrigin != "manual_confirmation" || link.ConfirmedAt <= 0 {
			coverage.Unconfirmed++
			continue
		}
		if link.ExternalID != track.SpotifyRecording.RecordingID {
			return nil, coverage, fmt.Errorf("confirmed recording differs from corpus identity")
		}
		hash, err := referenceFileSHA256(ctx, source.Path)
		if err != nil {
			if ctx.Err() != nil {
				return nil, coverage, ctx.Err()
			}
			coverage.UnavailableSource++
			continue
		}
		after, err := analysis.ResolveLocalSongSource(candidates[0])
		if err != nil || after.Fingerprint != source.Fingerprint || hash != track.SpotifyRecording.AudioSHA256 {
			coverage.ChangedAudio++
			continue
		}
		cache, err := database.GetExternalAnalysis(link.ExternalID, options.Endpoint)
		if err != nil {
			return nil, coverage, fmt.Errorf("read normalized cache: %w", err)
		}
		if cache == nil {
			coverage.MissingCache++
			continue
		}
		if !cache.ExpiresAt.After(options.Now) || cache.Observation.RetrievedAt.After(options.Now) {
			coverage.StaleCache++
			continue
		}
		failure, err := database.GetExternalAnalysisStatus(link.ExternalID, options.Endpoint)
		if err != nil {
			return nil, coverage, fmt.Errorf("read cache status: %w", err)
		}
		if failure != nil && !failure.CheckedAt.Before(cache.Observation.RetrievedAt) {
			coverage.NewerFailure++
			continue
		}
		if strings.TrimSpace(cache.AdapterRevision) == "" {
			return nil, coverage, fmt.Errorf("cache adapter revision is missing")
		}
		analyzer := cache.Observation.AnalyzerVersion
		if coverage.Exported == 0 {
			snapshot.AdapterRevision = cache.AdapterRevision
			if analyzer != "" {
				snapshot.AnalyzerVersion = &analyzer
			}
		} else {
			previous := ""
			if snapshot.AnalyzerVersion != nil {
				previous = *snapshot.AnalyzerVersion
			}
			if cache.AdapterRevision != snapshot.AdapterRevision || analyzer != previous {
				return nil, coverage, fmt.Errorf("mixed cache adapter or analyzer versions require separate snapshots")
			}
		}
		observation := cache.Observation
		snapshot.Entries = append(snapshot.Entries, SpotifyReferenceEntry{TrackID: track.ID, Recording: *track.SpotifyRecording, Endpoint: observation.SourceEndpoint, SchemaVersion: strconv.Itoa(cache.SchemaVersion), RetrievedAt: observation.RetrievedAt, BPM: observation.BPM, Key: observation.Key, Mode: observation.Mode, TempoConfidence: observation.BPMConfidence, KeyConfidence: observation.KeyConfidence})
		coverage.Exported++
	}
	if coverage.Exported == 0 {
		return nil, coverage, nil
	}
	snapshot.CacheSnapshotHash, err = snapshot.Hash()
	if err != nil {
		return nil, coverage, err
	}
	if err = snapshot.Validate(); err != nil {
		return nil, coverage, err
	}
	return &snapshot, coverage, nil
}

func referencePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if runtime.GOOS == "windows" {
		absolute = strings.ToLower(absolute)
	}
	return absolute, nil
}

// Always hash anew; metadata-only cache reuse could miss same-size/mtime edits.
// Check the opened file and final path refer to the same unchanged file.
func referenceFileSHA256(ctx context.Context, path string) (string, error) {
	before, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() {
		return "", fmt.Errorf("audio is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return "", fmt.Errorf("audio changed while opening")
	}
	hash := sha256.New()
	buffer := make([]byte, 128*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := file.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
			total += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	after, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !os.SameFile(opened, after) || total != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return "", fmt.Errorf("audio changed while hashing")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
