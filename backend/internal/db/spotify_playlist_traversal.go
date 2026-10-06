package db

import "errors"

var ErrSpotifyTraversalSuperseded = errors.New("Spotify playlist traversal superseded")

type SpotifyPlaylistTraversal struct {
	ContextKey   string
	SpotifyID    string
	Generation   int64
	RuntimeEpoch string
}

// BeginSpotifyPlaylistTraversal advances the publication owner before fetching
// a root. Checkpoint content can be reused, but prior workers lose write access.
func (d *DB) BeginSpotifyPlaylistTraversal(contextKey, id string) (SpotifyPlaylistTraversal, error) {
	return d.beginSpotifyPlaylistTraversal(contextKey, id, "")
}

func (d *DB) BeginSpotifyPlaylistTraversalForRuntime(fence SpotifyMetadataFence, id string) (SpotifyPlaylistTraversal, error) {
	if fence.Epoch == "" {
		return SpotifyPlaylistTraversal{}, ErrSpotifyMetadataRuntimeSuperseded
	}
	return d.beginSpotifyPlaylistTraversal(fence.ContextKey, id, fence.Epoch)
}

func (d *DB) beginSpotifyPlaylistTraversal(contextKey, id, epoch string) (SpotifyPlaylistTraversal, error) {
	token := SpotifyPlaylistTraversal{ContextKey: contextKey, SpotifyID: id, RuntimeEpoch: epoch}
	if !validSnapshotKey(SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: "traversal", ContextKey: contextKey}) {
		return token, errors.New("invalid playlist traversal identity")
	}
	err := d.conn.QueryRow(`INSERT INTO spotify_playlist_traversals(context_key,spotify_id,generation,complete)
 SELECT ?,?,1,0 WHERE ?=COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'')
 AND (?='' OR ?=COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_runtime_epoch'),''))
 ON CONFLICT(context_key,spotify_id) DO UPDATE SET generation=generation+1,complete=0
 RETURNING generation`, contextKey, id, contextKey, epoch, epoch).Scan(&token.Generation)
	return token, err
}

// Publication ownership and every snapshot/relation update share one transaction.
func (d *DB) PutSpotifyPlaylistSnapshots(token SpotifyPlaylistTraversal, snapshots []SpotifyEntitySnapshot, complete bool) error {
	var fence *SpotifyMetadataFence
	if token.RuntimeEpoch != "" {
		fence = &SpotifyMetadataFence{Epoch: token.RuntimeEpoch, ContextKey: token.ContextKey}
	}
	return d.putSpotifyEntitySnapshots(snapshots, &token, complete, fence)
}
