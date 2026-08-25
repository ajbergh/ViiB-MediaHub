package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const PlexCredentialsSettingKey = "plex_credentials"

// PlexSource is a persisted remote music source. Credentials are deliberately
// stored separately in the encrypted settings table and never appear here.
type PlexSource struct {
	ID                string `json:"id"`
	MachineIdentifier string `json:"machineIdentifier"`
	BaseURL           string `json:"baseUrl"`
	Name              string `json:"name"`
	Version           string `json:"version,omitempty"`
	LibraryID         string `json:"libraryId,omitempty"`
	LibraryTitle      string `json:"libraryTitle,omitempty"`
	ConnectedAt       int64  `json:"connectedAt"`
	LastSyncAt        int64  `json:"lastSyncAt,omitempty"`
	LastSyncStatus    string `json:"lastSyncStatus"`
	LastSyncError     string `json:"lastSyncError,omitempty"`
	Available         bool   `json:"available"`
	Active            bool   `json:"active"`
}

// PlexCatalogTrack contains the unified song fields plus PMS source identity.
type PlexCatalogTrack struct {
	SongID      string
	SourceID    string
	LibraryID   string
	MachineID   string
	RatingKey   string
	MetadataKey string
	MediaKey    string
	ArtworkKey  string
	Container   string
	AudioCodec  string
	UpdatedAt   int64
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	TrackNumber int
	DiscNumber  int
	Genres      []string
	Year        int
	Duration    float64
	AddedAt     int64
}

// PlexTrackSource is the lookup required to stream/proxy a catalog track.
type PlexTrackSource struct {
	SongID      string
	SourceID    string
	LibraryID   string
	MachineID   string
	RatingKey   string
	MetadataKey string
	MediaKey    string
	ArtworkKey  string
	Container   string
	AudioCodec  string
	UpdatedAt   int64
	BaseURL     string
	Available   bool
}

