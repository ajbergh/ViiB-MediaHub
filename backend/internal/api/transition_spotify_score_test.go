package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

type blockingTransitionResponseWriter struct {
	*httptest.ResponseRecorder
	entered chan struct{}
	release chan struct{}
	written chan struct{}
	once    sync.Once
}

func (w *blockingTransitionResponseWriter) Write(data []byte) (int, error) {
	w.once.Do(func() {
		close(w.entered)
		<-w.release
	})
	n, err := w.ResponseRecorder.Write(data)
	select {
	case <-w.written:
	default:
		close(w.written)
	}
	return n, err
}

func TestTransitionSpotifyScoreRangeValidation(t *testing.T) {
	for _, raw := range []string{
		"spotifyScoreMetric=unknown", "minSpotifyScore=0", "spotifyScoreMetric=energy&minSpotifyScore=NaN",
		"spotifyScoreMetric=energy&maxSpotifyScore=1.01", "spotifyScoreMetric=energy&minSpotifyScore=.9&maxSpotifyScore=.1",
		"spotifyScoreMetric=energy&spotifyScoreMetric=valence", "spotifyScoreMetric=energy&minSpotifyScore=0&minSpotifyScore=1",
	} {
		q, _ := url.ParseQuery(raw)
		if _, err := parseTransitionRecommendationFilters(q); err == nil {
			t.Fatalf("accepted invalid query %s", raw)
		}
	}
	q, _ := url.ParseQuery("spotifyScoreMetric=energy&minSpotifyScore=0&maxSpotifyScore=0")
	filters, err := parseTransitionRecommendationFilters(q)
	if err != nil || filters.MinSpotifyScore == nil || *filters.MinSpotifyScore != 0 {
		t.Fatal(filters, err)
	}
	now := time.Now()
	score := db.SpotifyScoreSummary{Value: 0, ExpiresAt: now.Add(time.Minute)}
	if !transitionSpotifyScoreMatches(score, filters, now) {
		t.Fatal("valid zero excluded")
	}
	for _, change := range []func(*db.SpotifyScoreSummary){
		func(s *db.SpotifyScoreSummary) { s.Stale = true },
		func(s *db.SpotifyScoreSummary) { s.ExpiresAt = now },
		func(s *db.SpotifyScoreSummary) { s.Value = math.NaN() },
		func(s *db.SpotifyScoreSummary) { s.Value = -.1 },
		func(s *db.SpotifyScoreSummary) { s.Value = .1 },
	} {
		altered := score
		change(&altered)
		if transitionSpotifyScoreMatches(altered, filters, now) {
			t.Fatal("invalid/ineligible score accepted", altered)
		}
	}
}

