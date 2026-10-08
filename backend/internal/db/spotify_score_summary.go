package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
)

type SpotifyScoreSummary struct {
	Value       float64   `json:"value"`
	Stale       bool      `json:"stale"`
	RetrievedAt time.Time `json:"retrievedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Endpoint    string    `json:"endpoint"`
}

type importedSpotifyScoreCandidate struct {
	songID, fingerprint, recording string
	revision                       downloadFileRevision
	fileHash                       string
	field                          SpotifyScalarField
}

// One private read transaction for linked, current-source library score summaries.
// No provider requests or waveform/detailed payloads are involved.
func (d *DB) GetSpotifyScalarCandidateBatchForRuntime(fence SpotifyMetadataReadFence, sources map[string]string, now time.Time) (map[string][]SpotifyScalarField, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if fence.Pending {
		err = checkSpotifyMetadataReadFenceTx(tx, fence)
	} else {
		err = checkSpotifyMetadataFenceTx(tx, SpotifyMetadataFence{Epoch: fence.Epoch, ContextKey: fence.ContextKey})
	}
	if err != nil {
		return nil, err
	}
	// Library and mixing scores are effective evidence, unlike the explicitly
	// unverified candidate inspection surfaces. Admit neither copy until ownership
	// is confirmed, while still validating the pending read fence above.
	if fence.Pending {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return map[string][]SpotifyScalarField{}, nil
	}
	rows, err := tx.Query(`SELECT i.song_id,i.source_fingerprint,i.external_id,o.field_key,o.metric,o.units,o.value_json,o.confidence,o.endpoint,o.schema_version,o.adapter_revision,o.retrieved_at,o.expires_at FROM track_external_identity i JOIN spotify_audio_observations o ON o.spotify_id=i.external_id WHERE i.provider='spotify' AND o.context_key=? AND o.schema_version=1 AND NOT EXISTS(SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=i.song_id AND x.source_fingerprint=i.source_fingerprint) ORDER BY i.song_id,o.field_key,o.endpoint`, fence.ContextKey)
	if err != nil {
		return nil, err
	}
	result := map[string][]SpotifyScalarField{}
	count := 0
	for rows.Next() {
		count++
		if count > 100000 {
			rows.Close()
			return nil, errors.New("scalar batch row limit exceeded")
		}
		var song, fp, raw string
		var f SpotifyScalarField
		var confidence sql.NullFloat64
		var retrieved, expires int64
		if err := rows.Scan(&song, &fp, &f.RecordingID, &f.Key, &f.Metric, &f.Units, &raw, &confidence, &f.Endpoint, &f.SchemaVersion, &f.AdapterRevision, &retrieved, &expires); err != nil {
			rows.Close()
			return nil, err
		}
		if fp == "" || sources[song] != fp {
			continue
		}
		f.Value = json.RawMessage(raw)
		f.RetrievedAt = time.UnixMilli(retrieved).UTC()
		f.ExpiresAt = time.UnixMilli(expires).UTC()
		f.Stale = !now.Before(f.ExpiresAt)
		if confidence.Valid {
			value := confidence.Float64
			f.Confidence = &value
		}
		if !validProviderScalarCandidate(f) {
			continue
		}
		if len(result[song]) >= 64 {
			rows.Close()
			return nil, errors.New("scalar batch song limit exceeded")
		}
		result[song] = append(result[song], f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	durable, err := d.GetDownloadedSpotifyScalarCandidateBatch(sources, now)
	if err != nil {
		return nil, err
	}
	for song, fields := range durable {
		if len(result[song])+len(fields) > 128 {
			return nil, errors.New("scalar batch merged limit exceeded")
		}
		result[song] = append(result[song], fields...)
	}
	finalTx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	err = checkSpotifyMetadataFenceTx(finalTx, SpotifyMetadataFence{Epoch: fence.Epoch, ContextKey: fence.ContextKey})
	finalTx.Rollback()
	if err != nil {
		return nil, err
	}
	return d.RevalidateSpotifyScalarCandidateBatch(sources, result)
}

func (d *DB) GetSpotifyScoreSummariesForRuntime(fence SpotifyMetadataReadFence, sources map[string]string, now time.Time) (map[string]map[string]SpotifyScoreSummary, error) {
	fields, err := d.GetSpotifyScalarCandidateBatchForRuntime(fence, sources, now)
	if err != nil {
		return nil, err
	}
	return SpotifyScoreSummariesFromFields(fields), nil
}

// GetDownloadedSpotifyScoreSummaries returns imported score candidates for
// exact current source revisions. Pending-owner reads must not call this; the
// API intentionally admits durable imports only after owner confirmation.
func (d *DB) GetDownloadedSpotifyScoreSummaries(sources map[string]string, now time.Time) (map[string]map[string]SpotifyScoreSummary, error) {
	fields, err := d.GetDownloadedSpotifyScalarCandidateBatch(sources, now)
	if err != nil {
		return nil, err
	}
	return SpotifyScoreSummariesFromFields(fields), nil
}

func (d *DB) GetDownloadedSpotifyScalarCandidateBatch(sources map[string]string, now time.Time) (map[string][]SpotifyScalarField, error) {
	result := map[string][]SpotifyScalarField{}
	ids := make([]string, 0, len(sources))
	for songID, fingerprint := range sources {
		if songID != "" && fingerprint != "" {
			ids = append(ids, songID)
		}
	}
	sort.Strings(ids)
	const batchSize = 400
	candidates := map[string][]importedSpotifyScoreCandidate{}
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := make([]byte, 0, (end-start)*2)
		args := make([]any, 0, end-start)
		for index, songID := range ids[start:end] {
			if index > 0 {
				placeholders = append(placeholders, ',')
			}
			placeholders = append(placeholders, '?')
			args = append(args, songID)
		}
		query := `SELECT b.song_id,b.source_fingerprint,b.spotify_id,b.file_path,b.content_sha256,b.file_size,b.mtime_ns,COALESCE(s.file_hash,''),
 i.field_key,i.metric,i.units,i.value_json,i.confidence,i.resource,i.schema_version,i.adapter_revision,i.retrieved_at,i.expires_at
 FROM spotify_download_import_bindings b JOIN songs s ON s.id=b.song_id AND s.file_path=b.file_path
 JOIN track_external_identity l ON l.song_id=b.song_id AND l.provider='spotify' AND l.external_id=b.spotify_id AND l.source_fingerprint=b.source_fingerprint
 JOIN spotify_download_scalar_imports i ON i.file_path=b.file_path AND i.content_sha256=b.content_sha256 AND i.file_size=b.file_size AND i.mtime_ns=b.mtime_ns AND i.spotify_id=b.spotify_id
 WHERE b.song_id IN (` + string(placeholders) + `)
 AND i.schema_version=1 AND NOT EXISTS(SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=b.song_id AND x.source_fingerprint=b.source_fingerprint)
 ORDER BY b.song_id,i.field_key,i.resource`
		rows, err := d.conn.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var item importedSpotifyScoreCandidate
			var raw string
			var confidence sql.NullFloat64
			var retrieved, expires int64
			if err := rows.Scan(&item.songID, &item.fingerprint, &item.recording, &item.revision.path, &item.revision.digest, &item.revision.size, &item.revision.mtime, &item.fileHash,
				&item.field.Key, &item.field.Metric, &item.field.Units, &raw, &confidence, &item.field.Endpoint, &item.field.SchemaVersion, &item.field.AdapterRevision, &retrieved, &expires); err != nil {
				rows.Close()
				return nil, err
			}
			if sources[item.songID] != item.fingerprint {
				continue
			}
			if len(candidates[item.songID]) >= 64 {
				rows.Close()
				return nil, errors.New("downloaded score summary limit exceeded")
			}
			if !json.Valid([]byte(raw)) || strings.TrimSpace(raw) == "null" {
				continue
			}
			item.field.Value = json.RawMessage(raw)
			if confidence.Valid {
				value := confidence.Float64
				item.field.Confidence = &value
			}
			item.field.RetrievedAt = time.UnixMilli(retrieved).UTC()
			item.field.ExpiresAt = time.UnixMilli(expires).UTC()
			item.field.Stale = !now.Before(item.field.ExpiresAt)
			item.field.DurableImport = true
			item.field.RecordingID = item.recording
			candidates[item.songID] = append(candidates[item.songID], item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	for songID, items := range candidates {
		if len(items) == 0 {
			continue
		}
		expected := items[0].revision
		if info, err := os.Lstat(expected.path); err != nil || !info.Mode().IsRegular() || info.Size() != expected.size || info.ModTime().UnixNano() != expected.mtime || LocalSourceFingerprint(Song{FilePath: expected.path, FileHash: items[0].fileHash}, info) != sources[songID] {
			continue
		}
		actual, err := readDownloadRevision(context.Background(), expected.path)
		if err != nil || actual != expected {
			continue
		}
		var admitted int
		if err := d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_import_bindings b JOIN songs s ON s.id=b.song_id AND s.file_path=b.file_path JOIN track_external_identity l ON l.song_id=b.song_id AND l.provider='spotify' AND l.external_id=b.spotify_id AND l.source_fingerprint=b.source_fingerprint WHERE b.song_id=? AND b.source_fingerprint=? AND b.spotify_id=? AND b.file_path=? AND b.content_sha256=? AND b.file_size=? AND b.mtime_ns=? AND COALESCE(s.file_hash,'')=? AND NOT EXISTS(SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=b.song_id AND x.source_fingerprint=b.source_fingerprint)`, songID, sources[songID], items[0].recording, expected.path, expected.digest, expected.size, expected.mtime, items[0].fileHash).Scan(&admitted); err != nil {
			continue
		}
		if admitted != 1 {
			continue
		}
		for _, item := range items {
			if item.revision != expected || item.fingerprint != sources[songID] || item.recording != items[0].recording {
				continue
			}
			if validProviderScalarCandidate(item.field) {
				result[songID] = append(result[songID], item.field)
			}
		}
	}
	return d.RevalidateSpotifyScalarCandidateBatch(sources, result)
}