// EnsurePlexSchema installs additive source metadata without changing the
// existing songs schema. songs remains the single catalog used by all ViiB UI.
func (d *DB) EnsurePlexSchema() error {
	if err := d.EnsureLibrarySyncSchema(); err != nil {
		return err
	}
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS plex_sources (
			id TEXT PRIMARY KEY,
			machine_identifier TEXT NOT NULL UNIQUE,
			base_url TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '',
			library_id TEXT NOT NULL DEFAULT '',
			library_title TEXT NOT NULL DEFAULT '',
			connected_at INTEGER NOT NULL,
			last_sync_at INTEGER,
			last_sync_status TEXT NOT NULL DEFAULT 'never',
			last_sync_error TEXT NOT NULL DEFAULT '',
			available INTEGER NOT NULL DEFAULT 1,
			active INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_plex_sources_active ON plex_sources(active);
		CREATE TABLE IF NOT EXISTS plex_tracks (
			song_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			library_id TEXT NOT NULL,
			machine_identifier TEXT NOT NULL,
			rating_key TEXT NOT NULL,
			metadata_key TEXT NOT NULL DEFAULT '',
			media_key TEXT NOT NULL,
			artwork_key TEXT NOT NULL DEFAULT '',
			container TEXT NOT NULL DEFAULT '',
			audio_codec TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(song_id) REFERENCES songs(id) ON DELETE CASCADE,
			FOREIGN KEY(source_id) REFERENCES plex_sources(id) ON DELETE CASCADE,
			UNIQUE(source_id, library_id, rating_key)
		);
		CREATE INDEX IF NOT EXISTS idx_plex_tracks_source_library ON plex_tracks(source_id, library_id);
		CREATE INDEX IF NOT EXISTS idx_plex_tracks_machine_rating ON plex_tracks(machine_identifier, rating_key);
	`)
	if err != nil {
		return fmt.Errorf("create Plex source schema: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func (d *DB) SavePlexSource(source PlexSource) error {
	if err := d.EnsurePlexSchema(); err != nil {
		return err
	}
	if source.ID == "" || source.MachineIdentifier == "" || source.BaseURL == "" {
		return errors.New("plex source id, machine identifier, and base URL are required")
	}
	if source.ConnectedAt == 0 {
		source.ConnectedAt = time.Now().UnixMilli()
	}
	if source.LastSyncStatus == "" {
		source.LastSyncStatus = "never"
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if source.Active {
		if _, err := tx.Exec(`UPDATE plex_sources SET active = 0`); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`
		INSERT INTO plex_sources (id, machine_identifier, base_url, name, version, library_id, library_title, connected_at, last_sync_at, last_sync_status, last_sync_error, available, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			machine_identifier=excluded.machine_identifier,
			base_url=excluded.base_url,
			name=excluded.name,
			version=excluded.version,
			library_id=excluded.library_id,
			library_title=excluded.library_title,
			last_sync_at=excluded.last_sync_at,
			last_sync_status=excluded.last_sync_status,
			last_sync_error=excluded.last_sync_error,
			available=excluded.available,
			active=excluded.active
	`, source.ID, source.MachineIdentifier, source.BaseURL, source.Name, source.Version,
		source.LibraryID, source.LibraryTitle, source.ConnectedAt, nullableInt64(source.LastSyncAt),
		source.LastSyncStatus, source.LastSyncError, boolInt(source.Available), boolInt(source.Active))
	if err != nil {
		return fmt.Errorf("save Plex source: %w", err)
	}
	return tx.Commit()
}

func scanPlexSource(row interface{ Scan(...any) error }) (*PlexSource, error) {
	var source PlexSource
	var lastSync sql.NullInt64
	var available, active int
	err := row.Scan(&source.ID, &source.MachineIdentifier, &source.BaseURL, &source.Name, &source.Version,
		&source.LibraryID, &source.LibraryTitle, &source.ConnectedAt, &lastSync, &source.LastSyncStatus,
		&source.LastSyncError, &available, &active)
	if err != nil {
		return nil, err
	}
	if lastSync.Valid {
		source.LastSyncAt = lastSync.Int64
	}
	source.Available = available != 0
	source.Active = active != 0
	return &source, nil
}

const plexSourceSelect = `SELECT id, machine_identifier, base_url, name, version, library_id, library_title, connected_at, last_sync_at, last_sync_status, last_sync_error, available, active FROM plex_sources`

func (d *DB) GetActivePlexSource() (*PlexSource, error) {
	if err := d.EnsurePlexSchema(); err != nil {
		return nil, err
	}
	source, err := scanPlexSource(d.conn.QueryRow(plexSourceSelect + ` WHERE active = 1 LIMIT 1`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return source, err
}

func (d *DB) GetPlexSource(id string) (*PlexSource, error) {
	if err := d.EnsurePlexSchema(); err != nil {
		return nil, err
	}
	source, err := scanPlexSource(d.conn.QueryRow(plexSourceSelect+` WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return source, err
}

// SetPlexLibrary changes the desired library selection without deleting the
// currently cached catalog. Old Plex rows are retained until a complete,
// successful synchronization of the new selection can reconcile them safely.
func (d *DB) SetPlexLibrary(sourceID, libraryID, libraryTitle string) error {
	if err := d.EnsurePlexSchema(); err != nil {
		return err
	}
	result, err := d.conn.Exec(`
		UPDATE plex_sources
		SET library_id=?, library_title=?, last_sync_status='never', last_sync_error=''
		WHERE id=?
	`, libraryID, libraryTitle, sourceID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) UpdatePlexConnection(sourceID, baseURL, name, version string, available bool) error {
	if err := d.EnsurePlexSchema(); err != nil {
		return err
	}
	_, err := d.conn.Exec(`UPDATE plex_sources SET base_url=?, name=?, version=?, available=? WHERE id=?`, baseURL, name, version, boolInt(available), sourceID)
	return err
}

func (d *DB) SetPlexSyncState(sourceID, status, message string, available bool, syncedAt int64) error {
	if err := d.EnsurePlexSchema(); err != nil {
		return err
	}
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := d.conn.Exec(`UPDATE plex_sources SET last_sync_status=?, last_sync_error=?, available=?, last_sync_at=COALESCE(?, last_sync_at) WHERE id=?`,
		status, message, boolInt(available), nullableInt64(syncedAt), sourceID)
	return err
}

func plexSyntheticPath(machineID, libraryID, ratingKey string) string {
	escape := func(value string) string { return strings.ReplaceAll(value, "/", "_") }
	return "plex://" + escape(machineID) + "/" + escape(libraryID) + "/" + escape(ratingKey)
}

func plexArtworkPath(songID, artworkKey string, updatedAt int64) string {
	if artworkKey == "" {
		return ""
	}
	version := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", artworkKey, updatedAt)))
	return fmt.Sprintf("plex://art/%s?v=%x", songID, version[:8])
}

type existingPlexCatalogTrack struct {
	updatedAt   int64
	libraryID   string
	metadataKey string
	mediaKey    string
	artworkKey  string
	container   string
	audioCodec  string
}

func (record existingPlexCatalogTrack) matches(libraryID string, track PlexCatalogTrack) bool {
	// updatedAt is the only PMS metadata version signal persisted today. If PMS
	// omits it (zero), be conservative and refresh the row rather than assuming
	// title/artist/album/etc. are unchanged merely because source keys match.
	return track.UpdatedAt > 0 &&
		record.libraryID == libraryID &&
		record.updatedAt == track.UpdatedAt &&
		record.metadataKey == track.MetadataKey &&
		record.mediaKey == track.MediaKey &&
		record.artworkKey == track.ArtworkKey &&
		record.container == track.Container &&
		record.audioCodec == track.AudioCodec
}

// SyncPlexLibrary atomically applies a complete successful PMS snapshot. The
// snapshot becomes authoritative for this active source only after the remote
// read has succeeded. That means a library switch while PMS is offline leaves
// the previous cache intact; the successful new snapshot performs the cleanup.
func (d *DB) SyncPlexLibrary(sourceID, libraryID string, tracks []PlexCatalogTrack) (added, updated, removed int, err error) {
	if err = d.EnsurePlexSchema(); err != nil {
		return
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback()

	existing := map[string]existingPlexCatalogTrack{}
	rows, err := tx.Query(`
		SELECT song_id, updated_at, library_id, metadata_key, media_key, artwork_key, container, audio_codec
		FROM plex_tracks WHERE source_id=?
	`, sourceID)
	if err != nil {
		return 0, 0, 0, err
	}
	for rows.Next() {
		var id string
		var record existingPlexCatalogTrack
		if err := rows.Scan(&id, &record.updatedAt, &record.libraryID, &record.metadataKey, &record.mediaKey,
			&record.artworkKey, &record.container, &record.audioCodec); err != nil {
			rows.Close()
			return 0, 0, 0, err
		}
		existing[id] = record
	}
	if err := rows.Close(); err != nil {
		return 0, 0, 0, err
	}

	seen := make(map[string]struct{}, len(tracks))
	for _, track := range tracks {
		// Presence and playability are different concepts in PMS. If a successful
		// metadata snapshot still reports the rating key but temporarily omits a
		// Media/Part, retain any existing cached song instead of reconciling it as
		// deleted. A new unplayable item is simply deferred until a usable part is
		// returned by a later sync.
		if track.SongID == "" || track.RatingKey == "" {
			continue
		}
		seen[track.SongID] = struct{}{}
		if track.MediaKey == "" {
			continue
		}
		previous, exists := existing[track.SongID]
		if exists && previous.matches(libraryID, track) {
			continue
		}
		genres, _ := json.Marshal(NormalizeGenres(track.Genres))
		coverPath := plexArtworkPath(track.SongID, track.ArtworkKey, track.UpdatedAt)
		filePath := plexSyntheticPath(track.MachineID, libraryID, track.RatingKey)
		if exists {
			updated++
		} else {
			added++
		}
		_, err = tx.Exec(`
			INSERT INTO songs (id, title, artist, album, album_artist, track_number, disc_number, genre, year, duration, file_path, cover_path, added_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				title=excluded.title, artist=excluded.artist, album=excluded.album, album_artist=excluded.album_artist,
				track_number=excluded.track_number, disc_number=excluded.disc_number, genre=excluded.genre,
				year=excluded.year, duration=excluded.duration, file_path=excluded.file_path, cover_path=excluded.cover_path,
				added_at=excluded.added_at, ignored=0
		`, track.SongID, track.Title, track.Artist, track.Album, track.AlbumArtist, track.TrackNumber, track.DiscNumber,
			string(genres), track.Year, track.Duration, filePath, coverPath, track.AddedAt)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("upsert Plex song: %w", err)
		}
		_, err = tx.Exec(`
			INSERT INTO plex_tracks (song_id, source_id, library_id, machine_identifier, rating_key, metadata_key, media_key, artwork_key, container, audio_codec, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(song_id) DO UPDATE SET source_id=excluded.source_id, library_id=excluded.library_id,
				machine_identifier=excluded.machine_identifier, rating_key=excluded.rating_key, metadata_key=excluded.metadata_key,
				media_key=excluded.media_key, artwork_key=excluded.artwork_key, container=excluded.container,
				audio_codec=excluded.audio_codec, updated_at=excluded.updated_at
		`, track.SongID, sourceID, libraryID, track.MachineID, track.RatingKey, track.MetadataKey,
			track.MediaKey, track.ArtworkKey, track.Container, track.AudioCodec, track.UpdatedAt)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("upsert Plex source metadata: %w", err)
		}
	}

	// Reconcile every cached row belonging to this currently configured source.
	// This deliberately includes rows from a previously selected library, but it
	// runs only after the caller obtained a complete successful new snapshot.
	for id := range existing {
		if _, ok := seen[id]; ok {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM plex_tracks WHERE song_id=?`, id); err != nil {
			return 0, 0, 0, err
		}
		if _, err := tx.Exec(`DELETE FROM songs WHERE id=?`, id); err != nil {
			return 0, 0, 0, err
		}
		removed++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, err
	}
	return added, updated, removed, nil
}

func (d *DB) GetPlexTrackSource(songID string) (*PlexTrackSource, error) {
	if err := d.EnsurePlexSchema(); err != nil {
		return nil, err
	}
	var record PlexTrackSource
	var available int
	err := d.conn.QueryRow(`
		SELECT t.song_id, t.source_id, t.library_id, t.machine_identifier, t.rating_key, t.metadata_key,
		       t.media_key, t.artwork_key, t.container, t.audio_codec, t.updated_at, s.base_url, s.available
		FROM plex_tracks t JOIN plex_sources s ON s.id=t.source_id WHERE t.song_id=?
	`, songID).Scan(&record.SongID, &record.SourceID, &record.LibraryID, &record.MachineID, &record.RatingKey,
		&record.MetadataKey, &record.MediaKey, &record.ArtworkKey, &record.Container, &record.AudioCodec,
		&record.UpdatedAt, &record.BaseURL, &available)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record.Available = available != 0
	return &record, nil
}

// RemovePlexSource deletes only ViiB's cached catalog/configuration rows. It
// performs no network request and can never delete or modify media on PMS.
func (d *DB) RemovePlexSource(sourceID string) error {
	if err := d.EnsurePlexSchema(); err != nil {
		return err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT song_id FROM plex_tracks WHERE source_id=?`, sourceID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM plex_tracks WHERE song_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM songs WHERE id=?`, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM plex_sources WHERE id=?`, sourceID); err != nil {
		return err
	}
	return tx.Commit()
}
