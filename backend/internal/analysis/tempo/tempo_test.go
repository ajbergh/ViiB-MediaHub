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
