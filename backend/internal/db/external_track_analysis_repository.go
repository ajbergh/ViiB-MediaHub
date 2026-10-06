// Stores optional external analysis observations and failure/cooldown state separately from local analysis.
package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"time"
)

const ExternalAnalysisSchemaVersion = 1

type ExternalAnalysisCache struct {
	Observation     spotifyanalysis.Observation `json:"observation"`
	SchemaVersion   int                         `json:"schemaVersion"`
	AdapterRevision string                      `json:"adapterRevision"`
	ObservationHash string                      `json:"observationHash"`
	ExpiresAt       time.Time                   `json:"expiresAt"`
}
type ExternalAnalysisStatus struct {
	Code      spotifyanalysis.Code `json:"code"`
	CheckedAt time.Time            `json:"checkedAt"`
	RetryAt   time.Time            `json:"retryAt"`
}

func validExternalKey(id, endpoint string) bool {
	return ValidSpotifyRecordingID(id) && (endpoint == "audio_analysis" || endpoint == "audio_features")
}

// PutExternalAnalysis commits normalized scalars and any separate domain artifacts atomically. Its hash identifies
// these normalized bytes, not a raw response. Older requests cannot overwrite
// newer observations. Expiry is caller policy, not a claimed Spotify allowance.
func (d *DB) PutExternalAnalysis(o spotifyanalysis.Observation, revision string, expires time.Time) error {
	return d.putExternalAnalysis(o, revision, expires, nil)
}

func (d *DB) PutExternalAnalysisForRuntime(fence SpotifyMetadataFence, o spotifyanalysis.Observation, revision string, expires time.Time) error {
	return d.putExternalAnalysis(o, revision, expires, &fence)
}

