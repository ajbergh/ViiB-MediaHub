// Tests comparison of frozen Spotify references bound to corpus identity separately from ground-truth labels.
package analysisbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func referenceFixture(t *testing.T) (CorpusManifest, ResultSet, SpotifyReferenceSnapshot) {
	t.Helper()
	identity := SpotifyRecordingIdentity{RecordingID: strings.Repeat("A", 22), RecordingVersion: "synthetic-recording-v1", AudioSHA256: strings.Repeat("a", 64), Confirmed: true}
	manifest := CorpusManifest{Version: "fixture-v1", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{
		track("half", SplitHeldOut, 140, nil, "A minor"),
		track("relative", SplitHeldOut, 100, nil, "C major"),
		track("harmonic", SplitHeldOut, 100, nil, "C major"),
		track("unavailable", SplitHeldOut, 100, nil, "C major"),
		track("missing", SplitHeldOut, 100, nil, "C major"),
		track("unconfirmed", SplitHeldOut, 100, nil, "C major"),
	}}
	snapshot := SpotifyReferenceSnapshot{Version: SpotifyReferenceSnapshotVersion, EvidenceClass: EvidenceSyntheticCI, License: "generated fixtures", LabelSource: "synthetic reference scalars", AdapterRevision: "fixture-adapter-v1", Entries: []SpotifyReferenceEntry{}}
	bpm := 140.0
	key, mode := 0, 1
	for i := range manifest.Tracks {
		id := identity
		id.RecordingVersion += "-" + manifest.Tracks[i].ID
		if manifest.Tracks[i].ID != "unconfirmed" {
			manifest.Tracks[i].SpotifyRecording = &id
		}
		if manifest.Tracks[i].ID == "missing" {
			continue
		}
		e := SpotifyReferenceEntry{TrackID: manifest.Tracks[i].ID, Recording: id, Endpoint: "synthetic:features", SchemaVersion: "features-v1", RetrievedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), BPM: &bpm, Key: &key, Mode: &mode}
		if e.TrackID == "unavailable" {
			e.BPM = nil
			e.Key = nil
			e.Mode = nil
		}
		snapshot.Entries = append(snapshot.Entries, e)
	}
	snapshot.CacheSnapshotHash, _ = snapshot.Hash()
	results := ResultSet{Algorithm: "fixture-local-v1", Results: []DetectorResult{
		{ID: "half", BPM: float64Ptr(70), Key: "C major"},
		{ID: "relative", BPM: float64Ptr(140), Key: "A minor"},
		{ID: "harmonic", BPM: float64Ptr(140), Key: "G major"},
	}}
	return manifest, results, snapshot
}

func TestSpotifyReferenceSeparatesCoverageAndFrozenMetrics(t *testing.T) {
	manifest, results, snapshot := referenceFixture(t)
	before, _ := json.Marshal(manifest)
	report, err := CompareSpotifyReference(manifest, results, snapshot, SplitHeldOut)
	if err != nil {
		t.Fatal(err)
	}
	if report.Coverage.TracksInSplit != 6 || report.Coverage.Matched != 4 || report.Coverage.Missing != 1 || report.Coverage.Unconfirmed != 1 || report.Coverage.Unlabelled != 1 {
		t.Fatalf("coverage=%+v", report.Coverage)
	}
	if report.Comparison == nil || report.Comparison.Tempo.Labeled != 3 || report.Comparison.Tempo.StrictWithinHalf != 2 || report.Comparison.Tempo.HalfDoubleErrors != 1 || report.Comparison.Key.Exact != 1 || report.RelativeKeyMatches != 1 || report.HarmonicNeighborMatches != 1 {
		t.Fatalf("report=%+v comparison=%+v", report, report.Comparison)
	}
	if report.Comparison.Corpus.Phase0Ready || report.EvidenceClass != EvidenceSyntheticCI {
		t.Fatal("synthetic references must not close real-audio gate")
	}
	after, _ := json.Marshal(manifest)
	if string(before) != string(after) {
		t.Fatal("reference comparison changed local labels")
	}
	again, err := CompareSpotifyReference(manifest, results, snapshot, SplitHeldOut)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(report)
	secondJSON, _ := json.Marshal(again)
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("offline rerun is not deterministic")
	}
}

