package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// PrivateCacheStatusOrphanGrace retains negative results for 30 days after both
// their check and retry deadline. This is a local retention default, not a
// provider TTL. Payload-associated status keeps the payload's lifecycle.
const PrivateCacheStatusOrphanGrace = 30 * 24 * time.Hour

// PrivateCacheRetentionResult counts payload groups (excluding cascaded rows),
// status-only rows, lease transitions, traversal deletions and orphan staging.
// Their sum is at most limit.
type PrivateCacheRetentionResult struct {
	Collected         int
	StatusCollected   int
	TraversalsExpired int
	TraversalsDeleted int
	StagingCollected  int
}

// MaintainSpotifyPrivateCache performs at most limit cleanup/lease transitions in
// the confirmed active context. Every payload gets an additional original TTL
// of stale-last-good grace. It does not impose admission quotas or touch durable
// imports, unlink choices, queue-owned staging or the durable generation allocator.
// Inactive traversal leases expire separately from actual playlist completion. Orphan statuses
// retain their check/retry grace and active-work pins.
// Pins and deletion share a writer transaction, so queue/account publication
// cannot change ownership between selection and deletion.
func (d *DB) MaintainSpotifyPrivateCache(ctx context.Context, now time.Time, limit int) (PrivateCacheRetentionResult, error) {
	var result PrivateCacheRetentionResult
	if now.IsZero() || now.UnixMilli() <= 0 || limit < 1 || limit > 128 {
		return result, errors.New("invalid private cache maintenance bounds")
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	// Acquire SQLite's writer reservation before inspecting pins. This also makes
	// a runtime reservation/account switch serialize with the entire sweep.
	if _, err = tx.ExecContext(ctx, `UPDATE settings SET value=value WHERE key='spotify_metadata_active_context'`); err != nil {
		return result, err
	}
	expired, err := tx.ExecContext(ctx, `UPDATE spotify_playlist_traversals SET lease_expired=1,lease_expired_at=? WHERE rowid IN (
 SELECT t.rowid FROM spotify_playlist_traversals t
 WHERE t.context_key<>'' AND t.context_key=COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'')
 AND t.complete=0 AND t.lease_expired=0 AND t.last_touched_at>0 AND t.last_touched_at<=?
 AND NOT EXISTS(SELECT 1 FROM spotify_download_lineage_staging s JOIN spotify_downloads d ON d.id=s.download_id
 WHERE s.context_key=t.context_key AND d.status IN ('queued','downloading','converting')
 AND ((s.origin_kind='playlist' AND s.origin_id=t.spotify_id) OR (s.origin_kind='library' AND (s.entity_id=t.spotify_id OR (s.origin_id='saved_playlists' AND s.entity_id='')))))
 ORDER BY t.last_touched_at,t.rowid LIMIT ?)`, now.UnixMilli(), now.Add(-SpotifyPlaylistTraversalLease).UnixMilli(), limit)
	if err != nil {
		return PrivateCacheRetentionResult{}, err
	}
	count, err := expired.RowsAffected()
	if err != nil {
		return PrivateCacheRetentionResult{}, err
	}
	result.TraversalsExpired = int(count)
	rows, err := tx.QueryContext(ctx, privateCacheCandidatesSQL, now.UnixMilli(), limit-result.TraversalsExpired, now.Add(-PrivateCacheStatusOrphanGrace).UnixMilli(), now.Add(-SpotifyPlaylistTraversalRetention).UnixMilli(), now.Add(-SpotifyLineageOrphanGrace).UnixMilli())
	if err != nil {
		return PrivateCacheRetentionResult{}, err
	}
	type candidate struct {
		family string
		rowid  int64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.family, &c.rowid); err != nil {
			rows.Close()
			return PrivateCacheRetentionResult{}, err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return PrivateCacheRetentionResult{}, err
	}
	for _, c := range candidates {
		if err = collectPrivateCachePayload(ctx, tx, c.family, c.rowid, now.UnixMilli()); err != nil {
			return PrivateCacheRetentionResult{}, err
		}
		switch c.family {
		case "staging":
			result.StagingCollected++
		case "traversal":
			result.TraversalsDeleted++
		case "resource_status", "analysis_status", "field_attempt":
			result.StatusCollected++
		default:
			result.Collected++
		}
	}
	if err = tx.Commit(); err != nil {
		return PrivateCacheRetentionResult{}, err
	}
	return result, nil
}

// All dynamic identifiers come from the fixed family switch, never provider input.
func collectPrivateCachePayload(ctx context.Context, tx *sql.Tx, family string, rowid, now int64) error {
	var paired string
	switch family {
	case "staging":
		_, err := tx.ExecContext(ctx, `DELETE FROM spotify_download_lineage_staging WHERE rowid=?`, rowid)
		return err
	case "traversal":
		_, err := tx.ExecContext(ctx, `DELETE FROM spotify_playlist_traversals WHERE rowid=?`, rowid)
		return err
	case "resource_status", "analysis_status", "field_attempt":
		table := map[string]string{"resource_status": "spotify_metadata_resource_status", "analysis_status": "external_track_analysis_status", "field_attempt": "spotify_audio_field_attempts"}[family]
		_, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE rowid=?", rowid)
		return err
	case "snapshot":
		paired = `DELETE FROM spotify_metadata_resource_status WHERE EXISTS(SELECT 1 FROM spotify_entity_snapshots s WHERE s.rowid=? AND s.context_key=spotify_metadata_resource_status.context_key AND s.entity_type=spotify_metadata_resource_status.entity_type AND s.spotify_id=spotify_metadata_resource_status.spotify_id AND s.resource=spotify_metadata_resource_status.resource AND spotify_metadata_resource_status.checked_at<=s.retrieved_at)`
	case "artifact":
		// Multiple independently last-good arrays can share one resource status.
		// Keep that status until every other artifact for the resource is absent.
		paired = `DELETE FROM spotify_metadata_resource_status WHERE entity_type='track' AND EXISTS(SELECT 1 FROM spotify_audio_artifacts a WHERE a.rowid=? AND a.context_key=spotify_metadata_resource_status.context_key AND a.spotify_id=spotify_metadata_resource_status.spotify_id AND a.resource=spotify_metadata_resource_status.resource AND spotify_metadata_resource_status.checked_at<=a.retrieved_at AND NOT EXISTS(SELECT 1 FROM spotify_audio_artifacts other WHERE other.context_key=a.context_key AND other.spotify_id=a.spotify_id AND other.resource=a.resource AND other.rowid<>a.rowid))`
	case "scalar":
		paired = `DELETE FROM spotify_audio_field_attempts WHERE EXISTS(SELECT 1 FROM spotify_audio_observations o WHERE o.rowid=? AND o.context_key=spotify_audio_field_attempts.context_key AND o.spotify_id=spotify_audio_field_attempts.spotify_id AND o.endpoint=spotify_audio_field_attempts.endpoint AND o.field_key=spotify_audio_field_attempts.field_key AND spotify_audio_field_attempts.checked_at<=o.retrieved_at)`
	case "analysis":
		paired = `DELETE FROM external_track_analysis_status WHERE EXISTS(SELECT 1 FROM external_track_analysis a WHERE a.rowid=? AND a.account_context=external_track_analysis_status.account_context AND a.provider=external_track_analysis_status.provider AND a.external_id=external_track_analysis_status.external_id AND a.endpoint=external_track_analysis_status.endpoint AND a.schema_version=external_track_analysis_status.schema_version AND external_track_analysis_status.checked_at<=a.retrieved_at)`
	default:
		return errors.New("unknown private cache family")
	}
	args := []any{rowid}
	// A last-good payload can predate a long cooldown. Payload expiry must not
	// shorten that retry deadline or discard in-progress resource state.
	switch family {
	case "snapshot", "artifact":
		paired += ` AND retry_at<=? AND state NOT IN ('pending','running')`
		args = append(args, now)
	case "analysis":
		paired += ` AND retry_at<=? AND code NOT IN ('pending','running')`
		args = append(args, now)
	}
	if _, err := tx.ExecContext(ctx, paired, args...); err != nil {
		return err
	}
	table := map[string]string{"snapshot": "spotify_entity_snapshots", "artifact": "spotify_audio_artifacts", "scalar": "spotify_audio_observations", "analysis": "external_track_analysis"}[family]
	_, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE rowid=?", rowid)
	return err
}

const privateCacheCandidatesSQL = `WITH RECURSIVE
 active AS (SELECT value AS context_key FROM settings WHERE key='spotify_metadata_active_context' AND value<>''),
 downloads AS (SELECT d.id,d.spotify_id,a.context_key FROM spotify_downloads d CROSS JOIN active a WHERE d.status IN ('queued','downloading','converting')),
 origins AS (SELECT s.*,d.spotify_id AS recording FROM spotify_download_lineage_staging s JOIN downloads d ON d.id=s.download_id AND d.context_key=s.context_key),
 reachable(entity_type,spotify_id,context_key) AS (
 SELECT 'track',spotify_id,context_key FROM downloads
 UNION
 SELECT r.child_type,r.child_id,p.context_key FROM reachable p
 JOIN spotify_entity_snapshots parent ON parent.entity_type=p.entity_type AND parent.spotify_id=p.spotify_id AND parent.context_key=p.context_key
 JOIN spotify_entity_relations r ON r.entity_type=parent.entity_type AND r.spotify_id=parent.spotify_id AND r.context_key=parent.context_key AND r.resource=parent.resource
 JOIN spotify_entity_snapshots child ON child.entity_type=r.child_type AND child.spotify_id=r.child_id AND child.context_key=r.context_key
 WHERE r.unavailable=0 AND r.child_type IN ('album','artist')),
 snapshot_pins AS (
 SELECT s.rowid FROM spotify_entity_snapshots s WHERE
 EXISTS(SELECT 1 FROM reachable p WHERE p.entity_type=s.entity_type AND p.spotify_id=s.spotify_id AND p.context_key=s.context_key)
 OR EXISTS(SELECT 1 FROM spotify_playlist_traversals t WHERE t.context_key=s.context_key AND t.spotify_id=s.spotify_id AND s.entity_type='playlist' AND t.complete=0 AND t.lease_expired=0)
 OR EXISTS(SELECT 1 FROM origins o WHERE o.context_key=s.context_key AND (
 (o.origin_kind='playlist' AND o.origin_revision<>'' AND s.entity_type='playlist' AND s.spotify_id=o.origin_id AND s.capture_revision=o.origin_revision)
 OR (o.origin_kind='album' AND s.entity_type='album' AND s.spotify_id=o.origin_id)
 OR (o.origin_kind='library' AND o.entity_id<>'' AND o.origin_revision='' AND s.entity_type='library' AND (
 (o.origin_id='saved_tracks' AND s.spotify_id='rest_saved_tracks' AND o.entity_id=o.recording)
 OR (o.origin_id='saved_albums' AND s.spotify_id IN ('rest_saved_albums','libraryV3_Albums'))
 OR (o.origin_id='saved_playlists' AND s.spotify_id IN ('rest_saved_playlists','libraryV3_Playlists')))
 AND EXISTS(SELECT 1 FROM spotify_entity_relations r WHERE r.entity_type=s.entity_type AND r.spotify_id=s.spotify_id AND r.context_key=s.context_key AND r.resource=s.resource AND r.relation_kind='library_items' AND r.unavailable=0 AND r.child_id=o.entity_id AND r.child_type=CASE o.origin_id WHEN 'saved_tracks' THEN 'track' WHEN 'saved_albums' THEN 'album' WHEN 'saved_playlists' THEN 'playlist' END AND (o.position<0 OR r.position=o.position)))))) ,
 status_pins(entity_type,spotify_id,context_key) AS (
 SELECT * FROM reachable
 UNION SELECT r.child_type,r.child_id,p.context_key FROM reachable p
 JOIN spotify_entity_relations r ON r.entity_type=p.entity_type AND r.spotify_id=p.spotify_id AND r.context_key=p.context_key
 WHERE r.unavailable=0 AND r.child_type IN ('album','artist')
 UNION SELECT origin_kind,origin_id,context_key FROM origins WHERE origin_kind IN ('album','playlist')
 UNION SELECT 'library',s.spotify_id,o.context_key FROM origins o JOIN spotify_metadata_resource_status s ON s.context_key=o.context_key AND s.entity_type='library' WHERE o.origin_kind='library'),
 payloads(family,rid,retrieved,expires) AS (
 SELECT 'snapshot',s.rowid,s.retrieved_at,s.expires_at FROM spotify_entity_snapshots s JOIN active a ON a.context_key=s.context_key WHERE NOT EXISTS(SELECT 1 FROM snapshot_pins p WHERE p.rowid=s.rowid)
 UNION ALL SELECT 'artifact',s.rowid,s.retrieved_at,s.expires_at FROM spotify_audio_artifacts s JOIN active a ON a.context_key=s.context_key WHERE NOT EXISTS(SELECT 1 FROM downloads d WHERE d.context_key=s.context_key AND d.spotify_id=s.spotify_id)
 UNION ALL SELECT 'scalar',s.rowid,s.retrieved_at,s.expires_at FROM spotify_audio_observations s JOIN active a ON a.context_key=s.context_key WHERE NOT EXISTS(SELECT 1 FROM downloads d WHERE d.context_key=s.context_key AND d.spotify_id=s.spotify_id)
 UNION ALL SELECT 'analysis',s.rowid,s.retrieved_at,s.expires_at FROM external_track_analysis s JOIN active a ON a.context_key=s.account_context WHERE NOT EXISTS(SELECT 1 FROM downloads d WHERE d.context_key=s.account_context AND d.spotify_id=s.external_id))
 , candidates(family,rid,expires) AS (
 SELECT family,rid,expires FROM payloads WHERE retrieved>=0 AND expires>retrieved AND expires<=?1 AND (expires-retrieved)<= (?1-expires)
 UNION ALL SELECT 'resource_status',s.rowid,MAX(s.checked_at,s.retry_at) FROM spotify_metadata_resource_status s JOIN active a ON a.context_key=s.context_key
 WHERE s.checked_at>0 AND s.retry_at>=0 AND s.checked_at<=?3 AND s.retry_at<=?3 AND s.state NOT IN ('pending','running')
 AND NOT EXISTS(SELECT 1 FROM spotify_entity_snapshots p WHERE p.context_key=s.context_key AND p.entity_type=s.entity_type AND p.spotify_id=s.spotify_id AND p.resource=s.resource)
 AND NOT EXISTS(SELECT 1 FROM spotify_audio_artifacts p WHERE s.entity_type='track' AND p.context_key=s.context_key AND p.spotify_id=s.spotify_id AND p.resource=s.resource)
 AND NOT EXISTS(SELECT 1 FROM spotify_audio_observations p WHERE s.entity_type='track' AND p.context_key=s.context_key AND p.spotify_id=s.spotify_id AND p.endpoint=s.resource)
 AND NOT EXISTS(SELECT 1 FROM external_track_analysis p WHERE s.entity_type='track' AND p.provider='spotify' AND p.account_context=s.context_key AND p.external_id=s.spotify_id AND p.endpoint=s.resource)
 AND NOT EXISTS(SELECT 1 FROM status_pins p WHERE p.context_key=s.context_key AND p.entity_type=s.entity_type AND p.spotify_id=s.spotify_id)
 AND NOT EXISTS(SELECT 1 FROM spotify_playlist_traversals t WHERE s.entity_type='playlist' AND t.context_key=s.context_key AND t.spotify_id=s.spotify_id AND t.complete=0 AND t.lease_expired=0)
 UNION ALL SELECT 'analysis_status',s.rowid,MAX(s.checked_at,s.retry_at) FROM external_track_analysis_status s JOIN active a ON a.context_key=s.account_context
 WHERE s.provider='spotify' AND s.checked_at>0 AND s.retry_at>=0 AND s.checked_at<=?3 AND s.retry_at<=?3 AND s.code NOT IN ('pending','running')
 AND NOT EXISTS(SELECT 1 FROM external_track_analysis p WHERE p.provider=s.provider AND p.account_context=s.account_context AND p.external_id=s.external_id AND p.endpoint=s.endpoint AND p.schema_version=s.schema_version)
 AND NOT EXISTS(SELECT 1 FROM spotify_audio_artifacts p WHERE p.context_key=s.account_context AND p.spotify_id=s.external_id AND p.resource=s.endpoint)
 AND NOT EXISTS(SELECT 1 FROM spotify_audio_observations p WHERE p.context_key=s.account_context AND p.spotify_id=s.external_id AND p.endpoint=s.endpoint)
 AND NOT EXISTS(SELECT 1 FROM downloads d WHERE d.context_key=s.account_context AND d.spotify_id=s.external_id)
 UNION ALL SELECT 'field_attempt',s.rowid,s.checked_at FROM spotify_audio_field_attempts s JOIN active a ON a.context_key=s.context_key
 WHERE s.checked_at>0 AND s.checked_at<=?3 AND s.state NOT IN ('pending','running')
 AND NOT EXISTS(SELECT 1 FROM spotify_audio_observations p WHERE p.context_key=s.context_key AND p.spotify_id=s.spotify_id AND p.endpoint=s.endpoint AND p.field_key=s.field_key)
 AND NOT EXISTS(SELECT 1 FROM downloads d WHERE d.context_key=s.context_key AND d.spotify_id=s.spotify_id)
 UNION ALL SELECT 'traversal',t.rowid,CASE WHEN t.complete=1 THEN t.last_touched_at ELSE t.lease_expired_at END FROM spotify_playlist_traversals t JOIN active a ON a.context_key=t.context_key
 WHERE ((t.complete=1 AND t.last_touched_at>0 AND t.last_touched_at<=?4) OR (t.complete=0 AND t.lease_expired=1 AND t.lease_expired_at>0 AND t.lease_expired_at<=?4))
 AND NOT EXISTS(SELECT 1 FROM origins o WHERE o.context_key=t.context_key AND ((o.origin_kind='playlist' AND o.origin_id=t.spotify_id) OR (o.origin_kind='library' AND (o.entity_id=t.spotify_id OR (o.origin_id='saved_playlists' AND o.entity_id='')))))
 UNION ALL SELECT 'staging',s.rowid,s.staged_at FROM spotify_download_lineage_staging s JOIN active a ON a.context_key=s.context_key
 WHERE s.staged_at>0 AND s.staged_at<=?5 AND NOT EXISTS(SELECT 1 FROM spotify_downloads d WHERE d.id=s.download_id))
 SELECT family,rid FROM candidates ORDER BY expires,family,rid LIMIT ?2`