func betterSpotifyScoreSummary(candidate, previous SpotifyScoreSummary) bool {
	return betterProviderScalarCandidate(
		SpotifyScalarField{Stale: candidate.Stale, RetrievedAt: candidate.RetrievedAt, Endpoint: candidate.Endpoint},
		SpotifyScalarField{Stale: previous.Stale, RetrievedAt: previous.RetrievedAt, Endpoint: previous.Endpoint})
}

func isSpotifyScoreField(key string) bool {
	switch key {
	case "spotify_energy_score", "spotify_danceability_score", "spotify_acousticness_score", "spotify_instrumentalness_score", "spotify_liveness_score", "spotify_speechiness_score", "spotify_valence_score":
		return true
	default:
		return false
	}
}

func expectedSpotifyScoreMetric(key string) string {
	if !isSpotifyScoreField(key) {
		return ""
	}
	return "spotify_" + key[len("spotify_"):len(key)-len("_score")]
}

// SpotifyScoreSummariesFromFields preserves the score compatibility projection.
func SpotifyScoreSummariesFromFields(fields map[string][]SpotifyScalarField) map[string]map[string]SpotifyScoreSummary {
	result := map[string]map[string]SpotifyScoreSummary{}
	for song, candidates := range fields {
		for _, f := range SelectProviderScalarFields(candidates) {
			if !isSpotifyScoreField(f.Key) {
				continue
			}
			var value float64
			if json.Unmarshal(f.Value, &value) != nil {
				continue
			}
			if result[song] == nil {
				result[song] = map[string]SpotifyScoreSummary{}
			}
			result[song][f.Key] = SpotifyScoreSummary{Value: value, Stale: f.Stale, RetrievedAt: f.RetrievedAt, ExpiresAt: f.ExpiresAt, Endpoint: f.Endpoint}
		}
	}
	return result
}

