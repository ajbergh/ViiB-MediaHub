// Command analysisbench reports reproducible Phase 0 fixture and codec metadata.
// It is deliberately not linked into the desktop application.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

type report struct {
	Fixtures          []fixtureReport                 `json:"fixtures"`
	Codecs            []analysisbench.CodecCapability `json:"codecs"`
	WAV               *analysisbench.WAVInfo          `json:"wav,omitempty"`
	Comparison        *analysisbench.ComparisonReport `json:"comparison,omitempty"`
	Gate              *analysisbench.Phase0GateReport `json:"gate,omitempty"`
	Probes            []analysisbench.CodecProbe      `json:"probes,omitempty"`
	WrittenWAV        []string                        `json:"writtenWav,omitempty"`
	SyntheticManifest string                          `json:"syntheticManifest,omitempty"`
}

type fixtureReport struct {
	Name            string                         `json:"name"`
	Kind            string                         `json:"kind"`
	SampleRate      int                            `json:"sampleRate"`
	Channels        int                            `json:"channels"`
	Frames          int                            `json:"frames"`
	DurationSeconds float64                        `json:"durationSeconds"`
	Expected        analysisbench.ExpectedAnalysis `json:"expected"`
}

func main() {
	format := flag.String("format", "json", "output format: json")
	wavPath := flag.String("wav", "", "optional WAV file to inspect without decoding its data chunk")
	manifestPath := flag.String("manifest", "", "optional label-only corpus manifest JSON")
	resultsPath := flag.String("results", "", "detector result JSON; requires -manifest")
	gateManifestPath := flag.String("gate-manifest", "", "corpus manifest for a held-out Phase 0 go/no-go evaluation")
	candidateResultsPath := flag.String("candidate-results", "", "Go analyzer result JSON; requires -gate-manifest and -browser-results")
	browserResultsPath := flag.String("browser-results", "", "browser baseline result JSON; requires -gate-manifest and -candidate-results")
	determinismResultsPaths := flag.String("determinism-results", "", "comma-separated additional Go result JSON files from macOS/Linux/Windows for the determinism tripwire")
	probePaths := flag.String("probe", "", "comma-separated local .mp3/.ogg paths for unlabeled decoder/analyzer smoke measurements")
	analyzeManifestPath := flag.String("analyze", "", "local .mp3/.ogg corpus manifest to run through the Go analyzers")
	outputPath := flag.String("out", "", "non-overwriting Go analyzer result JSON; requires -analyze")
	split := flag.String("split", analysisbench.SplitHeldOut, "corpus split to compare: held_out or tuning")
	writeWAVDir := flag.String("write-wav-dir", "", "optional empty directory for generated synthetic PCM16 WAV artifacts")
	writeSyntheticManifest := flag.String("write-synthetic-manifest", "", "optional output path for a generated-fixture comparison manifest; requires -write-wav-dir")
	flag.Parse()
	if *format != "json" {
		fmt.Fprintln(os.Stderr, "analysisbench: only -format=json is supported")
		os.Exit(2)
	}
	if (*manifestPath == "") != (*resultsPath == "") {
		fmt.Fprintln(os.Stderr, "analysisbench: -manifest and -results must be provided together")
		os.Exit(2)
	}
	if (*gateManifestPath == "") != (*candidateResultsPath == "") || (*gateManifestPath == "") != (*browserResultsPath == "") {
		fmt.Fprintln(os.Stderr, "analysisbench: -gate-manifest, -candidate-results, and -browser-results must be provided together")
		os.Exit(2)
	}
	if *gateManifestPath != "" && *manifestPath != "" {
		fmt.Fprintln(os.Stderr, "analysisbench: use either -manifest/-results for one comparison or the -gate-* inputs for Phase 0 evaluation")
		os.Exit(2)
	}
	if (*analyzeManifestPath == "") != (*outputPath == "") {
		fmt.Fprintln(os.Stderr, "analysisbench: -analyze and -out must be provided together")
		os.Exit(2)
	}
	if *analyzeManifestPath != "" && *manifestPath != "" {
		fmt.Fprintln(os.Stderr, "analysisbench: use -analyze/-out to produce results, then -manifest/-results to compare them")
		os.Exit(2)
	}
	if *writeSyntheticManifest != "" && *writeWAVDir == "" {
		fmt.Fprintln(os.Stderr, "analysisbench: -write-synthetic-manifest requires -write-wav-dir")
		os.Exit(2)
	}

	fixtures, err := analysisbench.Phase0SyntheticFixtures()
	if err != nil {
		fmt.Fprintf(os.Stderr, "analysisbench: build fixtures: %v\n", err)
		os.Exit(1)
	}
	result := report{Codecs: analysisbench.CodecMatrix()}
	for _, fixture := range fixtures {
		result.Fixtures = append(result.Fixtures, fixtureReport{
			Name: fixture.Name, Kind: fixture.Kind, SampleRate: fixture.SampleRate,
			Channels: fixture.Channels, Frames: fixture.Frames(),
			DurationSeconds: fixture.DurationSeconds(), Expected: fixture.Expected,
		})
	}
	if *writeWAVDir != "" {
		paths, err := analysisbench.WriteFixturesWAV(*writeWAVDir, fixtures)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: write WAV fixtures: %v\n", err)
			os.Exit(1)
		}
		result.WrittenWAV = paths
	}
	if *writeSyntheticManifest != "" {
		if _, err := analysisbench.WriteSyntheticCorpusManifest(*writeSyntheticManifest, *writeWAVDir, fixtures); err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: write synthetic manifest: %v\n", err)
			os.Exit(1)
		}
		result.SyntheticManifest = *writeSyntheticManifest
	}
	if *wavPath != "" {
		file, err := os.Open(*wavPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: open WAV: %v\n", err)
			os.Exit(1)
		}
		info, inspectErr := analysisbench.InspectWAV(file)
		closeErr := file.Close()
		if inspectErr != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: inspect WAV: %v\n", inspectErr)
			os.Exit(1)
		}
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: close WAV: %v\n", closeErr)
			os.Exit(1)
		}
		result.WAV = &info
	}
	if *manifestPath != "" {
		manifest, err := analysisbench.LoadManifest(*manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: load manifest: %v\n", err)
			os.Exit(1)
		}
		results, err := analysisbench.LoadResultSet(*resultsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: load results: %v\n", err)
			os.Exit(1)
		}
		comparison, err := analysisbench.Compare(manifest, results, *split)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: compare results: %v\n", err)
			os.Exit(1)
		}
		result.Comparison = &comparison
	}
	if *analyzeManifestPath != "" {
		manifest, err := analysisbench.LoadManifest(*analyzeManifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: load analysis manifest: %v\n", err)
			os.Exit(1)
		}
		produced, err := track.ProduceBenchmarkResults(context.Background(), manifest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: run Go analyzers: %v\n", err)
			os.Exit(1)
		}
		if err := analysisbench.WriteResultSet(*outputPath, produced); err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: write Go results: %v\n", err)
			os.Exit(1)
		}
	}
	if *probePaths != "" {
		for _, path := range strings.Split(*probePaths, ",") {
			if strings.TrimSpace(path) == "" {
				continue
			}
			probe, err := track.ProbeBenchmarkFile(context.Background(), strings.TrimSpace(path))
			if err != nil {
				fmt.Fprintf(os.Stderr, "analysisbench: probe media: %v\n", err)
				os.Exit(1)
			}
			result.Probes = append(result.Probes, probe)
		}
	}
	if *gateManifestPath != "" {
		manifest, err := analysisbench.LoadManifest(*gateManifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: load gate manifest: %v\n", err)
			os.Exit(1)
		}
		candidate, err := analysisbench.LoadResultSet(*candidateResultsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: load candidate results: %v\n", err)
			os.Exit(1)
		}
		browserBaseline, err := analysisbench.LoadResultSet(*browserResultsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: load browser results: %v\n", err)
			os.Exit(1)
		}
		determinismSets := []analysisbench.ResultSet{candidate}
		for _, path := range strings.Split(*determinismResultsPaths, ",") {
			if strings.TrimSpace(path) == "" {
				continue
			}
			additional, err := analysisbench.LoadResultSet(strings.TrimSpace(path))
			if err != nil {
				fmt.Fprintf(os.Stderr, "analysisbench: load determinism result %q: %v\n", path, err)
				os.Exit(1)
			}
			determinismSets = append(determinismSets, additional)
		}
		gate, err := analysisbench.EvaluatePhase0Gate(manifest, candidate, browserBaseline, analysisbench.EvaluateDeterminism(determinismSets))
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: evaluate Phase 0 gate: %v\n", err)
			os.Exit(1)
		}
		result.Gate = &gate
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "analysisbench: encode report: %v\n", err)
		os.Exit(1)
	}
}
