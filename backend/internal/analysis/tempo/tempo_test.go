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

func TestOnsetAutocorrelationRecognizesSyntheticTempo(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("autocorrelation", 128, 12, 44100, 1)
	if err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions()
	options.Method = MethodOnsetAutocorrelation
	actual := EstimatePCMWithOptions(fixture.Samples, fixture.SampleRate, options)
	if !actual.Known || math.Abs(actual.BPM-128) > .5 {
		t.Fatalf("autocorrelation estimate = %#v, want ~128 BPM", actual)
	}
}

func TestMultiFeatureConsensusRecognizesSyntheticTempo(t *testing.T) {
	fixture, err := analysisbench.NewNoisyClickTrack("multifeature", 128, 12, 44100, 1, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions()
	options.Method = MethodMultiFeatureConsensus
	actual := EstimatePCMWithOptions(fixture.Samples, fixture.SampleRate, options)
	if !actual.Known || math.Abs(actual.BPM-128) > .5 || actual.Stability < .5 {
		t.Fatalf("multifeature estimate = %#v, want stable ~128 BPM", actual)
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
