package db

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

func TestDownloadedRESTTrackRetainsReachableAlbumAfterQueueClear(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	trackID := referenceID
	albumID := strings.Repeat("A", 22)
	artistID := strings.Repeat("B", 22)
	artist := map[string]any{"id": artistID, "type": "artist", "uri": "spotify:artist:" + artistID, "name": "Fixture Artist"}
	album := map[string]any{"id": albumID, "type": "album", "uri": "spotify:album:" + albumID, "name": "Fixture Album", "artists": []any{artist}}
	track := map[string]any{"id": trackID, "type": "track", "uri": "spotify:track:" + trackID, "name": "Fixture Track", "duration_ms": 180000, "artists": []any{artist}, "album": album}
	sparseLong := map[string]any{"id": trackID, "type": "track", "uri": "spotify:track:" + trackID, "opaque_extra": strings.Repeat("x", 1024)}
	raw, err := json.Marshal(map[string]any{"tracks": []any{track, sparseLong}})
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("https://api.spotify.com/v1/tracks?ids=" + trackID + "," + trackID)
	entities, err := catalog.CaptureREST(target, raw)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, entity := range entities {
		snapshot := SpotifyEntitySnapshot{
			SpotifySnapshotKey: SpotifySnapshotKey{EntityType: entity.EntityType, SpotifyID: entity.ID, Resource: entity.Resource, ContextKey: "owner"},
			SchemaVersion:      1, AdapterRevision: "rest-capture-fixture", Payload: entity.Payload,
			RetrievedAt: now, ExpiresAt: now.Add(time.Hour),
		}
		for _, relation := range entity.Relations {
			snapshot.Relations = append(snapshot.Relations, SpotifyEntityRelation{
				Kind: relation.Kind, Position: relation.Position, ChildType: relation.ChildType, ChildID: relation.ChildID,
				Unavailable: relation.Unavailable, Metadata: relation.Metadata,
			})
		}
		if err := d.PutSpotifyEntitySnapshot(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	finishEvidence(t, d, path, "rest-track-job", trackID)
	fingerprint := scanEvidence(t, d, path, "rest-track-song")
	if _, err := d.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	if err := d.RetireSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	d.Close()

	reopened, err := New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetDownloadedSpotifyCatalog(t.Context(), "rest-track-song", fingerprint)
	if err != nil || got == nil || got.CatalogStatus == nil || got.CatalogStatus.State != "available" {
		t.Fatalf("durable REST graph unavailable: %+v %v", got, err)
	}
	foundAlbum, foundTrackAlbum, foundArtist, foundAlbumArtist := false, false, false, false
	for _, snapshot := range got.Snapshots {
		if snapshot.EntityType == "album" && snapshot.SpotifyID == albumID {
			foundAlbum = true
		}
		if snapshot.EntityType == "artist" && snapshot.SpotifyID == artistID {
			foundArtist = true
		}
	}
	for _, relation := range got.Relations {
		if relation.ParentType == "track" && relation.ParentID == trackID && relation.Kind == "album" && relation.ChildType == "album" && relation.ChildID == albumID && !relation.Unavailable {
			foundTrackAlbum = true
		}
		if relation.ParentType == "album" && relation.ParentID == albumID && relation.Kind == "artists" && relation.ChildType == "artist" && relation.ChildID == artistID && !relation.Unavailable {
			foundAlbumArtist = true
		}
	}
	if !foundAlbum || !foundTrackAlbum || !foundArtist || !foundAlbumArtist {
		t.Fatalf("queue cleanup/retirement lost REST track→album→artist graph: album=%v track_album=%v artist=%v album_artist=%v graph=%+v", foundAlbum, foundTrackAlbum, foundArtist, foundAlbumArtist, got)
	}
}
