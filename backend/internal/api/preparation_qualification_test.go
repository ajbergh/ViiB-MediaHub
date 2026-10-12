package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

var preparationInventory = flag.String("preparation-inventory", "", "local-only JSON [{id,path}] inventory; never included in reports")
var preparationCount = flag.Int("preparation-count", 1, "number of inventory tracks to qualify (1 or 105)")
var preparationReportPath = flag.String("preparation-report", "", "new, non-overwriting qualification report JSON")

type preparationBenchSong struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type preparationBenchArtifacts struct {
	Count              int   `json:"count"`
	StoredBytes        int64 `json:"storedBytes"`
	DecodedJSONBytes   int64 `json:"decodedJsonBytes"`
	MaximumStoredBytes int64 `json:"maximumStoredBytes"`
}

type preparationBenchPhase struct {
	Name                       string                               `json:"name"`
	SelectionSeconds           float64                              `json:"selectionSeconds"`
	MissingCandidates          int                                  `json:"missingCandidates"`
	PreparationSeconds         float64                              `json:"preparationSeconds"`
	Progress                   track.RunProgress                    `json:"progress"`
	DecoderOpens               int                                  `json:"decoderOpens"`
	TimingAbstentions          int                                  `json:"timingAbstentions"`
	TempoAbstentions           int                                  `json:"tempoAbstentions"`
	KeyAbstentions             int                                  `json:"keyAbstentions"`
	AudioSeconds               float64                              `json:"audioSeconds"`
	Artifacts                  map[string]preparationBenchArtifacts `json:"artifacts"`
	ListHTTPStatus             int                                  `json:"listHttpStatus"`
	ListPayloadBytes           int                                  `json:"listPayloadBytes"`
	ListReadSeconds            float64                              `json:"listReadSeconds"`
	DatabaseBytes              int64                                `json:"databaseBytes"`
	WALBytes                   int64                                `json:"walBytes"`
	ProcessPeakWorkingSetBytes *uint64                              `json:"processPeakWorkingSetBytes,omitempty"`
}

type preparationBenchReport struct {
	CapturedAt       time.Time               `json:"capturedAt"`
	CompletedAt      time.Time               `json:"completedAt"`
	OS               string                  `json:"os"`
	Architecture     string                  `json:"architecture"`
	GoVersion        string                  `json:"goVersion"`
	CPUCount         int                     `json:"cpuCount"`
	CPUModel         string                  `json:"cpuModel,omitempty"`
	Algorithm        string                  `json:"algorithm"`
	Tracks           int                     `json:"tracks"`
	InputBytes       int64                   `json:"inputBytes"`
	Codecs           map[string]int          `json:"codecs"`
	ProviderMode     string                  `json:"providerMode"`
	ProviderRequests int                     `json:"providerRequests"`
	Limitations      []string                `json:"limitations"`
	Phases           []preparationBenchPhase `json:"phases"`
}

