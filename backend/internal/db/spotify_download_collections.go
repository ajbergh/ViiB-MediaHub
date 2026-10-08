package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/metadata"
)

const requestedCollectionScope = "requested_collection_observations_v1"

type requestedCatalogPage struct {
	snapshot           SpotifyEntitySnapshot
	resource           string
	retrieved, expires int64
}

// Collection promotion is selective: playlist pages must carry the requested
// revision; saved-library pages must contain the explicitly requested entity.
// It never fetches providers or traverses unrelated collection children.
func requestedCollectionPages(ctx context.Context, tx *sql.Tx, o SpotifyDownloadOrigin, recording string, now int64) ([]requestedCatalogPage, bool, string, error) {
	var query string
	var args []any
	if o.Kind == "playlist" {
		if o.Revision == "" {
			return nil, false, "", nil
		}
		query = `SELECT entity_type,spotify_id,resource,context_key,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,capture_revision FROM spotify_entity_snapshots s
 WHERE s.entity_type='playlist' AND s.spotify_id=? AND s.capture_revision=? AND s.resource!='playlist_traversal_partial_v1'`
		args = []any{o.ID, o.Revision}
	} else {
		if o.EntityID == "" || o.Revision != "" {
			return nil, false, "", nil
		}
		scopes := map[string][2]string{"saved_tracks": {"rest_saved_tracks", "rest_saved_tracks"}, "saved_albums": {"rest_saved_albums", "libraryV3_Albums"}, "saved_playlists": {"rest_saved_playlists", "libraryV3_Playlists"}}
		types := map[string]string{"saved_tracks": "track", "saved_albums": "album", "saved_playlists": "playlist"}
		if o.ID == "saved_tracks" && o.EntityID != recording {
			return nil, false, "", nil
		}
		scope, ok := scopes[o.ID]
		if !ok {
			return nil, false, "", nil
		}
		query = `SELECT entity_type,spotify_id,resource,context_key,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,capture_revision FROM spotify_entity_snapshots s
 WHERE s.entity_type='library' AND s.spotify_id IN (?,?) AND EXISTS(SELECT 1 FROM spotify_entity_relations r WHERE r.entity_type=s.entity_type AND r.spotify_id=s.spotify_id AND r.resource=s.resource AND r.context_key=s.context_key AND r.relation_kind='library_items' AND r.unavailable=0 AND r.child_type=? AND r.child_id=? AND (?<0 OR r.position=?))`
		args = []any{scope[0], scope[1], types[o.ID], o.EntityID, o.Position, o.Position}
	}
	query += ` AND s.context_key=COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'') AND s.context_key<>'' AND s.schema_version=1 AND s.expires_at>? ORDER BY s.entity_type,s.spotify_id,s.resource LIMIT 257`
	args = append(args, now)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, "", err
	}
	pages := []requestedCatalogPage{}
	size := 0
	complete := true
	for rows.Next() {
		var p requestedCatalogPage
		s := &p.snapshot
		if err = rows.Scan(&s.EntityType, &s.SpotifyID, &s.Resource, &s.ContextKey, &s.SchemaVersion, &s.AdapterRevision, &s.Payload, &s.PayloadHash, &p.retrieved, &p.expires, &s.CaptureRevision); err != nil {
			rows.Close()
			return nil, false, "", err
		}
		size += len(s.Payload)
		if len(pages) >= 256 || size > 8<<20 {
			rows.Close()
			return nil, false, "snapshot_limit", nil
		}
		safe, e := metadata.Sanitize(s.Payload, metadata.CatalogLimit)
		hash := sha256.Sum256(s.Payload)
		if e != nil || !bytes.Equal(safe, s.Payload) || hex.EncodeToString(hash[:]) != s.PayloadHash || !validSnapshotKey(s.SpotifySnapshotKey) || !validSnapshotCaptureRevision(*s) || s.AdapterRevision == "" || len(s.AdapterRevision) > 256 || p.retrieved < 0 || p.expires <= p.retrieved {
			complete = false
			continue
		}
		pages = append(pages, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, "", err
	}
	relationCount, relationBytes := 0, 0
	for i := range pages {
		s := &pages[i].snapshot
		rows, err = tx.QueryContext(ctx, `SELECT relation_kind,position,child_type,child_id,unavailable,metadata_json FROM spotify_entity_relations WHERE entity_type=? AND spotify_id=? AND resource=? AND context_key=? ORDER BY relation_kind,position LIMIT 20001`, s.EntityType, s.SpotifyID, s.Resource, s.ContextKey)
		if err != nil {
			return nil, false, "", err
		}
		for rows.Next() {
			var r SpotifyEntityRelation
			if err = rows.Scan(&r.Kind, &r.Position, &r.ChildType, &r.ChildID, &r.Unavailable, &r.Metadata); err != nil {
				rows.Close()
				return nil, false, "", err
			}
			relationCount++
			relationBytes += len(r.Metadata)
			if relationCount > 20000 || relationBytes > 2<<20 {
				rows.Close()
				return nil, false, "relation_limit", nil
			}
			safe, e := metadata.Sanitize(r.Metadata, metadata.ScalarLimit)
			if e != nil || !bytes.Equal(safe, r.Metadata) || r.Kind == "" || len(r.Kind) > 128 || r.Position < 0 || len(r.ChildType) > 128 || (r.ChildID != "" && !ValidSpotifyRecordingID(r.ChildID)) {
				complete = false
				continue
			}
			s.Relations = append(s.Relations, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, false, "", err
		}
		// Original capture identity remains explicit, while the bounded resource key
		// distinguishes immutable observations across revisions and retrievals.
		pages[i].resource, err = requestedObservationResource(*s, pages[i].retrieved, pages[i].expires)
		if err != nil {
			return nil, false, "", err
		}
	}
	if len(pages) == 0 {
		return pages, false, "", nil
	}
	if o.Kind == "playlist" {
		count := -1
		observed := map[int]string{}
		expected := []string(nil)
		observedRows := map[int]SpotifyEntityRelation{}
		matched := false
		for _, p := range pages {
			if p.snapshot.Resource == "playlist_traversal_complete_v1" {
				var checkpoint struct {
					Revision string            `json:"revision"`
					Complete bool              `json:"complete"`
					RowCount *int              `json:"rowCount"`
					Items    []json.RawMessage `json:"items"`
				}
				if json.Unmarshal(p.snapshot.Payload, &checkpoint) == nil && checkpoint.Complete && checkpoint.Revision == o.Revision && checkpoint.RowCount != nil && checkpoint.Items != nil && *checkpoint.RowCount == len(checkpoint.Items) && *checkpoint.RowCount <= 20000 {
					count = *checkpoint.RowCount
					expected = make([]string, count)
					for position, raw := range checkpoint.Items {
						var row struct {
							Track *struct {
								ID string `json:"id"`
							} `json:"track"`
						}
						if json.Unmarshal(raw, &row) != nil {
							complete = false
							continue
						}
						if row.Track != nil && ValidSpotifyRecordingID(row.Track.ID) {
							expected[position] = row.Track.ID
						}
					}
				}
				continue
			}
			for _, r := range p.snapshot.Relations {
				if r.Kind != "tracks" && r.Kind != "playlist_items" {
					continue
				}
				signature := r.ChildType + ":" + r.ChildID
				if r.Unavailable {
					signature += "!"
				}
				if old, ok := observed[r.Position]; ok && old != signature {
					complete = false
				}
				observed[r.Position] = signature
				observedRows[r.Position] = r
				if !r.Unavailable && r.ChildType == "track" && r.ChildID == recording && (o.Position < 0 || r.Position == o.Position) {
					matched = true
				}
			}
		}
		if count < 0 || !matched || len(observed) != count {
			complete = false
		}
		for position := 0; position < count; position++ {
			row, ok := observedRows[position]
			if !ok {
				complete = false
				continue
			}
			if expected[position] == "" {
				if !row.Unavailable {
					complete = false
				}
			} else if row.Unavailable || row.ChildType != "track" || row.ChildID != expected[position] {
				complete = false
			}
		}
	}
	return pages, complete, "", nil
}

func requestedObservationResource(s SpotifyEntitySnapshot, retrieved, expires int64) (string, error) {
	digest := sha256.New()
	err := json.NewEncoder(digest).Encode(struct {
		Type, ID, Resource, Revision, Hash string
		Retrieved, Expires                 int64
		Relations                          []SpotifyEntityRelation
	}{s.EntityType, s.SpotifyID, s.Resource, s.CaptureRevision, s.PayloadHash, retrieved, expires, s.Relations})
	if err != nil {
		return "", err
	}
	for _, r := range s.Relations {
		digest.Write(r.Metadata)
		digest.Write([]byte{0})
	}
	return "request:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func promoteDownloadCollections(ctx context.Context, tx *sql.Tx, downloadID string, file downloadFileRevision, recording string, now int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT origin_kind,origin_id,origin_revision,position,entity_id FROM spotify_download_lineage_staging WHERE download_id=? AND context_key=COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'') AND context_key<>'' AND origin_kind IN ('playlist','library') ORDER BY origin_kind,origin_id,origin_revision,position,entity_id LIMIT 33`, downloadID)
	if err != nil {
		return err
	}
	origins := []SpotifyDownloadOrigin{}
	for rows.Next() {
		var o SpotifyDownloadOrigin
		if err = rows.Scan(&o.Kind, &o.ID, &o.Revision, &o.Position, &o.EntityID); err != nil {
			rows.Close()
			return err
		}
		if !validDownloadOrigin(o) {
			rows.Close()
			return errors.New("invalid staged collection origin")
		}
		origins = append(origins, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	state, reason := "available", "requested_observations_retained"
	if len(origins) == 0 {
		state, reason = "not_available", "no_requested_collections"
	}
	if len(origins) > 32 {
		state, reason = "oversized", "snapshot_limit"
	}
	var originCount int
	if err = tx.QueryRowContext(ctx, `WITH combined AS (
 SELECT origin_kind,origin_id,origin_revision,position,entity_id FROM spotify_download_lineage_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?
 UNION SELECT origin_kind,origin_id,origin_revision,position,entity_id FROM spotify_download_lineage_staging WHERE download_id=? AND context_key=COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'') AND context_key<>'') SELECT COUNT(*) FROM combined`, file.path, file.digest, file.size, file.mtime, recording, downloadID).Scan(&originCount); err != nil {
		return err
	}
	if len(origins) > 0 && originCount > 64 {
		state, reason = "oversized", "origin_limit"
	}
	candidates := map[string]requestedCatalogPage{}
	candidateBytes, candidateRelations, candidateRelationBytes := 0, 0, 0
	for _, o := range origins {
		if state == "oversized" {
			break
		}
		pages, complete, limit, e := requestedCollectionPages(ctx, tx, o, recording, now)
		if e != nil {
			return e
		}
		if limit != "" {
			state, reason = "oversized", limit
			break
		}
		if !complete {
			state, reason = "incomplete", "requested_collection_unavailable"
		}
		for _, p := range pages {
			if _, ok := candidates[p.resource]; ok {
				continue
			}
			candidates[p.resource] = p
			candidateBytes += len(p.snapshot.Payload)
			candidateRelations += len(p.snapshot.Relations)
			for _, r := range p.snapshot.Relations {
				candidateRelationBytes += len(r.Metadata)
			}
			if len(candidates) > 256 || candidateBytes > 8<<20 {
				state, reason = "oversized", "snapshot_limit"
				break
			}
			if candidateRelations > 20000 || candidateRelationBytes > 2<<20 {
				state, reason = "oversized", "relation_limit"
				break
			}
		}
	}
	args := []any{file.path, file.digest, file.size, file.mtime, recording}
	var count, size, relations, relationBytes int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(length(payload)),0) FROM spotify_download_catalog_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?`, args...).Scan(&count, &size); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(length(metadata_json)),0) FROM spotify_download_catalog_relations WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?`, args...).Scan(&relations, &relationBytes); err != nil {
		return err
	}
	fresh := []requestedCatalogPage{}
	if state != "oversized" {
		for _, p := range candidates {
			var exists int
			lookup := append(append([]any{}, args...), p.snapshot.EntityType, p.snapshot.SpotifyID, p.resource)
			if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM spotify_download_catalog_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=? AND entity_type=? AND spotify_id=? AND resource=?`, lookup...).Scan(&exists); err != nil {
				return err
			}
			if exists != 0 {
				continue
			}
			fresh = append(fresh, p)
			count++
			size += int64(len(p.snapshot.Payload))
			relations += int64(len(p.snapshot.Relations))
			for _, r := range p.snapshot.Relations {
				relationBytes += int64(len(r.Metadata))
			}
		}
		if count > 256 || size > 8<<20 {
			state, reason = "oversized", "snapshot_limit"
		} else if relations > 20000 || relationBytes > 2<<20 {
			state, reason = "oversized", "relation_limit"
		}
	}
	if state != "oversized" {
		for _, p := range fresh {
			s := p.snapshot
			insert := append(append([]any{}, args...), s.EntityType, s.SpotifyID, p.resource, s.SchemaVersion, s.AdapterRevision, s.Payload, s.PayloadHash, p.retrieved, p.expires, s.Resource, s.CaptureRevision)
			if _, err = tx.ExecContext(ctx, `INSERT INTO spotify_download_catalog_imports(file_path,content_sha256,file_size,mtime_ns,recording_id,entity_type,spotify_id,resource,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,captured_resource,capture_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, insert...); err != nil {
				return err
			}
			for _, r := range s.Relations {
				insert = append(append([]any{}, args...), s.EntityType, s.SpotifyID, p.resource, r.Kind, r.Position, r.ChildType, r.ChildID, r.Unavailable, r.Metadata)
				if _, err = tx.ExecContext(ctx, `INSERT INTO spotify_download_catalog_relations(file_path,content_sha256,file_size,mtime_ns,recording_id,parent_type,parent_id,resource,relation_kind,position,child_type,child_id,unavailable,metadata_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, insert...); err != nil {
					return err
				}
			}
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO spotify_download_collection_status(file_path,content_sha256,file_size,mtime_ns,recording_id,state,reason,checked_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(file_path,content_sha256,file_size,mtime_ns,recording_id) DO UPDATE SET state=excluded.state,reason=excluded.reason,checked_at=excluded.checked_at`, file.path, file.digest, file.size, file.mtime, recording, state, reason, now)
	return err
}

func readDownloadCollectionStatus(ctx context.Context, conn *sql.DB, file downloadFileRevision, recording string, result *DownloadedSpotifyCatalog) error {
	var status DownloadedCatalogImportStatus
	var checked int64
	err := conn.QueryRowContext(ctx, `SELECT state,reason,checked_at FROM spotify_download_collection_status WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?`, file.path, file.digest, file.size, file.mtime, recording).Scan(&status.State, &status.Reason, &checked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	valid := map[string]string{"requested_observations_retained": "available", "requested_collection_unavailable": "incomplete", "origin_limit": "oversized", "snapshot_limit": "oversized", "relation_limit": "oversized", "no_requested_collections": "not_available"}
	if valid[status.Reason] != status.State || checked < 0 {
		return errors.New("invalid requested collection status")
	}
	status.CheckedAt = time.UnixMilli(checked).UTC()
	status.Scope = requestedCollectionScope
	result.CollectionStatus = &status
	return nil
}