// RevalidateSpotifyScalarCandidateBatch rejects captured observations after
// unlink, suppression or same-source relink. Candidate origin is never inferred
// from the current link, which could refer to a different recording.
func (d *DB) RevalidateSpotifyScalarCandidateBatch(sources map[string]string, candidates map[string][]SpotifyScalarField) (map[string][]SpotifyScalarField, error) {
	result := map[string][]SpotifyScalarField{}
	ids := make([]string, 0, len(candidates))
	for song := range candidates {
		ids = append(ids, song)
	}
	sort.Strings(ids)
	for start := 0; start < len(ids); start += 400 {
		end := start + 400
		if end > len(ids) {
			end = len(ids)
		}
		args := make([]any, 0, end-start)
		marks := make([]string, 0, end-start)
		for _, id := range ids[start:end] {
			args = append(args, id)
			marks = append(marks, "?")
		}
		rows, err := d.conn.Query(`SELECT i.song_id,i.source_fingerprint,i.external_id FROM track_external_identity i WHERE i.provider='spotify' AND i.song_id IN (`+strings.Join(marks, ",")+`) AND NOT EXISTS(SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=i.song_id AND x.source_fingerprint=i.source_fingerprint)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var song, fp, recording string
			if err := rows.Scan(&song, &fp, &recording); err != nil {
				rows.Close()
				return nil, err
			}
			if fp == "" || sources[song] != fp {
				continue
			}
			for _, field := range candidates[song] {
				if field.RecordingID == recording && ValidSpotifyRecordingID(recording) {
					result[song] = append(result[song], field)
				}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
