// Tests and fixtures for dj waveform behavior.

package api

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	trackanalysis "github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
)

func TestPlexDJWaveformMatchesLocalBackend(t *testing.T) {
	const token = "waveform-secret"
	samples := make([]float32, 22050)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / 22050))
	}
	audio := encodePCM16WAV(samples, 22050)
	localPath := filepath.Join(t.TempDir(), "tone.wav")
	if err := os.WriteFile(localPath, audio, 0o600); err != nil {
		t.Fatal(err)
	}
	want, err := generateWaveform(localPath)
	if err != nil {
		t.Fatal(err)
	}
	var requests int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("X-Plex-Token") != token || strings.Contains(r.URL.RawQuery, token) {
			t.Errorf("Plex token was missing from header or exposed in URL")
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(audio)
	}))
	defer upstream.Close()
	database, api, track := setupPlexProxyTest(t, upstream.URL, "/audio", token, true)
	track.Container = "wav"
	if _, _, _, err := database.SyncPlexLibrary(track.SourceID, track.LibraryID, []db.PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/dj/waveform/{id}", api.getDJWaveform)
	request := func() WaveformResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/dj/waveform/"+track.SongID, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
		}
		var result WaveformResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	got := request()
	if got.Duration != want.Duration || got.SampleRate != want.SampleRate || got.Resolution != want.Resolution || len(got.Peaks) != len(want.Peaks) {
		t.Fatalf("Plex waveform metadata differs from local: got %#v, want %#v", got, want)
	}
	for i := range want.Peaks {
		if math.Abs(got.Peaks[i]-want.Peaks[i]) > 0.0001 {
			t.Fatalf("peak %d: Plex %v, local %v", i, got.Peaks[i], want.Peaks[i])
		}
	}
	if requests != 1 {
		t.Fatalf("Plex requests = %d, want 1", requests)
	}
	request()
	if requests != 1 {
		t.Fatalf("cached waveform fetched Plex audio again: %d requests", requests)
	}
}

func TestUnsupportedPlexDJWaveformFallsBackWithoutStreaming(t *testing.T) {
	var requests int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer upstream.Close()
	database, api, track := setupPlexProxyTest(t, upstream.URL, "/audio", "secret", true)
	track.Container = "flac"
	if _, _, _, err := database.SyncPlexLibrary(track.SourceID, track.LibraryID, []db.PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}
	source, err := api.resolveAnalysisSource(t.Context(), track.SongID)
	if err != nil {
		t.Fatal(err)
	}
	claim, claimed, err := database.ClaimTrackAnalysisLease(track.SongID, source.Fingerprint, 1, "unsupported-test")
	if err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	defer database.ReleaseTrackAnalysisLease(track.SongID, claim)
	router := chi.NewRouter()
	router.Get("/api/dj/waveform/{id}", api.getDJWaveform)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/dj/waveform/"+track.SongID, nil))
	if recorder.Code != http.StatusUnprocessableEntity || requests != 0 {
		t.Fatalf("status = %d, Plex requests = %d, want 422 and 0", recorder.Code, requests)
	}
}

// encodePCM16WAV builds a minimal mono RIFF/WAVE PCM16 file. The test owns this
// rather than importing the Phase 0 benchmark harness, so production test code
// does not depend on research tooling.
func encodePCM16WAV(samples []float32, sampleRate int) []byte {
	var body bytes.Buffer
	for _, sample := range samples {
		scaled := math.Round(float64(sample) * 32767)
		scaled = math.Max(-32768, math.Min(32767, scaled))
		_ = binary.Write(&body, binary.LittleEndian, int16(scaled))
	}
	data := body.Bytes()

	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(36+len(data)))
	out.WriteString("WAVE")
	out.WriteString("fmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&out, binary.LittleEndian, uint16(1)) // mono
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate*2)) // byte rate
	_ = binary.Write(&out, binary.LittleEndian, uint16(2))            // block align
	_ = binary.Write(&out, binary.LittleEndian, uint16(16))           // bits per sample
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(data)))
	out.Write(data)
	return out.Bytes()
}

