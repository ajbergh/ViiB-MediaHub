package track

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

// ProduceBenchmarkResults runs the backend-owned track analyzer over local
// .mp3 and .ogg manifest entries. Other codecs remain deliberately outside
// this focused Phase 0 slice.
func ProduceBenchmarkResults(ctx context.Context, manifest analysisbench.CorpusManifest) (analysisbench.ResultSet, error) {
	return ProduceBenchmarkResultsWithRegistry(ctx, manifest, analysis.NewDefaultDecoderRegistry())
}

// ProbeBenchmarkFile runs a focused MP3/Ogg decoder smoke measurement without
// claiming that unlabeled media contributes to any accuracy metric.
func ProbeBenchmarkFile(ctx context.Context, path string) (analysisbench.CodecProbe, error) {
	return ProbeBenchmarkFileWithRegistry(ctx, path, analysis.NewDefaultDecoderRegistry())
}

func ProbeBenchmarkFileWithRegistry(ctx context.Context, path string, registry *analysis.DecoderRegistry) (analysisbench.CodecProbe, error) {
	if !isBenchmarkCodec(path) {
		return analysisbench.CodecProbe{}, fmt.Errorf("probe %q uses %q; this Phase 0 runner currently supports only .mp3 and .ogg", filepath.Base(path), filepath.Ext(path))
	}
	if registry == nil {
		return analysisbench.CodecProbe{}, fmt.Errorf("analysis benchmark requires a decoder registry")
	}
	result, timing, err := AnalyzeFile(ctx, registry, path, DefaultOptions())
	probe := analysisbench.CodecProbe{
		Name: filepath.Base(path), Codec: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."), Algorithm: AlgorithmVersion,
		Status: result.Status,
		Timing: analysisbench.TrackTiming{
			Codec: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."), SampleRate: timing.SampleRate, SourceChannels: timing.SourceChannels,
			AudioSeconds: timing.AudioSeconds, DeclaredAudioSeconds: timing.DeclaredAudioSeconds, WallSeconds: timing.WallSeconds, DecodeAndStreamSeconds: timing.DecodeAndStreamSeconds, DSPSeconds: timing.DSPSeconds,
		},
	}
	if err != nil {
		code, _ := ClassifyError(err)
		probe.Status = "failed"
		probe.Error = code
		probe.ErrorMessage = err.Error()
		probe.Timing.Error = code
		return probe, nil
	}
	if result.Tempo.Known {
		probe.BPM = benchmarkFloat64Pointer(result.Tempo.BPM)
		probe.TempoConfidence = benchmarkFloat64Pointer(result.Tempo.Confidence)
	}
	if result.Key.Known {
		probe.Key = result.Key.Key
		probe.KeyConfidence = benchmarkFloat64Pointer(result.Key.Confidence)
	}
	return probe, nil
}

// ProduceBenchmarkResultsWithRegistry allows deterministic decoder
// substitution in tests while the CLI uses the production registry.
func ProduceBenchmarkResultsWithRegistry(ctx context.Context, manifest analysisbench.CorpusManifest, registry *analysis.DecoderRegistry) (analysisbench.ResultSet, error) {
	if err := manifest.Validate(); err != nil {
		return analysisbench.ResultSet{}, err
	}
	if registry == nil {
		return analysisbench.ResultSet{}, fmt.Errorf("analysis benchmark requires a decoder registry")
	}
	for _, corpusTrack := range manifest.Tracks {
		if !isBenchmarkCodec(corpusTrack.Path) {
			return analysisbench.ResultSet{}, fmt.Errorf("track %q uses %q; this Phase 0 runner currently supports only .mp3 and .ogg", corpusTrack.ID, filepath.Ext(corpusTrack.Path))
		}
	}

	throughput := analysisbench.ThroughputMetrics{Environment: benchmarkEnvironment()}
	resultSet := analysisbench.ResultSet{Algorithm: AlgorithmVersion, Throughput: &throughput}
	for _, corpusTrack := range manifest.Tracks {
		if err := ctx.Err(); err != nil {
			return analysisbench.ResultSet{}, err
		}
		result, timing, err := AnalyzeFile(ctx, registry, corpusTrack.Path, DefaultOptions())
		measurement := analysisbench.TrackTiming{
			ID: corpusTrack.ID, Codec: strings.TrimPrefix(strings.ToLower(filepath.Ext(corpusTrack.Path)), "."),
			SampleRate: timing.SampleRate, SourceChannels: timing.SourceChannels,
			AudioSeconds: timing.AudioSeconds, DeclaredAudioSeconds: timing.DeclaredAudioSeconds, WallSeconds: timing.WallSeconds,
			DecodeAndStreamSeconds: timing.DecodeAndStreamSeconds, DSPSeconds: timing.DSPSeconds,
		}
		detectorResult := analysisbench.DetectorResult{ID: corpusTrack.ID}
		if err != nil {
			code, _ := ClassifyError(err)
			detectorResult.Error = code
			detectorResult.ErrorMessage = err.Error()
			measurement.Error = code
		} else {
			if result.Tempo.Known {
				detectorResult.BPM = benchmarkFloat64Pointer(result.Tempo.BPM)
				detectorResult.TempoConfidence = benchmarkFloat64Pointer(result.Tempo.Confidence)
			}
			if result.Key.Known {
				detectorResult.Key = result.Key.Key
				detectorResult.KeyConfidence = benchmarkFloat64Pointer(result.Key.Confidence)
			}
		}
		resultSet.Results = append(resultSet.Results, detectorResult)
		throughput.Tracks = append(throughput.Tracks, measurement)
		throughput.AudioSeconds += measurement.AudioSeconds
		throughput.WallSeconds += measurement.WallSeconds
		throughput.DecodeAndStreamSeconds += measurement.DecodeAndStreamSeconds
		throughput.DSPSeconds += measurement.DSPSeconds
	}
	throughput.TotalRealtimeMultiple = benchmarkRealtimeMultiple(throughput.AudioSeconds, throughput.WallSeconds)
	throughput.DecodeRealtimeMultiple = benchmarkRealtimeMultiple(throughput.AudioSeconds, throughput.DecodeAndStreamSeconds)
	throughput.DSPRealtimeMultiple = benchmarkRealtimeMultiple(throughput.AudioSeconds, throughput.DSPSeconds)
	return resultSet, nil
}

func isBenchmarkCodec(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".ogg":
		return true
	default:
		return false
	}
}

func benchmarkEnvironment() analysisbench.BenchmarkEnvironment {
	return analysisbench.BenchmarkEnvironment{
		OS: runtime.GOOS, Architecture: runtime.GOARCH, CPUModel: strings.TrimSpace(os.Getenv("PROCESSOR_IDENTIFIER")),
		CPUCount: runtime.NumCPU(), GoVersion: runtime.Version(),
	}
}

func benchmarkRealtimeMultiple(audioSeconds, wallSeconds float64) *float64 {
	if audioSeconds <= 0 || wallSeconds <= 0 {
		return nil
	}
	value := audioSeconds / wallSeconds
	return &value
}

func benchmarkFloat64Pointer(value float64) *float64 { return &value }
