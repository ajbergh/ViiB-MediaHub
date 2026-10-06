package db

import (
	"database/sql"
	"time"
)

// SpotifyMetadataReadFence admits retained private data without activating it.
// Pending reads require the retained provider owner and an empty active context.
// This fence cannot be used for writes.
type SpotifyMetadataReadFence struct {
	Epoch      string
	ContextKey string
	Provider   string
	Pending    bool
}

func (d *DB) GetSpotifyAudioArtifactForRuntime(fence SpotifyMetadataReadFence, id, resource, kind string) (*SpotifyAudioArtifact, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := checkSpotifyMetadataReadFenceTx(tx, fence); err != nil {
		return nil, err
	}
	artifact, err := getSpotifyAudioArtifact(tx, id, resource, kind, fence.ContextKey)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return artifact, nil
}

// GetSpotifyActiveAudioArtifactForRuntime also supports a fresh login whose
// authenticated profile has not been captured yet. It never admits pending data.
func (d *DB) GetSpotifyActiveAudioArtifactForRuntime(fence SpotifyMetadataFence, id, resource, kind string) (*SpotifyAudioArtifact, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := checkSpotifyMetadataFenceTx(tx, fence); err != nil {
		return nil, err
	}
	artifact, err := getSpotifyAudioArtifact(tx, id, resource, kind, fence.ContextKey)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return artifact, nil
}

func checkSpotifyMetadataReadFenceTx(tx *sql.Tx, fence SpotifyMetadataReadFence) error {
	var epoch, active, owner, provider string
	err := tx.QueryRow(`SELECT COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_runtime_epoch'),''), COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),''), COALESCE((SELECT context_key FROM spotify_metadata_owner WHERE singleton=1),''), COALESCE((SELECT provider FROM spotify_metadata_owner WHERE singleton=1),'')`).Scan(&epoch, &active, &owner, &provider)
	if err != nil {
		return err
	}
	if fence.Epoch == "" || fence.ContextKey == "" || fence.Epoch != epoch {
		return ErrSpotifyMetadataRuntimeSuperseded
	}
	if fence.Pending {
		if active != "" || owner != fence.ContextKey || provider != fence.Provider || (provider != "oauth" && provider != "webplayer") {
			return ErrSpotifyMetadataRuntimeSuperseded
		}
	} else if active != fence.ContextKey || owner != fence.ContextKey || provider != fence.Provider || (provider != "oauth" && provider != "webplayer") {
		return ErrSpotifyMetadataRuntimeSuperseded
	}

	return nil
}

// Retained observations are candidates only; this does not activate effective
// provider bindings or read unscoped failure rows.
func (d *DB) GetExternalAnalysisForRuntime(fence SpotifyMetadataReadFence, id, endpoint string) (*ExternalAnalysisCache, error) {
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
	cache, err := getExternalAnalysis(tx, id, endpoint, &fence.ContextKey)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return cache, nil
}

func (d *DB) GetExternalAnalysisStatusForRuntime(fence SpotifyMetadataReadFence, id, endpoint string) (*ExternalAnalysisStatus, error) {
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
	status, err := getExternalAnalysisStatus(tx, id, endpoint, &fence.ContextKey)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return status, nil
}

// ValidateSpotifyMetadataRead checks admission without inspecting provider data.
func (d *DB) ValidateSpotifyMetadataRead(fence SpotifyMetadataReadFence) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := checkSpotifyMetadataReadFenceTx(tx, fence); err != nil {
		return err
	}
	return tx.Commit()
}

// Read field observations and response attempts from one ownership snapshot.
func (d *DB) GetSpotifyScalarCandidatesForRuntime(fence SpotifyMetadataReadFence, id string, now time.Time) ([]SpotifyScalarField, []SpotifyFieldAttempt, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	if fence.Pending {
		err = checkSpotifyMetadataReadFenceTx(tx, fence)
	} else {
		err = checkSpotifyMetadataFenceTx(tx, SpotifyMetadataFence{Epoch: fence.Epoch, ContextKey: fence.ContextKey})
	}
	if err != nil {
		return nil, nil, err
	}
	fields, err := getSpotifyScalarFields(tx, id, now, &fence.ContextKey)
	if err != nil {
		return nil, nil, err
	}
	attempts, err := getSpotifyFieldAttempts(tx, id, &fence.ContextKey)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return fields, attempts, nil
}

// Shared cooldown includes independent resource attempts only for this owner.
func (d *DB) GetExternalAnalysisCooldownForRuntime(fence SpotifyMetadataFence) (time.Time, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback()
	if err := checkSpotifyMetadataFenceTx(tx, fence); err != nil {
		return time.Time{}, err
	}
	var retry sql.NullInt64
	err = tx.QueryRow(`SELECT MAX(retry_at) FROM (
 SELECT retry_at FROM external_track_analysis_status WHERE provider='spotify' AND code='rate_limited' AND schema_version=? AND account_context=?
 UNION ALL SELECT retry_at FROM spotify_metadata_resource_status WHERE state='cooldown' AND reason='rate_limited' AND context_key=?)`, ExternalAnalysisSchemaVersion, fence.ContextKey, fence.ContextKey).Scan(&retry)
	if err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, err
	}
	if !retry.Valid {
		return time.Time{}, nil
	}
	return time.UnixMilli(retry.Int64).UTC(), nil
}

// HasFreshSpotifyTrackCatalogForRuntime excludes related/search projections.
func (d *DB) HasFreshSpotifyTrackCatalogForRuntime(fence SpotifyMetadataFence, id string, now time.Time) (bool, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err = checkSpotifyMetadataFenceTx(tx, fence); err != nil {
		return false, err
	}
	var count int
	err = tx.QueryRow(`SELECT COUNT(*) FROM spotify_entity_snapshots WHERE entity_type='track' AND spotify_id=? AND context_key=? AND schema_version=1 AND expires_at>? AND resource IN (?, 'getTrack:page:0:0')`, id, fence.ContextKey, now.UnixMilli(), "rest:/v1/tracks/"+id+":page::").Scan(&count)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return count > 0, nil
}
