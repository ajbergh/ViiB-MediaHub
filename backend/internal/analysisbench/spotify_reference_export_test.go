// Tests export of normalized cached references from a read-only database with recording and file provenance.
package analysisbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

type exportFixture struct {
	database    *db.DB
	path        string
	audio       string
	manifest    CorpusManifest
	options     SpotifyReferenceExportOptions
	recordingID string
}

func newExportFixture(t *testing.T) exportFixture {
	t.Helper()
	root := t.TempDir()
	audio := filepath.Join(root, "fixture.ogg")
	if err := os.WriteFile(audio, []byte("synthetic container bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "library.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	song := db.Song{ID: "local-song", FilePath: audio, Title: "fixture", Artist: "generator", Album: "synthetic", AddedAt: 1}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSongSource(song)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshTrackAnalysisSourceRevision(song.ID, source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	recordingID := strings.Repeat("A", 22)
	if ok, err := database.ConfirmSpotifyRecording(song.ID, recordingID, source.Fingerprint, true); err != nil || !ok {
		t.Fatalf("confirm: %v %v", ok, err)
	}
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	observation := spotifyanalysis.Observation{TrackID: recordingID, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now.Add(-time.Hour), BPM: float64Ptr(120)}
	if err := database.PutExternalAnalysis(observation, "fixture-adapter-v1", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	hash, err := referenceFileSHA256(context.Background(), audio)
	if err != nil {
		t.Fatal(err)
	}
	identity := SpotifyRecordingIdentity{RecordingID: recordingID, RecordingVersion: "synthetic-v1", AudioSHA256: hash, Confirmed: true}
	corpusTrack := track("corpus-id", SplitHeldOut, 60, nil, "A minor")
	corpusTrack.Path = audio
	corpusTrack.SpotifyRecording = &identity
	return exportFixture{database: database, path: path, audio: audio, recordingID: recordingID, manifest: CorpusManifest{Version: "fixture-v1", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{corpusTrack}}, options: SpotifyReferenceExportOptions{Endpoint: "audio_features", License: "generated", LabelSource: "fixture-cache", Now: now}}
}

func exportFixtureReadOnly(t *testing.T, f exportFixture) (*SpotifyReferenceSnapshot, SpotifyReferenceExportCoverage, error) {
	t.Helper()
	reader, err := db.OpenReferenceReadOnly(f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	return ExportSpotifyReference(context.Background(), reader, f.manifest, f.options)
}

func TestReferenceExportChecksAudioAndPreservesCacheProvenance(t *testing.T) {
	f := newExportFixture(t)
	snapshot, coverage, err := exportFixtureReadOnly(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil || coverage.Exported != 1 || snapshot.AdapterRevision != "fixture-adapter-v1" || snapshot.AnalyzerVersion != nil || snapshot.Entries[0].TrackID != "corpus-id" || snapshot.Entries[0].SchemaVersion != "1" {
		t.Fatalf("snapshot=%+v coverage=%+v", snapshot, coverage)
	}
	if snapshot.Entries[0].Recording != *f.manifest.Tracks[0].SpotifyRecording || *snapshot.Entries[0].BPM != 120 || snapshot.Entries[0].Key != nil {
		t.Fatal("identity/scalar provenance lost")
	}
	again, _, err := exportFixtureReadOnly(t, f)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(snapshot)
	second, _ := json.Marshal(again)
	if string(first) != string(second) {
		t.Fatal("unchanged cache export is not deterministic")
	}
	if _, err := CompareSpotifyReference(f.manifest, ResultSet{Algorithm: "fixture", Results: []DetectorResult{{ID: "corpus-id", BPM: float64Ptr(60)}}}, *snapshot, SplitHeldOut); err != nil {
		t.Fatal(err)
	}
}

func TestReferenceExportOmitsIneligiblePositions(t *testing.T) {
	for _, kind := range []string{"unconfirmed", "missing-song", "ambiguous", "unavailable", "stale-identity", "changed-same-stat", "missing-cache", "expired-cache", "newer-failure"} {
		t.Run(kind, func(t *testing.T) {
			f := newExportFixture(t)
			switch kind {
			case "unconfirmed":
				f.manifest.Tracks[0].SpotifyRecording = nil
			case "missing-song":
				f.manifest.Tracks[0].Path = filepath.Join(t.TempDir(), "absent.ogg")
			case "ambiguous":
				if err := f.database.SaveSong(&db.Song{ID: "duplicate", FilePath: filepath.Dir(f.audio) + string(os.PathSeparator) + "." + string(os.PathSeparator) + filepath.Base(f.audio), Title: "copy", Artist: "generator", Album: "synthetic", AddedAt: 1}); err != nil {
					t.Fatal(err)
				}
			case "unavailable":
				if err := os.Remove(f.audio); err != nil {
					t.Fatal(err)
				}
			case "stale-identity":
				if err := os.WriteFile(f.audio, []byte("changed size"), 0600); err != nil {
					t.Fatal(err)
				}
			case "changed-same-stat":
				info, err := os.Stat(f.audio)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(f.audio, []byte("Xynthetic container bytes"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(f.audio, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "missing-cache":
				f.options.Endpoint = "audio_analysis"
			case "expired-cache":
				f.options.Now = f.options.Now.Add(2 * time.Hour)
			case "newer-failure":
				if err := f.database.PutExternalAnalysisStatus(f.recordingID, "audio_features", db.ExternalAnalysisStatus{Code: spotifyanalysis.NotFound, CheckedAt: f.options.Now, RetryAt: f.options.Now}); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, coverage, err := exportFixtureReadOnly(t, f)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot != nil || coverage.Exported != 0 {
				t.Fatalf("ineligible snapshot=%+v coverage=%+v", snapshot, coverage)
			}
			counts := coverage.Unconfirmed + coverage.MissingSong + coverage.Ambiguous + coverage.UnavailableSource + coverage.StaleIdentity + coverage.ChangedAudio + coverage.MissingCache + coverage.StaleCache + coverage.NewerFailure
			if counts != 1 {
				t.Fatalf("coverage=%+v", coverage)
			}
			if kind == "changed-same-stat" && coverage.ChangedAudio != 1 {
				t.Fatal("full-file hash did not detect hidden content change")
			}
		})
	}
}

func TestReferenceExportRejectsContradictoryIdentityAndCancellation(t *testing.T) {
	f := newExportFixture(t)
	f.manifest.Tracks[0].SpotifyRecording.RecordingID = strings.Repeat("B", 22)
	if _, _, err := exportFixtureReadOnly(t, f); err == nil {
		t.Fatal("contradictory identity accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := ExportSpotifyReference(ctx, f.database, f.manifest, f.options); err == nil {
		t.Fatal("cancelled export succeeded")
	}
}

func TestReferenceExportDoesNotChangeDatabaseOrLocalLabels(t *testing.T) {
	f := newExportFixture(t)
	if err := f.database.Close(); err != nil {
		t.Fatal(err)
	}
	bytesBefore, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	before := sha256.Sum256(bytesBefore)
	manifestBefore, _ := json.Marshal(f.manifest)
	if _, _, err := exportFixtureReadOnly(t, f); err != nil {
		t.Fatal(err)
	}
	bytesAfter, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	after := sha256.Sum256(bytesAfter)
	if hex.EncodeToString(before[:]) != hex.EncodeToString(after[:]) {
		t.Fatal("read-only export changed database bytes")
	}
	manifestAfter, _ := json.Marshal(f.manifest)
	if string(manifestBefore) != string(manifestAfter) {
		t.Fatal("export changed local labels")
	}
}

func TestReferenceExportRejectsMixedProvenance(t *testing.T) {
	for _, kind := range []string{"adapter", "analyzer"} {
		t.Run(kind, func(t *testing.T) {
			f := newExportFixture(t)
			secondAudio := filepath.Join(filepath.Dir(f.audio), "second.ogg")
			if err := os.WriteFile(secondAudio, []byte("second synthetic container"), 0600); err != nil {
				t.Fatal(err)
			}
			song := db.Song{ID: "second", FilePath: secondAudio, Title: "second", Artist: "generator", Album: "synthetic", AddedAt: 1}
			if err := f.database.SaveSong(&song); err != nil {
				t.Fatal(err)
			}
			source, err := analysis.ResolveLocalSongSource(song)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.database.RefreshTrackAnalysisSourceRevision(song.ID, source.Fingerprint); err != nil {
				t.Fatal(err)
			}
			id := strings.Repeat("B", 22)
			if ok, err := f.database.ConfirmSpotifyRecording(song.ID, id, source.Fingerprint, true); err != nil || !ok {
				t.Fatalf("confirm=%v err=%v", ok, err)
			}
			hash, err := referenceFileSHA256(context.Background(), secondAudio)
			if err != nil {
				t.Fatal(err)
			}
			secondTrack := track("second", SplitHeldOut, 60, nil, "A minor")
			secondTrack.Path = secondAudio
			secondTrack.SpotifyRecording = &SpotifyRecordingIdentity{RecordingID: id, RecordingVersion: "synthetic-v1", AudioSHA256: hash, Confirmed: true}
			f.manifest.Tracks = append(f.manifest.Tracks, secondTrack)
			observation := spotifyanalysis.Observation{TrackID: id, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: f.options.Now.Add(-time.Hour), BPM: float64Ptr(120)}
			revision := "fixture-adapter-v1"
			if kind == "adapter" {
				revision = "fixture-adapter-v2"
			} else {
				observation.AnalyzerVersion = "fixture-analyzer-v2"
			}
			if err := f.database.PutExternalAnalysis(observation, revision, f.options.Now.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if _, _, err := exportFixtureReadOnly(t, f); err == nil {
				t.Fatal("mixed provenance accepted")
			}
		})
	}
}

func TestReferenceExportPreservesPartialNullableKeyMode(t *testing.T) {
	f := newExportFixture(t)
	mode := 1
	observation := spotifyanalysis.Observation{TrackID: f.recordingID, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: f.options.Now.Add(-time.Minute), BPM: float64Ptr(120), Mode: &mode}
	if err := f.database.PutExternalAnalysis(observation, "fixture-adapter-v1", f.options.Now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	snapshot, coverage, err := exportFixtureReadOnly(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil || coverage.Exported != 1 || snapshot.Entries[0].Key != nil || snapshot.Entries[0].Mode == nil || *snapshot.Entries[0].Mode != 1 {
		t.Fatal("partial provider value was lost or invented")
	}
	report, err := CompareSpotifyReference(f.manifest, ResultSet{Algorithm: "fixture"}, *snapshot, SplitHeldOut)
	if err != nil {
		t.Fatal(err)
	}
	if report.Comparison == nil || report.Comparison.Key.Labeled != 0 || report.Coverage.UnavailableKey != 1 || report.Coverage.Unlabelled != 0 {
		t.Fatal("partial key/mode was treated as complete")
	}
}
