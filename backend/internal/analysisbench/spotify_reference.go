package analysisbench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strings"
	"time"
)

const SpotifyReferenceSnapshotVersion = "spotify-reference-v1"

// SpotifyRecordingIdentity must come from explicit recording confirmation,
// never filename/title similarity. AudioSHA256 and RecordingVersion prevent
// reusing labels after the local recording or its version changes.
type SpotifyRecordingIdentity struct {
	RecordingID      string `json:"recordingId"`
	RecordingVersion string `json:"recordingVersion"`
	AudioSHA256      string `json:"audioSha256"`
	Confirmed        bool   `json:"confirmed"`
}

type SpotifyReferenceEntry struct {
	TrackID         string                   `json:"trackId"`
	Recording       SpotifyRecordingIdentity `json:"recording"`
	Endpoint        string                   `json:"endpoint"`
	SchemaVersion   string                   `json:"schemaVersion"`
	RetrievedAt     time.Time                `json:"retrievedAt"`
	BPM             *float64                 `json:"bpm"`
	Key             *int                     `json:"key"`
	Mode            *int                     `json:"mode"`
	TempoConfidence *float64                 `json:"tempoConfidence"`
	KeyConfidence   *float64                 `json:"keyConfidence"`
}

// This file contains normalized scalars and provenance only. It is an offline
// input: the comparator neither retrieves Spotify data nor opens local audio.
type SpotifyReferenceSnapshot struct {
	Version           string                  `json:"version"`
	EvidenceClass     string                  `json:"evidenceClass"`
	License           string                  `json:"license"`
	LabelSource       string                  `json:"labelSource"`
	AdapterRevision   string                  `json:"adapterRevision"`
	AnalyzerVersion   *string                 `json:"analyzerVersion"`
	CacheSnapshotHash string                  `json:"cacheSnapshotHash"`
	Entries           []SpotifyReferenceEntry `json:"entries"`
}

