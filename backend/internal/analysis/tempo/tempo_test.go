package tempo

import (
	"bytes"
	"context"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestEstimatePCMRecognizesFractionalSyntheticTempo(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("fractional", 128.5, 8, 44100, 1)
	if err != nil {
		t.Fatal(err)
	}
	actual := EstimatePCM(fixture.Samples, fixture.SampleRate)
	if !actual.Known || math.Abs(actual.BPM-128.5) > .5 || actual.AlgorithmVersion != AlgorithmVersion {
		t.Fatalf("estimate = %#v", actual)
	}
}
func TestEstimatePCMReturnsUnknownForSilence(t *testing.T) {
	if actual := EstimatePCM(make([]float32, 44100), 44100); actual.Known {
		t.Fatalf("silence = %#v", actual)
	}
}

func TestOnsetAccumulatorMatchesOneShotAcrossChunkBoundaries(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("chunked", 124, 8, 44100, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := EstimatePCM(fixture.Samples, fixture.SampleRate)
	accumulator := NewOnsetAccumulator(fixture.SampleRate)
	for start := 0; start < len(fixture.Samples); {
		end := min(start+1733, len(fixture.Samples))
		accumulator.Feed(fixture.Samples[start:end])
		start = end
	}
	got := accumulator.Estimate()
	if !got.Known || math.Abs(got.BPM-want.BPM) > .001 || got.Confidence != want.Confidence {
		t.Fatalf("chunked = %#v, one-shot = %#v", got, want)
	}
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestEstimateLocalSongStreamsCanonicalWAV(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("song", 126, 8, 22050, 1)
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
	estimate, source, err := EstimateLocalSong(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "song")
	if err != nil || !estimate.Known || math.Abs(estimate.BPM-126) > .5 || source.SongID != "song" {
		t.Fatalf("estimate=%#v source=%#v err=%v", estimate, source, err)
	}
}

func TestAnalyzeAndPersistLocalSongKeepsMeasuredTempoSeparateFromLegacyBPM(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("song", 126, 8, 22050, 1)
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
	if err := database.UpdateSongMood("song", "", "", "", 120, false); err != nil {
		t.Fatal(err)
	}
	if _, err := AnalyzeAndPersistLocalSong(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "song", 1); err != nil {
		t.Fatal(err)
	}
	record, err := database.GetTrackAnalysis("song")
	if err != nil || record.Status != db.TrackAnalysisComplete || record.BPM == nil || math.Abs(*record.BPM-126) > .5 || record.BPMSource == nil || *record.BPMSource != "measured" {
		t.Fatalf("record=%#v err=%v", record, err)
	}
	songs, err := database.GetSongsByIDs([]string{"song"})
	if err != nil || len(songs) != 1 || songs[0].BPM != 120 {
		t.Fatalf("legacy songs=%#v err=%v", songs, err)
	}
}

func TestAnalyzeAndPersistLocalSongRecordsPartialStatusForSilence(t *testing.T) {
	fixture, err := analysisbench.NewSilence("silent", 4, 22050, 1)
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
	if err := database.SaveSong(&db.Song{ID: "silent", Title: "Silence", Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}); err != nil {
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
	if record.Status != db.TrackAnalysisPartial || record.BPM != nil || record.TempoKind == nil || *record.TempoKind != "unknown" {
		t.Fatalf("record=%#v", record)
	}
}

func TestEstimatePCMWithRangePresets(t *testing.T) {
	fixture160, err := analysisbench.NewClickTrack("fast", 160, 8, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Under Range60to120, 160 BPM folds to 80 BPM.
	est60to120 := EstimatePCMWithOptions(fixture160.Samples, fixture160.SampleRate, Options{Range: Range60to120})
	if !est60to120.Known || math.Abs(est60to120.BPM-80) > .5 {
		t.Fatalf("Range60to120 on 160 BPM = %#v, want ~80", est60to120)
	}

	fixture70, err := analysisbench.NewClickTrack("slow", 70, 8, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Under Range100to200, 70 BPM folds to 140 BPM.
	est100to200 := EstimatePCMWithOptions(fixture70.Samples, fixture70.SampleRate, Options{Range: Range100to200})
	if !est100to200.Known || math.Abs(est100to200.BPM-140) > .5 {
		t.Fatalf("Range100to200 on 70 BPM = %#v, want ~140", est100to200)
	}

	// Under Range70to140, 70 BPM stays 70 BPM.
	est70to140 := EstimatePCMWithOptions(fixture70.Samples, fixture70.SampleRate, Options{Range: Range70to140})
	if !est70to140.Known || math.Abs(est70to140.BPM-70) > .5 {
		t.Fatalf("Range70to140 on 70 BPM = %#v, want ~70", est70to140)
	}
}

func TestEstimatePCMAlternateCandidateAndStability(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("steady", 128, 8, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	actual := EstimatePCM(fixture.Samples, fixture.SampleRate)
	if !actual.Known {
		t.Fatal("expected known tempo")
	}
	if actual.Alternate <= 0 || math.Abs(actual.Alternate-64) > .5 {
		t.Fatalf("expected alternate metrical candidate ~64, got %v", actual.Alternate)
	}
	if actual.Stability < 0.85 {
		t.Fatalf("expected high stability (>= 0.85) for steady click track, got %v", actual.Stability)
	}
}

func TestEstimatePCMTempoRampStability(t *testing.T) {
	fixture, err := analysisbench.NewTempoRampClickTrack("ramp", 100, 140, 8, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	actual := EstimatePCM(fixture.Samples, fixture.SampleRate)
	// For a tempo ramp, stability should be below 0.85 (dynamic candidate).
	if actual.Stability >= 0.85 {
		t.Fatalf("expected ramp stability < 0.85, got %v", actual.Stability)
	}
}

func TestAnalyzeAndPersistLocalSongWithOptionsRecordsAlternateAndStability(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("song", 128, 8, 22050, 1)
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
	estimate, err := AnalyzeAndPersistLocalSongWithOptions(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "song", 1, Options{Range: Range70to140})
	if err != nil {
		t.Fatal(err)
	}
	if !estimate.Known {
		t.Fatalf("expected known estimate, got %#v", estimate)
	}
	record, err := database.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if record.BPMAltCandidate == nil || math.Abs(*record.BPMAltCandidate-64) > .5 {
		t.Fatalf("expected BPMAltCandidate ~64, got %#v", record.BPMAltCandidate)
	}
	if record.TempoStability == nil || *record.TempoStability < 0.85 {
		t.Fatalf("expected TempoStability >= 0.85, got %#v", record.TempoStability)
	}
	if record.TempoKind == nil || *record.TempoKind != "static" {
		t.Fatalf("expected TempoKind 'static', got %#v", record.TempoKind)
	}
}
