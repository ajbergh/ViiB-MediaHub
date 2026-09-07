package key

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestNotationMappingsAll24Keys(t *testing.T) {
	expectedCamelotMajor := []string{"8B", "3B", "10B", "5B", "12B", "7B", "2B", "9B", "4B", "11B", "6B", "1B"}
	expectedCamelotMinor := []string{"5A", "12A", "7A", "2A", "9A", "4A", "11A", "6A", "1A", "8A", "3A", "10A"}

	expectedOpenMajor := []string{"1d", "8d", "3d", "10d", "5d", "12d", "7d", "2d", "9d", "4d", "11d", "6d"}
	expectedOpenMinor := []string{"10m", "5m", "12m", "7m", "2m", "9m", "4m", "11m", "6m", "1m", "8m", "3m"}

	for tonic := 0; tonic < 12; tonic++ {
		camMaj := Camelot(tonic, ModeMajor)
		if camMaj != expectedCamelotMajor[tonic] {
			t.Errorf("Camelot(%d, major) = %s, want %s", tonic, camMaj, expectedCamelotMajor[tonic])
		}
		camMin := Camelot(tonic, ModeMinor)
		if camMin != expectedCamelotMinor[tonic] {
			t.Errorf("Camelot(%d, minor) = %s, want %s", tonic, camMin, expectedCamelotMinor[tonic])
		}

		openMaj := OpenKey(tonic, ModeMajor)
		if openMaj != expectedOpenMajor[tonic] {
			t.Errorf("OpenKey(%d, major) = %s, want %s", tonic, openMaj, expectedOpenMajor[tonic])
		}
		openMin := OpenKey(tonic, ModeMinor)
		if openMin != expectedOpenMinor[tonic] {
			t.Errorf("OpenKey(%d, minor) = %s, want %s", tonic, openMin, expectedOpenMinor[tonic])
		}
	}
}

func TestHarmonicRelations(t *testing.T) {
	cases := []struct {
		keyA, keyB string
		wantRel    string
		wantComp   bool
	}{
		{"8A", "8A", "same", true},
		{"8A", "9A", "adjacent", true},
		{"8A", "7A", "adjacent", true},
		{"1A", "12A", "adjacent", true},
		{"12A", "1A", "adjacent", true},
		{"8A", "8B", "relative", true},
		{"8B", "8A", "relative", true},
		{"8A", "10A", "other", false},
		{"8A", "2B", "other", false},
		{"", "8A", "unknown", false},
		{"N/A", "8A", "unknown", false},
	}
	for _, tc := range cases {
		rel, comp := HarmonicRelation(tc.keyA, tc.keyB)
		if rel != tc.wantRel || comp != tc.wantComp {
			t.Errorf("HarmonicRelation(%q, %q) = (%q, %v), want (%q, %v)", tc.keyA, tc.keyB, rel, comp, tc.wantRel, tc.wantComp)
		}
	}
}

func TestEstimatePCMRecognizesAllSyntheticTriads(t *testing.T) {
	for tonic := 0; tonic < 12; tonic++ {
		for _, minor := range []bool{false, true} {
			fixture, err := analysisbench.NewTriad("triad", tonic, minor, 1.5, 22050, 1, 440)
			if err != nil {
				t.Fatal(err)
			}
			actual := EstimatePCM(fixture.Samples, fixture.SampleRate)
			if !actual.Known {
				t.Fatalf("%s: expected known key estimate", fixture.Expected.Key)
			}
			expectedMode := ModeMajor
			if minor {
				expectedMode = ModeMinor
			}
			if actual.Tonic != tonic || actual.Mode != expectedMode {
				t.Fatalf("%s: EstimatePCM() = tonic %d mode %s, want tonic %d mode %s (actual key=%q)",
					fixture.Expected.Key, actual.Tonic, actual.Mode, tonic, expectedMode, actual.Key)
			}
			if actual.Confidence <= 0 {
				t.Fatalf("%s: expected positive confidence, got %v", fixture.Expected.Key, actual.Confidence)
			}
			if actual.Camelot == "" || actual.OpenKey == "" {
				t.Fatalf("%s: expected Camelot and OpenKey notations populated", fixture.Expected.Key)
			}
		}
	}
}

func TestEstimatePCMReturnsUnknownForSilence(t *testing.T) {
	actual := EstimatePCM(make([]float32, 22050), 22050)
	if actual.Known {
		t.Fatalf("EstimatePCM(silence) = %+v, want unknown", actual)
	}
}

