package api

import (
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"net/http/httptest"
	"testing"
	"time"
)

func TestImportedCatalogAbsentWithoutProviderRuntime(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	for _, id := range []string{"song", "missing"} {
		w := httptest.NewRecorder()
		a.V2Routes().ServeHTTP(w, httptest.NewRequest("GET", "/analysis/"+id+"/imported-catalog", nil))
		if w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d %s", id, w.Code, w.Body.String())
		}
	}
}

func TestImportedCatalogAvailableOfflineAfterQueueAndPrivateCleanup(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	const recording = "AAAAAAAAAAAAAAAAAAAAAA"
	if err := a.db.ActivateSpotifyMetadataContext("catalog-owner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := a.db.PutSpotifyEntitySnapshot(db.SpotifyEntitySnapshot{SpotifySnapshotKey: db.SpotifySnapshotKey{EntityType: "track", SpotifyID: recording, Resource: "track", ContextKey: "catalog-owner"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{"name":"Retained","popularity":0,"unknown":false,"token":"redact"}`), RetrievedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := a.db.AddDownload(&db.SpotifyDownload{ID: "catalog-job", SpotifyID: recording, Type: "track", Title: "Retained", Status: "queued", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.db.MarkDownloadStarted("catalog-job"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := a.db.MarkDownloadCompletedWithEvidence(t.Context(), "catalog-job", path); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := a.db.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	if err := a.db.RetireSpotifyMetadataContext("catalog-owner"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.V2Routes().ServeHTTP(w, httptest.NewRequest("GET", "/analysis/song/imported-catalog", nil))
	if w.Code != 200 || w.Header().Get("ETag") == "" {
		t.Fatalf("offline catalog: %d %s", w.Code, w.Body.String())
	}
	var value db.DownloadedSpotifyCatalog
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if len(value.Snapshots) != 1 || value.Provenance != "spotify_download_import" || value.CatalogStatus == nil || value.CatalogStatus.Scope != "track_album_artist_graph_v2" {
		t.Fatalf("catalog: %+v", value)
	}
	var payload map[string]any
	if err := json.Unmarshal(value.Snapshots[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["popularity"] != float64(0) || payload["unknown"] != false || payload["token"] != nil {
		t.Fatalf("domain presence/sanitization: %+v", payload)
	}
}

func TestImportedCatalogEmptyOutcomeIsAvailableOffline(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	if err := a.db.AddDownload(&db.SpotifyDownload{ID: "empty-catalog", SpotifyID: "AAAAAAAAAAAAAAAAAAAAAA", Type: "track", Title: "Fixture", Status: "queued", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.db.MarkDownloadStarted("empty-catalog"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := a.db.MarkDownloadCompletedWithEvidence(t.Context(), "empty-catalog", path); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := a.db.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.V2Routes().ServeHTTP(w, httptest.NewRequest("GET", "/analysis/song/imported-catalog", nil))
	var value db.DownloadedSpotifyCatalog
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil || w.Code != 200 || value.CatalogStatus == nil || value.CatalogStatus.State != "not_available" || value.CatalogStatus.Scope != "track_album_artist_graph_v2" || len(value.Snapshots) != 0 || len(value.Relations) != 0 || w.Header().Get("ETag") == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("empty outcome: %d %s %v", w.Code, w.Body.String(), err)
	}
}

func TestImportedRequestedLibraryPageAvailableOffline(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	recording := "AAAAAAAAAAAAAAAAAAAAAA"
	if err := a.db.ActivateSpotifyMetadataContext("collection-owner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	page := db.SpotifyEntitySnapshot{SpotifySnapshotKey: db.SpotifySnapshotKey{EntityType: "library", SpotifyID: "rest_saved_tracks", Resource: "saved-page", ContextKey: "collection-owner"}, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(`{"items":[],"unknown":false,"token":"redact"}`), Relations: []db.SpotifyEntityRelation{{Kind: "library_items", Position: 0, ChildType: "track", ChildID: recording}}}
	if err := a.db.PutSpotifyEntitySnapshot(page); err != nil {
		t.Fatal(err)
	}
	if err := a.db.AddDownload(&db.SpotifyDownload{ID: "collection-job", SpotifyID: recording, Type: "track", Status: "queued", AddedAt: 1, Origins: []db.SpotifyDownloadOrigin{{Kind: "library", ID: "saved_tracks", EntityID: recording, Position: -1}}}); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.db.MarkDownloadStarted("collection-job"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := a.db.MarkDownloadCompletedWithEvidence(t.Context(), "collection-job", path); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := a.db.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	if err := a.db.RetireSpotifyMetadataContext("collection-owner"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.V2Routes().ServeHTTP(w, httptest.NewRequest("GET", "/analysis/song/imported-catalog", nil))
	var value db.DownloadedSpotifyCatalog
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil || w.Code != 200 || value.CollectionStatus == nil || value.CollectionStatus.State != "available" || len(value.Snapshots) != 1 || len(value.Relations) != 1 || value.Snapshots[0].CapturedResource != "saved-page" {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") == "" {
		t.Fatal(w.Header())
	}
	var payload map[string]any
	if err := json.Unmarshal(value.Snapshots[0].Payload, &payload); err != nil || payload["unknown"] != false || payload["token"] != nil {
		t.Fatal(payload, err)
	}
}
