package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/ajbergh/viib-mediahub/internal/spotify/metadata"
)

type SpotifySnapshotKey struct {
	EntityType string `json:"entityType"`
	SpotifyID  string `json:"spotifyId"`
	Resource   string `json:"resource"`
	ContextKey string `json:"-"`
}
type SpotifyEntityRelation struct {
	Kind        string `json:"kind"`
	Position    int    `json:"position"`
	ChildType   string `json:"childType"`
	ChildID     string `json:"childId"`
	Unavailable bool   `json:"unavailable"`
	Metadata    []byte `json:"-"`
}
type SpotifyEntitySnapshot struct {
	CaptureRevision string `json:"captureRevision,omitempty"`
	SpotifySnapshotKey
	SchemaVersion   int                     `json:"schemaVersion"`
	AdapterRevision string                  `json:"adapterRevision"`
	Payload         []byte                  `json:"-"`
	PayloadHash     string                  `json:"payloadHash"`
	RetrievedAt     time.Time               `json:"retrievedAt"`
	ExpiresAt       time.Time               `json:"expiresAt"`
	Relations       []SpotifyEntityRelation `json:"relations"`
}

func validSnapshotKey(key SpotifySnapshotKey) bool {
	validID := ValidSpotifyRecordingID(key.SpotifyID)
	switch key.EntityType {
	case "library":
		validID = key.SpotifyID == "libraryV3_Albums" || key.SpotifyID == "libraryV3_Playlists" || key.SpotifyID == "rest_saved_tracks" || key.SpotifyID == "rest_saved_albums" || key.SpotifyID == "rest_saved_playlists"
	case "track", "album", "artist", "playlist":
	default:
		return false
	}
	return validID && key.Resource != "" && len(key.Resource) <= 128 && key.ContextKey != "" && len(key.ContextKey) <= 256
}

// PutSpotifyEntitySnapshot commits a sanitized last-good payload and its ordered
// relations together. Older requests cannot replace newer snapshots or relations.
func (d *DB) PutSpotifyEntitySnapshot(snapshot SpotifyEntitySnapshot) error {
	return d.PutSpotifyEntitySnapshots([]SpotifyEntitySnapshot{snapshot})
}

// PutSpotifyEntitySnapshots publishes a bounded collected catalog transaction.
// A failed entity leaves every prior snapshot and relation unchanged.
func (d *DB) PutSpotifyEntitySnapshots(snapshots []SpotifyEntitySnapshot) error {
	return d.putSpotifyEntitySnapshots(snapshots, nil, false, nil)
}