// This opt-in benchmark references local audio and owns only a temporary DB.
// Unlike New(), the API value below starts no scanner, scheduler or provider.
func BenchmarkLocalPreparationQualification(b *testing.B) {
	if *preparationInventory == "" {
		b.Skip("requires -preparation-inventory; see docs/SPOTIFY_PREPARATION_PERFORMANCE.md")
	}
	if b.N != 1 || (*preparationCount != 1 && *preparationCount != 105) || *preparationReportPath == "" {
		b.Fatal("requires -benchtime=1x, -preparation-count=1 or 105, and -preparation-report")
	}
	if _, err := os.Stat(*preparationReportPath); !errors.Is(err, os.ErrNotExist) {
		b.Fatal("report path already exists or cannot be checked")
	}
	input, err := os.Open(*preparationInventory)
	if err != nil {
		b.Fatal(err)
	}
	var songs []preparationBenchSong
	raw, err := io.ReadAll(io.LimitReader(input, (4<<20)+1))
	input.Close()
	if err != nil || len(raw) > 4<<20 || json.Unmarshal(raw, &songs) != nil {
		b.Fatal("invalid bounded local inventory")
	}
	if len(songs) < *preparationCount {
		b.Fatal("inventory contains too few tracks")
	}
	b.StopTimer()
	report, err := qualifyLocalPreparation(b.Context(), songs[:*preparationCount], b.TempDir(), func(progress track.RunProgress) {
		if progress.Processed%10 == 0 || progress.Processed == progress.Total {
			fmt.Fprintf(os.Stderr, "preparation qualification: %d/%d processed\n", progress.Processed, progress.Total)
		}
	})
	if err != nil {
		b.Fatal(err)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		b.Fatal(err)
	}
	output, err := os.OpenFile(*preparationReportPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		b.Fatal("report must be a new writable file:", err)
	}
	_, writeErr := output.Write(append(encoded, '\n'))
	closeErr := output.Close()
	if writeErr != nil {
		b.Fatal(writeErr)
	}
	if closeErr != nil {
		b.Fatal(closeErr)
	}
	for _, phase := range report.Phases {
		b.ReportMetric(phase.PreparationSeconds, phase.Name+"-seconds")
		b.ReportMetric(float64(phase.DecoderOpens), phase.Name+"-decodes")
	}
}

