// Tests and fixtures for smart playlist behavior.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/dj"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

func TestDJSmartPlaylistUsesCurrentProviderBPMThroughPublication(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "dj-provider-bpm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	runtime := newSpotifyAuthRuntime(database, spotifyauth.WebPlayerOptions{})
	owner := runtime.metadataContext
	const songID = "dj-provider-song"
	const spotifyID = "CCCCCCCCCCCCCCCCCCCCCC"
	mediaPath := filepath.Join(t.TempDir(), "song.wav")
	if err := os.WriteFile(mediaPath, []byte("dj route current source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: songID, Title: "Provider Tempo", Artist: "Fixture Artist", Album: "Fixture Album", FilePath: mediaPath, Duration: 240, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, songID)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshTrackAnalysisSourceRevision(songID, source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	localBPM := 90.0
	local := &db.LocalScalarObservation{SourceFingerprint: source.Fingerprint, AlgorithmVersion: "dj-route-local-v1", MeasuredAt: time.Now().UnixMilli(), BPM: &localBPM}
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: songID, Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "dj-route-local-v1", SourceFingerprint: source.Fingerprint, Local: local}); err != nil {
		t.Fatal(err)
	}
	if ok, err := database.ConfirmSpotifyRecording(songID, spotifyID, source.Fingerprint, true); err != nil || !ok {
		t.Fatalf("confirm current recording: %v %v", ok, err)
	}
	providerBPM := 135.0
	now := time.Now()
	if err := database.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: spotifyID, AccountContext: owner, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now, BPM: &providerBPM}, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fixture LLM unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	for key, value := range map[string]string{"llm_provider": "ollama", "llm_model": "fixture", "llm_base_url": upstream.URL} {
		if err := database.SetSetting(key, value); err != nil {
			t.Fatal(err)
		}
	}
	api := &API{db: database, spotifyAuth: runtime}
	defer api.Close()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/smart-playlist", strings.NewReader(`{"prompt":"party dance","mode":"dj","source":"local","targetDurationMinutes":2}`))
	api.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("DJ smart playlist = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		DJ dj.DJResponse `json:"dj"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, phase := range response.DJ.Phases {
		if phase.SongCount > 0 {
			found = true
			if phase.AvgBPM != 135 || phase.MinBPM != 135 || phase.MaxBPM != 135 {
				t.Fatalf("DJ phase ignored current provider tempo: %+v", phase)
			}
		}
	}
	if !found {
		t.Fatalf("DJ response selected no songs: %+v", response.DJ)
	}

	auditStarted := make(chan struct{})
	releaseAudit := make(chan struct{})
	var llmRequests atomic.Int32
	tracingLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if llmRequests.Add(1) == 3 {
			close(auditStarted)
			<-releaseAudit
		}
		http.Error(w, "fixture LLM unavailable", http.StatusServiceUnavailable)
	}))
	defer tracingLLM.Close()
	if err := database.SetSetting("llm_base_url", tracingLLM.URL); err != nil {
		t.Fatal(err)
	}
	raceRecorder := httptest.NewRecorder()
	raceDone := make(chan struct{})
	go func() {
		api.Routes().ServeHTTP(raceRecorder, httptest.NewRequest(http.MethodPost, "/smart-playlist", strings.NewReader(`{"prompt":"party dance","mode":"dj","source":"local","targetDurationMinutes":2}`)))
		close(raceDone)
	}()
	select {
	case <-auditStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("DJ request did not reach the audit while assembling the provider-tempo queue")
	}
	retired := make(chan struct{})
	go func() { runtime.beginRetirement(); close(retired) }()
	select {
	case <-retired:
	case <-time.After(2 * time.Second):
		close(releaseAudit)
		t.Fatal("session retirement blocked on DJ audit; metadata fence was held too long")
	}
	close(releaseAudit)
	select {
	case <-raceDone:
	case <-time.After(5 * time.Second):
		t.Fatal("DJ request did not finish after audit release")
	}
	if raceRecorder.Code != http.StatusConflict {
		t.Fatalf("session replacement during DJ audit = %d, want 409: %s", raceRecorder.Code, raceRecorder.Body.String())
	}
	fallbackRecorder := httptest.NewRecorder()
	api.Routes().ServeHTTP(fallbackRecorder, httptest.NewRequest(http.MethodPost, "/smart-playlist", strings.NewReader(`{"prompt":"party dance","mode":"dj","source":"local","targetDurationMinutes":2}`)))
	if fallbackRecorder.Code != http.StatusOK {
		t.Fatalf("DJ route should retain local scoring after session retirement: %d: %s", fallbackRecorder.Code, fallbackRecorder.Body.String())
	}
	var fallback struct {
		DJ dj.DJResponse `json:"dj"`
	}
	if err := json.NewDecoder(fallbackRecorder.Body).Decode(&fallback); err != nil {
		t.Fatal(err)
	}
	for _, phase := range fallback.DJ.Phases {
		if phase.SongCount > 0 && (phase.AvgBPM != 90 || phase.MinBPM != 90 || phase.MaxBPM != 90) {
			t.Fatalf("retired provider tempo was reused after session change: %+v", phase)
		}
	}
}

func TestExtractDecadeFromPrompt(t *testing.T) {
	minYear, maxYear := extractDecadeFromPrompt("90s west coast hip-hop")
	if minYear != 1990 || maxYear != 1999 {
		t.Fatalf("extractDecadeFromPrompt() = %d-%d, want 1990-1999", minYear, maxYear)
	}
}

func TestSongMatchesYearPrefersOriginalReleaseYear(t *testing.T) {
	tests := []struct {
		name string
		song db.Song
		want bool
	}{
		{"90s original despite 2004 remaster", db.Song{Year: 2004, OriginalYear: 1994}, true},
		{"2004 release without original year", db.Song{Year: 2004}, false},
		{"unknown year cannot satisfy explicit era", db.Song{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := songMatchesYear(tt.song, 1990, 1999); got != tt.want {
				t.Fatalf("songMatchesYear() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyPlayHistoryFiltersSupportsDatabaseSongs(t *testing.T) {
	api := &API{}
	songs := []any{
		db.Song{ID: "recent", Artist: "Artist A", PlayCount: 9},
		db.Song{ID: "first", Artist: "Artist A", PlayCount: 1},
		db.Song{ID: "duplicate", Artist: "artist a", PlayCount: 2},
		db.Song{ID: "other", Artist: "Artist B", PlayCount: 3},
	}

	got := api.applyPlayHistoryFilters(songs, map[string]bool{"recent": true}, "favorites", true, 10)
	if len(got) != 2 {
		t.Fatalf("filtered song count = %d, want 2", len(got))
	}
	if got[0].(db.Song).ID != "other" || got[1].(db.Song).ID != "first" {
		t.Fatalf("unexpected filtered songs: %#v", got)
	}
}
