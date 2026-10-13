package api

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

const playlistPartialResource = "playlist_traversal_partial_v1"

type playlistPartialCheckpoint struct {
	Generation int64                    `json:"generation,omitempty"`
	Revision   string                   `json:"revision"`
	Next       string                   `json:"next"`
	NextOffset int                      `json:"nextOffset"`
	Items      []spotifyPlaylistItem    `json:"items"`
	Entities   []catalog.CapturedEntity `json:"entities"`
}

func validPlaylistPartial(id string, p playlistPartialCheckpoint) bool {
	target, err := url.Parse(p.Next)
	if err != nil || p.Revision == "" || p.Items == nil || p.NextOffset != len(p.Items) || p.NextOffset <= 0 || len(p.Entities) > 20000 {
		return false
	}
	offset, err := strconv.Atoi(target.Query().Get("offset"))
	return err == nil && offset == p.NextOffset && target.Scheme == "https" && target.Host == "api.spotify.com" && target.User == nil && target.Fragment == "" && target.Path == "/v1/playlists/"+id+"/tracks"
}

// Partial checkpoint persistence does not publish any contained domain entity.
func (a *API) savePlaylistPartial(ctx context.Context, id string, p playlistPartialCheckpoint) bool {
	if !validPlaylistPartial(id, p) {
		return false
	}
	buffer := &catalogCaptureBuffer{}
	if !buffer.bindPlaylistRevision(id, p.Revision) {
		return false
	}
	buffer.add(p.Entities)
	var captured bool
	p.Entities, captured = buffer.snapshot()
	if !captured {
		return false
	}
	if traversal, ok := ctx.Value(playlistTraversalContextKey{}).(db.SpotifyPlaylistTraversal); ok {
		p.Generation = traversal.Generation
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 2<<20 {
		return false
	}
	runtime := a.spotifyTokens()
	err = runtime.withAccount(ctx, func() error {
		now := time.Now().UTC()
		snapshot := db.SpotifyEntitySnapshot{SpotifySnapshotKey: db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: playlistPartialResource, ContextKey: runtime.metadataContext}, SchemaVersion: 1, AdapterRevision: "catalog-domain-v1", RetrievedAt: now, ExpiresAt: now.Add(24 * time.Hour), Payload: raw}
		if traversal, ok := ctx.Value(playlistTraversalContextKey{}).(db.SpotifyPlaylistTraversal); ok {
			return a.db.PutSpotifyPlaylistSnapshots(traversal, []db.SpotifyEntitySnapshot{snapshot}, false)
		}
		return a.db.PutSpotifyEntitySnapshotsForRuntime(db.SpotifyMetadataFence{Epoch: runtime.metadataEpoch, ContextKey: runtime.metadataContext}, []db.SpotifyEntitySnapshot{snapshot})
	})
	return err == nil
}
func (a *API) loadPlaylistPartial(ctx context.Context, id, revision string) (playlistPartialCheckpoint, bool) {
	runtime := a.spotifyTokens()
	var snapshot *db.SpotifyEntitySnapshot
	err := runtime.withAccount(ctx, func() error {
		var err error
		snapshot, err = a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: playlistPartialResource, ContextKey: runtime.metadataContext})
		return err
	})
	var p playlistPartialCheckpoint
	if err != nil || snapshot == nil || snapshot.SchemaVersion != 1 || snapshot.AdapterRevision != "catalog-domain-v1" || !time.Now().Before(snapshot.ExpiresAt) || json.Unmarshal(snapshot.Payload, &p) != nil || p.Revision != revision || !validPlaylistPartial(id, p) {
		return p, false
	}
	// Restore through the same aggregate bounds used by live capture.
	buffer := &catalogCaptureBuffer{}
	if !buffer.bindPlaylistRevision(id, p.Revision) {
		return playlistPartialCheckpoint{}, false
	}
	buffer.add(p.Entities)
	if !buffer.bindPlaylistRevision(id, p.Revision) {
		return playlistPartialCheckpoint{}, false
	}
	var captured bool
	p.Entities, captured = buffer.snapshot()
	if !captured {
		return playlistPartialCheckpoint{}, false
	}
	return p, true
}
