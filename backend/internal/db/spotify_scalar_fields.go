package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

// SpotifyScalarField retains native provider semantics independently of local
// measurements and compatibility projections. Context ownership stays private.
type SpotifyScalarField struct {
	RecordingID     string          `json:"recordingId,omitempty"`
	Key             string          `json:"key"`
	Metric          string          `json:"metric"`
	Units           string          `json:"units"`
	Value           json.RawMessage `json:"value"`
	Confidence      *float64        `json:"confidence,omitempty"`
	Endpoint        string          `json:"endpoint"`
	SchemaVersion   int             `json:"schemaVersion"`
	AdapterRevision string          `json:"adapterRevision"`
	RetrievedAt     time.Time       `json:"retrievedAt"`
	ExpiresAt       time.Time       `json:"expiresAt"`
	Stale           bool            `json:"stale"`
	DurableImport   bool            `json:"durableImport,omitempty"`
}

func putSpotifyScalarFieldsTx(tx preparationExecutor, o spotifyanalysis.Observation, revision string, expires time.Time) error {
	// Historical observations remain in the compatibility cache without being
	// assigned ownership or promoted into the authoritative field store.
	if o.AccountContext == "" {
		return nil
	}
	fields := []struct {
		key, metric, units string
		value              any
		confidence         *float64
	}{
		{"tempo_bpm", "tempo", "bpm", o.BPM, o.BPMConfidence},
		{"key_mode", "tonic_and_mode", "pitch_class_and_mode", nil, o.KeyConfidence},
		{"provider_loudness_db", "spotify_track_loudness", "dB", o.LoudnessDB, nil},
		{"time_signature", "measured_meter", "beats_per_bar", o.TimeSignature, o.TimeSignatureConfidence},
		{"duration_seconds", "recording_duration", "seconds", o.DurationSeconds, nil},
		{"duration_milliseconds", "recording_duration", "milliseconds", o.DurationMilliseconds, nil},
	}
	if o.Key != nil && o.Mode != nil {
		fields[1].value = struct {
			Tonic          int      `json:"tonic"`
			Mode           int      `json:"mode"`
			ModeConfidence *float64 `json:"modeConfidence,omitempty"`
		}{*o.Key, *o.Mode, o.ModeConfidence}
	}
	for _, score := range []struct {
		name  string
		value *float64
	}{{"energy", o.Energy}, {"danceability", o.Danceability}, {"acousticness", o.Acousticness}, {"instrumentalness", o.Instrumentalness}, {"liveness", o.Liveness}, {"speechiness", o.Speechiness}, {"valence", o.Valence}} {
		fields = append(fields, struct {
			key, metric, units string
			value              any
			confidence         *float64
		}{"spotify_" + score.name + "_score", "spotify_" + score.name, "unit_interval", score.value, nil})
	}
	for _, f := range fields {
		value, err := json.Marshal(f.value)
		if err != nil {
			return err
		}
		state, reason := "available", ""
		if string(value) == "null" {
			state = "not_returned"
		}
		for _, rejection := range o.RejectedFields {
			path := strings.TrimPrefix(rejection.Path, "track.")
			matches := map[string]string{"tempo": "tempo_bpm", "key": "key_mode", "mode": "key_mode", "loudness": "provider_loudness_db", "time_signature": "time_signature", "duration": "duration_seconds", "duration_ms": "duration_milliseconds"}
			key := matches[path]
			if key == "" {
				key = "spotify_" + path + "_score"
			}
			if key == f.key && string(value) == "null" {
				state = "invalid_field"
				reason = rejection.Reason
			}
		}
		_, err = tx.Exec(`INSERT INTO spotify_audio_field_attempts(spotify_id,endpoint,context_key,field_key,state,reason,checked_at,adapter_revision) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(spotify_id,endpoint,context_key,field_key) DO UPDATE SET state=excluded.state,reason=excluded.reason,checked_at=excluded.checked_at,adapter_revision=excluded.adapter_revision WHERE excluded.checked_at>spotify_audio_field_attempts.checked_at`, o.TrackID, o.SourceEndpoint, o.AccountContext, f.key, state, reason, o.RetrievedAt.UnixMilli(), revision)
		if err != nil {
			return err
		}
		if string(value) == "null" {
			continue
		}
		_, err = tx.Exec(`INSERT INTO spotify_audio_observations(spotify_id,endpoint,context_key,field_key,metric,units,value_json,confidence,schema_version,adapter_revision,retrieved_at,expires_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(spotify_id,endpoint,context_key,field_key) DO UPDATE SET
 metric=excluded.metric,units=excluded.units,value_json=excluded.value_json,confidence=excluded.confidence,schema_version=excluded.schema_version,adapter_revision=excluded.adapter_revision,retrieved_at=excluded.retrieved_at,expires_at=excluded.expires_at
 WHERE excluded.retrieved_at>spotify_audio_observations.retrieved_at`, o.TrackID, o.SourceEndpoint, o.AccountContext, f.key, f.metric, f.units, string(value), f.confidence, ExternalAnalysisSchemaVersion, revision, o.RetrievedAt.UnixMilli(), expires.UnixMilli())
		if err != nil {
			return err
		}
	}
	return nil
}

