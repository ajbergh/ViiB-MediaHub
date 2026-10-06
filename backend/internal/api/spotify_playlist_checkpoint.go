package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

const playlistCompletedResource = "playlist_traversal_complete_v1"

type playlistCompletedCheckpoint struct {
	Revision string                `json:"revision"`
	Complete bool                  `json:"complete"`
	RowCount int                   `json:"rowCount"`
	Items    []spotifyPlaylistItem `json:"items"`
}

// Current root identity/revision is obtained upstream before consulting this
// compatibility checkpoint. Original domain pages remain stored separately.
func (a *API) completedPlaylistRows(ctx context.Context, id, revision string) ([]spotifyPlaylistItem, bool) {
	if revision == "" {
		return nil, false
	}
	runtime := a.spotifyTokens()
	var snapshot *db.SpotifyEntitySnapshot
	err := runtime.withAccount(ctx, func() error {
		var err error
		snapshot, err = a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: playlistCompletedResource, ContextKey: runtime.metadataContext})
		return err
	})
	if err != nil || snapshot == nil || snapshot.SchemaVersion != 1 || snapshot.AdapterRevision != "catalog-domain-v1" || !time.Now().Before(snapshot.ExpiresAt) {
		return nil, false
	}
	var checkpoint playlistCompletedCheckpoint
	if json.Unmarshal(snapshot.Payload, &checkpoint) != nil || !checkpoint.Complete || checkpoint.Revision != revision || checkpoint.RowCount != len(checkpoint.Items) || checkpoint.Items == nil {
		return nil, false
	}
	return checkpoint.Items, true
}

func completedPlaylistCapture(id, revision string, items []spotifyPlaylistItem) []catalog.CapturedEntity {
	if revision == "" {
		return nil
	}
	if items == nil {
		items = []spotifyPlaylistItem{}
	}
	raw, err := json.Marshal(playlistCompletedCheckpoint{Revision: revision, Complete: true, RowCount: len(items), Items: items})
	if err != nil || len(raw) > 2<<20 {
		return nil
	}
	return []catalog.CapturedEntity{{EntityType: "playlist", ID: id, Resource: playlistCompletedResource, Payload: raw}}
}
