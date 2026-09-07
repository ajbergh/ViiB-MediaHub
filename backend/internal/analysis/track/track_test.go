package track

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

// catalogSong writes a fixture as WAV, registers it as a canonical local song,
// and returns an open database handle.
func catalogSong(t *testing.T, fixture analysisbench.PCMFixture, songID string) *db.DB {
	t.Helper()
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, fixture); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	database, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	path := filepath.Join(directory, songID+".wav")
	if err := os.WriteFile(path, wav.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: songID, Title: songID, Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	return database
}

// tonalClickTrack sums a click track and a sustained triad so one pass has both
// percussive onsets and tonal content.
func tonalClickTrack(t *testing.T, name string, bpm float64, tonic int, minor bool, seconds float64, sampleRate int) analysisbench.PCMFixture {
	t.Helper()
	clicks, err := analysisbench.NewClickTrack(name, bpm, seconds, sampleRate, 1)
	if err != nil {
		t.Fatal(err)
	}
	triad, err := analysisbench.NewTriad(name, tonic, minor, seconds, sampleRate, 1, 440)
	if err != nil {
		t.Fatal(err)
	}
	mixed := clicks
	mixed.Samples = make([]float32, len(clicks.Samples))
	for i := range clicks.Samples {
		value := clicks.Samples[i]
		if i < len(triad.Samples) {
			value += triad.Samples[i] * 0.6
		}
		if value > 1 {
			value = 1
		} else if value < -1 {
			value = -1
		}
		mixed.Samples[i] = value
	}
	return mixed
}

