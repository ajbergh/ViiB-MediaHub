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

	"github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/analysis/tempo"
	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

type report struct {
	Fixtures          []fixtureReport                   `json:"fixtures"`
	Codecs            []analysisbench.CodecCapability   `json:"codecs"`
	WAV               *analysisbench.WAVInfo            `json:"wav,omitempty"`
	Comparison        *analysisbench.ComparisonReport   `json:"comparison,omitempty"`
	Gate              *analysisbench.Phase0GateReport   `json:"gate,omitempty"`
	Probes            []analysisbench.CodecProbe        `json:"probes,omitempty"`
	SpotifyImport     *analysisbench.CorpusImportReport `json:"spotifyImport,omitempty"`
	WrittenWAV        []string                          `json:"writtenWav,omitempty"`
	SyntheticManifest string                            `json:"syntheticManifest,omitempty"`
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
	gateOutputPath := flag.String("gate-out", "", "non-overwriting Phase 0 gate report JSON; requires -gate-manifest")
	probePaths := flag.String("probe", "", "comma-separated local .mp3/.ogg paths for unlabeled decoder/analyzer smoke measurements")
	analyzeManifestPath := flag.String("analyze", "", "local .mp3/.ogg corpus manifest to run through the Go analyzers")
	outputPath := flag.String("out", "", "non-overwriting Go analyzer result JSON; requires -analyze")
	analyzeSplit := flag.String("analyze-split", "", "optional reserved corpus split to analyze: tuning or held_out; requires -analyze")
	tempoMinOnsetCrest := flag.Float64("tempo-min-onset-crest", 0, "optional Phase 0 tempo refusal threshold; requires -analyze and must be tuned only on the tuning split")
	tempoMethod := flag.String("tempo-method", "", "optional Phase 0 tempo candidate method: peak-interval, onset-autocorrelation, or multifeature-consensus; requires -analyze")
	keyMaxChromaFlatness := flag.Float64("key-max-chroma-flatness", 0, "optional Phase 0 key refusal threshold; requires -analyze and must be tuned only on the tuning split")
	keyMaxFrequency := flag.Float64("key-max-frequency", 0, "optional Phase 0 chroma upper frequency in Hz; requires -analyze and must be tuned only on the tuning split")
	keyExtraction := flag.String("key-extraction", "", "optional Phase 0 key extraction: direct-chroma or hpcp-peaks; requires -analyze")
	keyProfile := flag.String("key-profile", "", "optional Phase 0 key profile: krumhansl or temperley; requires -analyze")
	spotifyCorpusRoot := flag.String("import-spotify-corpus", "", "directory tree containing per-folder Spotify BPM/key CSV files and local .mp3/.ogg media")
	spotifyCorpusOutput := flag.String("import-spotify-out", "", "non-overwriting analysisbench manifest JSON; requires -import-spotify-corpus")
	spotifyEvidenceClass := flag.String("import-spotify-evidence-class", "", "required evidence class declaration: lawful-real-audio or synthetic-ci")
	spotifyLicense := flag.String("import-spotify-license", "", "required local-audio license declaration, for example private-local")
	spotifyLabelSource := flag.String("import-spotify-label-source", "Spotify confirmed BPM/key data", "label-source text recorded in imported manifest entries")
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
	if *gateOutputPath != "" && *gateManifestPath == "" {
		fmt.Fprintln(os.Stderr, "analysisbench: -gate-out requires -gate-manifest")
		os.Exit(2)
	}
	if (*analyzeManifestPath == "") != (*outputPath == "") {
		fmt.Fprintln(os.Stderr, "analysisbench: -analyze and -out must be provided together")
		os.Exit(2)
	}
	if (*tempoMinOnsetCrest != 0 || *tempoMethod != "" || *keyMaxChromaFlatness != 0 || *keyMaxFrequency != 0 || *keyProfile != "" || *keyExtraction != "") && *analyzeManifestPath == "" {
		fmt.Fprintln(os.Stderr, "analysisbench: Phase 0 calibration flags require -analyze")
		os.Exit(2)
	}
	if *analyzeSplit != "" && *analyzeManifestPath == "" {
		fmt.Fprintln(os.Stderr, "analysisbench: -analyze-split requires -analyze")
		os.Exit(2)
	}
	if *tempoMinOnsetCrest < 0 || *keyMaxChromaFlatness < 0 || *keyMaxChromaFlatness > 1 || (*keyMaxFrequency != 0 && (*keyMaxFrequency < 25 || *keyMaxFrequency > 3500)) {
		fmt.Fprintln(os.Stderr, "analysisbench: invalid Phase 0 calibration threshold")
		os.Exit(2)
	}
	if *tempoMethod != "" && *tempoMethod != string(tempo.MethodPeakInterval) && *tempoMethod != string(tempo.MethodOnsetAutocorrelation) && *tempoMethod != string(tempo.MethodMultiFeatureConsensus) {
		fmt.Fprintln(os.Stderr, "analysisbench: invalid Phase 0 tempo method")
		os.Exit(2)
	}
	if *keyProfile != "" && *keyProfile != string(key.ProfileKrumhansl) && *keyProfile != string(key.ProfileTemperley) {
		fmt.Fprintln(os.Stderr, "analysisbench: invalid Phase 0 key profile")
		os.Exit(2)
	}
	if *keyExtraction != "" && *keyExtraction != string(key.ExtractionDirectChroma) && *keyExtraction != string(key.ExtractionHPCPPeaks) {
		fmt.Fprintln(os.Stderr, "analysisbench: invalid Phase 0 key extraction")
		os.Exit(2)
	}
	if (*spotifyCorpusRoot == "") != (*spotifyCorpusOutput == "") {
		fmt.Fprintln(os.Stderr, "analysisbench: -import-spotify-corpus and -import-spotify-out must be provided together")
		os.Exit(2)
	}
	if *spotifyCorpusRoot != "" && (strings.TrimSpace(*spotifyEvidenceClass) == "" || strings.TrimSpace(*spotifyLicense) == "") {
		fmt.Fprintln(os.Stderr, "analysisbench: -import-spotify-evidence-class and -import-spotify-license are required for a Spotify corpus import")
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
		if *analyzeSplit != "" {
			manifest, err = analysisbench.ManifestForSplit(manifest, *analyzeSplit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "analysisbench: select analysis split: %v\n", err)
				os.Exit(1)
			}
		}
		options := track.DefaultOptions()
		if *tempoMinOnsetCrest > 0 {
			options.Tempo.MinOnsetCrestFactor = *tempoMinOnsetCrest
		}
		if *tempoMethod != "" {
			options.Tempo.Method = tempo.Method(*tempoMethod)
		}
		if *keyMaxChromaFlatness > 0 {
			options.Key.MaxChromaFlatness = *keyMaxChromaFlatness
		}
		if *keyMaxFrequency > 0 {
			options.Key.MaxFrequency = *keyMaxFrequency
		}
		if *keyProfile != "" {
			options.Key.Profile = key.Profile(*keyProfile)
		}
		if *keyExtraction != "" {
			options.Key.Extraction = key.Extraction(*keyExtraction)
		}
		produced, err := track.ProduceBenchmarkResultsWithOptions(context.Background(), manifest, options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: run Go analyzers: %v\n", err)
			os.Exit(1)
		}
		if err := analysisbench.WriteResultSet(*outputPath, produced); err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: write Go results: %v\n", err)
			os.Exit(1)
		}
	}
	if *spotifyCorpusRoot != "" {
		imported, err := analysisbench.ImportSpotifyCorpus(*spotifyCorpusRoot, analysisbench.SpotifyCorpusOptions{
			EvidenceClass: *spotifyEvidenceClass, License: *spotifyLicense, LabelSource: *spotifyLabelSource,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: import Spotify corpus: %v\n", err)
			os.Exit(1)
		}
		if err := analysisbench.WriteSpotifyCorpusManifest(*spotifyCorpusOutput, imported.Manifest); err != nil {
			fmt.Fprintf(os.Stderr, "analysisbench: write Spotify corpus manifest: %v\n", err)
			os.Exit(1)
		}
		imported.ManifestOutput = *spotifyCorpusOutput
		result.SpotifyImport = &imported
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
		if *gateOutputPath != "" {
			if err := analysisbench.WritePhase0GateReport(*gateOutputPath, gate); err != nil {
				fmt.Fprintf(os.Stderr, "analysisbench: write Phase 0 gate report: %v\n", err)
				os.Exit(1)
			}
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
