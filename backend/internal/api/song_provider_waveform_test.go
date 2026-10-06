package api

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/waveform"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSongProviderWaveformRequiresCurrentLink(t *testing.T) {
	a, _, calls := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "song.wav")
	if err := os.WriteFile(path, []byte("fixture audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.db.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := waveform.DecodeResponse(waveformWireFixture(200), cachedSpotifyID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := a.db.PutSpotifyWaveform(cachedSpotifyID, runtime.metadataContext, decoded, waveform.ContractRevision, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/analysis/{songID}/waveform/spotify-three-band", a.getSongProviderWaveform)
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/analysis/song/waveform/spotify-three-band", nil))
		return w
	}
	if call().Code != 404 {
		t.Fatal("unlinked provider artifact exposed")
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil || !ok {
		t.Fatal(err)
	}
	w := call()
	var response struct {
		Alignment      string  `json:"alignment"`
		Representation string  `json:"representation"`
		Lows           []int32 `json:"lows"`
	}
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || w.Code != 200 || response.Alignment != "local_duration_unavailable" || response.Representation != "spotify_three_band" || len(response.Lows) != len(decoded.Lows) {
		t.Fatalf("song waveform %d %s", w.Code, w.Body.String())
	}
	detailed, err := spotifyanalysis.ValidateDomainPayload(cachedSpotifyID, "audio_analysis", []byte(`{"track":{},"beats":[{"start":0,"duration":1,"confidence":0}]}`))
	if err != nil {
		t.Fatal(err)
	}
	detailed.AccountContext = runtime.metadataContext
	detailed.RetrievedAt = now
	if err = a.db.PutExternalAnalysis(detailed, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	router.Get("/analysis/{songID}/provider-analysis/{kind}", a.getSongProviderAnalysis)
	detailedCall := func(pending bool) {
		result := httptest.NewRecorder()
		router.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/analysis/song/provider-analysis/beats?limit=25", nil))
		var payload struct {
			Provenance string `json:"provenance"`
			Unverified bool   `json:"unverified"`
		}
		if result.Code != 200 || json.Unmarshal(result.Body.Bytes(), &payload) != nil || payload.Provenance != "spotify_private_cache" || payload.Unverified != pending {
			t.Fatalf("private detailed %d %s", result.Code, result.Body.String())
		}
	}
	detailedCall(false)
	if err := a.db.BindSpotifyMetadataOwner(runtime.metadataEpoch, "webplayer", "account", runtime.metadataContext); err != nil {
		t.Fatal(err)
	}
	owner, err := a.db.ReserveSpotifyMetadataRuntime(runtime.metadataEpoch)
	if err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	runtime.pendingOwner = owner
	runtime.mu.Unlock()
	before := calls.Load()
	detailedCall(true)
	w = call()
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"unverified":true`) || !strings.Contains(w.Body.String(), `"readOnly":true`) || calls.Load() != before {
		t.Fatalf("offline waveform %d %s", w.Code, w.Body.String())
	}
	runtime.mu.Lock()
	runtime.pendingOwner.Provider = "oauth"
	runtime.mu.Unlock()
	if call().Code != 503 || calls.Load() != before {
		t.Fatal("provider mismatch admitted")
	}
	runtime.mu.Lock()
	runtime.pendingOwner.Provider = "webplayer"
	runtime.mu.Unlock()
	for _, test := range []struct {
		frames    int64
		alignment string
	}{{48000, "duration_compatible"}, {4800000, "duration_mismatch"}} {
		raw, err := waveformartifact.Encode(waveformartifact.Overview{SampleRate: 48000, Frames: test.frames, Resolution: int(test.frames), Peaks: []float64{.5}})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.db.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: "amplitude", SongID: "song", Kind: waveformartifact.Kind, FormatVersion: waveformartifact.FormatVersion, AlgorithmVersion: waveformartifact.AlgorithmVersion, Encoding: waveformartifact.Encoding, Provenance: "measured", SourceFingerprint: source.Fingerprint, Data: raw}); err != nil {
			t.Fatal(err)
		}
		w = call()
		if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Alignment != test.alignment {
			t.Fatalf("alignment %s", w.Body.String())
		}
	}
	if err := os.WriteFile(path, []byte("replaced audio revision"), 0600); err != nil {
		t.Fatal(err)
	}
	if call().Code != 404 {
		t.Fatal("changed source borrowed provider waveform")
	}
}

func TestSongProviderWaveformDurableDownloadSurvivesRetirement(t *testing.T) {
	a, _, calls := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "download.wav")
	if err := os.WriteFile(path, []byte("fixture downloaded audio"), 0600); err != nil {
		t.Fatal(err)
	}
	decoded, err := waveform.DecodeResponse(waveformWireFixture(200), cachedSpotifyID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = a.db.PutSpotifyWaveform(cachedSpotifyID, runtime.metadataContext, decoded, waveform.ContractRevision, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	detailed, err := spotifyanalysis.ValidateDomainPayload(cachedSpotifyID, "audio_analysis", []byte(`{"track":{},"beats":[{"start":0,"duration":1,"confidence":0}]}`))
	if err != nil {
		t.Fatal(err)
	}
	detailed.AccountContext = runtime.metadataContext
	detailed.RetrievedAt = now
	if err = a.db.PutExternalAnalysis(detailed, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = a.db.AddDownload(&db.SpotifyDownload{ID: "import", SpotifyID: cachedSpotifyID, Type: "track", Title: "Fixture", Status: "queued", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if changed, err := a.db.MarkDownloadStarted("import"); err != nil || !changed {
		t.Fatal("start", err)
	}
	if changed, err := a.db.MarkDownloadCompletedWithEvidence(t.Context(), "import", path); err != nil || !changed {
		t.Fatal("complete", err)
	}
	if _, err = a.db.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	if err = a.db.RetireSpotifyMetadataContext(runtime.metadataContext); err != nil {
		t.Fatal(err)
	}
	if _, err = a.db.SaveSongsWithResult([]db.Song{{ID: "imported", Title: "Fixture", Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}}); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/analysis/{songID}/waveform/spotify-three-band", a.getSongProviderWaveform)
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/analysis/imported/waveform/spotify-three-band", nil))
		return w
	}
	router.Get("/analysis/{songID}/imported-analysis/{kind}", a.getSongImportedAnalysis)
	detailedResponse := httptest.NewRecorder()
	router.ServeHTTP(detailedResponse, httptest.NewRequest(http.MethodGet, "/analysis/imported/imported-analysis/beats", nil))
	if detailedResponse.Code != 200 || !strings.Contains(detailedResponse.Body.String(), `"confidence":0`) {
		t.Fatalf("durable beats %d %s", detailedResponse.Code, detailedResponse.Body.String())
	}
	for _, test := range []struct {
		query  string
		status int
	}{{"?offset=1&limit=1", 200}, {"?offset=2", 416}, {"?limit=1001", 400}, {"?offset=-1", 400}, {"?limit=oops", 400}} {
		ranged := httptest.NewRecorder()
		router.ServeHTTP(ranged, httptest.NewRequest(http.MethodGet, "/analysis/imported/imported-analysis/beats"+test.query, nil))
		if ranged.Code != test.status {
			t.Fatalf("range %s: %d", test.query, ranged.Code)
		}
		if test.status == 200 && !strings.Contains(ranged.Body.String(), `"items":[]`) {
			t.Fatalf("expected empty terminal page %s", ranged.Body.String())
		}
	}
	before := calls.Load()
	w := call()
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"provenance":"spotify_durable_import"`) || !strings.Contains(w.Body.String(), `"unverified":false`) || calls.Load() != before {
		t.Fatalf("durable read %d %s", w.Code, w.Body.String())
	}
	if err = os.WriteFile(path, []byte("replaced download"), 0600); err != nil {
		t.Fatal(err)
	}
	if w = call(); w.Code != 404 {
		t.Fatalf("changed source %d %s", w.Code, w.Body.String())
	}
}
