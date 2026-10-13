package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/waveform"
	"github.com/go-chi/chi/v5"
	"google.golang.org/protobuf/encoding/protowire"
)

func waveformWireFixture(status int) []byte {
	number := func(tag protowire.Number, n uint64) []byte {
		return protowire.AppendVarint(protowire.AppendTag(nil, tag, protowire.VarintType), n)
	}
	message := func(tag protowire.Number, raw []byte) []byte {
		return protowire.AppendBytes(protowire.AppendTag(nil, tag, protowire.BytesType), raw)
	}
	join := func(parts ...[]byte) []byte {
		var raw []byte
		for _, part := range parts {
			raw = append(raw, part...)
		}
		return raw
	}
	domain := join(number(1, 44100), number(2, 20), number(3, 7), number(3, 8), number(4, 9), number(4, 10), number(5, 11), number(5, 12))
	any := join(message(1, []byte(waveform.TypeURL)), message(2, domain))
	entity := join(message(1, join(number(1, uint64(status)), message(2, []byte("provider-tag")))), message(2, []byte("spotify:track:"+cachedSpotifyID)), message(3, any))
	return message(2, join(number(2, waveform.ExtensionKind), message(3, entity)))
}
func TestSpotifyWaveformRefreshCacheAndBoundedRange(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "spclient.wg.spotify.com" {
			t.Fatal("unexpected origin")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(waveformWireFixture(200))))}, nil
	})}
	router := chi.NewRouter()
	router.Post("/spotify/analysis/{trackID}/waveform/refresh", a.refreshSpotifyWaveform)
	router.Get("/spotify/analysis/{trackID}/artifact", a.getSpotifyAnalysisArtifact)
	refreshPath := "/spotify/analysis/" + cachedSpotifyID + "/waveform/refresh"
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, refreshPath, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"available"`) {
			t.Fatalf("refresh failed: %d %s", w.Code, w.Body.String())
		}
	}
	if calls != 1 {
		t.Fatal("fresh cache repeated provider dispatch")
	}
	path := "/spotify/analysis/" + cachedSpotifyID + "/artifact?endpoint=three_band_waveform&offset=1&limit=1"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"lows":[8]`) || !strings.Contains(w.Body.String(), `"representation":"spotify_three_band"`) {
		t.Fatalf("range failed: %d %s", w.Code, w.Body.String())
	}
	for _, rangeQuery := range []string{"offset=-1", "offset=999", "limit=10001"} {
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/spotify/analysis/"+cachedSpotifyID+"/artifact?endpoint=three_band_waveform&"+rangeQuery, nil))
		if w.Code != 400 && w.Code != 416 {
			t.Fatal("invalid range accepted")
		}
	}
	if calls != 1 {
		t.Fatal("artifact reads requested provider")
	}
}
func TestSpotifyWaveformFailurePreservesLastGoodAndCooldown(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	cached, err := waveform.DecodeResponse(waveformWireFixture(200), cachedSpotifyID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = a.db.PutSpotifyWaveform(cachedSpotifyID, runtime.metadataContext, cached, waveform.ContractRevision, now.Add(-2*time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(waveformWireFixture(404))))}, nil
	})}
	router := chi.NewRouter()
	router.Post("/spotify/analysis/{trackID}/waveform/refresh", a.refreshSpotifyWaveform)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/spotify/analysis/"+cachedSpotifyID+"/waveform/refresh", nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"lastGoodAvailable":true`) {
			t.Fatalf("last good lost: %d %s", w.Code, w.Body.String())
		}
	}
	if calls != 1 {
		t.Fatal("negative retry state did not suppress repeated requests")
	}
	artifact, err := a.db.GetSpotifyAudioArtifact(cachedSpotifyID, "three_band_waveform", "spotify_three_band", runtime.metadataContext)
	if err != nil || artifact == nil || artifact.ProviderETag != "provider-tag" {
		t.Fatal("waveform failure erased cached artifact")
	}
}
