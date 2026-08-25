package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/plex"
)

func setupPlexProxyTest(t *testing.T, upstreamURL, mediaKey, token string, available bool) (*db.DB, *API, db.PlexCatalogTrack) {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsurePlexSchema(); err != nil {
		t.Fatal(err)
	}
	machineID, sourceID, libraryID := "machine-proxy", plex.StableSourceID("machine-proxy"), "2"
	if err := database.SavePlexSource(db.PlexSource{ID: sourceID, MachineIdentifier: machineID, BaseURL: upstreamURL, Name: "Plex", LibraryID: libraryID, Active: true, Available: available}); err != nil {
		t.Fatal(err)
	}
	track := db.PlexCatalogTrack{
		SongID: plex.StableTrackID(machineID, "99"), SourceID: sourceID, LibraryID: libraryID, MachineID: machineID,
		RatingKey: "99", MetadataKey: "/library/metadata/99", MediaKey: mediaKey, ArtistArtworkKey: "/artist-artwork", Title: "Proxy Track", Artist: "Artist", Album: "Album", Duration: 10, AddedAt: 1,
	}
	if _, _, _, err := database.SyncPlexLibrary(sourceID, libraryID, []db.PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}
	credentials, _ := json.Marshal(plex.Credentials{ClientIdentifier: "viib-test", ServerTokens: map[string]string{machineID: token}})
	if err := database.SetSetting(db.PlexCredentialsSettingKey, string(credentials)); err != nil {
		t.Fatal(err)
	}
	return database, &API{db: database}, track
}

func TestPlexArtistArtworkProxyKeepsTokenServerSide(t *testing.T) {
	const token = "artist-art-secret"
	var sawToken bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artist-artwork" {
			http.NotFound(w, r)
			return
		}
		sawToken = r.Header.Get("X-Plex-Token") == token
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("artist-image"))
	}))
	defer upstream.Close()

	_, handler, _ := setupPlexProxyTest(t, upstream.URL, "/audio", token, true)
	recorder := httptest.NewRecorder()
	handler.PlexRoutes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/artist-artwork/Artist", nil))

	if recorder.Code != http.StatusOK || !sawToken || recorder.Header().Get("Content-Type") != "image/jpeg" || recorder.Body.String() != "artist-image" {
		t.Fatalf("artist proxy failed: code=%d token=%v headers=%#v body=%q", recorder.Code, sawToken, recorder.Header(), recorder.Body.String())
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte(token)) {
		t.Fatal("Plex token leaked into artist artwork response")
	}
}

func TestPlexAudioProxyForwardsRangeAndKeepsTokenServerSide(t *testing.T) {
	const token = "super-secret-plex-token"
	var sawRange, sawToken, sawQueryLeak bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRange = r.Header.Get("Range") == "bytes=2-5"
		sawToken = r.Header.Get("X-Plex-Token") == token
		sawQueryLeak = r.URL.Query().Get("X-Plex-Token") != "" || bytes.Contains([]byte(r.URL.RawQuery), []byte(token))
		if !sawRange {
			http.Error(w, "missing range", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "audio/flac")
		w.Header().Set("Content-Range", "bytes 2-5/10")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("2345"))
	}))
	defer upstream.Close()

	database, handler, track := setupPlexProxyTest(t, upstream.URL, "/audio", token, true)
	_ = database
	req := httptest.NewRequest(http.MethodGet, "/api/audio/"+track.SongID, nil)
	req.Header.Set("Range", "bytes=2-5")
	recorder := httptest.NewRecorder()
	handler.ServeAudioSourceAware(recorder, req)
	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("status=%d body=%q", response.StatusCode, recorder.Body.String())
	}
	if !sawRange || !sawToken || sawQueryLeak {
		t.Fatalf("range/token handling bad: range=%v token=%v queryLeak=%v", sawRange, sawToken, sawQueryLeak)
	}
	if response.Header.Get("Content-Range") != "bytes 2-5/10" || response.Header.Get("Accept-Ranges") != "bytes" || response.Header.Get("Content-Type") != "audio/flac" || response.Header.Get("Content-Length") != "4" {
		t.Fatalf("proxy did not preserve media headers: %#v", response.Header)
	}
	if recorder.Body.String() != "2345" {
		t.Fatalf("proxy body=%q", recorder.Body.String())
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte(token)) {
		t.Fatal("Plex token leaked into browser-visible response")
	}
}

