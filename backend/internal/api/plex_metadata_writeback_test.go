package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestPlexMetadataWritebackPreviewsApprovesLocksAndVerifies(t *testing.T) {
	const token = "writeback-token"
	genres := []string{"Rock"}
	year := 1999
	var puts int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != token {
			t.Fatalf("missing Plex token for %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/library/metadata/99":
			plexGenres := make([]any, 0, len(genres))
			for _, genre := range genres {
				plexGenres = append(plexGenres, map[string]any{"tag": genre})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": []any{map[string]any{
				"ratingKey": "99", "type": "track", "title": "Proxy Track", "year": year, "Genre": plexGenres,
			}}}})
		case r.Method == http.MethodGet && r.URL.Path == "/media/providers":
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"MediaProvider": []any{map[string]any{
				"identifier": "com.plexapp.plugins.library", "Feature": []any{map[string]any{"type": "manage"}},
			}}}})
		case r.Method == http.MethodPut && r.URL.Path == "/library/metadata/99":
			query := r.URL.Query()
			if query.Get("genre.locked") != "1" || query.Get("genre[0].tag.tag") != "Dream Pop" || query.Get("genre[1].tag.tag") != "Indie Rock" || query.Get("year.value") != "1988" || query.Get("year.locked") != "1" {
				t.Fatalf("missing locked writeback fields: %q", r.URL.RawQuery)
			}
			genres, year, puts = []string{"Dream Pop", "Indie Rock"}, 1988, puts+1
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	database, handler, track := setupPlexProxyTest(t, upstream.URL, "/audio", token, true)
	if _, err := database.ApplyAIEnrichmentBatch([]db.AIEnrichmentUpdate{{
		SongID: track.SongID, Genres: []string{"dream pop", "indie rock"}, OriginalYear: 1988,
	}}, true); err != nil {
		t.Fatal(err)
	}

	previewRecorder := httptest.NewRecorder()
	handler.PlexRoutes().ServeHTTP(previewRecorder, httptest.NewRequest(http.MethodPost, "/metadata-writeback/preview", strings.NewReader(`{}`)))
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewRecorder.Code, previewRecorder.Body.String())
	}
	var preview plexMetadataWritebackPreview
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Confirmation == "" || len(preview.Items) != 1 || preview.Items[0].Status != "ready" || len(preview.Items[0].Changes) != 2 {
		t.Fatalf("unexpected preview: %#v", preview)
	}

	syncBody, _ := json.Marshal(plexMetadataWritebackRequest{Confirmation: preview.Confirmation})
	syncRecorder := httptest.NewRecorder()
	handler.PlexRoutes().ServeHTTP(syncRecorder, httptest.NewRequest(http.MethodPost, "/metadata-writeback/sync", strings.NewReader(string(syncBody))))
	if syncRecorder.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", syncRecorder.Code, syncRecorder.Body.String())
	}
	var result plexMetadataWritebackResult
	if err := json.Unmarshal(syncRecorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || result.Failed != 0 || puts != 1 {
		t.Fatalf("unexpected writeback result=%#v puts=%d", result, puts)
	}
	candidates, _, err := database.GetPlexAIWritebackCandidates(track.SourceID, nil, 100)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("successful writeback was left pending: %#v err=%v", candidates, err)
	}
}

func TestPlexMetadataWritebackRejectsMissingPreview(t *testing.T) {
	_, handler, _ := setupPlexProxyTest(t, "http://127.0.0.1:1", "/audio", "token", true)
	recorder := httptest.NewRecorder()
	handler.PlexRoutes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/metadata-writeback/sync", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "Preview and approve") {
		t.Fatalf("expected preview-required conflict, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
