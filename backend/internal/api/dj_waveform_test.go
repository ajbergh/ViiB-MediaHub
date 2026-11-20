// Tests and fixtures for dj waveform behavior.

package api

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
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

func TestWaveformSingleflightPartialAndFullCancellation(t *testing.T) {
	audio := encodePCM16WAV(make([]float32, 22050), 22050)
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()

	var opens atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opens.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
			_, _ = w.Write(audio)
		case <-r.Context().Done():
		}
	}))
	defer func() { unblock(); upstream.Close() }()

	database, api, track := setupPlexProxyTest(t, upstream.URL, "/audio", "secret", true)
	track.Container = "wav"
	if _, _, _, err := database.SyncPlexLibrary(track.SourceID, track.LibraryID, []db.PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/dj/waveform/{id}", api.getDJWaveform)

	ctx1, cancel1 := context.WithCancel(t.Context())
	done1 := make(chan *httptest.ResponseRecorder, 1)
	done2 := make(chan *httptest.ResponseRecorder, 1)

	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/dj/waveform/"+track.SongID, nil).WithContext(ctx1)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		done1 <- w
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not enter upstream")
	}

	go func() {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dj/waveform/"+track.SongID, nil))
		done2 <- w
	}()
	// Wait until the second request has joined the flight before canceling the first.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		api.djWaveformMu.Lock()
		var waiters int
		for _, f := range api.djWaveformFlights {
			waiters += f.waiters
		}
		api.djWaveformMu.Unlock()
		if waiters == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel1()
	<-done1
	unblock()

	select {
	case w := <-done2:
		if w.Code != http.StatusOK {
			t.Fatalf("surviving waiter status=%d body=%s", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("surviving waiter stuck")
	}
	if opens.Load() != 1 {
		t.Fatalf("opens=%d, want 1", opens.Load())
	}
}

func TestWaveformSingleflightAllWaitersCancelReleasesLease(t *testing.T) {
	audio := encodePCM16WAV(make([]float32, 22050), 22050)
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "tone.wav")
	if err := os.WriteFile(wavPath, audio, 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := db.New(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	const songID = "local-cancel-song"
	if err := database.SaveSong(&db.Song{
		ID:       songID,
		Title:    "Tone",
		Artist:   "Artist",
		Album:    "Album",
		Duration: 1,
		FilePath: wavPath,
		AddedAt:  1,
		FileHash: "tone-hash",
	}); err != nil {
		t.Fatal(err)
	}

	api := &API{db: database}
	source, err := api.resolveAnalysisSource(t.Context(), songID)
	if err != nil {
		t.Fatal(err)
	}
	leaseToken, claimed, err := database.ClaimWaveformLease(songID, source.Fingerprint)
	if err != nil || !claimed {
		t.Fatalf("ClaimWaveformLease failed: claimed=%v err=%v", claimed, err)
	}

	router := chi.NewRouter()
	router.Get("/api/dj/waveform/{id}", api.getDJWaveform)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodGet, "/api/dj/waveform/"+songID, nil).WithContext(ctx)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		api.djWaveformMu.Lock()
		n := len(api.djWaveformFlights)
		api.djWaveformMu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		api.djWaveformMu.Lock()
		n := len(api.djWaveformFlights)
		api.djWaveformMu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := database.ReleaseWaveformLease(songID, leaseToken); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dj/waveform/"+songID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("retry after full cancel failed: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSchedulePruneLibraryChangesThresholdAndSingleFlight(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		t.Fatal(err)
	}
	tx, err := database.SaveSongsWithResult([]db.Song{{
		ID: "s1", Title: "One", Artist: "A", Album: "B", Duration: 10,
		FilePath: filepath.Join(t.TempDir(), "1.wav"), AddedAt: 1, FileHash: "h1",
	}})
	if err != nil || tx.Inserted == 0 {
		t.Fatalf("seed song failed: %v", err)
	}
	for i := 0; i < 25; i++ {
		if err := database.RecordSongLibraryChange("s1"); err != nil {
			t.Fatal(err)
		}
	}

	api := &API{db: database}
	// Below retain+threshold: should not prune.
	api.schedulePruneLibraryChangesWithRetain(20, 10, 26)
	time.Sleep(30 * time.Millisecond)
	oldest, ok, err := database.OldestLibraryChangeRevision()
	if err != nil || !ok || oldest != 1 {
		t.Fatalf("unexpected early prune: oldest=%d ok=%v err=%v", oldest, ok, err)
	}

	// Above retain+threshold (10 + 10 = 20): triggers async prune.
	api.schedulePruneLibraryChangesWithRetain(10, 10, 26)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		oldest, ok, err = database.OldestLibraryChangeRevision()
		if err == nil && ok && oldest == 16 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("async prune did not advance oldest revision: oldest=%d ok=%v err=%v", oldest, ok, err)
}

func TestLibraryRevisionAndJobSSEStreamsWakeOnNotify(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureJobSchema(); err != nil {
		t.Fatal(err)
	}

	api := &API{db: database}
	router := chi.NewRouter()
	router.Get("/v2/library/events", api.libraryRevisionEventsV2)
	router.Get("/v2/jobs/events", api.jobEventsV2)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// Library revision SSE stream wakes immediately on RecordSongLibraryChange.
	libCtx, cancelLib := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancelLib()
	libReq, _ := http.NewRequestWithContext(libCtx, http.MethodGet, srv.URL+"/v2/library/events", nil)
	libResp, err := srv.Client().Do(libReq)
	if err != nil {
		t.Fatal(err)
	}
	defer libResp.Body.Close()

	libLines := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(libResp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				libLines <- line
			}
		}
	}()

	if err := database.RecordSongLibraryChange("song-1"); err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-libLines:
		if !strings.Contains(line, `"revision":1`) {
			t.Fatalf("unexpected library SSE payload: %s", line)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("library SSE stream did not wake promptly on change")
	}

	// Job SSE stream wakes immediately on job mutations.
	jobCtx, cancelJob := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancelJob()
	jobReq, _ := http.NewRequestWithContext(jobCtx, http.MethodGet, srv.URL+"/v2/jobs/events", nil)
	jobResp, err := srv.Client().Do(jobReq)
	if err != nil {
		t.Fatal(err)
	}
	defer jobResp.Body.Close()

	jobLines := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(jobResp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				jobLines <- line
			}
		}
	}()

	// Consume initial empty snapshot.
	select {
	case <-jobLines:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("missing initial job SSE snapshot")
	}

	if err := database.CreateJob(db.Job{ID: "job-sse-1", Type: "refresh_genre_stats", Status: db.JobStatusQueued}); err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-jobLines:
		if !strings.Contains(line, `"job-sse-1"`) {
			t.Fatalf("unexpected job SSE payload: %s", line)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("job SSE stream did not wake promptly on CreateJob")
	}
}

