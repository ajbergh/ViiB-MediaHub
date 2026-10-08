package db

import (
	"context"
	"errors"
)

// SpotifyPrivateCacheUsage describes logical cache content, not SQLite file size.
// PayloadBytes excludes keys, indexes and SQLite overhead. DecodedPayloadBytes
// substitutes declared decoded artifact sizes for compressed artifact bytes.
// No provider payload, identifier or credential is returned.
type SpotifyPrivateCacheUsage struct {
	Tables              []SpotifyPrivateCacheTableUsage
	Rows                int64
	PayloadBytes        int64
	DecodedPayloadBytes int64
}

type SpotifyPrivateCacheTableUsage struct {
	Table               string
	Rows                int64
	PayloadBytes        int64
	DecodedPayloadBytes int64
}

// GetSpotifyPrivateCacheUsage inspects exactly one context, including a reserved
// or retired context. Empty context explicitly selects legacy unowned rows; it
// never means all accounts. This is a read-only diagnostic, not a cache read
// eligibility decision. One SQL statement provides a consistent snapshot and
// honors cancellation without holding a writer transaction.
func (d *DB) GetSpotifyPrivateCacheUsage(ctx context.Context, contextKey string) (SpotifyPrivateCacheUsage, error) {
	var result SpotifyPrivateCacheUsage
	if len(contextKey) > 256 {
		return result, errors.New("invalid Spotify cache context")
	}
	rows, err := d.conn.QueryContext(ctx, `WITH selected(context_key) AS (VALUES (?))
 SELECT 'spotify_entity_snapshots',COUNT(*),COALESCE(SUM(length(CAST(payload AS BLOB))),0),COALESCE(SUM(length(CAST(payload AS BLOB))),0) FROM spotify_entity_snapshots WHERE context_key=(SELECT context_key FROM selected)
 UNION ALL SELECT 'spotify_entity_relations',COUNT(*),COALESCE(SUM(length(CAST(metadata_json AS BLOB))),0),COALESCE(SUM(length(CAST(metadata_json AS BLOB))),0) FROM spotify_entity_relations WHERE context_key=(SELECT context_key FROM selected)
 UNION ALL SELECT 'spotify_audio_artifacts',COUNT(*),COALESCE(SUM(length(CAST(payload AS BLOB))),0),COALESCE(SUM(decoded_size),0) FROM spotify_audio_artifacts WHERE context_key=(SELECT context_key FROM selected)
 UNION ALL SELECT 'spotify_audio_observations',COUNT(*),COALESCE(SUM(length(CAST(value_json AS BLOB))),0),COALESCE(SUM(length(CAST(value_json AS BLOB))),0) FROM spotify_audio_observations WHERE context_key=(SELECT context_key FROM selected)
 UNION ALL SELECT 'spotify_audio_field_attempts',COUNT(*),0,0 FROM spotify_audio_field_attempts WHERE context_key=(SELECT context_key FROM selected)
 UNION ALL SELECT 'external_track_analysis',COUNT(*),COALESCE(SUM(length(CAST(observation_json AS BLOB))),0),COALESCE(SUM(length(CAST(observation_json AS BLOB))),0) FROM external_track_analysis WHERE account_context=(SELECT context_key FROM selected)
 UNION ALL SELECT 'external_track_analysis_status',COUNT(*),0,0 FROM external_track_analysis_status WHERE account_context=(SELECT context_key FROM selected)
 UNION ALL SELECT 'spotify_metadata_resource_status',COUNT(*),0,0 FROM spotify_metadata_resource_status WHERE context_key=(SELECT context_key FROM selected)
 UNION ALL SELECT 'spotify_playlist_traversals',COUNT(*),0,0 FROM spotify_playlist_traversals WHERE context_key=(SELECT context_key FROM selected)
 UNION ALL SELECT 'spotify_download_lineage_staging',COUNT(*),0,0 FROM spotify_download_lineage_staging WHERE context_key=(SELECT context_key FROM selected)`, contextKey)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var table SpotifyPrivateCacheTableUsage
		if err := rows.Scan(&table.Table, &table.Rows, &table.PayloadBytes, &table.DecodedPayloadBytes); err != nil {
			return SpotifyPrivateCacheUsage{}, err
		}
		result.Tables = append(result.Tables, table)
		result.Rows += table.Rows
		result.PayloadBytes += table.PayloadBytes
		result.DecodedPayloadBytes += table.DecodedPayloadBytes
	}
	if err := rows.Err(); err != nil {
		return SpotifyPrivateCacheUsage{}, err
	}
	return result, nil
}
