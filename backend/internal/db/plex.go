package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const PlexCredentialsSettingKey = "plex_credentials"

// AIDJSource controls which catalog source may contribute tracks to an AI DJ
// result. "all" deliberately includes only Plex sources that are currently
// reachable, so a generated queue cannot contain a known-offline remote track.
type AIDJSource string

const (
	AIDJSourceAll   AIDJSource = "all"
	AIDJSourceLocal AIDJSource = "local"
	AIDJSourcePlex  AIDJSource = "plex"
)

// AIDJLibrarySummary provides planner-scale data without materializing the
// library. Source eligibility mirrors FilterSongsForAIDJ, including offline
// Plex exclusions.
type AIDJLibrarySummary struct {
	SongCount          int
	AverageDurationSec int
}

// NormalizeAIDJSource validates a client-provided source preference and
// provides the backwards-compatible default for older clients.
func NormalizeAIDJSource(value string) (AIDJSource, error) {
	switch AIDJSource(strings.ToLower(strings.TrimSpace(value))) {
	case "", AIDJSourceAll:
		return AIDJSourceAll, nil
	case AIDJSourceLocal:
		return AIDJSourceLocal, nil
	case AIDJSourcePlex:
		return AIDJSourcePlex, nil
	default:
		return "", fmt.Errorf("invalid AI DJ source %q", value)
	}
}

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
	SongID           string
	SourceID         string
	LibraryID        string
	MachineID        string
	RatingKey        string
	MetadataKey      string
	MediaKey         string
	ArtworkKey       string
	ArtistArtworkKey string
	Container        string
	AudioCodec       string
	UpdatedAt        int64
	Title            string
	Artist           string
	Album            string
	AlbumArtist      string
	TrackNumber      int
	DiscNumber       int
	Genres           []string
	Year             int
	Duration         float64
	AddedAt          int64
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

// PlexAIWritebackCandidate is AI-enriched metadata eligible for an explicit,
// user-approved write to its Plex track. It is kept apart from the PMS
// snapshot so a later Plex read cannot erase an unsynchronized proposal.
type PlexAIWritebackCandidate struct {
	SongID       string
	SourceID     string
	RatingKey    string
	Title        string
	Artist       string
	Album        string
	Genres       []string
	OriginalYear int
	GeneratedAt  int64
}

// FilterSongsForAIDJ labels songs with their catalog source and keeps only
// tracks eligible for the requested source. Plex tracks are retained only
// while their source is available; cached metadata remains in the library but
// cannot create an unplayable AI DJ queue.
func (d *DB) FilterSongsForAIDJ(songs []Song, requestedSource string) ([]Song, error) {
	source, err := NormalizeAIDJSource(requestedSource)
	if err != nil {
		return nil, err
	}
	if err := d.EnsurePlexSchema(); err != nil {
		return nil, err
	}

	type plexSongInfo struct {
		name      string
		available bool
	}
	plexSongs := make(map[string]plexSongInfo)
	ids := make([]string, 0, len(songs))
	for _, song := range songs {
		if song.ID != "" {
			ids = append(ids, song.ID)
		}
	}

	// SQLite's default variable limit is commonly 999. Chunking keeps this
	// safe for full-library DJ planning without requiring a temporary table.
	for start := 0; start < len(ids); start += 900 {
		end := start + 900
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := make([]string, end-start)
		args := make([]any, end-start)
		for i, id := range ids[start:end] {
			placeholders[i] = "?"
			args[i] = id
		}
		query := fmt.Sprintf(`
			SELECT t.song_id, s.name, s.available
			FROM plex_tracks t
			JOIN plex_sources s ON s.id = t.source_id
			WHERE t.song_id IN (%s)
		`, strings.Join(placeholders, ","))
		rows, err := d.conn.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("look up AI DJ Plex sources: %w", err)
		}
		for rows.Next() {
			var songID, name string
			var available int
			if err := rows.Scan(&songID, &name, &available); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan AI DJ Plex source: %w", err)
			}
			plexSongs[songID] = plexSongInfo{name: name, available: available != 0}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read AI DJ Plex sources: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close AI DJ Plex source lookup: %w", err)
		}
	}

	eligible := make([]Song, 0, len(songs))
	for _, song := range songs {
		info, isPlex := plexSongs[song.ID]
		if isPlex {
			song.Source = string(AIDJSourcePlex)
			song.SourceName = info.name
			if !info.available || source == AIDJSourceLocal {
				continue
			}
		} else {
			song.Source = string(AIDJSourceLocal)
			song.SourceName = ""
			if source == AIDJSourcePlex {
				continue
			}
		}
		eligible = append(eligible, song)
	}
	return eligible, nil
}