func (d *DB) putSpotifyEntitySnapshots(snapshots []SpotifyEntitySnapshot, traversal *SpotifyPlaylistTraversal, complete bool, fence *SpotifyMetadataFence) error {
	if len(snapshots) > 20000 {
		return errors.New("catalog batch exceeds entity limit")
	}
	size := 0
	for _, snapshot := range snapshots {
		size += len(snapshot.Payload)
		for _, relation := range snapshot.Relations {
			size += len(relation.Metadata)
		}
		if size > 8<<20 {
			return errors.New("catalog batch exceeds byte limit")
		}
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if fence != nil {
		if err := checkSpotifyMetadataFenceTx(tx, *fence); err != nil {
			return err
		}
		for _, snapshot := range snapshots {
			if snapshot.ContextKey != fence.ContextKey {
				return ErrSpotifyMetadataRuntimeSuperseded
			}
		}
	}
	if traversal != nil {
		result, err := tx.Exec(`UPDATE spotify_playlist_traversals SET complete=? WHERE context_key=? AND spotify_id=? AND generation=? AND complete=0 AND context_key=COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'')`, complete, traversal.ContextKey, traversal.SpotifyID, traversal.Generation)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrSpotifyTraversalSuperseded
		}
		for _, snapshot := range snapshots {
			if snapshot.ContextKey != traversal.ContextKey {
				return errors.New("playlist publication context mismatch")
			}
		}
	}
	for _, snapshot := range snapshots {
		if err := putSpotifyEntitySnapshotTx(tx, snapshot); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validSnapshotCaptureRevision(snapshot SpotifyEntitySnapshot) bool {
	return len(snapshot.CaptureRevision) <= 256 && strings.IndexFunc(snapshot.CaptureRevision, unicode.IsControl) < 0 && (snapshot.CaptureRevision == "" || snapshot.EntityType == "playlist")
}

func putSpotifyEntitySnapshotTx(tx *sql.Tx, snapshot SpotifyEntitySnapshot) error {
	if !validSnapshotCaptureRevision(snapshot) || !validSnapshotKey(snapshot.SpotifySnapshotKey) || snapshot.SchemaVersion <= 0 || snapshot.AdapterRevision == "" || len(snapshot.AdapterRevision) > 256 || snapshot.RetrievedAt.IsZero() || !snapshot.ExpiresAt.After(snapshot.RetrievedAt) || len(snapshot.Relations) > 20000 {
		return errors.New("invalid Spotify snapshot provenance")
	}
	payload, err := metadata.Sanitize(snapshot.Payload, metadata.CatalogLimit)
	if err != nil {
		return err
	}
	relations := append([]SpotifyEntityRelation(nil), snapshot.Relations...)
	seen := map[string]map[int]bool{}
	relationBytes := 0
	for i := range relations {
		relation := &relations[i]
		if relation.Kind == "" || len(relation.Kind) > 128 || relation.Position < 0 || len(relation.ChildType) > 128 || (relation.ChildID != "" && !ValidSpotifyRecordingID(relation.ChildID)) {
			return errors.New("invalid Spotify relation")
		}
		if seen[relation.Kind] == nil {
			seen[relation.Kind] = map[int]bool{}
		}
		if seen[relation.Kind][relation.Position] {
			return errors.New("duplicate Spotify relation position")
		}
		seen[relation.Kind][relation.Position] = true
		if len(relation.Metadata) == 0 {
			relation.Metadata = []byte(`{}`)
		}
		relation.Metadata, err = metadata.Sanitize(relation.Metadata, metadata.ScalarLimit)
		if err != nil {
			return err
		}
		relationBytes += len(relation.Metadata)
		if relationBytes > metadata.CatalogLimit {
			return errors.New("Spotify relation metadata exceeds limit")
		}
	}
	sum := sha256.Sum256(payload)
	// Partial traversals from concurrent workers may arrive out of order. For
	// the same revision retain the farthest prefix, and make completion terminal.
	// Enforce this in the UPSERT so read/check/write races cannot regress progress.
	result, err := tx.Exec(`INSERT INTO spotify_entity_snapshots(entity_type,spotify_id,resource,context_key,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,capture_revision)
 VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(entity_type,spotify_id,resource,context_key) DO UPDATE SET
 schema_version=excluded.schema_version,adapter_revision=excluded.adapter_revision,payload=excluded.payload,payload_hash=excluded.payload_hash,retrieved_at=excluded.retrieved_at,expires_at=excluded.expires_at,capture_revision=excluded.capture_revision
  WHERE excluded.retrieved_at>=spotify_entity_snapshots.retrieved_at
  AND (excluded.resource!='playlist_traversal_partial_v1'
    OR COALESCE(json_extract(excluded.payload,'$.generation'),0)>COALESCE(json_extract(spotify_entity_snapshots.payload,'$.generation'),0)
    OR COALESCE(json_extract(excluded.payload,'$.revision'),'')!=COALESCE(json_extract(spotify_entity_snapshots.payload,'$.revision'),'')
    OR COALESCE(json_extract(excluded.payload,'$.complete'),0)=1
    OR (COALESCE(json_extract(spotify_entity_snapshots.payload,'$.complete'),0)!=1
      AND COALESCE(json_extract(excluded.payload,'$.nextOffset'),0)>=COALESCE(json_extract(spotify_entity_snapshots.payload,'$.nextOffset'),0)))`, snapshot.EntityType, snapshot.SpotifyID, snapshot.Resource, snapshot.ContextKey, snapshot.SchemaVersion, snapshot.AdapterRevision, payload, hex.EncodeToString(sum[:]), snapshot.RetrievedAt.UnixMilli(), snapshot.ExpiresAt.UnixMilli(), snapshot.CaptureRevision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	_, err = tx.Exec(`DELETE FROM spotify_entity_relations WHERE entity_type=? AND spotify_id=? AND resource=? AND context_key=?`, snapshot.EntityType, snapshot.SpotifyID, snapshot.Resource, snapshot.ContextKey)
	if err != nil {
		return err
	}
	for _, relation := range relations {
		_, err = tx.Exec(`INSERT INTO spotify_entity_relations(entity_type,spotify_id,resource,context_key,relation_kind,position,child_type,child_id,unavailable,metadata_json) VALUES(?,?,?,?,?,?,?,?,?,?)`, snapshot.EntityType, snapshot.SpotifyID, snapshot.Resource, snapshot.ContextKey, relation.Kind, relation.Position, relation.ChildType, relation.ChildID, relation.Unavailable, relation.Metadata)
		if err != nil {
			return err
		}
	}
	return nil
}
func (d *DB) GetSpotifyEntitySnapshot(key SpotifySnapshotKey) (*SpotifyEntitySnapshot, error) {
	if !validSnapshotKey(key) {
		return nil, errors.New("invalid Spotify snapshot key")
	}
	value := SpotifyEntitySnapshot{SpotifySnapshotKey: key}
	var retrieved, expires int64
	err := d.conn.QueryRow(`SELECT schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,capture_revision FROM spotify_entity_snapshots WHERE entity_type=? AND spotify_id=? AND resource=? AND context_key=?`, key.EntityType, key.SpotifyID, key.Resource, key.ContextKey).Scan(&value.SchemaVersion, &value.AdapterRevision, &value.Payload, &value.PayloadHash, &retrieved, &expires, &value.CaptureRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !validSnapshotCaptureRevision(value) {
		return nil, errors.New("invalid Spotify snapshot capture revision")
	}
	sanitized, err := metadata.Sanitize(value.Payload, metadata.CatalogLimit)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(sanitized)
	if hex.EncodeToString(sum[:]) != value.PayloadHash {
		return nil, errors.New("Spotify snapshot integrity mismatch")
	}
	value.RetrievedAt = time.UnixMilli(retrieved).UTC()
	value.ExpiresAt = time.UnixMilli(expires).UTC()
	rows, err := d.conn.Query(`SELECT relation_kind,position,child_type,child_id,unavailable,metadata_json FROM spotify_entity_relations WHERE entity_type=? AND spotify_id=? AND resource=? AND context_key=? ORDER BY relation_kind,position`, key.EntityType, key.SpotifyID, key.Resource, key.ContextKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var relation SpotifyEntityRelation
		if err = rows.Scan(&relation.Kind, &relation.Position, &relation.ChildType, &relation.ChildID, &relation.Unavailable, &relation.Metadata); err != nil {
			return nil, err
		}
		value.Relations = append(value.Relations, relation)
	}
	return &value, rows.Err()
}

// RetireSpotifyMetadataContext removes private snapshots and attempt state only.
// Source-bound download evidence and local preparation belong to separate stores.
func (d *DB) RetireSpotifyMetadataContext(contextKey string) error {
	if contextKey == "" {
		return errors.New("account context required")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"spotify_download_lineage_staging", "spotify_entity_relations", "spotify_entity_snapshots", "spotify_metadata_resource_status", "spotify_audio_artifacts", "spotify_playlist_traversals", "spotify_metadata_owner"} {
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE context_key=?", contextKey); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type SpotifyMetadataResourceStatus struct {
	State     string    `json:"state"`
	Reason    string    `json:"reason"`
	CheckedAt time.Time `json:"checkedAt"`
	RetryAt   time.Time `json:"retryAt"`
}

// PutSpotifyMetadataResourceStatus records attempts separately from last-good data.
func (d *DB) PutSpotifyMetadataResourceStatus(key SpotifySnapshotKey, status SpotifyMetadataResourceStatus) error {
	return putSpotifyMetadataResourceStatus(d.conn, key, status)
}

func (d *DB) PutSpotifyMetadataResourceStatusForRuntime(fence SpotifyMetadataFence, key SpotifySnapshotKey, status SpotifyMetadataResourceStatus) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if key.ContextKey != fence.ContextKey {
		return ErrSpotifyMetadataRuntimeSuperseded
	}
	if err := checkSpotifyMetadataFenceTx(tx, fence); err != nil {
		return err
	}
	if err := putSpotifyMetadataResourceStatus(tx, key, status); err != nil {
		return err
	}
	return tx.Commit()
}

func putSpotifyMetadataResourceStatus(executor preparationExecutor, key SpotifySnapshotKey, status SpotifyMetadataResourceStatus) error {
	if !validSnapshotKey(key) || status.CheckedAt.IsZero() || status.RetryAt.Before(status.CheckedAt) || len(status.Reason) > 128 {
		return errors.New("invalid metadata resource status")
	}
	switch status.State {
	case "pending", "running", "available", "not_returned", "unavailable", "low_confidence", "unsupported", "failed", "cooldown", "canceled":
	default:
		return errors.New("invalid metadata resource state")
	}
	_, err := executor.Exec(`INSERT INTO spotify_metadata_resource_status(entity_type,spotify_id,resource,context_key,state,reason,checked_at,retry_at) VALUES(?,?,?,?,?,?,?,?)
 ON CONFLICT(entity_type,spotify_id,resource,context_key) DO UPDATE SET state=excluded.state,reason=excluded.reason,checked_at=excluded.checked_at,retry_at=excluded.retry_at WHERE excluded.checked_at>spotify_metadata_resource_status.checked_at`, key.EntityType, key.SpotifyID, key.Resource, key.ContextKey, status.State, status.Reason, status.CheckedAt.UnixMilli(), status.RetryAt.UnixMilli())
	return err
}
func (d *DB) GetSpotifyMetadataResourceStatus(key SpotifySnapshotKey) (*SpotifyMetadataResourceStatus, error) {
	if !validSnapshotKey(key) {
		return nil, errors.New("invalid metadata resource key")
	}
	var status SpotifyMetadataResourceStatus
	var checked, retry int64
	err := d.conn.QueryRow(`SELECT state,reason,checked_at,retry_at FROM spotify_metadata_resource_status WHERE entity_type=? AND spotify_id=? AND resource=? AND context_key=?`, key.EntityType, key.SpotifyID, key.Resource, key.ContextKey).Scan(&status.State, &status.Reason, &checked, &retry)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	status.CheckedAt = time.UnixMilli(checked).UTC()
	status.RetryAt = time.UnixMilli(retry).UTC()
	return &status, nil
}

// PurgeSpotifyMetadata retires private session caches without touching imported facts.
func (d *DB) PurgeSpotifyMetadata() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"spotify_download_lineage_staging", "spotify_entity_relations", "spotify_entity_snapshots", "spotify_metadata_resource_status", "spotify_audio_artifacts", "spotify_playlist_traversals", "spotify_metadata_owner"} {
		if _, err = tx.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}
	return tx.Commit()
}