// Hash excludes the hash field itself and uses the Go struct's canonical JSON
// field order. Input whitespace is immaterial; entry order remains frozen.
func (s SpotifyReferenceSnapshot) Hash() (string, error) {
	s.CacheSnapshotHash = ""
	encoded, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

var recordingIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (r SpotifyRecordingIdentity) validate() error {
	if !r.Confirmed || !recordingIDPattern.MatchString(r.RecordingID) || strings.TrimSpace(r.RecordingVersion) == "" || !sha256Pattern.MatchString(r.AudioSHA256) {
		return fmt.Errorf("requires explicit recording confirmation, 22-character recording ID, recording version and lowercase audio SHA-256")
	}
	return nil
}

func (s SpotifyReferenceSnapshot) Validate() error {
	if s.Version != SpotifyReferenceSnapshotVersion {
		return fmt.Errorf("unsupported Spotify reference snapshot version")
	}
	if s.EvidenceClass != EvidenceSyntheticCI && s.EvidenceClass != EvidenceLawfulRealAudio {
		return fmt.Errorf("invalid snapshot evidence class")
	}
	if strings.TrimSpace(s.License) == "" || strings.TrimSpace(s.LabelSource) == "" || strings.TrimSpace(s.AdapterRevision) == "" || (s.AnalyzerVersion != nil && strings.TrimSpace(*s.AnalyzerVersion) == "") {
		return fmt.Errorf("snapshot provenance is incomplete")
	}
	if !sha256Pattern.MatchString(s.CacheSnapshotHash) {
		return fmt.Errorf("snapshot hash is required")
	}
	hash, err := s.Hash()
	if err != nil {
		return err
	}
	if hash != s.CacheSnapshotHash {
		return fmt.Errorf("snapshot hash mismatch")
	}
	seen := map[string]bool{}
	for i, e := range s.Entries {
		if e.TrackID == "" || seen[e.TrackID] {
			return fmt.Errorf("snapshot entry %d has missing or duplicate track ID", i)
		}
		seen[e.TrackID] = true
		if err := e.Recording.validate(); err != nil {
			return fmt.Errorf("snapshot entry %d: %w", i, err)
		}
		if strings.TrimSpace(e.Endpoint) == "" || strings.TrimSpace(e.SchemaVersion) == "" || e.RetrievedAt.IsZero() {
			return fmt.Errorf("snapshot entry %d has incomplete retrieval provenance", i)
		}
		if e.BPM != nil && (*e.BPM <= 0 || math.IsNaN(*e.BPM) || math.IsInf(*e.BPM, 0)) {
			return fmt.Errorf("snapshot entry %d has invalid BPM", i)
		}
		if (e.Key != nil && (*e.Key < 0 || *e.Key > 11)) || (e.Mode != nil && *e.Mode != 0 && *e.Mode != 1) {
			return fmt.Errorf("snapshot entry %d has invalid key/mode", i)
		}
		for _, confidence := range []*float64{e.TempoConfidence, e.KeyConfidence} {
			if confidence != nil && (*confidence < 0 || *confidence > 1 || math.IsNaN(*confidence) || math.IsInf(*confidence, 0)) {
				return fmt.Errorf("snapshot entry %d has invalid confidence", i)
			}
		}
	}
	return nil
}

func LoadSpotifyReferenceSnapshot(path string) (SpotifyReferenceSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return SpotifyReferenceSnapshot{}, err
	}
	defer f.Close()
	const maxBytes = 16 << 20
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return SpotifyReferenceSnapshot{}, err
	}
	if len(data) > maxBytes {
		return SpotifyReferenceSnapshot{}, fmt.Errorf("snapshot exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var s SpotifyReferenceSnapshot
	if err := decoder.Decode(&s); err != nil {
		return s, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return s, fmt.Errorf("snapshot contains trailing JSON")
	}
	return s, s.Validate()
}

type SpotifyReferenceCoverage struct {
	TracksInSplit  int `json:"tracksInSplit"`
	Matched        int `json:"matched"`
	Missing        int `json:"missing"`
	Unconfirmed    int `json:"unconfirmed"`
	UnavailableBPM int `json:"unavailableBpm"`
	UnavailableKey int `json:"unavailableKey"`
	Unlabelled     int `json:"unlabelled"`
}

type SpotifyReferenceReport struct {
	Version                 string                   `json:"version"`
	EvidenceClass           string                   `json:"evidenceClass"`
	License                 string                   `json:"license"`
	LabelSource             string                   `json:"labelSource"`
	AdapterRevision         string                   `json:"adapterRevision"`
	AnalyzerVersion         *string                  `json:"analyzerVersion"`
	CacheSnapshotHash       string                   `json:"cacheSnapshotHash"`
	Coverage                SpotifyReferenceCoverage `json:"coverage"`
	Comparison              *ComparisonReport        `json:"comparison,omitempty"`
	RelativeKeyMatches      int                      `json:"relativeKeyMatches"`
	HarmonicNeighborMatches int                      `json:"harmonicNeighborMatches"`
}

// CompareSpotifyReference keeps the ordinary local-label comparison separate.
// Absent provider values are coverage gaps, never invented expected-unknown
// labels. Identity contradictions fail closed rather than score wrong audio.
func CompareSpotifyReference(manifest CorpusManifest, results ResultSet, snapshot SpotifyReferenceSnapshot, split string) (SpotifyReferenceReport, error) {
	var report SpotifyReferenceReport
	if err := manifest.Validate(); err != nil {
		return report, err
	}
	if err := results.Validate(); err != nil {
		return report, err
	}
	if err := snapshot.Validate(); err != nil {
		return report, err
	}
	if split != SplitHeldOut && split != SplitTuning {
		return report, fmt.Errorf("unsupported corpus split")
	}
	if manifest.EvidenceClass != snapshot.EvidenceClass {
		return report, fmt.Errorf("manifest and snapshot evidence classes differ")
	}
	tracks := map[string]CorpusTrack{}
	for _, track := range manifest.Tracks {
		tracks[track.ID] = track
	}
	entries := map[string]SpotifyReferenceEntry{}
	for _, entry := range snapshot.Entries {
		track, exists := tracks[entry.TrackID]
		if !exists {
			return report, fmt.Errorf("snapshot contains a track absent from manifest")
		}
		if track.SpotifyRecording != nil {
			if err := track.SpotifyRecording.validate(); err != nil {
				return report, fmt.Errorf("manifest recording identity: %w", err)
			}
			if *track.SpotifyRecording != entry.Recording {
				return report, fmt.Errorf("snapshot recording identity differs from manifest")
			}
		}
		entries[entry.TrackID] = entry
	}
	report = SpotifyReferenceReport{Version: snapshot.Version, EvidenceClass: snapshot.EvidenceClass, License: snapshot.License, LabelSource: snapshot.LabelSource, AdapterRevision: snapshot.AdapterRevision, AnalyzerVersion: snapshot.AnalyzerVersion, CacheSnapshotHash: snapshot.CacheSnapshotHash}
	referenceManifest := CorpusManifest{Version: manifest.Version, EvidenceClass: manifest.EvidenceClass}
	outputs := map[string]DetectorResult{}
	for _, output := range results.Results {
		outputs[output.ID] = output
	}
	for _, track := range manifest.Tracks {
		if track.Split != split {
			continue
		}
		report.Coverage.TracksInSplit++
		entry, exists := entries[track.ID]
		if !exists {
			report.Coverage.Missing++
			continue
		}
		if track.SpotifyRecording == nil {
			report.Coverage.Unconfirmed++
			continue
		}
		report.Coverage.Matched++
		if entry.BPM == nil {
			report.Coverage.UnavailableBPM++
		}
		hasKey := entry.Key != nil && entry.Mode != nil
		if !hasKey {
			report.Coverage.UnavailableKey++
		}
		if entry.BPM == nil && !hasKey {
			report.Coverage.Unlabelled++
			continue
		}
		track.LabelSource = snapshot.LabelSource
		track.ExpectedBPM = entry.BPM
		track.AcceptedMetricBPM = nil
		track.ExpectedUnknown = false
		track.ExpectedKey = ""
		if hasKey {
			mode := "major"
			if *entry.Mode == 0 {
				mode = "minor"
			}
			track.ExpectedKey = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}[*entry.Key] + " " + mode
			expected, _ := parseKey(track.ExpectedKey)
			actual, err := parseKey(outputs[track.ID].Key)
			if err == nil && expected != actual {
				if expected.minor != actual.minor {
					minor, major := expected, actual
					if !minor.minor {
						minor, major = actual, expected
					}
					if major.pitchClass == (minor.pitchClass+3)%12 {
						report.RelativeKeyMatches++
					}
				} else if keysCompatible(expected, actual) {
					report.HarmonicNeighborMatches++
				}
			}
		}
		referenceManifest.Tracks = append(referenceManifest.Tracks, track)
	}
	if len(referenceManifest.Tracks) > 0 {
		comparison, err := Compare(referenceManifest, results, split)
		if err != nil {
			return report, err
		}
		report.Comparison = &comparison
	}
	return report, nil
}