func TestChromaAccumulatorMatchesOneShotAcrossChunks(t *testing.T) {
	fixture, err := analysisbench.NewTriad("chunked-c-maj", 0, false, 2.0, 22050, 1, 440)
	if err != nil {
		t.Fatal(err)
	}
	oneShot := EstimatePCM(fixture.Samples, fixture.SampleRate)

	acc := NewChromaAccumulator(fixture.SampleRate)
	chunkSize := 512
	for start := 0; start < len(fixture.Samples); start += chunkSize {
		end := start + chunkSize
		if end > len(fixture.Samples) {
			end = len(fixture.Samples)
		}
		acc.Feed(fixture.Samples[start:end])
	}
	chunked := acc.Estimate()

	if !chunked.Known || chunked.Tonic != oneShot.Tonic || chunked.Mode != oneShot.Mode {
		t.Fatalf("chunked = %+v, want oneShot = %+v", chunked, oneShot)
	}
	if math.Abs(chunked.Confidence-oneShot.Confidence) > 1e-4 {
		t.Fatalf("confidence delta too large: chunked=%v oneShot=%v", chunked.Confidence, oneShot.Confidence)
	}
}

func TestAnalyzeAndPersistLocalSongKeepsMeasuredKeyAndPreservesTempo(t *testing.T) {
	fixture, err := analysisbench.NewTriad("song", 9, true, 1.5, 22050, 1, 440) // A minor (8A)
	if err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, fixture); err != nil {
		t.Fatal(err)
	}
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	path := filepath.Join(t.TempDir(), "song.wav")
	if err := os.WriteFile(path, wav.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, "song")
	if err != nil {
		t.Fatal(err)
	}
	// Pre-seed an existing tempo analysis on the same source
	bpmVal := 124.0
	bpmConf := 0.95
	bpmSource := "measured"
	tempoKind := "static"
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{
		SongID:            "song",
		Status:            db.TrackAnalysisComplete,
		AnalysisVersion:   1,
		AlgorithmVersion:  "tempo-v1",
		SourceFingerprint: source.Fingerprint,
		BPM:               &bpmVal,
		BPMConfidence:     &bpmConf,
		BPMSource:         &bpmSource,
		TempoKind:         &tempoKind,
	}); err != nil {
		t.Fatal(err)
	}

	estimate, err := AnalyzeAndPersistLocalSong(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "song", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !estimate.Known || estimate.Tonic != 9 || estimate.Mode != ModeMinor {
		t.Fatalf("estimate = %#v, want A minor", estimate)
	}

	record, err := database.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	// Key fields must be populated
	if record.KeyTonic == nil || *record.KeyTonic != 9 || record.KeyMode == nil || *record.KeyMode != ModeMinor {
		t.Fatalf("record key = %#v, want A minor", record)
	}
	if record.CamelotKey == nil || *record.CamelotKey != "8A" {
		t.Fatalf("record Camelot = %#v, want 8A", record.CamelotKey)
	}
	if record.OpenKey == nil || *record.OpenKey != "1m" {
		t.Fatalf("record OpenKey = %#v, want 1m", record.OpenKey)
	}
	// Pre-existing tempo fields must be preserved
	if record.BPM == nil || *record.BPM != 124.0 || record.BPMSource == nil || *record.BPMSource != "measured" {
		t.Fatalf("pre-existing tempo was lost: %#v", record)
	}
}

func TestAnalyzeAndPersistLocalSongRecordsPartialForSilence(t *testing.T) {
	fixture, err := analysisbench.NewSilence("silent", 2, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, fixture); err != nil {
		t.Fatal(err)
	}
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	path := filepath.Join(t.TempDir(), "silent.wav")
	if err := os.WriteFile(path, wav.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "silent", Title: "Silent", Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	estimate, err := AnalyzeAndPersistLocalSong(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "silent", 1)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.Known {
		t.Fatalf("expected unknown estimate for silence, got %#v", estimate)
	}
	record, err := database.GetTrackAnalysis("silent")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != db.TrackAnalysisPartial || record.KeyTonic != nil {
		t.Fatalf("expected partial status and nil key for silence, got %#v", record)
	}
}

func BenchmarkEstimateKeyTriad(b *testing.B) {
	fixture, err := analysisbench.NewTriad("benchmark", 0, false, 1.5, 22050, 1, 440)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if result := EstimatePCM(fixture.Samples, fixture.SampleRate); !result.Known {
			b.Fatal("key unexpectedly returned unknown")
		}
	}
}
