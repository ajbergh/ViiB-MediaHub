package db

import (
	"database/sql"
	"strings"
	"time"
)

type StemSet struct {
	ID, SongID, SourceAudioHash, AudioSHA256                                 string
	ModelName, ModelVersion, GeneratorName, GeneratorVersion, Layout, Status string
	SampleRate, Channels                                                     int
	Frames                                                                   int64
	DurationSeconds                                                          float64
	DecoderDelayFrames, StartTrimFrames                                      int64
	PackagePath, DiscoverySource                                             string
	ManifestSchemaVersion                                                    int
	GeneratedAt, ValidationErrorCode, ValidationErrorMessage                 string
	ManuallyInvalidated, ExplicitlyLinked                                    bool
	UpdatedAt                                                                int64
	Stems                                                                    []StemArtifact
}

type StemArtifact struct {
	Name, RelativePath, SHA256 string
	SizeBytes                  int64
	SampleRate, Channels       int
	Frames                     int64
	Encoding                   string
}
type StemLocation struct {
	ID, Path  string
	Enabled   bool
	CreatedAt int64
}

func (d *DB) UpsertStemSet(s StemSet) error {
	if err := d.EnsureStemSchema(); err != nil {
		return err
	}
	if s.UpdatedAt == 0 {
		s.UpdatedAt = time.Now().UnixMilli()
	}
	_, err := d.conn.Exec(`INSERT INTO track_stem_sets(id,song_id,source_audio_hash,audio_sha256,model_name,model_version,generator_name,generator_version,stem_layout,status,sample_rate,channels,frames,duration_seconds,decoder_delay_frames,start_trim_frames,package_path,discovery_source,manifest_schema_version,generated_at,validation_error_code,validation_error_message,manually_invalidated,explicitly_linked,updated_at)
	VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(song_id,package_path) DO UPDATE SET source_audio_hash=excluded.source_audio_hash,audio_sha256=excluded.audio_sha256,model_name=excluded.model_name,model_version=excluded.model_version,generator_name=excluded.generator_name,generator_version=excluded.generator_version,stem_layout=excluded.stem_layout,status=excluded.status,sample_rate=excluded.sample_rate,channels=excluded.channels,frames=excluded.frames,duration_seconds=excluded.duration_seconds,decoder_delay_frames=excluded.decoder_delay_frames,start_trim_frames=excluded.start_trim_frames,discovery_source=excluded.discovery_source,manifest_schema_version=excluded.manifest_schema_version,generated_at=excluded.generated_at,validation_error_code=excluded.validation_error_code,validation_error_message=excluded.validation_error_message,explicitly_linked=MAX(track_stem_sets.explicitly_linked,excluded.explicitly_linked),updated_at=excluded.updated_at`,
		s.ID, s.SongID, s.SourceAudioHash, s.AudioSHA256, s.ModelName, s.ModelVersion, s.GeneratorName, s.GeneratorVersion, s.Layout, s.Status, s.SampleRate, s.Channels, s.Frames, s.DurationSeconds, s.DecoderDelayFrames, s.StartTrimFrames, s.PackagePath, s.DiscoverySource, s.ManifestSchemaVersion, s.GeneratedAt, s.ValidationErrorCode, s.ValidationErrorMessage, s.ManuallyInvalidated, s.ExplicitlyLinked, s.UpdatedAt)
	if err != nil {
		return err
	}
	var persistedID string
	if err = d.conn.QueryRow(`SELECT id FROM track_stem_sets WHERE song_id=? AND package_path=?`, s.SongID, s.PackagePath).Scan(&persistedID); err != nil {
		return err
	}
	s.ID = persistedID
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM track_stems WHERE stem_set_id=?`, s.ID); err != nil {
		return err
	}
	for _, a := range s.Stems {
		if _, err = tx.Exec(`INSERT INTO track_stems(stem_set_id,name,relative_path,sha256,size_bytes,sample_rate,channels,frames,encoding) VALUES(?,?,?,?,?,?,?,?,?)`, s.ID, a.Name, a.RelativePath, a.SHA256, a.SizeBytes, a.SampleRate, a.Channels, a.Frames, a.Encoding); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) ListStemSets(songID string) ([]StemSet, error) {
	if err := d.EnsureStemSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT id,song_id,source_audio_hash,audio_sha256,model_name,model_version,generator_name,generator_version,stem_layout,status,sample_rate,channels,frames,duration_seconds,decoder_delay_frames,start_trim_frames,package_path,discovery_source,manifest_schema_version,generated_at,validation_error_code,validation_error_message,manually_invalidated,explicitly_linked,updated_at FROM track_stem_sets WHERE song_id=? ORDER BY explicitly_linked DESC, CASE discovery_source WHEN 'adjacent' THEN 0 ELSE 1 END, package_path COLLATE NOCASE, package_path`, songID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StemSet{}
	for rows.Next() {
		var s StemSet
		var inv, linked int
		if err = rows.Scan(&s.ID, &s.SongID, &s.SourceAudioHash, &s.AudioSHA256, &s.ModelName, &s.ModelVersion, &s.GeneratorName, &s.GeneratorVersion, &s.Layout, &s.Status, &s.SampleRate, &s.Channels, &s.Frames, &s.DurationSeconds, &s.DecoderDelayFrames, &s.StartTrimFrames, &s.PackagePath, &s.DiscoverySource, &s.ManifestSchemaVersion, &s.GeneratedAt, &s.ValidationErrorCode, &s.ValidationErrorMessage, &inv, &linked, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.ManuallyInvalidated = inv != 0
		s.ExplicitlyLinked = linked != 0
		ar, er := d.conn.Query(`SELECT name,relative_path,sha256,size_bytes,sample_rate,channels,frames,encoding FROM track_stems WHERE stem_set_id=? ORDER BY name`, s.ID)
		if er != nil {
			return nil, er
		}
		for ar.Next() {
			var a StemArtifact
			if er = ar.Scan(&a.Name, &a.RelativePath, &a.SHA256, &a.SizeBytes, &a.SampleRate, &a.Channels, &a.Frames, &a.Encoding); er != nil {
				ar.Close()
				return nil, er
			}
			s.Stems = append(s.Stems, a)
		}
		er = ar.Err()
		ar.Close()
		if er != nil {
			return nil, er
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListStemStatuses returns a path-free status summary for the requested songs.
// It reads only registry status columns and is intended for page-sized library
// batches. A ready set takes precedence, matching the per-song status route;
// otherwise the preferred (explicitly linked, then adjacent-first) set wins.
func (d *DB) ListStemStatuses(songIDs []string) (map[string]string, error) {
	statuses := make(map[string]string, len(songIDs))
	unique := make([]string, 0, len(songIDs))
	for _, id := range songIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := statuses[id]; exists {
			continue
		}
		statuses[id] = "none"
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return statuses, nil
	}
	if err := d.EnsureStemSchema(); err != nil {
		return nil, err
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(unique)), ",")
	args := make([]any, len(unique))
	for i, id := range unique {
		args[i] = id
	}
	rows, err := d.conn.Query(`SELECT song_id,status FROM track_stem_sets WHERE song_id IN (`+placeholders+`) ORDER BY song_id,explicitly_linked DESC,CASE discovery_source WHEN 'adjacent' THEN 0 ELSE 1 END,package_path COLLATE NOCASE,package_path`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := make(map[string]bool, len(unique))
	for rows.Next() {
		var songID, status string
		if err := rows.Scan(&songID, &status); err != nil {
			return nil, err
		}
		if status == "ready" {
			statuses[songID] = "ready"
			seen[songID] = true
			continue
		}
		if !seen[songID] {
			statuses[songID] = status
			seen[songID] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return statuses, nil
}

func (d *DB) SetStemLocations(locations []StemLocation) error {
	if err := d.EnsureStemSchema(); err != nil {
		return err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM stem_locations`); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, l := range locations {
		if l.CreatedAt == 0 {
			l.CreatedAt = now
		}
		if _, err = tx.Exec(`INSERT INTO stem_locations(id,path,enabled,created_at) VALUES(?,?,?,?)`, l.ID, l.Path, l.Enabled, l.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *DB) ListStemLocations() ([]StemLocation, error) {
	if err := d.EnsureStemSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT id,path,enabled,created_at FROM stem_locations ORDER BY path COLLATE NOCASE,path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StemLocation{}
	for rows.Next() {
		var l StemLocation
		var enabled int
		if err = rows.Scan(&l.ID, &l.Path, &enabled, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.Enabled = enabled != 0
		out = append(out, l)
	}
	return out, rows.Err()
}
func (d *DB) DeleteStemSet(songID, id string) error {
	if err := d.EnsureStemSchema(); err != nil {
		return err
	}
	res, err := d.conn.Exec(`DELETE FROM track_stem_sets WHERE song_id=? AND id=?`, songID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (d *DB) ClearExplicitStemLinks(songID string) error {
	if err := d.EnsureStemSchema(); err != nil {
		return err
	}
	_, err := d.conn.Exec(`UPDATE track_stem_sets SET explicitly_linked=0 WHERE song_id=?`, songID)
	return err
}