// Formats with no pure-Go backend decoder must stay deferred to the renderer.
// Grouping them in one list keeps the capability claim honest: adding a decoder
// should require moving an entry out of this test, not silently widening it.
func TestGenerateWaveformDefersFormatsWithNoBackendDecoder(t *testing.T) {
	t.Parallel()

	for _, extension := range []string{".opus", ".flac", ".m4a", ".aac", ".wma", ".unknown"} {
		t.Run(extension, func(t *testing.T) {
			t.Parallel()
			_, err := generateWaveform(filepath.Join(t.TempDir(), "track"+extension))
			if !errors.Is(err, errClientWaveformRequired) {
				t.Fatalf("generateWaveform(%q) error = %v, want client generation marker", extension, err)
			}
		})
	}
}

// Opus rides in an Ogg container but is a different codec. The shared registry
// keys them separately, so Vorbis support must never imply Opus support.
func TestGenerateWaveformReportsOpusSeparatelyFromVorbis(t *testing.T) {
	t.Parallel()

	_, opusErr := generateWaveform(filepath.Join(t.TempDir(), "track.opus"))
	if !errors.Is(opusErr, errClientWaveformRequired) || !strings.Contains(opusErr.Error(), "opus format") {
		t.Fatalf("Opus error = %v, want distinct opus client-generation marker", opusErr)
	}
	// Ogg/Vorbis now has a backend decoder, so a missing file must surface as a
	// source error rather than as an unsupported-format deferral.
	_, vorbisErr := generateWaveform(filepath.Join(t.TempDir(), "missing.ogg"))
	if errors.Is(vorbisErr, errClientWaveformRequired) {
		t.Fatalf("Ogg/Vorbis was deferred to the browser despite a backend decoder: %v", vorbisErr)
	}
	if vorbisErr == nil || !strings.Contains(vorbisErr.Error(), "open local source") {
		t.Fatalf("Ogg/Vorbis error = %v, want source open failure", vorbisErr)
	}
}

// Extension matching must be case-insensitive; a capitalized MP3 is still an
// MP3 and must not fall back to the browser.
func TestGenerateWaveformNormalizesExtensionCase(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"missing.MP3", "missing.WAV", "missing.Ogg"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := generateWaveform(filepath.Join(t.TempDir(), name))
			if errors.Is(err, errClientWaveformRequired) {
				t.Fatalf("%s was incorrectly deferred to the browser: %v", name, err)
			}
			if err == nil || !strings.Contains(err.Error(), "open local source") {
				t.Fatalf("%s error = %v, want source open failure", name, err)
			}
		})
	}
}

// The point of the bridge: WAV now produces a real server-side waveform.
// Duration comes from decoded frames, and the peak count follows the declared
// resolution rather than the decoder's internal chunking.
func TestGenerateWaveformDecodesWAVServerSide(t *testing.T) {
	t.Parallel()

	const sampleRate = 22050
	const seconds = 2
	samples := make([]float32, sampleRate*seconds)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / sampleRate))
	}

	path := filepath.Join(t.TempDir(), "tone.wav")
	if err := os.WriteFile(path, encodePCM16WAV(samples, sampleRate), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	waveform, err := generateWaveform(path)
	if err != nil {
		t.Fatalf("generateWaveform() error = %v", err)
	}
	if waveform.SampleRate != sampleRate {
		t.Fatalf("SampleRate = %d, want %d", waveform.SampleRate, sampleRate)
	}
	if waveform.Resolution != analysis.DefaultWaveformResolution {
		t.Fatalf("Resolution = %d, want %d", waveform.Resolution, analysis.DefaultWaveformResolution)
	}
	if math.Abs(waveform.Duration-seconds) > 0.01 {
		t.Fatalf("Duration = %f, want ~%d", waveform.Duration, seconds)
	}
	// Ceiling, not floor: the trailing partial window is flushed so the tail of
	// a track is never dropped from the overview.
	wantPeaks := (len(samples) + analysis.DefaultWaveformResolution - 1) / analysis.DefaultWaveformResolution
	if len(waveform.Peaks) != wantPeaks {
		t.Fatalf("len(Peaks) = %d, want %d", len(waveform.Peaks), wantPeaks)
	}
	// A full-scale sine must read near 1.0 in every window, and no peak may
	// exceed the normalized range.
	for i, peak := range waveform.Peaks {
		if peak < 0.9 || peak > 1.0 {
			t.Fatalf("Peaks[%d] = %f, want a normalized full-scale peak", i, peak)
		}
	}
}