func TestPreparationQualificationMeasuresFreshAndWarmWithoutChangingAudio(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("clicks", 128, 4, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	var audio bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&audio, fixture); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "owner-private-filename.wav")
	if err := os.WriteFile(path, audio.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	input := []preparationBenchSong{{ID: "owner-private-track-name", Path: path}}
	report, err := qualifyLocalPreparation(t.Context(), input, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Phases) != 2 {
		t.Fatalf("missing phases: %+v", report)
	}
	fresh, warm := report.Phases[0], report.Phases[1]
	if fresh.DecoderOpens != 1 || fresh.Progress.Analyzed != 1 || warm.DecoderOpens != 0 || warm.Progress.Skipped != 1 || warm.MissingCandidates != 0 {
		t.Fatalf("invalid decode/selection counters: fresh=%+v warm=%+v", fresh, warm)
	}
	for _, phase := range report.Phases {
		if phase.ListHTTPStatus != http.StatusOK || phase.ListPayloadBytes == 0 || phase.DatabaseBytes+phase.WALBytes == 0 || phase.Artifacts[threeband.Kind].StoredBytes == 0 || phase.Artifacts[threeband.Kind].DecodedJSONBytes == 0 {
			t.Fatalf("missing measured sizes: %+v", phase)
		}
		if runtime.GOOS == "windows" && (phase.ProcessPeakWorkingSetBytes == nil || *phase.ProcessPeakWorkingSetBytes == 0) {
			t.Fatal("Windows RSS instrumentation unavailable")
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "owner-private") || strings.Contains(string(encoded), path) {
		t.Fatal("report leaked inventory identity/path")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, audio.Bytes()) {
		t.Fatal("qualification changed source audio")
	}
	if _, err := qualifyLocalPreparation(t.Context(), append(input, input...), t.TempDir(), nil); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
}

func TestPreparationQualificationAcceptsSettledTimingAbstention(t *testing.T) {
	fixture, err := analysisbench.NewSilence("silence", 4, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	var audio bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&audio, fixture); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "silence.wav")
	if err := os.WriteFile(path, audio.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := qualifyLocalPreparation(t.Context(), []preparationBenchSong{{ID: "silence", Path: path}}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Phases[0].TimingAbstentions != 1 || report.Phases[1].TimingAbstentions != 1 || report.Phases[1].DecoderOpens != 0 {
		t.Fatalf("settled abstention misclassified: %+v", report.Phases)
	}
}

func qualifyLocalPreparation(ctx context.Context, songs []preparationBenchSong, directory string, progress func(track.RunProgress)) (preparationBenchReport, error) {
	report := preparationBenchReport{CapturedAt: time.Now().UTC(), OS: runtime.GOOS, Architecture: runtime.GOARCH, GoVersion: runtime.Version(), CPUCount: runtime.NumCPU(), CPUModel: os.Getenv("PROCESSOR_IDENTIFIER"), Algorithm: track.AlgorithmVersion, Tracks: len(songs), ProviderMode: "disabled: no provider callback, client, credentials or cached provider state", Limitations: []string{
		"Unlabeled local preparation baseline; not an accuracy or provider-connected qualification.",
		"Fresh means an empty scratch analysis DB; warm reruns current preparation in that DB. Both are offline.",
		"Peak working set is the process lifetime high-water mark, including previous phases; absent on unsupported hosts.",
		"Memory belongs to the standalone benchmark executable, not the running MediaHub GUI or development server.",
		"Decoded artifact size is JSON serialization of the validated decoded representation, not a Go heap-size estimate.",
		"HTTP payload uses the real analysis-list handler in-process; no browser rendering or network transport measurement.",
		"OS filesystem caches are not flushed; run only when competing analysis work is idle and record host conditions separately.",
	}}
	report.Codecs = map[string]int{}
	seen := map[string]bool{}
	for _, song := range songs {
		if song.ID == "" || seen[song.ID] || !filepath.IsAbs(song.Path) {
			return report, errors.New("inventory requires unique IDs and absolute local paths")
		}
		seen[song.ID] = true
		info, err := os.Stat(song.Path)
		if err != nil || !info.Mode().IsRegular() {
			return report, errors.New("inventory contains unavailable/non-regular audio")
		}
		report.InputBytes += info.Size()
		report.Codecs[strings.ToLower(filepath.Ext(song.Path))]++
	}
	databasePath := filepath.Join(directory, "library.db")
	database, err := db.New(databasePath)
	if err != nil {
		return report, err
	}
	defer database.Close()
	ids := make([]string, 0, len(songs))
	for _, song := range songs {
		if err := database.SaveSong(&db.Song{ID: song.ID, Title: "Performance track", Artist: "Local inventory", FilePath: song.Path}); err != nil {
			return report, err
		}
		ids = append(ids, song.ID)
	}
	registry := analysis.NewDefaultDecoderRegistry()
	for _, name := range []string{"fresh_offline", "warm_offline"} {
		phase := preparationBenchPhase{Name: name}
		start := time.Now()
		selected, err := track.ExpandPreparationJobSelection(ctx, database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
		if err != nil {
			return report, err
		}
		phase.SelectionSeconds, phase.MissingCandidates = time.Since(start).Seconds(), len(selected)
		// Run all IDs on the warm pass as well: verify authoritative skip, not
		// merely an empty selection that never exercises the preparation runner.
		start = time.Now()
		phase.Progress, err = track.Run(ctx, database, registry, ids, track.RunOptions{Progress: progress, ResolveSource: func(_ context.Context, id string) (analysis.ResolvedSource, error) {
			source, err := analysis.ResolveLocalSource(database, id)
			open := source.Open
			source.OpenStream = func() (io.ReadCloser, error) { phase.DecoderOpens++; return open() }
			return source, err
		}})
		phase.PreparationSeconds = time.Since(start).Seconds()
		// SongID is diagnostic only and can contain an owner-supplied name.
		phase.Progress.SongID = ""
		if err != nil {
			return report, err
		}
		for _, id := range ids {
			record, err := database.GetTrackAnalysis(id)
			if err != nil {
				return report, err
			}
			states, err := database.GetTrackCapabilityStatuses(id, record.SourceFingerprint)
			if err != nil {
				return report, err
			}
			if states["core_preparation"].State != "available" {
				return report, errors.New("preparation source failures prevent baseline qualification")
			}
			if record.Local != nil && record.Local.BPM == nil && record.Local.KeyTonic == nil {
				phase.TimingAbstentions++
			}
			if record.Local != nil && record.Local.BPM == nil {
				phase.TempoAbstentions++
			}
			if record.Local != nil && record.Local.KeyTonic == nil {
				phase.KeyAbstentions++
			}
			if record.Local != nil {
				for _, field := range record.Local.Fields {
					if field.Key == "local_duration_seconds" {
						var seconds float64
						if err := json.Unmarshal(field.Value, &seconds); err != nil {
							return report, err
						}
						phase.AudioSeconds += seconds
					}
				}
			}
		}
		if name == "fresh_offline" && (phase.MissingCandidates != len(ids) || phase.DecoderOpens != len(ids)) {
			return report, errors.New("fresh preparation did not decode each track exactly once")
		}
		if name == "warm_offline" && (phase.MissingCandidates != 0 || phase.DecoderOpens != 0 || phase.Progress.Skipped != len(ids)) {
			return report, errors.New("warm preparation repeated settled work")
		}
		phase.Artifacts, err = preparationArtifactSizes(database, ids)
		if err != nil {
			return report, err
		}
		response := httptest.NewRecorder()
		start = time.Now()
		(&API{db: database}).listTrackAnalysisFeaturesV2(response, httptest.NewRequest(http.MethodGet, "/analysis", nil))
		phase.ListReadSeconds = time.Since(start).Seconds()
		phase.ListHTTPStatus, phase.ListPayloadBytes = response.Code, response.Body.Len()
		if response.Code != http.StatusOK {
			return report, errors.New("analysis list failed during qualification")
		}
		for path, destination := range map[string]*int64{databasePath: &phase.DatabaseBytes, databasePath + "-wal": &phase.WALBytes} {
			info, err := os.Stat(path)
			if err == nil {
				*destination = info.Size()
			} else if !errors.Is(err, os.ErrNotExist) {
				return report, err
			}
		}
		phase.ProcessPeakWorkingSetBytes = preparationPeakWorkingSet()
		report.Phases = append(report.Phases, phase)
	}
	report.CompletedAt = time.Now().UTC()
	return report, nil
}

type artifactByteCounter int64

func (count *artifactByteCounter) Write(data []byte) (int, error) {
	*count += artifactByteCounter(len(data))
	return len(data), nil
}

func preparationArtifactSizes(database *db.DB, ids []string) (map[string]preparationBenchArtifacts, error) {
	result := map[string]preparationBenchArtifacts{}
	families := []struct {
		kind      string
		format    int
		algorithm string
		limit     int64
		decode    func([]byte) (any, error)
	}{
		{threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion, threeband.MaxBytes, func(data []byte) (any, error) { return threeband.Decode(data) }},
		{waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion, waveformartifact.MaxBytes, func(data []byte) (any, error) { return waveformartifact.Decode(data) }},
		{beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion, 16 << 20, func(data []byte) (any, error) { return beatgrid.Decode(data) }},
		{features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion, features.MaxStructureStatusArtifactBytes, func(data []byte) (any, error) {
			return features.DecodeBounded(data, features.MaxStructureStatusArtifactBytes)
		}},
		{features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion, features.MaxBS1770ArtifactBytes, func(data []byte) (any, error) { return features.DecodeBS1770(data) }},
	}
	for _, family := range families {
		sizes := preparationBenchArtifacts{}
		for _, id := range ids {
			artifact, err := database.GetTrackAnalysisArtifactBounded(id, family.kind, family.format, family.algorithm, family.limit)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			decoded, err := family.decode(artifact.Data)
			if err != nil {
				return nil, fmt.Errorf("invalid %s artifact", family.kind)
			}
			var count artifactByteCounter
			if err := json.NewEncoder(&count).Encode(decoded); err != nil {
				return nil, err
			}
			sizes.Count++
			sizes.StoredBytes += int64(len(artifact.Data))
			sizes.DecodedJSONBytes += int64(count)
			sizes.MaximumStoredBytes = max(sizes.MaximumStoredBytes, int64(len(artifact.Data)))
		}
		result[family.kind] = sizes
	}
	return result, nil
}
