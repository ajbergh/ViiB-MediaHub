package track

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/analysis/tempo"
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

// tonalTriadMixLevel is the amplitude of the sustained triad relative to the
// clicks it is mixed with.
//
// It is deliberately low. An additive sine triad amplitude-modulates at the
// interval difference frequencies, and once that beating approaches the click
// level it dominates the rectified energy envelope: at 0.6 this fixture
// measured 113 BPM for both a 126 and a 128 BPM click track — the same wrong
// answer regardless of the real tempo, because the clicks were no longer what
// the onset detector was following. At 0.2 the triad is still loud enough for
// chroma to identify the key, so the fixture is valid for both dimensions.
// Raising it turns every tempo assertion here into a test of the beating.
const tonalTriadMixLevel = 0.2

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
			value += triad.Samples[i] * tonalTriadMixLevel
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
	// Assert the value, not just its presence. Checking only for non-nil let a
	// fixture whose measured tempo was wrong by 13 BPM pass as a happy path.
	if math.Abs(*record.BPM-128) > 0.5 {
		t.Fatalf("measured BPM = %v, want 128 +/- 0.5", *record.BPM)
	}
	if record.KeyTonic == nil || record.KeySource == nil || *record.KeySource != "measured" {
		t.Fatalf("record key = %#v, want measured key", record)
	}
	if *record.KeyTonic != 9 || record.KeyMode == nil || *record.KeyMode != "minor" {
		t.Fatalf("measured key = tonic %d mode %v, want 9/minor", *record.KeyTonic, record.KeyMode)
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
	// A percussive-only source has no tonal centre, so key must be refused
	// rather than guessed. This used to be a branch that accepted either
	// outcome, which let a fabricated key pass as valid.
	if result.Key.Known {
		t.Fatalf("click track reported key %q; a percussive source has no tonal centre", result.Key.Key)
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

// Local analysis persists tempo in track_analysis without using songs.bpm.
func TestAnalyzeAndPersistUsesTrackAnalysisBPM(t *testing.T) {
	fixture := tonalClickTrack(t, "song", 126, 9, true, 12, 22050)
	database := catalogSong(t, fixture, "song")

	if _, err := AnalyzeAndPersist(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "song", DefaultOptions()); err != nil {
		t.Fatal(err)
	}

	record, err := database.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if record.BPM == nil || math.Abs(*record.BPM-126) > 0.5 {
		t.Fatalf("measured BPM = %v, want ~126", *record.BPM)
	}
	// Fractional precision has to survive persistence; an integer round-trip
	// would accumulate into audible beatmatch drift.
	if *record.BPM == math.Trunc(*record.BPM) && math.Abs(*record.BPM-126) > 0 {
		t.Fatalf("measured BPM %v lost fractional precision", *record.BPM)
	}
	songs, err := database.GetSongsByIDs([]string{"song"})
	if err != nil || len(songs) != 1 {
		t.Fatalf("songs = %#v, err = %v", songs, err)
	}
	if songs[0].BPM != 0 {
		t.Fatalf("surfaced song BPM = %d, want legacy AI value hidden", songs[0].BPM)
	}

	effective := db.ResolveEffectiveBPM(db.EffectiveBPMInputs{Analysis: &record})
	if effective.Source != db.EffectiveBPMMeasured || !effective.SyncAllowed {
		t.Fatalf("effective BPM = %#v, want measured and Sync-eligible", effective)
	}
}

// A steady track must persist its alternate metrical candidate, stability, and
// static classification, and derive both DJ notations from the canonical key.
func TestAnalyzeAndPersistRecordsMetricalAlternateStabilityAndNotations(t *testing.T) {
	fixture := tonalClickTrack(t, "song", 128, 9, true, 12, 22050)
	database := catalogSong(t, fixture, "song")

	opts := Options{Tempo: tempo.Options{Range: tempo.Range70to140}}
	result, err := AnalyzeAndPersist(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "song", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Tempo.Known || !result.Key.Known {
		t.Fatalf("expected both dimensions measured, got %#v", result)
	}

	record, err := database.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	// Retaining the half-tempo reading is what makes a user-facing x2 / ÷2
	// correction possible instead of a blind re-analysis.
	if record.BPMAltCandidate == nil || math.Abs(*record.BPMAltCandidate-64) > 0.5 {
		t.Fatalf("BPMAltCandidate = %v (bpm %v), want ~64", *record.BPMAltCandidate, *record.BPM)
	}
	if record.TempoStability == nil || *record.TempoStability < 0.85 {
		t.Fatalf("TempoStability = %#v, want >= 0.85 for a steady click track", record.TempoStability)
	}
	if record.TempoKind == nil || *record.TempoKind != "static" {
		t.Fatalf("TempoKind = %#v, want static", record.TempoKind)
	}
	// One canonical tonic/mode, two derived notations — never three editable truths.
	if record.CamelotKey == nil || *record.CamelotKey != key.Camelot(*record.KeyTonic, *record.KeyMode) {
		t.Fatalf("CamelotKey = %#v, inconsistent with tonic/mode", record.CamelotKey)
	}
	if record.OpenKey == nil || *record.OpenKey != key.OpenKey(*record.KeyTonic, *record.KeyMode) {
		t.Fatalf("OpenKey = %#v, inconsistent with tonic/mode", record.OpenKey)
	}
}

// A percussion-only track has a real measured tempo and no determinable key,
// so its record is `partial`. That record must still drive Sync: requiring
// `complete` would throw away a good local measurement for every drum tool.
func TestPartialRecordStillYieldsMeasuredBPMForSync(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("clicks", 126, 12, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	database := catalogSong(t, fixture, "clicks")
	result, err := AnalyzeAndPersist(context.Background(), database, analysis.NewDefaultDecoderRegistry(), "clicks", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != db.TrackAnalysisPartial {
		t.Fatalf("status = %q, want %q for a track with no key", result.Status, db.TrackAnalysisPartial)
	}

	record, err := database.GetTrackAnalysis("clicks")
	if err != nil {
		t.Fatal(err)
	}
	effective := db.ResolveEffectiveBPM(db.EffectiveBPMInputs{Analysis: &record})
	if effective.Source != db.EffectiveBPMMeasured {
		t.Fatalf("effective source = %q, want %q — a partial record's measured tempo was discarded", effective.Source, db.EffectiveBPMMeasured)
	}
	if !effective.SyncAllowed {
		t.Fatal("measured tempo from a partial record must be Sync-eligible")
	}
	if effective.Value == nil || math.Abs(*effective.Value-126) > 0.5 {
		t.Fatalf("effective BPM = %v, want ~126", effective.Value)
	}
}