// GetAIDJLibrarySummary returns the eligible catalog size and average duration
// using a small aggregate query. It exists so semantic DJ planning does not
// need GetAllSongs merely to describe the catalog to the set planner.
func (d *DB) GetAIDJLibrarySummary(ctx context.Context, requestedSource string) (AIDJLibrarySummary, error) {
	source, err := NormalizeAIDJSource(requestedSource)
	if err != nil {
		return AIDJLibrarySummary{}, err
	}
	if err := d.EnsurePlexSchema(); err != nil {
		return AIDJLibrarySummary{}, err
	}
	joinedPlex := `SELECT 1 FROM plex_tracks t JOIN plex_sources p ON p.id = t.source_id WHERE t.song_id = songs.id`
	availablePlex := `SELECT 1 FROM plex_tracks t JOIN plex_sources p ON p.id = t.source_id WHERE t.song_id = songs.id AND p.available = 1`
	sourceClause := ""
	switch source {
	case AIDJSourceLocal:
		sourceClause = `NOT EXISTS (` + joinedPlex + `)`
	case AIDJSourcePlex:
		sourceClause = `EXISTS (` + availablePlex + `)`
	default:
		sourceClause = `(NOT EXISTS (` + joinedPlex + `) OR EXISTS (` + availablePlex + `))`
	}
	var summary AIDJLibrarySummary
	var average sql.NullFloat64
	query := `SELECT COUNT(*), AVG(duration) FROM songs WHERE COALESCE(ignored, 0) = 0 AND ` + sourceClause
	if err := d.conn.QueryRowContext(ctx, query).Scan(&summary.SongCount, &average); err != nil {
		return AIDJLibrarySummary{}, err
	}
	if average.Valid && average.Float64 > 0 {
		summary.AverageDurationSec = int(average.Float64)
	}
	return summary, nil
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
			artist_artwork_key TEXT NOT NULL DEFAULT '',
			container TEXT NOT NULL DEFAULT '',
			audio_codec TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(song_id) REFERENCES songs(id) ON DELETE CASCADE,
			FOREIGN KEY(source_id) REFERENCES plex_sources(id) ON DELETE CASCADE,
			UNIQUE(source_id, library_id, rating_key)
		);
		CREATE INDEX IF NOT EXISTS idx_plex_tracks_source_library ON plex_tracks(source_id, library_id);
		CREATE INDEX IF NOT EXISTS idx_plex_tracks_machine_rating ON plex_tracks(machine_identifier, rating_key);
		CREATE TABLE IF NOT EXISTS plex_artist_artwork (
			source_id TEXT NOT NULL,
			artist_name TEXT NOT NULL COLLATE NOCASE,
			artwork_key TEXT NOT NULL,
			updated_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(source_id, artist_name),
			FOREIGN KEY(source_id) REFERENCES plex_sources(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_plex_artist_artwork_active ON plex_artist_artwork(source_id, artist_name);
		CREATE TABLE IF NOT EXISTS plex_ai_metadata_writeback (
			song_id TEXT PRIMARY KEY,
			genres TEXT NOT NULL DEFAULT '[]',
			original_year INTEGER NOT NULL DEFAULT 0,
			generated_at INTEGER NOT NULL,
			approved_at INTEGER,
			synced_at INTEGER,
			last_error TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(song_id) REFERENCES songs(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_plex_ai_metadata_writeback_pending ON plex_ai_metadata_writeback(synced_at, generated_at);
	`)
	if err != nil {
		return fmt.Errorf("create Plex source schema: %w", err)
	}
	if _, err := d.conn.Exec(`ALTER TABLE plex_tracks ADD COLUMN artist_artwork_key TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("add Plex artist artwork column: %w", err)
	}
	return nil
}

// queuePlexAIWriteback records an AI value only when the song is Plex-backed.
// It runs within the enrichment transaction, keeping the proposal and the
// visible ViiB metadata consistent.
func queuePlexAIWriteback(tx *sql.Tx, songID string, genres []string, updateGenres bool, originalYear int, updateYear bool, generatedAt int64) error {
	if !updateGenres && !updateYear {
		return nil
	}
	encodedGenres := "[]"
	if updateGenres {
		payload, err := json.Marshal(NormalizeGenres(genres))
		if err != nil {
			return err
		}
		encodedGenres = string(payload)
	}
	_, err := tx.Exec(`
		INSERT INTO plex_ai_metadata_writeback (song_id, genres, original_year, generated_at, approved_at, synced_at, last_error)
		SELECT song_id, ?, ?, ?, NULL, NULL, '' FROM plex_tracks WHERE song_id = ?
		ON CONFLICT(song_id) DO UPDATE SET
			genres = CASE WHEN ? THEN excluded.genres ELSE plex_ai_metadata_writeback.genres END,
			original_year = CASE WHEN ? THEN excluded.original_year ELSE plex_ai_metadata_writeback.original_year END,
			generated_at = excluded.generated_at,
			approved_at = NULL,
			synced_at = NULL,
			last_error = ''
	`, encodedGenres, originalYear, generatedAt, songID, updateGenres, updateYear)
	return err
}

// GetPlexAIWritebackCandidates returns pending proposals for one source. A
// proposal remains pending until PMS accepts it and a new read verifies it.
func (d *DB) GetPlexAIWritebackCandidates(sourceID string, songIDs []string, limit int) (candidates []PlexAIWritebackCandidate, hasMore bool, err error) {
	if err = d.EnsurePlexSchema(); err != nil {
		return nil, false, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	args := []any{sourceID}
	filter := ""
	if len(songIDs) > 0 {
		placeholders := make([]string, len(songIDs))
		for i, id := range songIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		filter = " AND w.song_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	args = append(args, limit+1)
	rows, err := d.conn.Query(`
		SELECT w.song_id, t.source_id, t.rating_key, s.title, s.artist, s.album,
		       w.genres, w.original_year, w.generated_at
		FROM plex_ai_metadata_writeback w
		JOIN plex_tracks t ON t.song_id = w.song_id
		JOIN songs s ON s.id = w.song_id
		WHERE t.source_id = ?
		  AND (w.synced_at IS NULL OR w.generated_at > w.synced_at)
		  AND (w.genres <> '[]' OR w.original_year > 0)`+filter+`
		ORDER BY w.generated_at ASC, w.song_id ASC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list Plex AI metadata writeback candidates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var candidate PlexAIWritebackCandidate
		var genreJSON string
		if err := rows.Scan(&candidate.SongID, &candidate.SourceID, &candidate.RatingKey, &candidate.Title, &candidate.Artist, &candidate.Album,
			&genreJSON, &candidate.OriginalYear, &candidate.GeneratedAt); err != nil {
			return nil, false, fmt.Errorf("scan Plex AI metadata writeback candidate: %w", err)
		}
		if err := json.Unmarshal([]byte(genreJSON), &candidate.Genres); err != nil {
			return nil, false, fmt.Errorf("decode Plex AI metadata writeback genres: %w", err)
		}
		if len(candidates) == limit {
			hasMore = true
			break
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read Plex AI metadata writeback candidates: %w", err)
	}
	return candidates, hasMore, nil
}

// MarkPlexAIWritebackSynced stores an audit trail only after PMS accepts the
// write and a fresh metadata response verifies the requested values.
func (d *DB) MarkPlexAIWritebackSynced(songID string, approvedAt, syncedAt int64) error {
	if err := d.EnsurePlexSchema(); err != nil {
		return err
	}
	_, err := d.conn.Exec(`UPDATE plex_ai_metadata_writeback SET approved_at=?, synced_at=?, last_error='' WHERE song_id=?`, approvedAt, syncedAt, songID)
	return err
}

// MarkPlexAIWritebackFailed keeps a proposal pending while retaining a bounded,
// sanitized diagnostic for a later retry.
func (d *DB) MarkPlexAIWritebackFailed(songID, message string) error {
	if err := d.EnsurePlexSchema(); err != nil {
		return err
	}
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := d.conn.Exec(`UPDATE plex_ai_metadata_writeback SET last_error=? WHERE song_id=?`, message, songID)
	return err
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

func plexArtistArtworkPath(artistName, artworkKey string, updatedAt int64) string {
	if artistName == "" || artworkKey == "" {
		return ""
	}
	version := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", artworkKey, updatedAt)))
	return "/api/v2/plex/artist-artwork/" + url.PathEscape(artistName) + "?v=" + fmt.Sprintf("%x", version[:8])
}

type existingPlexCatalogTrack struct {
	updatedAt        int64
	libraryID        string
	metadataKey      string
	mediaKey         string
	artworkKey       string
	artistArtworkKey string
	container        string
	audioCodec       string
	genres           []string
	year             int
}

func (record existingPlexCatalogTrack) matches(libraryID string, track PlexCatalogTrack, genres []string) bool {
	// updatedAt is the only PMS metadata version signal persisted today. If PMS
	// omits it (zero), be conservative and refresh the row rather than assuming
	// title/artist/album/etc. are unchanged merely because source keys match.
	return track.UpdatedAt > 0 &&
		record.libraryID == libraryID &&
		record.updatedAt == track.UpdatedAt &&
		record.metadataKey == track.MetadataKey &&
		record.mediaKey == track.MediaKey &&
		record.artworkKey == track.ArtworkKey &&
		record.artistArtworkKey == track.ArtistArtworkKey &&
		record.container == track.Container &&
		record.audioCodec == track.AudioCodec &&
		record.year == track.Year &&
		stringSlicesEqual(record.genres, genres)
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type plexArtistArtwork struct {
	key       string
	updatedAt int64
}

func syncPlexArtistArtwork(tx *sql.Tx, sourceID string, artwork map[string]plexArtistArtwork) error {
	// A successful library snapshot is authoritative for artist portraits too.
	// Clear only the Plex-specific URLs; Spotify/local metadata stays intact as a
	// fallback when a server stops providing an artist thumbnail.
	if _, err := tx.Exec(`
		UPDATE artist_metadata SET plex_image_url=NULL
		WHERE artist_name IN (SELECT artist_name FROM plex_artist_artwork WHERE source_id=?)
	`, sourceID); err != nil {
		return fmt.Errorf("clear stale Plex artist artwork URLs: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM plex_artist_artwork WHERE source_id=?`, sourceID); err != nil {
		return fmt.Errorf("clear stale Plex artist artwork: %w", err)
	}
	now := time.Now().Unix()
	for artistName, portrait := range artwork {
		if artistName == "" || portrait.key == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO plex_artist_artwork (source_id, artist_name, artwork_key, updated_at)
			VALUES (?, ?, ?, ?)
		`, sourceID, artistName, portrait.key, portrait.updatedAt); err != nil {
			return fmt.Errorf("save Plex artist artwork: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO artist_metadata (artist_name, plex_image_url, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(artist_name) DO UPDATE SET
				plex_image_url=excluded.plex_image_url,
				updated_at=excluded.updated_at
		`, artistName, plexArtistArtworkPath(artistName, portrait.key, portrait.updatedAt), now); err != nil {
			return fmt.Errorf("publish Plex artist artwork: %w", err)
		}
	}
	return nil
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
		SELECT t.song_id, t.updated_at, t.library_id, t.metadata_key, t.media_key, t.artwork_key, t.artist_artwork_key, t.container, t.audio_codec, s.genre, s.year
		FROM plex_tracks t JOIN songs s ON s.id=t.song_id WHERE t.source_id=?
	`, sourceID)
	if err != nil {
		return 0, 0, 0, err
	}
	for rows.Next() {
		var id string
		var record existingPlexCatalogTrack
		var storedGenres string
		if err := rows.Scan(&id, &record.updatedAt, &record.libraryID, &record.metadataKey, &record.mediaKey,
			&record.artworkKey, &record.artistArtworkKey, &record.container, &record.audioCodec, &storedGenres, &record.year); err != nil {
			rows.Close()
			return 0, 0, 0, err
		}
		_ = json.Unmarshal([]byte(storedGenres), &record.genres)
		existing[id] = record
	}
	if err := rows.Close(); err != nil {
		return 0, 0, 0, err
	}

	seen := make(map[string]struct{}, len(tracks))
	artistArtwork := make(map[string]plexArtistArtwork)
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
		if track.Artist != "" && track.ArtistArtworkKey != "" {
			if _, alreadyMapped := artistArtwork[track.Artist]; !alreadyMapped {
				artistArtwork[track.Artist] = plexArtistArtwork{key: track.ArtistArtworkKey, updatedAt: track.UpdatedAt}
			}
		}
		if track.MediaKey == "" {
			continue
		}
		normalizedGenres := NormalizeGenres(track.Genres)
		previous, exists := existing[track.SongID]
		if exists && previous.matches(libraryID, track, normalizedGenres) {
			continue
		}
		genres, _ := json.Marshal(normalizedGenres)
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
			INSERT INTO plex_tracks (song_id, source_id, library_id, machine_identifier, rating_key, metadata_key, media_key, artwork_key, artist_artwork_key, container, audio_codec, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(song_id) DO UPDATE SET source_id=excluded.source_id, library_id=excluded.library_id,
				machine_identifier=excluded.machine_identifier, rating_key=excluded.rating_key, metadata_key=excluded.metadata_key,
				media_key=excluded.media_key, artwork_key=excluded.artwork_key, artist_artwork_key=excluded.artist_artwork_key, container=excluded.container,
				audio_codec=excluded.audio_codec, updated_at=excluded.updated_at
		`, track.SongID, sourceID, libraryID, track.MachineID, track.RatingKey, track.MetadataKey,
			track.MediaKey, track.ArtworkKey, track.ArtistArtworkKey, track.Container, track.AudioCodec, track.UpdatedAt)
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
	if err := syncPlexArtistArtwork(tx, sourceID, artistArtwork); err != nil {
		return 0, 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, err
	}
	return added, updated, removed, nil
}

// PlexArtistArtwork identifies the authenticated PMS artwork needed for an
// artist portrait. It never includes a browser-visible Plex token.
type PlexArtistArtwork struct {
	SourceID   string
	ArtworkKey string
}

func (d *DB) GetActivePlexArtistArtwork(artistName string) (*PlexArtistArtwork, error) {
	if err := d.EnsurePlexSchema(); err != nil {
		return nil, err
	}
	var artwork PlexArtistArtwork
	err := d.conn.QueryRow(`
		SELECT a.source_id, a.artwork_key
		FROM plex_artist_artwork a
		JOIN plex_sources s ON s.id=a.source_id
		WHERE s.active=1 AND a.artist_name=? COLLATE NOCASE
		LIMIT 1
	`, artistName).Scan(&artwork.SourceID, &artwork.ArtworkKey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &artwork, nil
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
	if _, err := tx.Exec(`
		UPDATE artist_metadata SET plex_image_url=NULL
		WHERE artist_name IN (SELECT artist_name FROM plex_artist_artwork WHERE source_id=?)
	`, sourceID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM plex_artist_artwork WHERE source_id=?`, sourceID); err != nil {
		return err
	}
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
