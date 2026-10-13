package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

func TestCatalogCaptureRevisionBinding(t *testing.T) {
	id := strings.Repeat("P", 22)
	root := catalog.CapturedEntity{EntityType: "playlist", ID: id, Resource: "root", Payload: []byte(`{}`)}
	b := &catalogCaptureBuffer{}
	b.add([]catalog.CapturedEntity{root, {EntityType: "track", ID: id, Resource: "related", Payload: []byte(`{}`)}})
	if !b.bindPlaylistRevision(id, "version1") {
		t.Fatal("root binding failed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.add([]catalog.CapturedEntity{{EntityType: "playlist", ID: id, Resource: "page", Payload: []byte(`{}`)}})
		}()
	}
	wg.Wait()
	entities, ok := b.snapshot()
	if !ok || len(entities) != 3 {
		t.Fatal(ok, len(entities))
	}
	for _, e := range entities {
		if e.EntityType == "playlist" && e.CaptureRevision != "version1" {
			t.Fatal(e)
		}
		if e.EntityType == "track" && e.CaptureRevision != "" {
			t.Fatal("nested track relabeled", e)
		}
	}
	if b.bindPlaylistRevision(id, "version2") {
		t.Fatal("revision rebound")
	}
	b.add([]catalog.CapturedEntity{{EntityType: "playlist", ID: id, Resource: "wrong", CaptureRevision: "version2", Payload: []byte(`{}`)}})
	if _, ok = b.snapshot(); ok {
		t.Fatal("mixed revisions admitted")
	}
	if b.bindPlaylistRevision(id, "version1") {
		t.Fatal("mixed revision buffer recovered silently")
	}
}

func TestCompletedPlaylistReuseDoesNotRelabelIndependentPages(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("P", 22)
	pageCalls := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		raw := `{"type":"playlist","id":"` + id + `","snapshot_id":"version1","tracks":{"offset":0,"items":[null],"next":"https://api.spotify.com/v1/playlists/` + id + `/tracks?offset=1&limit=1"}}`
		if strings.HasSuffix(r.URL.Path, "/tracks") {
			pageCalls++
			raw = `{"offset":1,"items":[null],"next":null}`
		} else if r.URL.Query().Get("fields") != "" {
			raw = `{"snapshot_id":"version1"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}, nil
	})}
	if _, _, _, err := a.fetchPlaylistTracks(t.Context(), id, nil); err != nil {
		t.Fatal(err)
	}
	key := db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: "rest:/v1/playlists/" + id + "/tracks:page:1:1", ContextKey: a.spotifyTokens().metadataContext}
	page, err := a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || page == nil || page.CaptureRevision != "version1" {
		t.Fatal("traversal page unbound", page, err)
	}
	page.CaptureRevision = ""
	page.RetrievedAt = time.Now().Add(time.Second)
	page.ExpiresAt = page.RetrievedAt.Add(time.Hour)
	if err = a.db.PutSpotifyEntitySnapshot(*page); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = a.fetchPlaylistTracks(t.Context(), id, nil); err != nil {
		t.Fatal(err)
	}
	if pageCalls != 1 {
		t.Fatal("completed checkpoint not reused", pageCalls)
	}
	page, err = a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || page == nil || page.CaptureRevision != "" {
		t.Fatal("reuse relabeled unrelated page", page, err)
	}
	checkpoint, err := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: playlistCompletedResource, ContextKey: key.ContextKey})
	if err != nil || checkpoint == nil || checkpoint.CaptureRevision != "version1" {
		t.Fatal("normalized checkpoint lost binding", checkpoint, err)
	}
}

func TestCatalogCaptureBufferMergesRelationCoverage(t *testing.T) {
	trackID := strings.Repeat("T", 22)
	albumID := strings.Repeat("A", 22)
	buffer := &catalogCaptureBuffer{}
	buffer.add([]catalog.CapturedEntity{{EntityType: "track", ID: trackID, Resource: "resource", Payload: []byte(`{}`), Relations: []catalog.CapturedRelation{{Kind: "album", Position: 0, ChildType: "album", ChildID: albumID}}}})
	buffer.add([]catalog.CapturedEntity{{EntityType: "track", ID: trackID, Resource: "resource", Payload: []byte(`{"opaque":"longer"}`)}})
	entities, ok := buffer.snapshot()
	if !ok || len(entities) != 1 || len(entities[0].Relations) != 1 || entities[0].Relations[0].ChildID != albumID || entities[0].Relations[0].Unavailable {
		t.Fatalf("buffer snapshot lost relation coverage: ok=%v entities=%+v", ok, entities)
	}
}
