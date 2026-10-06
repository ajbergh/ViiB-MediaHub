package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

func TestCompletedPlaylistCheckpointEligibility(t *testing.T) {
	for _, kind := range []string{"current", "revision", "expired", "incomplete", "row_count", "schema", "adapter", "retired", "restart"} {
		t.Run(kind, func(t *testing.T) {
			a, _ := newBPMRouteTestAPI(t, false)
			runtime := a.spotifyTokens()
			ctx, cancel := runtime.requestContext(t.Context())
			defer cancel()
			id := strings.Repeat("P", 22)
			checkpoint := playlistCompletedCheckpoint{Revision: "revision1", Complete: true, RowCount: 2, Items: []spotifyPlaylistItem{{}, {}}}
			if kind == "incomplete" {
				checkpoint.Complete = false
			}
			if kind == "row_count" {
				checkpoint.RowCount = 3
			}
			raw, _ := json.Marshal(checkpoint)
			now := time.Now().UTC()
			snapshot := db.SpotifyEntitySnapshot{SpotifySnapshotKey: db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: playlistCompletedResource, ContextKey: runtime.metadataContext}, SchemaVersion: 1, AdapterRevision: "catalog-domain-v1", RetrievedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), Payload: raw}
			if kind == "expired" {
				snapshot.ExpiresAt = now.Add(-time.Minute)
			}
			if kind == "schema" {
				snapshot.SchemaVersion = 2
			}
			if kind == "adapter" {
				snapshot.AdapterRevision = "other-contract"
			}
			if err := a.db.PutSpotifyEntitySnapshot(snapshot); err != nil {
				t.Fatal(err)
			}
			revision := "revision1"
			if kind == "revision" {
				revision = "revision2"
			}
			if kind == "retired" {
				runtime.beginRetirement()
			}
			if kind == "restart" {
				a.spotifyAuth = newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
				var stop func()
				ctx, stop = a.spotifyAuth.requestContext(t.Context())
				defer stop()
			}
			rows, ok := a.completedPlaylistRows(ctx, id, revision)
			if ok != (kind == "current") {
				t.Fatalf("checkpoint eligibility: %v", ok)
			}
			if ok && len(rows) != 2 {
				t.Fatal("ordered unavailable rows lost")
			}
		})
	}
}