func TestSpotifyReferenceRejectsTamperingAndWrongRecording(t *testing.T) {
	for _, kind := range []string{"hash", "recording", "fingerprint", "version", "duplicate", "foreign", "mode", "confidence", "provenance", "evidence"} {
		t.Run(kind, func(t *testing.T) {
			manifest, results, snapshot := referenceFixture(t)
			switch kind {
			case "hash":
				snapshot.Entries[0].BPM = float64Ptr(123)
			case "recording":
				snapshot.Entries[0].Recording.RecordingID = strings.Repeat("B", 22)
			case "fingerprint":
				snapshot.Entries[0].Recording.AudioSHA256 = strings.Repeat("b", 64)
			case "version":
				snapshot.Entries[0].Recording.RecordingVersion = "different-version"
			case "duplicate":
				snapshot.Entries = append(snapshot.Entries, snapshot.Entries[0])
			case "foreign":
				snapshot.Entries[0].TrackID = "not-in-manifest"
			case "mode":
				invalidMode := 2
				snapshot.Entries[0].Mode = &invalidMode
			case "confidence":
				snapshot.Entries[0].TempoConfidence = float64Ptr(1.1)
			case "provenance":
				snapshot.AdapterRevision = ""
			case "evidence":
				snapshot.EvidenceClass = EvidenceLawfulRealAudio
			}
			if kind != "hash" {
				snapshot.CacheSnapshotHash, _ = snapshot.Hash()
			}
			if _, err := CompareSpotifyReference(manifest, results, snapshot, SplitHeldOut); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
}

func TestSpotifyReferenceAbsentScalarsAreNotUnknownLabels(t *testing.T) {
	manifest, results, snapshot := referenceFixture(t)
	for i := range snapshot.Entries {
		snapshot.Entries[i].BPM = nil
		snapshot.Entries[i].Key = nil
		snapshot.Entries[i].Mode = nil
	}
	snapshot.CacheSnapshotHash, _ = snapshot.Hash()
	report, err := CompareSpotifyReference(manifest, results, snapshot, SplitHeldOut)
	if err != nil {
		t.Fatal(err)
	}
	if report.Comparison != nil || report.Coverage.Unlabelled != 4 {
		t.Fatalf("absent data was scored: %+v", report)
	}
}

func TestSpotifyReferenceLoaderRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	_, _, snapshot := referenceFixture(t)
	encoded, _ := json.Marshal(snapshot)
	for _, suffix := range []string{"", " {}"} {
		path := filepath.Join(t.TempDir(), "snapshot.json")
		if err := os.WriteFile(path, append(encoded, []byte(suffix)...), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadSpotifyReferenceSnapshot(path)
		if (suffix == "") != (err == nil) {
			t.Fatalf("suffix=%q err=%v", suffix, err)
		}
	}
	path := filepath.Join(t.TempDir(), "unknown.json")
	data := strings.Replace(string(encoded), `"version":`, `"secret": "unexpected", "version":`, 1)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSpotifyReferenceSnapshot(path); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestRelativeKeyCompatibilityUsesMajorAndMinorDirection(t *testing.T) {
	for _, pair := range []struct {
		a, b string
		want bool
	}{
		{"C major", "A minor", true}, {"A minor", "C major", true},
		{"C major", "D# minor", false}, {"D# minor", "C major", false},
		{"C major", "G major", true}, {"C major", "C minor", false},
	} {
		a, _ := parseKey(pair.a)
		b, _ := parseKey(pair.b)
		if got := keysCompatible(a, b); got != pair.want {
			t.Errorf("%s/%s=%v want %v", pair.a, pair.b, got, pair.want)
		}
	}
}

func TestSpotifyReferencePartialKeyModeRetainsUnknownCoverage(t *testing.T) {
	for _, missing := range []string{"key", "mode"} {
		t.Run(missing, func(t *testing.T) {
			manifest, results, snapshot := referenceFixture(t)
			for i := range snapshot.Entries {
				snapshot.Entries[i].BPM = nil
				if missing == "key" {
					snapshot.Entries[i].Key = nil
				} else {
					snapshot.Entries[i].Mode = nil
				}
			}
			snapshot.CacheSnapshotHash, _ = snapshot.Hash()
			report, err := CompareSpotifyReference(manifest, results, snapshot, SplitHeldOut)
			if err != nil {
				t.Fatal(err)
			}
			if report.Comparison != nil || report.Coverage.UnavailableKey != 4 || report.Coverage.Unlabelled != 4 {
				t.Fatalf("partial key/mode was scored: %+v", report)
			}
		})
	}
}
