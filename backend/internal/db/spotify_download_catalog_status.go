package db

import (
	"context"
	"database/sql"
	"time"
)

type DownloadedCatalogImportStatus struct {
	State     string    `json:"state"`
	Reason    string    `json:"reason"`
	CheckedAt time.Time `json:"checkedAt"`
	Scope     string    `json:"scope"`
}

// Records the completion-time material available for the reachable album/artist
// graph scope, independently of audio and immutable last-good imports.
func recordDownloadedCatalogOutcome(ctx context.Context, tx *sql.Tx, revision downloadFileRevision, recording string, now int64) error {
	var count, bytes, roots, relationCount, relationBytes, missing, retainedCount, retainedBytes int64
	err := tx.QueryRowContext(ctx, downloadedCatalogGraphCTE+`,
 retained AS (SELECT * FROM spotify_download_catalog_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?),
 new_material AS (SELECT e.* FROM eligible e WHERE NOT EXISTS(SELECT 1 FROM retained r WHERE r.entity_type=e.entity_type AND r.spotify_id=e.spotify_id AND r.resource=e.resource)),
 retained_relations AS (SELECT * FROM spotify_download_catalog_relations WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?)
, new_relations AS (SELECT e.* FROM relations e WHERE NOT EXISTS(SELECT 1 FROM retained_relations r WHERE r.parent_type=e.entity_type AND r.parent_id=e.spotify_id AND r.resource=e.resource AND r.relation_kind=e.relation_kind AND r.position=e.position))
 SELECT (SELECT COUNT(*) FROM eligible),(SELECT COALESCE(SUM(length(payload)),0) FROM eligible),
 (SELECT COUNT(*) FROM eligible WHERE entity_type='track' AND spotify_id=?),
 (SELECT COUNT(*) FROM retained_relations)+(SELECT COUNT(*) FROM new_relations),(SELECT COALESCE(SUM(length(metadata_json)),0) FROM retained_relations)+(SELECT COALESCE(SUM(length(metadata_json)),0) FROM new_relations),
 (SELECT COUNT(*) FROM relations r WHERE r.unavailable=0 AND r.child_type IN ('album','artist')
 AND NOT EXISTS(SELECT 1 FROM eligible s WHERE s.entity_type=r.child_type AND s.spotify_id=r.child_id)),
 (SELECT COUNT(*) FROM retained)+(SELECT COUNT(*) FROM new_material),
 (SELECT COALESCE(SUM(length(payload)),0) FROM retained)+(SELECT COALESCE(SUM(length(payload)),0) FROM new_material)`,
		now, recording, revision.path, revision.digest, revision.size, revision.mtime, recording, revision.path, revision.digest, revision.size, revision.mtime, recording, recording).Scan(&count, &bytes, &roots, &relationCount, &relationBytes, &missing, &retainedCount, &retainedBytes)
	if err != nil {
		return err
	}
	state, reason := "available", "track_album_artist_graph_retained"
	switch {
	case count > 256 || bytes > 8<<20 || retainedCount > 256 || retainedBytes > 8<<20:
		state, reason = "oversized", "snapshot_limit"
	case relationCount > 20000 || relationBytes > 2<<20:
		state, reason = "oversized", "relation_limit"
	case roots == 0:
		var present int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM spotify_entity_snapshots WHERE entity_type='track' AND spotify_id=? AND context_key=(SELECT value FROM settings WHERE key='spotify_metadata_active_context')`, recording).Scan(&present); err != nil {
			return err
		}
		if present > 0 {
			state, reason = "incomplete", "root_track_unavailable"
		} else {
			state, reason = "not_available", "no_eligible_catalog"
		}
	case missing > 0:
		state, reason = "incomplete", "related_entity_unavailable"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO spotify_download_catalog_import_status
 (file_path,content_sha256,file_size,mtime_ns,recording_id,state,reason,checked_at,scope) VALUES(?,?,?,?,?,?,?,?,?)
 ON CONFLICT(file_path,content_sha256,file_size,mtime_ns,recording_id) DO UPDATE SET state=excluded.state,reason=excluded.reason,checked_at=excluded.checked_at,scope=excluded.scope`, revision.path, revision.digest, revision.size, revision.mtime, recording, state, reason, now, downloadedCatalogGraphScope)
	return err
}