// GetSpotifyScalarFields returns active-owner candidates, including explicitly
// stale last-good fields. Endpoint precedence belongs to the effective resolver.
func (d *DB) GetSpotifyScalarFields(id string, now time.Time) ([]SpotifyScalarField, error) {
	if !ValidSpotifyRecordingID(id) {
		return nil, errors.New("invalid Spotify recording")
	}
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	return getSpotifyScalarFields(d.conn, id, now, nil)
}

func getSpotifyScalarFields(query interface {
	Query(string, ...any) (*sql.Rows, error)
}, id string, now time.Time, contextKey *string) ([]SpotifyScalarField, error) {
	if !ValidSpotifyRecordingID(id) {
		return nil, errors.New("invalid Spotify recording")
	}
	rows, err := query.Query(`SELECT field_key,metric,units,value_json,confidence,endpoint,schema_version,adapter_revision,retrieved_at,expires_at FROM spotify_audio_observations WHERE spotify_id=? AND context_key<>'' AND context_key=COALESCE(?,(SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'') ORDER BY field_key,endpoint`, id, contextKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SpotifyScalarField{}
	for rows.Next() {
		var f SpotifyScalarField
		var value string
		var retrieved, expires int64
		if err := rows.Scan(&f.Key, &f.Metric, &f.Units, &value, &f.Confidence, &f.Endpoint, &f.SchemaVersion, &f.AdapterRevision, &retrieved, &expires); err != nil {
			return nil, err
		}
		f.Value = json.RawMessage(value)
		f.RetrievedAt = time.UnixMilli(retrieved).UTC()
		f.ExpiresAt = time.UnixMilli(expires).UTC()
		f.Stale = !now.Before(f.ExpiresAt)
		result = append(result, f)
	}
	return result, rows.Err()
}

// SpotifyFieldAttempt describes the latest successful endpoint response for each
// field, independently of its retained last-good observation.
type SpotifyFieldAttempt struct {
	Key             string    `json:"key"`
	Endpoint        string    `json:"endpoint"`
	State           string    `json:"state"`
	Reason          string    `json:"reason,omitempty"`
	CheckedAt       time.Time `json:"checkedAt"`
	AdapterRevision string    `json:"adapterRevision"`
	DurableImport   bool      `json:"durableImport,omitempty"`
}

func (d *DB) GetSpotifyFieldAttempts(id string) ([]SpotifyFieldAttempt, error) {
	if !ValidSpotifyRecordingID(id) {
		return nil, errors.New("invalid Spotify recording")
	}
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	return getSpotifyFieldAttempts(d.conn, id, nil)
}

func getSpotifyFieldAttempts(query interface {
	Query(string, ...any) (*sql.Rows, error)
}, id string, contextKey *string) ([]SpotifyFieldAttempt, error) {
	if !ValidSpotifyRecordingID(id) {
		return nil, errors.New("invalid Spotify recording")
	}
	rows, err := query.Query(`SELECT field_key,endpoint,state,reason,checked_at,adapter_revision FROM spotify_audio_field_attempts WHERE spotify_id=? AND context_key<>'' AND context_key=COALESCE(?,(SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'') ORDER BY field_key,endpoint`, id, contextKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SpotifyFieldAttempt{}
	for rows.Next() {
		var f SpotifyFieldAttempt
		var at int64
		if err := rows.Scan(&f.Key, &f.Endpoint, &f.State, &f.Reason, &at, &f.AdapterRevision); err != nil {
			return nil, err
		}
		f.CheckedAt = time.UnixMilli(at).UTC()
		result = append(result, f)
	}
	return result, rows.Err()
}
