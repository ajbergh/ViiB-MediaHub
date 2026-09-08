package track

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

type benchmarkDecoder struct{ samples []float32 }

func (benchmarkDecoder) ID() string { return "benchmark-test" }
func (d benchmarkDecoder) Open(_ context.Context, source io.ReadCloser) (analysis.PCMStream, error) {
	return &benchmarkStream{source: source, samples: d.samples}, nil
}

type benchmarkStream struct {
	source  io.ReadCloser
	samples []float32
}

func (*benchmarkStream) Info() analysis.PCMInfo {
	return analysis.PCMInfo{SampleRate: 22050, Channels: 1}
}
func (s *benchmarkStream) Close() error { return s.source.Close() }
func (s *benchmarkStream) Read(_ context.Context, out []float32) (int, error) {
	if len(s.samples) == 0 {
		return 0, io.EOF
	}
	n := copy(out, s.samples)
	s.samples = s.samples[n:]
	if len(s.samples) == 0 {
		return n, io.EOF
	}
	return n, nil
}

func TestProduceBenchmarkResultsRunsProductionPathForMP3AndOgg(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("clicks", 128, 12, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	manifest := analysisbench.CorpusManifest{Version: "phase0-v1", EvidenceClass: analysisbench.EvidenceSyntheticCI, Tracks: []analysisbench.CorpusTrack{
		{ID: "mp3", Path: filepath.Join(directory, "track.mp3"), License: "private-local", LabelSource: "test fixture", Genre: "house", Split: analysisbench.SplitHeldOut, ExpectedBPM: benchmarkPointer(128)},
		{ID: "ogg", Path: filepath.Join(directory, "track.ogg"), License: "private-local", LabelSource: "test fixture", Genre: "techno", Split: analysisbench.SplitTuning, ExpectedBPM: benchmarkPointer(128)},
	}}
	for _, corpusTrack := range manifest.Tracks {
		if err := os.WriteFile(corpusTrack.Path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	registry := analysis.NewDecoderRegistry()
	if err := registry.Register([]string{".mp3", ".ogg"}, benchmarkDecoder{samples: fixture.Samples}); err != nil {
		t.Fatal(err)
	}
	resultSet, err := ProduceBenchmarkResultsWithRegistry(context.Background(), manifest, registry)
	if err != nil {
		t.Fatal(err)
	}
	if resultSet.Algorithm == "" || len(resultSet.Results) != 2 || resultSet.Throughput == nil {
		t.Fatalf("result set = %#v", resultSet)
	}
	if resultSet.Results[0].Error != "" || resultSet.Results[0].BPM == nil || resultSet.Results[0].Status == "" || resultSet.Results[0].TempoCrestFactor == nil || resultSet.Results[0].KeyFlatness == nil {
		t.Fatalf("MP3 result = %#v, want measured BPM", resultSet.Results[0])
	}
	if resultSet.Throughput.AudioSeconds < 23.9 || resultSet.Throughput.DSPSeconds <= 0 || resultSet.Throughput.DecodeAndStreamSeconds < 0 {
		t.Fatalf("throughput = %#v, want separate timing over both tracks", resultSet.Throughput)
	}
}

func TestProduceBenchmarkResultsRejectsDeferredCodecBeforeRunning(t *testing.T) {
	manifest := analysisbench.CorpusManifest{Version: "phase0-v1", EvidenceClass: analysisbench.EvidenceSyntheticCI, Tracks: []analysisbench.CorpusTrack{{
		ID: "wav", Path: "local.wav", License: "private-local", LabelSource: "test fixture", Genre: "house", Split: analysisbench.SplitHeldOut, ExpectedBPM: benchmarkPointer(128),
	}}}
	if _, err := ProduceBenchmarkResults(context.Background(), manifest); err == nil {
		t.Fatal("WAV was accepted despite the .mp3/.ogg-only Phase 0 scope")
	}
}

func TestProbeBenchmarkFileReportsDecoderEvidenceWithoutCorpusLabels(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("clicks", 128, 12, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "probe.mp3")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := analysis.NewDecoderRegistry()
	if err := registry.Register([]string{".mp3"}, benchmarkDecoder{samples: fixture.Samples}); err != nil {
		t.Fatal(err)
	}
	probe, err := ProbeBenchmarkFileWithRegistry(context.Background(), path, registry)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Error != "" || probe.BPM == nil || probe.Timing.AudioSeconds < 11.9 || probe.Timing.DSPSeconds <= 0 {
		t.Fatalf("probe = %#v, want decoder and analyzer evidence without a label claim", probe)
	}
	if _, err := ProbeBenchmarkFileWithRegistry(context.Background(), "unsupported.wav", registry); err == nil {
		t.Fatal("probe accepted deferred WAV scope")
	}
}

func benchmarkPointer(value float64) *float64 { return &value }
