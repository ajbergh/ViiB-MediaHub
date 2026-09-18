package track

import (
	"context"
	"encoding/json"
	"io"
	"math"
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
	var recordedOptions Options
	if err := json.Unmarshal(resultSet.Configuration, &recordedOptions); err != nil || recordedOptions != DefaultOptions() {
		t.Fatalf("configuration=%s error=%v, want exact default options", resultSet.Configuration, err)
	}
	if resultSet.Results[0].BPM == nil || resultSet.Results[0].AlternateBPM == nil || *resultSet.Results[0].AlternateBPM == *resultSet.Results[0].BPM {
		t.Fatalf("alternate=%v, want a distinct recorded alternative", resultSet.Results[0].AlternateBPM)
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

func TestBenchmarkRecordsCustomOptionsAndSuccessfulRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "silence.mp3")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := analysisbench.CorpusManifest{Version: "test", EvidenceClass: analysisbench.EvidenceSyntheticCI, Tracks: []analysisbench.CorpusTrack{{
		ID: "silence", Path: path, License: "generated", LabelSource: "generator", Genre: "silence", Split: analysisbench.SplitTuning, ExpectedUnknown: true,
	}}}
	registry := analysis.NewDecoderRegistry()
	if err := registry.Register([]string{".mp3"}, benchmarkDecoder{samples: make([]float32, 22050*8)}); err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions()
	options.Tempo.MinOnsetCrestFactor = 22
	options.Key.MaxFrequency = 3000
	results, err := ProduceBenchmarkResultsWithRegistryAndOptions(context.Background(), manifest, registry, options)
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), "results.json")
	if err := analysisbench.WriteResultSet(path, results); err != nil {
		t.Fatal(err)
	}
	loaded, err := analysisbench.LoadResultSet(path)
	if err != nil {
		t.Fatal(err)
	}
	var recorded Options
	if err := json.Unmarshal(loaded.Configuration, &recorded); err != nil || recorded != options {
		t.Fatalf("recorded=%+v want=%+v err=%v", recorded, options, err)
	}
	if loaded.Results[0].Status != "unknown" || loaded.Results[0].Error != "" {
		t.Fatalf("successful refusal not distinguished: %+v", loaded.Results[0])
	}
	report, err := analysisbench.Compare(manifest, loaded, analysisbench.SplitTuning)
	if err != nil || report.Unknown.Correct != 1 || len(report.Configuration) == 0 {
		t.Fatalf("comparison=%+v err=%v", report, err)
	}
}

func TestSyntheticBenchmarkRunsProductionDecoderAndPreservesRefusals(t *testing.T) {
	results, err := ProduceSyntheticBenchmarkResults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := analysisbench.Phase0SyntheticFixtures()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := analysisbench.SyntheticCorpusManifest(fixtures, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Results) != len(manifest.Tracks) || results.Algorithm != AlgorithmVersion || results.Throughput == nil || results.Throughput.AudioSeconds <= 0 {
		t.Fatalf("incomplete synthetic production evidence: %+v", results)
	}
	foundSilence, foundTempo := false, false
	for _, result := range results.Results {
		if result.Error != "" {
			t.Fatalf("generated fixture failed to decode: %+v", result)
		}
		if result.ID == "silence" {
			foundSilence = true
			if result.Status != "unknown" || result.BPM != nil || result.Key != "" {
				t.Fatalf("silence was not explicitly refused: %+v", result)
			}
		}
		if result.ID == "click-120" {
			foundTempo = true
			if result.BPM == nil || math.Abs(*result.BPM-120) > .5 {
				t.Fatalf("click fixture not measured: %+v", result)
			}
		}
	}
	if !foundSilence || !foundTempo {
		t.Fatal("production determinism evidence omitted known/unknown fixtures")
	}
}