func TestPlexAudioProxyCreatesDirectPlayDecisionAfterPMS503(t *testing.T) {
	const token = "decision-token"
	var mediaRequests, decisionRequests int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio":
			mediaRequests++
			if r.Header.Get("X-Plex-Token") != token {
				t.Fatalf("media request lost Plex token")
			}
			if r.Header.Get("X-Plex-Platform") != "Generic" || r.Header.Get("X-Plex-Device") != "ViiB MediaHub" || r.Header.Get("X-Plex-Session-Identifier") != "viib-test" {
				t.Fatalf("media request lost PMS playback identity: %#v", r.Header)
			}
			if r.Header.Get("Range") != "bytes=2-5" {
				t.Fatalf("media request lost Range: %q", r.Header.Get("Range"))
			}
			if mediaRequests == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("Content-Range", "bytes 2-5/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("2345"))
		case "/music/:/transcode/universal/decision":
			decisionRequests++
			if r.Header.Get("X-Plex-Token") != token {
				t.Fatalf("decision request lost Plex token")
			}
			if r.Header.Get("X-Plex-Platform") != "Generic" || r.Header.Get("X-Plex-Device") != "ViiB MediaHub" || r.Header.Get("X-Plex-Session-Identifier") != "viib-test" {
				t.Fatalf("missing PMS decision client identity: %#v", r.Header)
			}
			query := r.URL.Query()
			if query.Get("path") != "/library/metadata/99" || query.Get("protocol") != "http" || query.Get("hasMDE") != "1" || query.Get("mediaIndex") != "0" || query.Get("partIndex") != "0" || query.Get("directPlay") != "1" || query.Get("session") != "viib-test" {
				t.Fatalf("unexpected direct-play decision query: %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"MediaContainer":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	_, handler, track := setupPlexProxyTest(t, upstream.URL, "/audio", token, true)
	req := httptest.NewRequest(http.MethodGet, "/api/audio/"+track.SongID, nil)
	req.Header.Set("Range", "bytes=2-5")
	recorder := httptest.NewRecorder()
	handler.ServeAudioSourceAware(recorder, req)

	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "2345" {
		t.Fatalf("expected retried direct playback, got status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if mediaRequests != 2 || decisionRequests != 1 {
		t.Fatalf("unexpected request counts: media=%d decision=%d", mediaRequests, decisionRequests)
	}
}

func TestPlexAudioProxyPreservesRangeNotSatisfiable(t *testing.T) {
	const token = "range-token"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=999-" {
			t.Fatalf("upstream Range=%q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes */10")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
	}))
	defer upstream.Close()

	_, handler, track := setupPlexProxyTest(t, upstream.URL, "/audio", token, true)
	req := httptest.NewRequest(http.MethodGet, "/api/audio/"+track.SongID, nil)
	req.Header.Set("Range", "bytes=999-")
	recorder := httptest.NewRecorder()
	handler.ServeAudioSourceAware(recorder, req)

	if recorder.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416 passthrough, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Range"); got != "bytes */10" {
		t.Fatalf("expected unsatisfied Content-Range, got %q", got)
	}
}

func TestPlexAudioProxyMarksSourceUnavailableOnInterruptedUpstream(t *testing.T) {
	const token = "stream-token"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/flac")
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("short"))
	}))
	defer upstream.Close()

	database, handler, track := setupPlexProxyTest(t, upstream.URL, "/audio", token, true)
	recorder := httptest.NewRecorder()
	handler.ServeAudioSourceAware(recorder, httptest.NewRequest(http.MethodGet, "/api/audio/"+track.SongID, nil))

	source, err := database.GetActivePlexSource()
	if err != nil {
		t.Fatal(err)
	}
	if source == nil || source.Available {
		t.Fatalf("expected interrupted upstream to mark source unavailable: %#v", source)
	}
	if source.LastSyncError != "Plex server connection interrupted during playback" {
		t.Fatalf("unexpected interrupted-stream status: %#v", source)
	}
}

func TestClassifyPlexErrorPreservesSentinelBeforeRedaction(t *testing.T) {
	status, code, _, retryable := classifyPlexError(plex.ErrDNSFailure)
	if status != http.StatusBadGateway || code != "plex_dns_failed" || !retryable {
		t.Fatalf("unexpected DNS classification: status=%d code=%s retryable=%v", status, code, retryable)
	}
	status, code, _, retryable = classifyPlexError(plex.ErrInvalidToken)
	if status != http.StatusUnauthorized || code != "plex_auth_required" || retryable {
		t.Fatalf("unexpected auth classification: status=%d code=%s retryable=%v", status, code, retryable)
	}
}
