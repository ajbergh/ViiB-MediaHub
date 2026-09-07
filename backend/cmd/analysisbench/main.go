// Command analysisbench reports reproducible Phase 0 fixture and codec metadata.
// It is deliberately not linked into the desktop application.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

type report struct {
	Fixtures   []fixtureReport                 `json:"fixtures"`
	Codecs     []analysisbench.CodecCapability `json:"codecs"`
	WAV        *analysisbench.WAVInfo          `json:"wav,omitempty"`
	Comparison *analysisbench.ComparisonReport `json:"comparison,omitempty"`
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
	split := flag.String("split", analysisbench.SplitHeldOut, "corpus split to compare: held_out or tuning")
	flag.Parse()
	if *format != "json" {
		fmt.Fprintln(os.Stderr, "analysisbench: only -format=json is supported")
		os.Exit(2)
	}
	if (*manifestPath == "") != (*resultsPath == "") {
		fmt.Fprintln(os.Stderr, "analysisbench: -manifest and -results must be provided together")
		os.Exit(2)
	}

	fixtures, err := analysisbench.DefaultFixtures()
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

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "analysisbench: encode report: %v\n", err)
		os.Exit(1)
	}
}