func (d *DB) putExternalAnalysis(o spotifyanalysis.Observation, revision string, expires time.Time, fence *SpotifyMetadataFence) error {
	if o.BPMOrigin != nil || o.KeyOrigin != nil {
		return errors.New("preparation projection is not a provider observation")
	}
	if !validExternalKey(o.TrackID, o.SourceEndpoint) || o.Source != "spotify_internal" || revision == "" ||
		len(revision) > 256 || o.RetrievedAt.IsZero() || !expires.After(o.RetrievedAt) {
		return errors.New("invalid external observation provenance")
	}
	if err := spotifyanalysis.ValidateObservation(o); err != nil {
		return err
	}
	encoded, err := json.Marshal(o)
	if err != nil {
		return err
	}
	if len(encoded) > 16384 {
		return errors.New("external observation exceeds limit")
	}
	sum := sha256.Sum256(encoded)
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if fence != nil {
		if o.AccountContext != fence.ContextKey || o.DurableImport {
			return ErrSpotifyMetadataRuntimeSuperseded
		}
		if err := checkSpotifyMetadataFenceTx(tx, *fence); err != nil {
			return err
		}
	}
	if err = putSpotifyScalarFieldsTx(tx, o, revision, expires); err != nil {
		return err
	}
	if err = putSpotifyAudioDomainTx(tx, o, revision, expires); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO external_track_analysis
 (provider,external_id,endpoint,schema_version,observation_json,observation_hash,adapter_revision,retrieved_at,expires_at,account_context)
 VALUES ('spotify',?,?,?,?,?,?,?,?,?)
 ON CONFLICT(provider,external_id,endpoint,schema_version) DO UPDATE SET
 observation_json=excluded.observation_json,observation_hash=excluded.observation_hash,
 adapter_revision=excluded.adapter_revision,retrieved_at=excluded.retrieved_at,expires_at=excluded.expires_at,account_context=excluded.account_context
 WHERE excluded.retrieved_at>external_track_analysis.retrieved_at`,
		o.TrackID, o.SourceEndpoint, ExternalAnalysisSchemaVersion, string(encoded), hex.EncodeToString(sum[:]), revision,
		o.RetrievedAt.UnixMilli(), expires.UnixMilli(), o.AccountContext)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (d *DB) GetExternalAnalysis(id, endpoint string) (*ExternalAnalysisCache, error) {
	if !validExternalKey(id, endpoint) {
		return nil, errors.New("invalid external cache key")
	}
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	return getExternalAnalysis(d.conn, id, endpoint, nil)
}

func getExternalAnalysis(query interface{ QueryRow(string, ...any) *sql.Row }, id, endpoint string, contextKey *string) (*ExternalAnalysisCache, error) {
	if !validExternalKey(id, endpoint) {
		return nil, errors.New("invalid external cache key")
	}
	var cache ExternalAnalysisCache
	var encoded string
	var expiry int64
	var activeContext string
	err := query.QueryRow(`SELECT observation_json,schema_version,adapter_revision,observation_hash,expires_at,COALESCE(account_context,''),COALESCE(?,(SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'')
 FROM external_track_analysis WHERE provider='spotify' AND external_id=? AND endpoint=? AND schema_version=?`,
		contextKey, id, endpoint, ExternalAnalysisSchemaVersion).Scan(&encoded, &cache.SchemaVersion, &cache.AdapterRevision, &cache.ObservationHash, &expiry, &cache.Observation.AccountContext, &activeContext)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if cache.Observation.AccountContext == "" || cache.Observation.AccountContext != activeContext {
		return nil, nil
	}
	if len(encoded) > 16384 {
		return nil, errors.New("external observation exceeds limit")
	}
	if err = json.Unmarshal([]byte(encoded), &cache.Observation); err != nil {
		return nil, err
	}
	if cache.Observation.AccountContext != activeContext || cache.Observation.TrackID != id || cache.Observation.SourceEndpoint != endpoint ||
		cache.Observation.Source != "spotify_internal" {
		return nil, errors.New("external observation identity mismatch")
	}
	if err = spotifyanalysis.ValidateObservation(cache.Observation); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(encoded))
	if cache.ObservationHash != hex.EncodeToString(sum[:]) {
		return nil, errors.New("external observation hash mismatch")
	}
	cache.ExpiresAt = time.UnixMilli(expiry).UTC()
	return &cache, nil
}

// PutExternalAnalysisStatus stores safe codes only; a failure leaves good data
// intact. Readers compare checkedAt against observation.retrievedAt.
func (d *DB) PutExternalAnalysisStatus(id, endpoint string, status ExternalAnalysisStatus) error {
	return d.putExternalAnalysisStatus(id, endpoint, status, nil)
}

func (d *DB) PutExternalAnalysisStatusForRuntime(fence SpotifyMetadataFence, id, endpoint string, status ExternalAnalysisStatus) error {
	return d.putExternalAnalysisStatus(id, endpoint, status, &fence)
}

func (d *DB) putExternalAnalysisStatus(id, endpoint string, status ExternalAnalysisStatus, fence *SpotifyMetadataFence) error {
	if !validExternalKey(id, endpoint) || status.CheckedAt.IsZero() || status.RetryAt.Before(status.CheckedAt) {
		return errors.New("invalid external status")
	}
	switch status.Code {
	case spotifyanalysis.AuthenticationRequired, spotifyanalysis.AccessDenied, spotifyanalysis.NotFound,
		spotifyanalysis.AnalysisUnavailable, spotifyanalysis.RateLimited, spotifyanalysis.TemporarilyUnavailable, spotifyanalysis.ProviderChanged:
	default:
		return errors.New("invalid external status code")
	}
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return err
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
	}
	contextKey := ""
	if fence != nil {
		contextKey = fence.ContextKey
	}
	_, err = tx.Exec(`INSERT INTO external_track_analysis_status
 (provider,external_id,endpoint,schema_version,code,checked_at,retry_at,account_context) VALUES ('spotify',?,?,?,?,?,?,?)
 ON CONFLICT(provider,external_id,endpoint,schema_version) DO UPDATE SET
 code=excluded.code,checked_at=excluded.checked_at,retry_at=excluded.retry_at,account_context=excluded.account_context
 WHERE excluded.checked_at>external_track_analysis_status.checked_at`,
		id, endpoint, ExternalAnalysisSchemaVersion, status.Code, status.CheckedAt.UnixMilli(), status.RetryAt.UnixMilli(), contextKey)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (d *DB) GetExternalAnalysisStatus(id, endpoint string) (*ExternalAnalysisStatus, error) {
	if !validExternalKey(id, endpoint) {
		return nil, errors.New("invalid external cache key")
	}
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	return getExternalAnalysisStatus(d.conn, id, endpoint, nil)
}

func getExternalAnalysisStatus(query interface{ QueryRow(string, ...any) *sql.Row }, id, endpoint string, contextKey *string) (*ExternalAnalysisStatus, error) {
	if !validExternalKey(id, endpoint) {
		return nil, errors.New("invalid external cache key")
	}
	var status ExternalAnalysisStatus
	var checked, retry int64
	err := query.QueryRow(`SELECT code,checked_at,retry_at FROM external_track_analysis_status
 WHERE provider='spotify' AND external_id=? AND endpoint=? AND schema_version=? AND (? IS NULL OR account_context=?)`, id, endpoint, ExternalAnalysisSchemaVersion, contextKey, contextKey).
		Scan(&status.Code, &checked, &retry)
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

// PurgeExternalAnalysis explicitly clears reference data, including failure
// state. It never deletes local analysis, manual overrides, or recording links.
func (d *DB) PurgeExternalAnalysis() error {
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM spotify_audio_field_attempts"); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM spotify_audio_observations"); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM external_track_analysis"); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM external_track_analysis_status"); err != nil {
		return err
	}
	return tx.Commit()
}

// GetExternalAnalysisCooldown returns the longest persisted Spotify rate-limit
// delay, including other recordings/endpoints, after a service restart.
func (d *DB) GetExternalAnalysisCooldown() (time.Time, error) {
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return time.Time{}, err
	}
	var retry sql.NullInt64
	err := d.conn.QueryRow(`SELECT MAX(retry_at) FROM external_track_analysis_status
 WHERE provider='spotify' AND code='rate_limited' AND schema_version=?`, ExternalAnalysisSchemaVersion).Scan(&retry)
	if err != nil {
		return time.Time{}, err
	}
	// Independent resource attempts participate in the same provider cooldown.
	var metadataExists int
	if err = d.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='spotify_metadata_resource_status'").Scan(&metadataExists); err != nil {
		return time.Time{}, err
	}
	if metadataExists > 0 {
		var resourceRetry sql.NullInt64
		if err = d.conn.QueryRow("SELECT MAX(retry_at) FROM spotify_metadata_resource_status WHERE state='cooldown' AND reason='rate_limited'").Scan(&resourceRetry); err != nil {
			return time.Time{}, err
		}
		if resourceRetry.Valid && (!retry.Valid || resourceRetry.Int64 > retry.Int64) {
			retry = resourceRetry
		}
	}
	if !retry.Valid {
		return time.Time{}, nil
	}
	return time.UnixMilli(retry.Int64).UTC(), nil
}