func TestTransitionSpotifyScoreEndpointFiltersBeforeLimit(t *testing.T) {
	a, _, calls := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for index, id := range []string{"source", "a-high", "b-zero", "c-expired", "d-unlinked"} {
		fp := saveAnalysisTestSong(t, a.db, id, id, nil, 1, 0)
		if err := a.db.RefreshTrackAnalysisSourceRevision(id, fp); err != nil {
			t.Fatal(err)
		}
		result := features.Result{IntegratedLUFS: -10, Energy: []features.EnergyPoint{{Value: .5}}, Sections: []features.Section{}, CueSuggestions: []features.CueSuggestion{}}
		raw, err := result.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if err := a.db.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: id + ":energy", SongID: id, Kind: features.ArtifactKind, FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion, Encoding: features.Encoding, Provenance: "measured", SourceFingerprint: fp, Data: raw}); err != nil {
			t.Fatal(err)
		}
		if err := a.db.UpsertTrackAnalysis(db.TrackAnalysis{SongID: id, Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fp}); err != nil {
			t.Fatal(err)
		}
		if index == 0 || id == "d-unlinked" {
			continue
		}
		recording := []string{"", "AAAAAAAAAAAAAAAAAAAAAA", "BBBBBBBBBBBBBBBBBBBBBB", "CCCCCCCCCCCCCCCCCCCCCC"}[index]
		if ok, err := a.db.ConfirmSpotifyRecording(id, recording, fp, true); err != nil || !ok {
			t.Fatal(ok, err)
		}
		value := 0.0
		expiry := now.Add(time.Hour)
		if id == "a-high" {
			value = .9
		}
		if id == "c-expired" {
			expiry = now.Add(-time.Second)
		}
		if err := a.db.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: recording, AccountContext: runtime.metadataContext, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now.Add(-time.Hour), Energy: &value}, "fixture", expiry); err != nil {
			t.Fatal(err)
		}
	}
	get := func(query string) TransitionRecommendationsResponse {
		t.Helper()
		w := httptest.NewRecorder()
		a.V2Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations?limit=1"+query, nil))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var result TransitionRecommendationsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := calls.Load()
	if got := get(""); len(got.Recommendations) != 1 || got.Recommendations[0].SongID != "a-high" || got.CandidatesAfterFilters != 4 {
		t.Fatalf("off changed ranking %#v", got)
	}
	query := "&spotifyScoreMetric=energy&minSpotifyScore=0&maxSpotifyScore=0"
	got := get(query)
	if got.CandidatesBeforeFilters != 4 || got.CandidatesAfterFilters != 1 || len(got.Recommendations) != 1 || got.Recommendations[0].SongID != "b-zero" || got.Recommendations[0].FilterEvidence.SpotifyScore == nil || got.Recommendations[0].FilterEvidence.SpotifyScore.Value != 0 {
		t.Fatalf("zero/prelimit evidence %#v", got)
	}
	if err := a.db.DeleteSpotifyRecording("b-zero"); err != nil {
		t.Fatal(err)
	}
	if got := get(query); got.CandidatesBeforeFilters != 4 || got.CandidatesAfterFilters != 0 || len(got.Recommendations) != 0 {
		t.Fatalf("removed recording score admitted or local candidates lost %#v", got)
	}
	if got := get(""); got.CandidatesAfterFilters != 4 {
		t.Fatalf("recording removal disabled local mixing %#v", got)
	}
	sources, err := a.currentAnalysisSourceFingerprints([]string{"b-zero"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := a.db.ConfirmSpotifyRecording("b-zero", "BBBBBBBBBBBBBBBBBBBBBB", sources["b-zero"], true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if got := get(query); got.CandidatesAfterFilters != 1 {
		t.Fatalf("explicit relink did not restore candidate %#v", got)
	}
	candidateSong, err := a.db.GetSongByID("b-zero")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(candidateSong.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(candidateSong.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidateSong.FilePath, []byte("replacement audio with a different revision"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := get(query); got.CandidatesBeforeFilters != 3 || got.CandidatesAfterFilters != 0 || len(got.Recommendations) != 0 {
		t.Fatalf("changed physical source borrowed old local/provider evidence %#v", got)
	}
	if got := get(""); got.CandidatesAfterFilters != 3 {
		t.Fatalf("changed source retained local analysis %#v", got)
	}
	if err := os.WriteFile(candidateSong.FilePath, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(candidateSong.FilePath, stat.ModTime(), stat.ModTime()); err != nil {
		t.Fatal(err)
	}
	if got := get(query); got.CandidatesAfterFilters != 1 {
		t.Fatalf("restored exact source was not eligible %#v", got)
	}
	owner, err := a.db.ReserveSpotifyMetadataRuntime(runtime.metadataEpoch)
	if err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	runtime.pendingOwner = owner
	runtime.mu.Unlock()
	if got := get(query); got.CandidatesAfterFilters != 0 || len(got.Recommendations) != 0 {
		t.Fatalf("pending scores admitted %#v", got)
	}
	if err := a.db.RetireSpotifyMetadataContext(runtime.metadataContext); err != nil {
		t.Fatal(err)
	}
	if got := get(query); got.CandidatesAfterFilters != 0 {
		t.Fatalf("retired account score admitted %#v", got)
	}
	if got := get(""); got.CandidatesAfterFilters != 4 {
		t.Fatalf("account retirement disabled local mixing %#v", got)
	}
	if calls.Load() != before {
		t.Fatal("score filter made provider calls")
	}
}

func TestTransitionRecommendationSourceSnapshotRejectsMismatches(t *testing.T) {
	expected := map[string]string{"source": "source-v1", "candidate": "candidate-v1"}
	if !transitionRecommendationSourcesMatch(expected, map[string]string{"source": "source-v1", "candidate": "candidate-v1"}, []string{"source", "candidate"}) {
		t.Fatal("unchanged reference and candidate rejected")
	}
	if transitionRecommendationSourcesMatch(expected, map[string]string{"source": "source-v1", "candidate": "candidate-v2"}, []string{"source", "candidate"}) {
		t.Fatal("changed candidate source accepted")
	}
	if transitionRecommendationSourcesMatch(expected, map[string]string{"source": "source-v2", "candidate": "candidate-v1"}, []string{"source", "candidate"}) {
		t.Fatal("changed reference source accepted")
	}
	if transitionRecommendationSourcesMatch(expected, map[string]string{"source": "source-v1"}, []string{"source", "candidate"}) {
		t.Fatal("unavailable candidate source accepted")
	}
}

func TestTransitionRecommendationSourceRecheckDetectsPhysicalReplacement(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	api := &API{db: database}
	fingerprint := saveAnalysisTestSong(t, database, "candidate", "Candidate", nil, 1, 0)
	expected := map[string]string{"candidate": fingerprint}

	if current, err := api.transitionRecommendationSourcesCurrent(expected, []string{"candidate"}); err != nil || !current {
		t.Fatalf("unchanged source check = %v, %v", current, err)
	}
	song, err := database.GetSongByID("candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(song.FilePath, []byte("replacement source bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if current, err := api.transitionRecommendationSourcesCurrent(expected, []string{"candidate"}); err != nil || current {
		t.Fatalf("replaced source check = %v, %v; want mismatch", current, err)
	}
}

func TestTransitionSpotifyScoreResponseSerializesBeforeAccountRetirement(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"source", "candidate"} {
		fp := saveAnalysisTestSong(t, a.db, id, id, nil, 1, 0)
		if err := a.db.RefreshTrackAnalysisSourceRevision(id, fp); err != nil {
			t.Fatal(err)
		}
		result := features.Result{IntegratedLUFS: -10, Energy: []features.EnergyPoint{{Value: .5}}, Sections: []features.Section{}, CueSuggestions: []features.CueSuggestion{}}
		raw, err := result.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if err := a.db.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: id + ":energy", SongID: id, Kind: features.ArtifactKind, FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion, Encoding: features.Encoding, Provenance: "measured", SourceFingerprint: fp, Data: raw}); err != nil {
			t.Fatal(err)
		}
		if err := a.db.UpsertTrackAnalysis(db.TrackAnalysis{SongID: id, Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fp}); err != nil {
			t.Fatal(err)
		}
		if id == "candidate" {
			recording := "BBBBBBBBBBBBBBBBBBBBBB"
			if ok, err := a.db.ConfirmSpotifyRecording(id, recording, fp, true); err != nil || !ok {
				t.Fatal(ok, err)
			}
			value := 0.0
			if err := a.db.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: recording, AccountContext: runtime.metadataContext, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: time.Now(), Energy: &value}, "fixture", time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
		}
	}

	writer := &blockingTransitionResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		entered:          make(chan struct{}),
		release:          make(chan struct{}),
		written:          make(chan struct{}),
	}
	retirementAfterWrite := make(chan bool, 1)
	previousRetire := runtime.onRetire
	runtime.onRetire = func() error {
		select {
		case <-writer.written:
			retirementAfterWrite <- true
		default:
			retirementAfterWrite <- false
		}
		if previousRetire != nil {
			return previousRetire()
		}
		return nil
	}
	handlerDone := make(chan struct{})
	go func() {
		request := httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations?spotifyScoreMetric=energy&minSpotifyScore=0&maxSpotifyScore=0", nil)
		a.V2Routes().ServeHTTP(writer, request)
		close(handlerDone)
	}()
	select {
	case <-writer.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("recommendation response did not reach serialization")
	}

	retirementDone := make(chan error, 1)
	retirementStarted := make(chan struct{})
	go func() {
		close(retirementStarted)
		retirementDone <- runtime.disconnect()
	}()
	<-retirementStarted
	deadline := time.After(3 * time.Second)
	for runtime.changeMu.TryLock() {
		runtime.changeMu.Unlock()
		select {
		case <-deadline:
			t.Fatal("account retirement did not enter its lifecycle lock")
		default:
			goruntime.Gosched()
		}
	}
	close(writer.release)

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("recommendation response did not finish")
	}
	select {
	case err := <-retirementDone:
		if err != nil {
			t.Fatal("retire Spotify account:", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("account retirement did not finish after response serialization")
	}
	if afterWrite := <-retirementAfterWrite; !afterWrite {
		t.Fatal("account retirement reached private-data cleanup before the response was serialized")
	}
	if writer.Code != http.StatusOK {
		t.Fatalf("recommendation response = %d: %s", writer.Code, writer.Body.String())
	}
	var response TransitionRecommendationsResponse
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Recommendations) != 1 || response.Recommendations[0].SongID != "candidate" || response.Recommendations[0].FilterEvidence.SpotifyScore == nil || response.Recommendations[0].FilterEvidence.SpotifyScore.Value != 0 {
		t.Fatalf("serialized response lost verified pre-retirement evidence: %#v", response)
	}
}
