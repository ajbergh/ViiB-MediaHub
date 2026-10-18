// Command cuebench exercises production cue analysis on local tuning recordings.
// It never imports audio into the library or opens held-out manifest rows.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/cues"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

type measurement struct {
	ID           string                    `json:"id"`
	Genre        string                    `json:"genre"`
	Split        string                    `json:"split"`
	SourceSHA256 string                    `json:"sourceSha256,omitempty"`
	Duration     float64                   `json:"decodedDurationSeconds"`
	WallSeconds  float64                   `json:"wallSeconds"`
	Events       []features.StructureEvent `json:"events"`
	Cues         []cues.Cue                `json:"cues"`
	RoundTripOK  bool                      `json:"artifactAndGridRoundTripOk"`
	Error        string                    `json:"error,omitempty"`
}

// Round-robin by genre prevents a filename-sorted manifest limit from selecting
// just the first collection. Split filtering must precede even source hashing.
func selectTuning(m analysisbench.CorpusManifest, limit int) ([]analysisbench.CorpusTrack, error) {
	tuning, err := analysisbench.ManifestForSplit(m, analysisbench.SplitTuning)
	if err != nil {
		return nil, err
	}
	groups := map[string][]analysisbench.CorpusTrack{}
	for _, t := range tuning.Tracks {
		groups[t.Genre] = append(groups[t.Genre], t)
	}
	genres := []string{}
	for genre := range groups {
		genres = append(genres, genre)
	}
	sort.Strings(genres)
	selected := []analysisbench.CorpusTrack{}
	for index := 0; len(selected) < len(tuning.Tracks) && (limit <= 0 || len(selected) < limit); index++ {
		for _, genre := range genres {
			if index < len(groups[genre]) {
				selected = append(selected, groups[genre][index])
			}
			if limit > 0 && len(selected) == limit {
				break
			}
		}
	}
	return selected, nil
}

func analyze(ctx context.Context, t analysisbench.CorpusTrack, registry *analysis.DecoderRegistry) (row measurement) {
	row = measurement{ID: t.ID, Genre: t.Genre, Split: t.Split, Events: []features.StructureEvent{}, Cues: []cues.Cue{}}
	started := time.Now()
	defer func() { row.WallSeconds = time.Since(started).Seconds() }()
	f, err := os.Open(t.Path)
	if err != nil {
		row.Error = err.Error()
		return
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		row.Error = err.Error()
		return
	}
	row.SourceSHA256 = hex.EncodeToString(h.Sum(nil))
	r, _, err := track.AnalyzeFile(ctx, registry, t.Path, track.DefaultOptions())
	row.Duration = r.DurationSeconds
	if err != nil {
		row.Error = err.Error()
		return
	}
	if r.Features == nil {
		row.Error = "no features from decoder"
		return
	}
	if err = r.Features.ValidateNativeEvents(); err != nil {
		row.Error = err.Error()
		return
	}
	row.Events = r.Features.Events
	row.Cues, err = cues.Generate(r.DurationSeconds, r.BeatGrid, *r.Features, row.SourceSHA256)
	if err != nil {
		row.Error = err.Error()
		return
	}
	encoded, err := r.Features.Encode()
	if err != nil {
		row.Error = err.Error()
		return
	}
	decoded, err := features.DecodeCurrentBounded(encoded, features.MaxStructureStatusArtifactBytes)
	if err != nil {
		row.Error = err.Error()
		return
	}
	var restoredGrid *beatgrid.Grid
	if r.BeatGrid != nil {
		data, e := r.BeatGrid.Encode()
		if e != nil {
			row.Error = e.Error()
			return
		}
		grid, e := beatgrid.Decode(data)
		if e != nil {
			row.Error = e.Error()
			return
		}
		restoredGrid = &grid
	}
	// Deliberately wrong catalog duration checks canonical duration precedence.
	regenerated, err := cues.Generate(1, restoredGrid, decoded, row.SourceSHA256)
	if err != nil {
		row.Error = err.Error()
		return
	}
	if len(regenerated) != len(row.Cues) {
		row.Error = "cue count changed after artifact round trip"
		return
	}
	for i, c := range row.Cues {
		if c.Position < 0 || c.Position >= r.DurationSeconds || c.Slot != i+1 || i > 0 && c.Position <= row.Cues[i-1].Position {
			row.Error = "invalid cue order or bounds"
			return
		}
		if math.Abs(c.Position-regenerated[i].Position) > 1e-6 || c.Label != regenerated[i].Label || c.DownbeatAligned != regenerated[i].DownbeatAligned {
			row.Error = "cue changed after artifact/grid round trip"
			return
		}
		if c.DownbeatAligned {
			row.Error = "native inferred meter claimed verified downbeat"
			return
		}
	}
	row.RoundTripOK = true
	return
}

func main() {
	manifest := flag.String("manifest", "", "local corpus manifest; paths resolve from current working directory")
	out := flag.String("out", "", "new JSON report path (will not overwrite)")
	limit := flag.Int("limit", 24, "maximum tuning tracks, balanced by genre; 0 selects all tuning tracks")
	flag.Parse()
	if err := run(*manifest, *out, *limit); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(manifestPath, out string, limit int) error {
	if manifestPath == "" || out == "" || limit < 0 {
		return fmt.Errorf("require -manifest, -out and a nonnegative -limit")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		return fmt.Errorf("output already exists or cannot be checked: %s", out)
	}
	m, err := analysisbench.LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	selected, err := selectTuning(m, limit)
	if err != nil {
		return err
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	manifestHash := sha256.Sum256(manifestBytes)
	report := struct {
		Schema           string        `json:"schema"`
		ManifestSHA256   string        `json:"manifestSha256"`
		GeneratorVersion string        `json:"generatorVersion"`
		FeatureVersion   string        `json:"featureVersion"`
		Runtime          string        `json:"runtime"`
		Evidence         string        `json:"evidence"`
		Results          []measurement `json:"results"`
	}{"viib.cue-real-audio.v1", hex.EncodeToString(manifestHash[:]), cues.GeneratorVersion, features.AlgorithmVersion, runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version(), "Tuning-only engineering validation; no independent DJ cue annotations or acceptance score.", []measurement{}}
	registry := analysis.NewDefaultDecoderRegistry()
	failed := 0
	for _, t := range selected {
		row := analyze(context.Background(), t, registry)
		report.Results = append(report.Results, row)
		if row.Error != "" {
			failed++
		}
		fmt.Fprintf(os.Stderr, "%d/%d %s: %d cues, error=%q\n", len(report.Results), len(selected), t.ID, len(row.Cues), row.Error)
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(report)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d real-audio rows failed; see %s", failed, len(selected), out)
	}
	return nil
}
