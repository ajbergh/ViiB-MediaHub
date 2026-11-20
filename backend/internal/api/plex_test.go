// Tests and fixtures for plex behavior.

package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/plex"
)

func setupPlexProxyTest(t *testing.T, upstreamURL, mediaKey, token string, available bool) (*db.DB, *API, db.PlexCatalogTrack) {
	t.Helper()
	return setupPlexProxyTestWithDB(t, filepath.Join(t.TempDir(), "library.db"), upstreamURL, mediaKey, token, available)
}

func setupPlexProxyTestWithDB(t *testing.T, dbPath, upstreamURL, mediaKey, token string, available bool) (*db.DB, *API, db.PlexCatalogTrack) {
	t.Helper()
	database, err := db.New(dbPath)
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

func TestPlexAudioProxyKeepsSourceAvailableOnInterruptedTransfer(t *testing.T) {
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
	if source == nil || !source.Available || source.LastSyncError != "" {
		t.Fatalf("unexpected interrupted-stream status: %#v", source)
	}
}

func TestPlexAudioProxyDoesNotMarkCanceledRequestUnavailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("canceled request unexpectedly reached Plex")
	}))
	defer upstream.Close()
	database, handler, track := setupPlexProxyTest(t, upstream.URL, "/audio", "token", true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handler.ServeAudioSourceAware(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/audio/"+track.SongID, nil).WithContext(ctx))
	source, err := database.GetActivePlexSource()
	if err != nil || source == nil || !source.Available || source.LastSyncError != "" {
		t.Fatalf("canceled playback changed Plex source: %+v, err=%v", source, err)
	}
}

func TestPlexCatalogWriteFailureLeavesSourceAvailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/media/providers":
			_, _ = w.Write([]byte(`{"MediaContainer":{"MediaProvider":[{"identifier":"com.plexapp.plugins.library","Feature":[{"type":"content","key":"/library/sections","Directory":[{"id":2,"key":"/library/sections/2","title":"Music","type":"artist"}]}]}]}}`))
		case "/library/sections/2":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Type":[{"key":"all?type=10","type":"track","title":"Tracks"}]}}`))
		case "/library/sections/2/all":
			_, _ = w.Write([]byte(`{"MediaContainer":{"offset":0,"size":1,"totalSize":1,"Metadata":[{"ratingKey":"100","type":"track","title":"New Track","Media":[{"Part":[{"key":"/audio"}]}]}]}}`))
		case "/audio":
			w.Header().Set("Content-Type", "audio/flac")
			_, _ = w.Write([]byte("audio"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	dbPath := filepath.Join(t.TempDir(), "library.db")
	database, handler, track := setupPlexProxyTestWithDB(t, dbPath, upstream.URL, "/audio", "token", false)
	writer, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.Exec(`CREATE TRIGGER fail_plex_sync BEFORE INSERT ON songs BEGIN SELECT RAISE(ABORT, 'test local write failure'); END`); err != nil {
		t.Fatal(err)
	}
	run, err := handler.beginPlexSync(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	handler.runPlexSync(run)
	const message = "Plex responded, but ViiB could not save the music library. Try synchronizing again."
	source, err := database.GetActivePlexSource()
	if err != nil || source == nil || !source.Available || source.LastSyncStatus != "error" || source.LastSyncError != message {
		t.Fatalf("unexpected local sync failure state: %+v, err=%v", source, err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeAudioSourceAware(recorder, httptest.NewRequest(http.MethodGet, "/api/audio/"+track.SongID, nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "audio" {
		t.Fatalf("cached playback failed: status=%d body=%q", recorder.Code, recorder.Body.String())
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

func TestPlexSyncCanRecoverAfterRestartAndRejectsSupersededCommit(t *testing.T) {
	database, handler, track := setupPlexProxyTest(t, "http://127.0.0.1:32400", "/audio", "token", true)
	ctx := context.Background()
	const lastSuccess = 12345
	if err := database.SetPlexSyncState(track.SourceID, "complete", "", true, lastSuccess); err != nil {
		t.Fatal(err)
	}
	source, err := database.GetActivePlexSource()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.StartPlexSync(ctx, *source, "previous-process"); err != nil {
		t.Fatal(err)
	}
	restarted, err := handler.plexSyncSource()
	if err != nil || restarted.LastSyncStatus != "interrupted" || restarted.LastSyncAt != lastSuccess {
		t.Fatalf("state after restart: %+v, err=%v", restarted, err)
	}
	run, err := handler.beginPlexSync(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer handler.finishPlexSync(run)
	if _, err := handler.beginPlexSync(ctx, false); !errors.Is(err, errPlexSyncRunning) {
		t.Fatalf("duplicate sync: %v", err)
	}
	if _, _, _, err := database.CompletePlexSync(ctx, track.SourceID, track.LibraryID, "previous-process", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("old snapshot was accepted: %v", err)
	}
	if _, _, _, err := handler.completePlexSync(run, []db.PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}
	source, err = database.GetActivePlexSource()
	if err != nil || source.LastSyncStatus != "complete" || source.LastSyncAt <= lastSuccess {
		t.Fatalf("final state: %+v, err=%v", source, err)
	}
	if scheduled, err := handler.beginPlexSync(ctx, true); err != nil || scheduled != nil {
		t.Fatalf("automatic sync repeated a fresh import: %+v, err=%v", scheduled, err)
	}
	run, err = handler.beginPlexSync(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer handler.finishPlexSync(run)
	recorder := httptest.NewRecorder()
	handler.cancelPlexSync(recorder, httptest.NewRequest(http.MethodPost, "/sync/cancel", nil))
	canceled, err := database.GetActivePlexSource()
	if recorder.Code != http.StatusOK || !errors.Is(run.ctx.Err(), context.Canceled) || err != nil ||
		canceled.LastSyncStatus != "interrupted" || canceled.LastSyncAt != source.LastSyncAt {
		t.Fatalf("cancel result: status=%d source=%+v, err=%v", recorder.Code, canceled, err)
	}
}

func TestPlexAutoSyncDefaultsOnAndHonorsSavedPreference(t *testing.T) {
	_, handler, _ := setupPlexProxyTest(t, "http://127.0.0.1:32400", "/audio", "token", true)
	routes := handler.PlexRoutes()
	checkConfig := func(want bool) {
		t.Helper()
		response := httptest.NewRecorder()
		routes.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
		var config struct {
			AutoSync *bool `json:"autoSync"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil || response.Code != http.StatusOK ||
			config.AutoSync == nil || *config.AutoSync != want || handler.plexAutoSyncEnabled() != want {
			t.Fatalf("automatic sync for config and scheduler should be %t, got HTTP %d, body=%s, err=%v", want, response.Code, response.Body.String(), err)
		}
	}
	checkConfig(true)
	response := httptest.NewRecorder()
	routes.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/sync/schedule", bytes.NewBufferString(`{"enabled":false}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("saving opt-out failed: HTTP %d, body=%s", response.Code, response.Body.String())
	}
	checkConfig(false)
}

func TestPlexAudioRecoversRejectedServerTokenAndPreservesRange(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "fresh-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Range") != "bytes=2-4" {
			t.Error("retry lost the requested range")
		}
		w.Header().Set("Content-Range", "bytes 2-4/6")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("234"))
	}))
	defer upstream.Close()
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "account-token" {
			t.Error("resource request did not authenticate with the account token")
		}
		_, _ = w.Write([]byte(`[{"provides":"server","clientIdentifier":"machine-proxy","accessToken":"fresh-token"}]`))
	}))
	defer cloud.Close()
	database, handler, track := setupPlexProxyTest(t, upstream.URL, "/audio", "stale-token", true)
	handler.plexAuthFactory = func() *plex.AuthClient { return plex.NewAuthClientWithHTTP(cloud.URL, cloud.Client()) }
	if err := handler.savePlexCredentials(plex.Credentials{
		ClientIdentifier: "viib-test", AccountToken: "account-token", AccountTokenExpiry: time.Now().Add(48 * time.Hour).Unix(),
		ServerTokens: map[string]string{track.MachineID: "stale-token"},
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/audio/"+track.SongID, nil)
	req.Header.Set("Range", "bytes=2-4")
	recorder := httptest.NewRecorder()
	handler.ServeAudioSourceAware(recorder, req)
	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "234" {
		t.Fatalf("recovered request: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	credentials, err := handler.loadPlexCredentials()
	source, sourceErr := database.GetActivePlexSource()
	if err != nil || credentials.ServerTokens[track.MachineID] != "fresh-token" || sourceErr != nil || !source.Available {
		t.Fatalf("failed to persist recovery: credential error=%v, source=%+v, sourceErr=%v", err, source, sourceErr)
	}
}

func TestPlexAudioRecoversMovedMediaPart(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/library/metadata/99":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"99","type":"track","Media":[{"Part":[{"key":"/new-part"}]}]}]}}`))
		case "/new-part":
			_, _ = w.Write([]byte("audio"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	_, handler, track := setupPlexProxyTest(t, upstream.URL, "/old-part", "token", true)
	recorder := httptest.NewRecorder()
	handler.ServeAudioSourceAware(recorder, httptest.NewRequest(http.MethodGet, "/api/audio/"+track.SongID, nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "audio" {
		t.Fatalf("relocated part: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestPlexRouteValidationTriesAlternateWithoutSendingTokenToWrongServer(t *testing.T) {
	wrong := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity" || r.Header.Get("X-Plex-Token") != "" {
			t.Error("attempted to authenticate against a different Plex machine")
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"machineIdentifier":"other-machine"}}`))
	}))
	defer wrong.Close()
	right := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && r.Header.Get("X-Plex-Token") != "token" {
			t.Error("verified machine did not receive its token")
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"machineIdentifier":"expected-machine"}}`))
	}))
	defer right.Close()
	server, err := validatePlexRoutes(context.Background(), "expected-machine", "token", "viib-test", []string{wrong.URL, right.URL})
	if err != nil || server.URL != right.URL {
		t.Fatalf("alternate route: %+v, err=%v", server, err)
	}
}

func TestPlexAccountManagedPlaybackSwitchesToVerifiedRoute(t *testing.T) {
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()
	alternate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity", "/":
			_, _ = w.Write([]byte(`{"MediaContainer":{"machineIdentifier":"machine-proxy"}}`))
		case "/audio":
			if r.Header.Get("X-Plex-Token") != "resource-token" {
				t.Error("media request did not use the verified server's token")
			}
			_, _ = w.Write([]byte("alternate audio"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer alternate.Close()
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"provides": "server", "clientIdentifier": "machine-proxy", "accessToken": "resource-token",
			"connections": []map[string]any{{"uri": unreachable.URL}, {"uri": alternate.URL}},
		}})
	}))
	defer cloud.Close()
	database, handler, track := setupPlexProxyTest(t, unreachable.URL, "/audio", "resource-token", true)
	source, err := database.GetActivePlexSource()
	if err != nil {
		t.Fatal(err)
	}
	source.AccountManaged = true
	if err := database.SavePlexSource(*source); err != nil {
		t.Fatal(err)
	}
	handler.plexAuthFactory = func() *plex.AuthClient { return plex.NewAuthClientWithHTTP(cloud.URL, cloud.Client()) }
	if err := handler.savePlexCredentials(plex.Credentials{
		ClientIdentifier: "viib-test", AccountToken: "account-token", AccountTokenExpiry: time.Now().Add(48 * time.Hour).Unix(),
		ServerTokens: map[string]string{track.MachineID: "resource-token"},
	}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeAudioSourceAware(recorder, httptest.NewRequest(http.MethodGet, "/api/audio/"+track.SongID, nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "alternate audio" {
		t.Fatalf("failover playback: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	source, err = database.GetActivePlexSource()
	if err != nil || source.BaseURL != alternate.URL || !source.Available {
		t.Fatalf("alternate route was not persisted: %+v, err=%v", source, err)
	}
}

func TestPlexSourceReplacementRequiresConfirmationAndKeepsCatalogOnWriteFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "library.db")
	database, handler, oldTrack := setupPlexProxyTestWithDB(t, dbPath, "http://127.0.0.1:32400", "/audio", "old-token", true)
	server := plex.Server{MachineIdentifier: "new-machine", URL: "http://127.0.0.1:32401"}
	credentials, err := handler.loadPlexCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.activatePlexServer(server, &credentials, false, ""); !errors.Is(err, errPlexReplaceConfirmation) {
		t.Fatalf("unconfirmed replacement: %v", err)
	}
	writer, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.Exec(`CREATE TRIGGER fail_plex_replacement BEFORE INSERT ON plex_sources BEGIN SELECT RAISE(ABORT, 'test replacement failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := handler.activatePlexServer(server, &credentials, false, oldTrack.SourceID); err == nil {
		t.Fatal("expected the new source insert to fail")
	}
	source, err := database.GetActivePlexSource()
	track, trackErr := database.GetPlexTrackSource(oldTrack.SongID)
	saved, credentialErr := handler.loadPlexCredentials()
	if err != nil || source == nil || source.ID != oldTrack.SourceID || trackErr != nil || track == nil ||
		credentialErr != nil || saved.ServerTokens[oldTrack.MachineID] != "old-token" {
		t.Fatalf("failed replacement changed the previous source: source=%+v, err=%v, track=%+v, trackErr=%v, credentialErr=%v",
			source, err, track, trackErr, credentialErr)
	}
}