func TestAnalyzeAndPersistRecordsBothDimensionsInOneRecord(t *testing.T) {
	fixture := tonalClickTrack(t, "song", 128, 9, true, 12, 22050)
	database := catalogSong(t, fixture, "song")

	result, err := AnalyzeAndPersist(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "song", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Tempo.Known {
		t.Fatalf("tempo was not measured: %#v", result.Tempo)
	}
	if !result.Key.Known {
		t.Fatalf("key was not measured: %#v", result.Key)
	}
	if result.Status != db.TrackAnalysisComplete {
		t.Fatalf("status = %q, want %q", result.Status, db.TrackAnalysisComplete)
	}

	record, err := database.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if record.BPM == nil || record.BPMSource == nil || *record.BPMSource != "measured" {
		t.Fatalf("record tempo = %#v, want measured BPM", record)
	}
	if record.KeyTonic == nil || record.KeySource == nil || *record.KeySource != "measured" {
		t.Fatalf("record key = %#v, want measured key", record)
	}
	if record.AlgorithmVersion != AlgorithmVersion {
		t.Fatalf("algorithm version = %q, want %q", record.AlgorithmVersion, AlgorithmVersion)
	}
	if record.SourceFingerprint == "" {
		t.Fatal("record must carry the decoded source fingerprint")
	}
	if record.ErrorCode != nil {
		t.Fatalf("complete record must not carry an error code, got %q", *record.ErrorCode)
	}
}

func TestAnalyzeAndPersistRecordsPartialWhenOnlyTempoIsMeasured(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("clicks", 128, 12, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	database := catalogSong(t, fixture, "clicks")

	result, err := AnalyzeAndPersist(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "clicks", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Tempo.Known {
		t.Fatalf("click track tempo was not measured: %#v", result.Tempo)
	}
	record, err := database.GetTrackAnalysis("clicks")
	if err != nil {
		t.Fatal(err)
	}
	// A percussive-only source may or may not expose tonal evidence, but the
	// persisted status must never claim more than was measured.
	if result.Key.Known {
		if record.Status != db.TrackAnalysisComplete {
			t.Fatalf("status = %q, want %q when both dimensions are known", record.Status, db.TrackAnalysisComplete)
		}
		return
	}
	if record.Status != db.TrackAnalysisPartial {
		t.Fatalf("status = %q, want %q when key is unknown", record.Status, db.TrackAnalysisPartial)
	}
	if record.ErrorCode == nil || *record.ErrorCode != ErrorNoReliableKey {
		t.Fatalf("error code = %#v, want %q", record.ErrorCode, ErrorNoReliableKey)
	}
	if record.KeyTonic != nil || record.KeySource != nil {
		t.Fatalf("unmeasured key must stay null, got %#v", record)
	}
}

func TestAnalyzeAndPersistRecordsPartialWhenOnlyKeyIsMeasured(t *testing.T) {
	fixture, err := analysisbench.NewTriad("triad", 0, false, 6, 22050, 1, 440)
	if err != nil {
		t.Fatal(err)
	}
	database := catalogSong(t, fixture, "triad")

	result, err := AnalyzeAndPersist(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "triad", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Key.Known || result.Key.Tonic != 0 || result.Key.Mode != key.ModeMajor {
		t.Fatalf("key = %#v, want C major", result.Key)
	}
	if result.Tempo.Known {
		t.Fatalf("a sustained triad has no beats; tempo = %#v, want unknown", result.Tempo)
	}
	record, err := database.GetTrackAnalysis("triad")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != db.TrackAnalysisPartial {
		t.Fatalf("status = %q, want %q", record.Status, db.TrackAnalysisPartial)
	}
	if record.ErrorCode == nil || *record.ErrorCode != ErrorNoReliableTempo {
		t.Fatalf("error code = %#v, want %q", record.ErrorCode, ErrorNoReliableTempo)
	}
	if record.BPM != nil || record.BPMSource != nil {
		t.Fatalf("unmeasured tempo must stay null, got %#v", record)
	}
	if record.TempoKind == nil || *record.TempoKind != "unknown" {
		t.Fatalf("tempo kind = %#v, want unknown", record.TempoKind)
	}
}

func TestAnalyzeAndPersistRecordsFailureForSilence(t *testing.T) {
	fixture, err := analysisbench.NewSilence("silent", 4, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	database := catalogSong(t, fixture, "silent")

	result, err := AnalyzeAndPersist(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "silent", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.Tempo.Known || result.Key.Known {
		t.Fatalf("silence must not produce measurements: %#v", result)
	}
	record, err := database.GetTrackAnalysis("silent")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != db.TrackAnalysisFailed {
		t.Fatalf("status = %q, want %q", record.Status, db.TrackAnalysisFailed)
	}
	if record.ErrorCode == nil || *record.ErrorCode != ErrorInsufficientAudio {
		t.Fatalf("error code = %#v, want %q", record.ErrorCode, ErrorInsufficientAudio)
	}
}

// A second pass over an unchanged source must not degrade the record. This is
// the regression that per-dimension persistence could not satisfy, because each
// analyzer overwrote the other's status and algorithm version.
func TestSecondPassDoesNotDegradeCombinedRecord(t *testing.T) {
	fixture := tonalClickTrack(t, "song", 128, 9, true, 12, 22050)
	database := catalogSong(t, fixture, "song")
	registry := analysis.NewDefaultDecoderRegistry()

	first, err := AnalyzeAndPersist(context.Background(), database, registry, "song", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	second, err := AnalyzeAndPersist(context.Background(), database, registry, "song", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != second.Status {
		t.Fatalf("status changed between identical passes: %q then %q", first.Status, second.Status)
	}
	record, err := database.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if record.BPM == nil || record.KeyTonic == nil {
		t.Fatalf("second pass lost a dimension: %#v", record)
	}
	if record.AlgorithmVersion != AlgorithmVersion {
		t.Fatalf("algorithm version = %q, want %q", record.AlgorithmVersion, AlgorithmVersion)
	}
}

func TestAnalyzeRejectsUnsupportedCodecWithStableCode(t *testing.T) {
	directory := t.TempDir()
	database, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	path := filepath.Join(directory, "track.opus")
	if err := os.WriteFile(path, []byte("not audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "opus", Title: "Opus", Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}

	_, analyzeErr := Analyze(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "opus", DefaultOptions())
	if analyzeErr == nil {
		t.Fatal("expected an unsupported-codec error for .opus")
	}
	code, message := ClassifyError(analyzeErr)
	if code != ErrorUnsupportedCodec {
		t.Fatalf("code = %q, want %q", code, ErrorUnsupportedCodec)
	}
	if err := PersistFailure(database, "opus", analysis.ResolvedSource{}, code, message); err != nil {
		t.Fatal(err)
	}
	record, err := database.GetTrackAnalysis("opus")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != db.TrackAnalysisUnsupported {
		t.Fatalf("status = %q, want %q", record.Status, db.TrackAnalysisUnsupported)
	}
}

func TestClassifyErrorReportsMissingSourceAndCancellation(t *testing.T) {
	directory := t.TempDir()
	database, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSong(&db.Song{ID: "gone", Title: "Gone", Artist: "Artist", Album: "Album", FilePath: filepath.Join(directory, "missing.wav"), AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	_, analyzeErr := Analyze(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "gone", DefaultOptions())
	if analyzeErr == nil {
		t.Fatal("expected an error for a missing source file")
	}
	if code, _ := ClassifyError(analyzeErr); code != ErrorSourceUnavailable {
		t.Fatalf("code = %q, want %q", code, ErrorSourceUnavailable)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if code, _ := ClassifyError(canceled.Err()); code != ErrorCanceled {
		t.Fatalf("code = %q, want %q", code, ErrorCanceled)
	}
}