func TestDJWaveformRejectsLegacyCacheAndRebindsChangedSource(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	id := "song"
	song, err := a.db.GetSongByID(id)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, 22050)
	for i := range samples {
		samples[i] = .5
	}
	if err = os.WriteFile(song.FilePath, encodePCM16WAV(samples, 22050), 0600); err != nil {
		t.Fatal(err)
	}
	if err = a.db.SaveDJWaveform(id, &db.DJWaveform{Duration: 999, SampleRate: 22050, Resolution: 256, Peaks: []float64{1}}); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/dj/waveform/{id}", a.getDJWaveform)
	request := func() WaveformResponse {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dj/waveform/"+id, nil))
		if w.Code != 200 {
			t.Fatalf("waveform: %d %s", w.Code, w.Body.String())
		}
		var result WaveformResponse
		if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := request()
	if first.Duration != 1 || math.Abs(first.Peaks[0]-.5) > .0001 {
		t.Fatal("unbound legacy cache served")
	}
	if err = os.WriteFile(song.FilePath, encodePCM16WAV(make([]float32, 44100), 22050), 0600); err != nil {
		t.Fatal(err)
	}
	second := request()
	if second.Duration != 2 || second.Peaks[0] != 0 {
		t.Fatal("old waveform served for changed file")
	}
	third := request()
	if third.Duration != second.Duration || len(third.Peaks) != len(second.Peaks) {
		t.Fatal("bound waveform cache changed")
	}
}

func TestConcurrentPlexWaveformsShareDecodeWithoutScalarRow(t *testing.T) {
	audio := encodePCM16WAV(make([]float32, 22050), 22050)
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	var opens atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opens.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write(audio)
	}))
	defer upstream.Close()
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	database, api, track := setupPlexProxyTest(t, upstream.URL, "/audio", "secret", true)
	track.Container = "wav"
	if _, _, _, err := database.SyncPlexLibrary(track.SourceID, track.LibraryID, []db.PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/dj/waveform/{id}", api.getDJWaveform)
	results := make(chan *httptest.ResponseRecorder, 8)
	for range 8 {
		go func() {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dj/waveform/"+track.SongID, nil))
			results <- w
		}()
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no decode started")
	}
	// Keep the owner blocked while the other requests contend on SQLite.
	time.Sleep(100 * time.Millisecond)
	if opens.Load() != 1 {
		t.Fatalf("concurrent source opens=%d", opens.Load())
	}
	unblock()
	for range 8 {
		select {
		case w := <-results:
			if w.Code != 200 {
				t.Fatalf("waveform %d: %s", w.Code, w.Body.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("waveform waiter stuck")
		}
	}
	if opens.Load() != 1 {
		t.Fatalf("total source opens=%d", opens.Load())
	}
	if _, err := database.GetTrackAnalysis(track.SongID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("lazy waveform fabricated scalar row", err)
	}
}

func TestLazyWaveformWaitsForFullPreparationArtifact(t *testing.T) {
	audio := encodePCM16WAV(make([]float32, 22050), 22050)
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var opens atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opens.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write(audio)
	}))
	defer func() { unblock(); upstream.Close() }()
	database, api, catalog := setupPlexProxyTest(t, upstream.URL, "/audio", "secret", true)
	catalog.Container = "wav"
	if _, _, _, err := database.SyncPlexLibrary(catalog.SourceID, catalog.LibraryID, []db.PlexCatalogTrack{catalog}); err != nil {
		t.Fatal(err)
	}
	prepared := make(chan error, 1)
	go func() {
		_, err := trackanalysis.Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), []string{catalog.SongID}, trackanalysis.RunOptions{ResolveSource: api.resolveAnalysisSource})
		prepared <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("preparation did not open source")
	}
	router := chi.NewRouter()
	router.Get("/api/dj/waveform/{id}", api.getDJWaveform)
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dj/waveform/"+catalog.SongID, nil))
		response <- w
	}()
	select {
	case w := <-response:
		t.Fatalf("lazy request did not wait: %d", w.Code)
	case <-time.After(100 * time.Millisecond):
	}
	if opens.Load() != 1 {
		t.Fatalf("duplicate decode: %d", opens.Load())
	}
	unblock()
	select {
	case err := <-prepared:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("preparation stuck")
	}
	select {
	case w := <-response:
		if w.Code != 200 {
			t.Fatalf("waveform %d: %s", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lazy waiter stuck")
	}
	if opens.Load() != 1 {
		t.Fatalf("lazy request decoded completed preparation again: %d", opens.Load())
	}
}
