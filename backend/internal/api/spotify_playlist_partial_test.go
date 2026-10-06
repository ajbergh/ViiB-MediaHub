package api

import (
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

func TestPlaylistPartialIsBoundedAndDoesNotPublishEntities(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	runtime := a.spotifyTokens()
	ctx, cancel := runtime.requestContext(t.Context())
	defer cancel()
	id := strings.Repeat("P", 22)
	entity := catalog.CapturedEntity{EntityType: "playlist", ID: id, Resource: "original-page", Payload: []byte(`{"unknown":false}`)}
	p := playlistPartialCheckpoint{Revision: "revision1", Next: "https://api.spotify.com/v1/playlists/" + id + "/tracks?offset=2&limit=2", NextOffset: 2, Items: []spotifyPlaylistItem{{}, {}}, Entities: []catalog.CapturedEntity{entity}}
	if !a.savePlaylistPartial(ctx, id, p) {
		t.Fatal("valid partial not saved")
	}
	got, ok := a.loadPlaylistPartial(ctx, id, p.Revision)
	if !ok || len(got.Items) != 2 || len(got.Entities) != 1 || string(got.Entities[0].Payload) != string(entity.Payload) {
		t.Fatalf("partial round trip: %+v %v", got, ok)
	}
	published, err := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: entity.Resource, ContextKey: runtime.metadataContext})
	if err != nil || published != nil {
		t.Fatal("partial published original page", err)
	}
	if _, ok := a.loadPlaylistPartial(ctx, id, "other-revision"); ok {
		t.Fatal("revision mismatch resumed")
	}
	invalid := p
	invalid.NextOffset = 3
	if a.savePlaylistPartial(ctx, id, invalid) {
		t.Fatal("gap saved")
	}
	invalid = p
	invalid.Next = "https://other.example/tracks?offset=2"
	if a.savePlaylistPartial(ctx, id, invalid) {
		t.Fatal("foreign resume URL saved")
	}
	runtime.beginRetirement()
	if a.savePlaylistPartial(ctx, id, p) {
		t.Fatal("retired request wrote partial")
	}
	if _, ok := a.loadPlaylistPartial(ctx, id, p.Revision); ok {
		t.Fatal("retired request resumed")
	}
}
