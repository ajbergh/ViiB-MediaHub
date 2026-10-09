package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify"
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

const scrapedPlaylistUnboundResource = "embed_scrape_playlist_unbound_v1"

func scrapedPlaylistCapture(playlistID string, scraped *spotify.ScrapedPlaylist, tracks map[string]PlaylistTrackInfo) []catalog.CapturedEntity {
	if scraped == nil || len(scraped.Tracks) > 20000 {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"name": scraped.Name, "artwork": scraped.Artwork, "provenance": "embed_scrape_unbound", "revisionKnown": false,
	})
	if err != nil {
		return nil
	}
	relations := make([]catalog.CapturedRelation, 0, len(scraped.Tracks))
	for position, id := range scraped.Tracks {
		relation := catalog.CapturedRelation{Kind: "playlist_items", Position: position, ChildType: "track", Metadata: []byte("{}")}
		if db.ValidSpotifyRecordingID(id) {
			relation.ChildID = id
			_, available := tracks[id]
			relation.Unavailable = !available
		} else {
			relation.Unavailable = true
		}
		relations = append(relations, relation)
	}
	return []catalog.CapturedEntity{{EntityType: "playlist", ID: playlistID, Resource: scrapedPlaylistUnboundResource, Payload: payload, Relations: relations}}
}
