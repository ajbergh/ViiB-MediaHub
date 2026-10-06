package db

import (
	"encoding/json"
	"errors"
	"math"
	"time"
)

type SpotifyScoreSummary struct {
	Value       float64   `json:"value"`
	Stale       bool      `json:"stale"`
	RetrievedAt time.Time `json:"retrievedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Endpoint    string    `json:"endpoint"`
}

// One private read transaction for linked, current-source library score summaries.
// No provider requests or waveform/detailed payloads are involved.
func (d *DB) GetSpotifyScoreSummariesForRuntime(fence SpotifyMetadataReadFence, sources map[string]string, now time.Time) (map[string]map[string]SpotifyScoreSummary, error) {
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
	rows, err := tx.Query(`SELECT i.song_id,i.source_fingerprint,o.field_key,o.value_json,o.endpoint,o.retrieved_at,o.expires_at FROM track_external_identity i JOIN spotify_audio_observations o ON o.spotify_id=i.external_id WHERE i.provider='spotify' AND o.context_key=? AND o.units='unit_interval' AND o.schema_version=1 AND o.field_key IN ('spotify_energy_score','spotify_danceability_score','spotify_acousticness_score','spotify_instrumentalness_score','spotify_liveness_score','spotify_speechiness_score','spotify_valence_score') ORDER BY i.song_id,o.field_key,o.endpoint`, fence.ContextKey)
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]SpotifyScoreSummary{}
	count := 0
	for rows.Next() {
		count++
		if count > 100000 {
			rows.Close()
			return nil, errors.New("score summary row limit exceeded")
		}
		var song, fp, key, raw, endpoint string
		var retrieved, expires int64
		if err := rows.Scan(&song, &fp, &key, &raw, &endpoint, &retrieved, &expires); err != nil {
			rows.Close()
			return nil, err
		}
		if fp == "" || sources[song] != fp {
			continue
		}
		var value float64
		if json.Unmarshal([]byte(raw), &value) != nil || raw == "null" || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			continue
		}
		candidate := SpotifyScoreSummary{Value: value, Endpoint: endpoint, RetrievedAt: time.UnixMilli(retrieved).UTC(), ExpiresAt: time.UnixMilli(expires).UTC(), Stale: !now.Before(time.UnixMilli(expires))}
		if result[song] == nil {
			result[song] = map[string]SpotifyScoreSummary{}
		}
		previous, exists := result[song][key]
		if !exists || (previous.Stale && !candidate.Stale) || (previous.Stale == candidate.Stale && (candidate.RetrievedAt.After(previous.RetrievedAt) || (candidate.RetrievedAt.Equal(previous.RetrievedAt) && endpoint == "audio_analysis"))) {
			result[song][key] = candidate
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