func BenchmarkWaveformWaiterContention(b *testing.B) {
	audio := encodePCM16WAV(make([]float32, 22050), 22050)
	dir := b.TempDir()
	wavPath := filepath.Join(dir, "bench.wav")
	if err := os.WriteFile(wavPath, audio, 0o600); err != nil {
		b.Fatal(err)
	}

	database, err := db.New(filepath.Join(dir, "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = database.Close() })

	const songID = "bench-wave-song"
	if err := database.SaveSong(&db.Song{
		ID:       songID,
		Title:    "Bench",
		Artist:   "Artist",
		Album:    "Album",
		Duration: 1,
		FilePath: wavPath,
		AddedAt:  1,
		FileHash: "bench-hash",
	}); err != nil {
		b.Fatal(err)
	}

	api := &API{db: database}
	router := chi.NewRouter()
	router.Get("/api/dj/waveform/{id}", api.getDJWaveform)

	const waiters = 16
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := database.DeleteTrackAnalysisArtifact(songID, waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		var wg sync.WaitGroup
		wg.Add(waiters)
		for w := 0; w < waiters; w++ {
			go func() {
				defer wg.Done()
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/dj/waveform/%s", songID), nil))
				if rec.Code != http.StatusOK {
					b.Errorf("status=%d body=%s", rec.Code, rec.Body.String())
				}
			}()
		}
		wg.Wait()
	}
}
