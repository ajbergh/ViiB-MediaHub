// Package db provides SQLite database access for ViiB MediaHub.
//
// Schema includes tables for:
//   - songs: Audio file metadata with paths, usage stats, and AI analysis fields
//   - playlists: User-created playlists with song references
//   - plays: Play history with timestamps for tracking listening patterns
//   - scan_folders: Configured directories to scan for music
//   - settings: Key-value store for application configuration (with encryption for secrets)
//   - spotify_downloads: Download queue with status tracking
//   - album_metadata: Cached Spotify album metadata
//   - artist_metadata: Cached Spotify artist metadata
//   - indexed_genres: Pre-computed genre lists for fast lookup
//
// AI DJ Support:
//   - Mood/energy/tempo/BPM fields in songs table (populated by Gemini AI)
//   - Play history queries for discovery mode and recently played filtering
//   - Artist preference tracking based on cumulative play counts
//   - Genre indexing for fast local matching without API calls
//
// Security:
//   - Sensitive settings (Spotify credentials, API keys) are encrypted at rest
//   - Uses AES-256-GCM encryption with machine-bound keys
//   - Automatic migration of plaintext secrets to encrypted format
//   - LIKE pattern wildcards are escaped to prevent wildcard injection
//   - See internal/crypto package for encryption implementation
//
// Uses SQLite with WAL mode for concurrent access and foreign keys enabled.
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/crypto"
	_ "github.com/mattn/go-sqlite3"
)

// escapeLikePattern escapes SQL LIKE wildcards (%, _) in a string.
// This prevents user input from being interpreted as wildcards, which could
// cause unintended matches (e.g., "%" would match all records).
//
// SQLite uses backslash as the escape character by default with ESCAPE clause,
// but for simpler queries, we escape by doubling the wildcards or using
// a different approach. Since we're matching within JSON strings, we also
// need to preserve the JSON structure.
//
// For SQLite with ESCAPE clause: LIKE pattern ESCAPE '\'
//   - % -> \%
//   - _ -> \_
//   - \ -> \\
func escapeLikePattern(s string) string {
	// Escape backslashes first, then wildcards
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// buildGenreLikePattern creates a safe LIKE pattern for matching a genre name
// within a JSON array. Escapes any LIKE wildcards in the genre name.
func buildGenreLikePattern(genreName string) string {
	escaped := escapeLikePattern(genreName)
	return "%\"" + escaped + "\"%"
}

// DB is the primary SQLite database wrapper. It provides methods for managing
// songs, playlists, scan folders, Spotify downloads, and AI DJ features
// including play history tracking and mood analysis.
type DB struct {
	conn *sql.DB
}

// Song represents a persisted audio track with metadata and file locations
// stored in the database.
type Song struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Artist         string   `json:"artist"`
	Album          string   `json:"album"`
	AlbumArtist    string   `json:"albumArtist,omitempty"`
	TrackNumber    int      `json:"trackNumber,omitempty"`
	DiscNumber     int      `json:"discNumber,omitempty"`
	Genre          []string `json:"genre,omitempty"`
	Year           int      `json:"year,omitempty"`
	Duration       float64  `json:"duration"`
	FilePath       string   `json:"filePath"`
	CoverPath      string   `json:"coverPath,omitempty"`
	AddedAt        int64    `json:"addedAt"`
	PlayCount      int      `json:"playCount,omitempty"`
	LastPlayed     int64    `json:"lastPlayed,omitempty"`
	SkipCount      int      `json:"skipCount,omitempty"`
	FileHash       string   `json:"fileHash,omitempty"`
	Mood           string   `json:"mood,omitempty"`           // e.g., "happy", "sad", "energetic", "calm"
	Energy         string   `json:"energy,omitempty"`         // e.g., "high", "medium", "low"
	Tempo          string   `json:"tempo,omitempty"`          // e.g., "fast", "medium", "slow"
	BPM            int      `json:"bpm,omitempty"`            // Beats per minute (if analyzed)
	Instrumental   bool     `json:"instrumental,omitempty"`   // true if song has no vocals
	MoodAnalyzedAt int64    `json:"moodAnalyzedAt,omitempty"` // Timestamp of mood analysis
}

// Playlist represents a user-defined playlist persisted in the database.
type Playlist struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	SongIDs   []string `json:"songIds"`
	CoverPath string   `json:"coverPath,omitempty"`
	CreatedAt int64    `json:"createdAt"`
}

// ScanFolder represents a configured filesystem folder to scan for music
// files and their scan status.
type ScanFolder struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	AddedAt   int64  `json:"addedAt"`
	LastScan  int64  `json:"lastScan,omitempty"`
	SongCount int    `json:"songCount"`
}

// DirectorySignature stores a compact representation of directory state
// for quick change detection without walking contents.
type DirectorySignature struct {
	Path         string `json:"path"`
	FileCount    int    `json:"fileCount"`
	TotalSize    int64  `json:"totalSize"`
	LatestMtime  int64  `json:"latestMtime"`
	ContentHash  string `json:"contentHash"`
	LastVerified int64  `json:"lastVerified"`
}

// ScanState stores global scan state for journal-based change detection.
type ScanState struct {
	LastScanTime   int64 `json:"lastScanTime"`
	WindowsUSN     int64 `json:"windowsUsn,omitempty"`
	MacOSEventID   int64 `json:"macosEventId,omitempty"`
	LinuxLastMtime int64 `json:"linuxLastMtime,omitempty"`
	ScanDurationMs int64 `json:"scanDurationMs,omitempty"`
	FilesScanned   int   `json:"filesScanned,omitempty"`
	FilesChanged   int   `json:"filesChanged,omitempty"`
}

// FileMetadataCache stores file-level change detection data.
type FileMetadataCache struct {
	FilePath     string `json:"filePath"`
	FileSize     int64  `json:"fileSize"`
	Mtime        int64  `json:"mtime"`
	MetadataHash string `json:"metadataHash,omitempty"`
	LastVerified int64  `json:"lastVerified"`
}

// New opens the SQLite database located at dbPath and returns a configured
// DB instance ready for queries and updates.
func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{conn: conn}

	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

// Close releases the underlying database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS songs (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		artist TEXT NOT NULL,
		album TEXT NOT NULL,
		album_artist TEXT,
		track_number INTEGER,
		disc_number INTEGER,
		genre TEXT,
		year INTEGER,
		duration REAL,
		file_path TEXT UNIQUE NOT NULL,
		cover_path TEXT,
		added_at INTEGER NOT NULL,
		play_count INTEGER DEFAULT 0,
		last_played INTEGER,
		skip_count INTEGER DEFAULT 0,
		file_hash TEXT,
		mood TEXT,
		energy TEXT,
		tempo TEXT,
		bpm INTEGER,
		instrumental INTEGER DEFAULT 0,
		mood_analyzed_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_songs_album ON songs(album);
	CREATE INDEX IF NOT EXISTS idx_songs_artist ON songs(artist);
	CREATE INDEX IF NOT EXISTS idx_songs_file_path ON songs(file_path);

	CREATE TABLE IF NOT EXISTS playlists (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		song_ids TEXT NOT NULL,
		cover_path TEXT,
		created_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS scan_folders (
		id TEXT PRIMARY KEY,
		path TEXT UNIQUE NOT NULL,
		added_at INTEGER NOT NULL,
		last_scan INTEGER,
		song_count INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS genre_stats (
		name TEXT PRIMARY KEY,
		count INTEGER NOT NULL,
		artists TEXT NOT NULL, -- JSON array of top artists
		cover_url TEXT
	);

	CREATE TABLE IF NOT EXISTS spotify_downloads (
		id TEXT PRIMARY KEY,
		spotify_id TEXT NOT NULL,
		spotify_uri TEXT NOT NULL,
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		artist TEXT,
		album TEXT,
		status TEXT NOT NULL,
		progress INTEGER DEFAULT 0,
		error TEXT,
		file_path TEXT,
		added_at INTEGER NOT NULL,
		started_at INTEGER,
		completed_at INTEGER,
		metadata TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_downloads_status ON spotify_downloads(status);
	CREATE INDEX IF NOT EXISTS idx_downloads_added_at ON spotify_downloads(added_at);

	CREATE TABLE IF NOT EXISTS album_metadata (
		album_key TEXT PRIMARY KEY,
		album_name TEXT NOT NULL,
		artist_name TEXT NOT NULL,
		spotify_id TEXT,
		cover_url TEXT,
		local_cover_path TEXT,
		description TEXT,
		genre TEXT,
		release_date TEXT,
		spotify_url TEXT,
		copyright TEXT,
		spotify_checked INTEGER DEFAULT 0,
		spotify_found INTEGER DEFAULT 0,
		fetched_at INTEGER,
		updated_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_album_metadata_artist ON album_metadata(artist_name);

	CREATE TABLE IF NOT EXISTS artist_metadata (
		artist_name TEXT PRIMARY KEY,
		spotify_id TEXT,
		image_url TEXT,
		local_image_path TEXT,
		spotify_url TEXT,
		spotify_checked INTEGER DEFAULT 0,
		spotify_found INTEGER DEFAULT 0,
		fetched_at INTEGER,
		updated_at INTEGER
	);

	-- Fast scan: Directory signatures for quick change detection
	CREATE TABLE IF NOT EXISTS directory_signatures (
		path TEXT PRIMARY KEY,
		file_count INTEGER NOT NULL,
		total_size INTEGER NOT NULL,
		latest_mtime INTEGER NOT NULL,
		content_hash TEXT NOT NULL,
		last_verified INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_dir_sig_mtime ON directory_signatures(latest_mtime);

	-- Fast scan: Global scan state for journal-based detection
	CREATE TABLE IF NOT EXISTS scan_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		last_scan_time INTEGER NOT NULL,
		windows_usn INTEGER,
		macos_event_id INTEGER,
		linux_last_mtime INTEGER,
		scan_duration_ms INTEGER,
		files_scanned INTEGER,
		files_changed INTEGER
	);

	-- Fast scan: File metadata cache for cheap change detection
	CREATE TABLE IF NOT EXISTS file_metadata_cache (
		file_path TEXT PRIMARY KEY,
		file_size INTEGER NOT NULL,
		mtime INTEGER NOT NULL,
		metadata_hash TEXT,
		last_verified INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_metadata_cache_mtime ON file_metadata_cache(mtime);
	`

	_, err := d.conn.Exec(schema)
	if err != nil {
		return err
	}

	// Run column migrations for existing databases
	return d.migrateColumns()
}

// migrateColumns adds new columns to existing tables if they don't exist.
// SQLite doesn't support IF NOT EXISTS for ALTER TABLE, so we check first.
func (d *DB) migrateColumns() error {
	// Check for mood column and add mood/energy/tempo columns if missing
	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('songs') WHERE name='mood'`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		migrations := []string{
			`ALTER TABLE songs ADD COLUMN mood TEXT`,
			`ALTER TABLE songs ADD COLUMN energy TEXT`,
			`ALTER TABLE songs ADD COLUMN tempo TEXT`,
			`ALTER TABLE songs ADD COLUMN bpm INTEGER`,
			`ALTER TABLE songs ADD COLUMN instrumental INTEGER DEFAULT 0`,
			`ALTER TABLE songs ADD COLUMN mood_analyzed_at INTEGER`,
		}
		for _, m := range migrations {
			if _, err := d.conn.Exec(m); err != nil {
				// Ignore "duplicate column" errors
				if !strings.Contains(err.Error(), "duplicate column") {
					return err
				}
			}
		}
	}

	// Check for instrumental column (added later) and add if missing
	err = d.conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('songs') WHERE name='instrumental'`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		if _, err := d.conn.Exec(`ALTER TABLE songs ADD COLUMN instrumental INTEGER DEFAULT 0`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}

	// Migrate unencrypted sensitive settings to encrypted format
	if err := d.migrateUnencryptedSettings(); err != nil {
		return err
	}

	return nil
}

// migrateUnencryptedSettings encrypts any existing plaintext sensitive settings.
// This provides backward compatibility when upgrading from versions without encryption.
//
// The migration process:
//  1. Reads the raw value from the database (bypassing GetSetting decryption)
//  2. Checks if the value is already encrypted (has "enc:v1:" prefix)
//  3. If plaintext, encrypts using the crypto package
//  4. Updates the database with the encrypted value
//
// This function is called during database initialization and is idempotent -
// already encrypted values are skipped, and the migration can be run multiple
// times safely.
//
// Protected settings:
//   - spotify_credentials: OAuth tokens and client secrets
//   - gemini_api_key: Google Gemini AI API key
func (d *DB) migrateUnencryptedSettings() error {
	sensitiveKeys := []string{"spotify_credentials", "gemini_api_key"}

	for _, key := range sensitiveKeys {
		// Read raw value directly (bypass GetSetting which would try to decrypt)
		var value string
		err := d.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
		if err == sql.ErrNoRows {
			continue // Setting doesn't exist, nothing to migrate
		}
		if err != nil {
			return err
		}

		// Check if already encrypted
		if value == "" || crypto.IsEncrypted(value) {
			continue // Empty or already encrypted
		}

		// Encrypt and update
		encrypted, err := crypto.Encrypt(value)
		if err != nil {
			return fmt.Errorf("failed to encrypt setting %s during migration: %w", key, err)
		}

		_, err = d.conn.Exec("UPDATE settings SET value = ? WHERE key = ?", encrypted, key)
		if err != nil {
			return fmt.Errorf("failed to update encrypted setting %s: %w", key, err)
		}
	}

	return nil
}

// Song operations

// GetAllSongs returns all songs currently persisted in the library.
func (d *DB) GetAllSongs() ([]Song, error) {
	rows, err := d.conn.Query(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, duration, file_path, cover_path, added_at,
		       play_count, last_played, skip_count, file_hash,
		       mood, energy, tempo, bpm, instrumental, mood_analyzed_at
		FROM songs
		ORDER BY album, disc_number, track_number, title
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON sql.NullString
		var albumArtist, coverPath, fileHash sql.NullString
		var trackNum, discNum, year, playCount, skipCount sql.NullInt64
		var lastPlayed sql.NullInt64
		var mood, energy, tempo sql.NullString
		var bpm, instrumental, moodAnalyzedAt sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist,
			&trackNum, &discNum, &genreJSON, &year,
			&s.Duration, &s.FilePath, &coverPath, &s.AddedAt,
			&playCount, &lastPlayed, &skipCount, &fileHash,
			&mood, &energy, &tempo, &bpm, &instrumental, &moodAnalyzedAt,
		)
		if err != nil {
			return nil, err
		}

		if albumArtist.Valid {
			s.AlbumArtist = albumArtist.String
		}
		if trackNum.Valid {
			s.TrackNumber = int(trackNum.Int64)
		}
		if discNum.Valid {
			s.DiscNumber = int(discNum.Int64)
		}
		if genreJSON.Valid && genreJSON.String != "" {
			json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		if year.Valid {
			s.Year = int(year.Int64)
		}
		if coverPath.Valid {
			s.CoverPath = coverPath.String
		}
		if playCount.Valid {
			s.PlayCount = int(playCount.Int64)
		}
		if lastPlayed.Valid {
			s.LastPlayed = lastPlayed.Int64
		}
		if skipCount.Valid {
			s.SkipCount = int(skipCount.Int64)
		}
		if fileHash.Valid {
			s.FileHash = fileHash.String
		}
		if mood.Valid {
			s.Mood = mood.String
		}
		if energy.Valid {
			s.Energy = energy.String
		}
		if tempo.Valid {
			s.Tempo = tempo.String
		}
		if bpm.Valid {
			s.BPM = int(bpm.Int64)
		}
		if instrumental.Valid {
			s.Instrumental = instrumental.Int64 == 1
		}
		if moodAnalyzedAt.Valid {
			s.MoodAnalyzedAt = moodAnalyzedAt.Int64
		}

		songs = append(songs, s)
	}

	return songs, rows.Err()
}

// SaveSong inserts or updates a single Song record in the database.
func (d *DB) SaveSong(s *Song) error {
	genreJSON, _ := json.Marshal(s.Genre)

	_, err := d.conn.Exec(`
		INSERT INTO songs (
			id, title, artist, album, album_artist, track_number, disc_number,
			genre, year, duration, file_path, cover_path, added_at,
			play_count, last_played, skip_count, file_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			artist = excluded.artist,
			album = excluded.album,
			album_artist = excluded.album_artist,
			track_number = excluded.track_number,
			disc_number = excluded.disc_number,
			genre = excluded.genre,
			year = excluded.year,
			duration = excluded.duration,
			cover_path = excluded.cover_path,
			play_count = excluded.play_count,
			last_played = excluded.last_played,
			skip_count = excluded.skip_count,
			file_hash = excluded.file_hash
	`,
		s.ID, s.Title, s.Artist, s.Album, s.AlbumArtist, s.TrackNumber, s.DiscNumber,
		string(genreJSON), s.Year, s.Duration, s.FilePath, s.CoverPath, s.AddedAt,
		s.PlayCount, s.LastPlayed, s.SkipCount, s.FileHash,
	)
	return err
}

// SaveSongs inserts or updates multiple Song records in a transaction.
// Note: On conflict, we preserve existing genre data if the new genre is empty,
// to avoid overwriting enriched genres with empty metadata from file scans.
func (d *DB) SaveSongs(songs []Song) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO songs (
			id, title, artist, album, album_artist, track_number, disc_number,
			genre, year, duration, file_path, cover_path, added_at,
			play_count, last_played, skip_count, file_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			title = excluded.title,
			artist = excluded.artist,
			album = excluded.album,
			album_artist = excluded.album_artist,
			track_number = excluded.track_number,
			disc_number = excluded.disc_number,
			genre = CASE 
				WHEN excluded.genre IS NULL OR excluded.genre = '' OR excluded.genre = '[]' OR excluded.genre = 'null'
				THEN songs.genre 
				ELSE excluded.genre 
			END,
			year = excluded.year,
			duration = excluded.duration,
			cover_path = excluded.cover_path
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range songs {
		genreJSON, _ := json.Marshal(s.Genre)
		_, err = stmt.Exec(
			s.ID, s.Title, s.Artist, s.Album, s.AlbumArtist, s.TrackNumber, s.DiscNumber,
			string(genreJSON), s.Year, s.Duration, s.FilePath, s.CoverPath, s.AddedAt,
			s.PlayCount, s.LastPlayed, s.SkipCount, s.FileHash,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteSong removes a Song record by ID.
func (d *DB) DeleteSong(id string) error {
	_, err := d.conn.Exec("DELETE FROM songs WHERE id = ?", id)
	return err
}

// UpdateSongMood updates the mood/energy/tempo/instrumental metadata for a song.
func (d *DB) UpdateSongMood(songID, mood, energy, tempo string, bpm int, instrumental bool) error {
	_, err := d.conn.Exec(`
		UPDATE songs SET
			mood = ?,
			energy = ?,
			tempo = ?,
			bpm = ?,
			instrumental = ?,
			mood_analyzed_at = ?
		WHERE id = ?
	`, mood, energy, tempo, bpm, instrumental, time.Now().Unix(), songID)
	return err
}

// GetSongsWithoutMood returns songs that haven't been analyzed for mood yet.
func (d *DB) GetSongsWithoutMood(limit int) ([]Song, error) {
	rows, err := d.conn.Query(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, duration, file_path, cover_path, added_at,
		       play_count, last_played, skip_count, file_hash,
		       mood, energy, tempo, bpm, mood_analyzed_at
		FROM songs
		WHERE mood IS NULL OR mood = ''
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON sql.NullString
		var albumArtist, coverPath, fileHash sql.NullString
		var trackNum, discNum, year, playCount, skipCount sql.NullInt64
		var lastPlayed sql.NullInt64
		var mood, energy, tempo sql.NullString
		var bpm, moodAnalyzedAt sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist,
			&trackNum, &discNum, &genreJSON, &year,
			&s.Duration, &s.FilePath, &coverPath, &s.AddedAt,
			&playCount, &lastPlayed, &skipCount, &fileHash,
			&mood, &energy, &tempo, &bpm, &moodAnalyzedAt,
		)
		if err != nil {
			return nil, err
		}

		if albumArtist.Valid {
			s.AlbumArtist = albumArtist.String
		}
		if trackNum.Valid {
			s.TrackNumber = int(trackNum.Int64)
		}
		if discNum.Valid {
			s.DiscNumber = int(discNum.Int64)
		}
		if genreJSON.Valid && genreJSON.String != "" {
			json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		if year.Valid {
			s.Year = int(year.Int64)
		}
		if coverPath.Valid {
			s.CoverPath = coverPath.String
		}
		if fileHash.Valid {
			s.FileHash = fileHash.String
		}

		songs = append(songs, s)
	}

	return songs, rows.Err()
}

// GetSongsByMoodCriteria returns songs matching mood/energy/tempo criteria.
func (d *DB) GetSongsByMoodCriteria(mood, energy, tempo string, genreNames []string) ([]Song, error) {
	query := `
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, duration, file_path, cover_path, added_at,
		       play_count, last_played, skip_count, file_hash,
		       mood, energy, tempo, bpm, instrumental, mood_analyzed_at
		FROM songs
		WHERE 1=1
	`
	args := []any{}

	if mood != "" {
		query += " AND mood = ?"
		args = append(args, mood)
	}
	if energy != "" {
		query += " AND energy = ?"
		args = append(args, energy)
	}
	if tempo != "" {
		query += " AND tempo = ?"
		args = append(args, tempo)
	}

	// Add genre filter if specified (with LIKE wildcard escaping for security)
	if len(genreNames) > 0 {
		genreConditions := []string{}
		for _, g := range genreNames {
			genreConditions = append(genreConditions, "genre LIKE ? ESCAPE '\\'")
			args = append(args, buildGenreLikePattern(g))
		}
		query += " AND (" + strings.Join(genreConditions, " OR ") + ")"
	}

	query += " LIMIT 50"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON sql.NullString
		var albumArtist, coverPath, fileHash sql.NullString
		var trackNum, discNum, year, playCount, skipCount sql.NullInt64
		var lastPlayed sql.NullInt64
		var moodVal, energyVal, tempoVal sql.NullString
		var bpm, instrumental, moodAnalyzedAt sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist,
			&trackNum, &discNum, &genreJSON, &year,
			&s.Duration, &s.FilePath, &coverPath, &s.AddedAt,
			&playCount, &lastPlayed, &skipCount, &fileHash,
			&moodVal, &energyVal, &tempoVal, &bpm, &instrumental, &moodAnalyzedAt,
		)
		if err != nil {
			return nil, err
		}

		if albumArtist.Valid {
			s.AlbumArtist = albumArtist.String
		}
		if trackNum.Valid {
			s.TrackNumber = int(trackNum.Int64)
		}
		if discNum.Valid {
			s.DiscNumber = int(discNum.Int64)
		}
		if genreJSON.Valid && genreJSON.String != "" {
			json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		if year.Valid {
			s.Year = int(year.Int64)
		}
		if coverPath.Valid {
			s.CoverPath = coverPath.String
		}
		if fileHash.Valid {
			s.FileHash = fileHash.String
		}
		if moodVal.Valid {
			s.Mood = moodVal.String
		}
		if energyVal.Valid {
			s.Energy = energyVal.String
		}
		if tempoVal.Valid {
			s.Tempo = tempoVal.String
		}
		if bpm.Valid {
			s.BPM = int(bpm.Int64)
		}
		if instrumental.Valid {
			s.Instrumental = instrumental.Int64 == 1
		}
		if moodAnalyzedAt.Valid {
			s.MoodAnalyzedAt = moodAnalyzedAt.Int64
		}

		songs = append(songs, s)
	}

	return songs, rows.Err()
}

// ClearSongs deletes all songs from the database (dangerous).
func (d *DB) ClearSongs() error {
	_, err := d.conn.Exec("DELETE FROM songs")
	return err
}

// GetAllFilePaths returns all file paths currently in the songs table
func (d *DB) GetAllFilePaths() ([]string, error) {
	rows, err := d.conn.Query("SELECT file_path FROM songs")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

// DeleteSongsByFilePaths deletes songs with the given file paths and returns the count deleted
func (d *DB) DeleteSongsByFilePaths(paths []string) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("DELETE FROM songs WHERE file_path = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	deleted := 0
	for _, path := range paths {
		result, err := stmt.Exec(path)
		if err != nil {
			return deleted, err
		}
		affected, _ := result.RowsAffected()
		deleted += int(affected)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// GetSongByID returns a Song by its ID or nil if not found.
func (d *DB) GetSongByID(id string) (*Song, error) {
	var s Song
	var genreJSON sql.NullString
	var albumArtist, coverPath, fileHash sql.NullString
	var trackNum, discNum, year, playCount, skipCount sql.NullInt64
	var lastPlayed sql.NullInt64

	err := d.conn.QueryRow(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, duration, file_path, cover_path, added_at,
		       play_count, last_played, skip_count, file_hash
		FROM songs WHERE id = ?
	`, id).Scan(
		&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist,
		&trackNum, &discNum, &genreJSON, &year,
		&s.Duration, &s.FilePath, &coverPath, &s.AddedAt,
		&playCount, &lastPlayed, &skipCount, &fileHash,
	)
	if err != nil {
		return nil, err
	}

	if albumArtist.Valid {
		s.AlbumArtist = albumArtist.String
	}
	if trackNum.Valid {
		s.TrackNumber = int(trackNum.Int64)
	}
	if discNum.Valid {
		s.DiscNumber = int(discNum.Int64)
	}
	if genreJSON.Valid && genreJSON.String != "" {
		json.Unmarshal([]byte(genreJSON.String), &s.Genre)
	}
	if year.Valid {
		s.Year = int(year.Int64)
	}
	if coverPath.Valid {
		s.CoverPath = coverPath.String
	}
	if playCount.Valid {
		s.PlayCount = int(playCount.Int64)
	}
	if lastPlayed.Valid {
		s.LastPlayed = lastPlayed.Int64
	}
	if skipCount.Valid {
		s.SkipCount = int(skipCount.Int64)
	}
	if fileHash.Valid {
		s.FileHash = fileHash.String
	}

	return &s, nil
}

// UpdatePlayCount increments the play count for the given song ID.
func (d *DB) UpdatePlayCount(id string) error {
	_, err := d.conn.Exec(`
		UPDATE songs 
		SET play_count = play_count + 1, last_played = ?
		WHERE id = ?
	`, time.Now().UnixMilli(), id)
	return err
}

// GetRecentlyPlayedSongIDs returns IDs of songs played in the last N hours.
func (d *DB) GetRecentlyPlayedSongIDs(hours int) ([]string, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).UnixMilli()
	rows, err := d.conn.Query(`
		SELECT id FROM songs 
		WHERE last_played > ?
		ORDER BY last_played DESC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetFrequentlyPlayedSongs returns songs ordered by play count (most played first).
func (d *DB) GetFrequentlyPlayedSongs(limit int) ([]Song, error) {
	rows, err := d.conn.Query(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, duration, file_path, cover_path, added_at,
		       play_count, last_played, skip_count, file_hash,
		       mood, energy, tempo, bpm, instrumental, mood_analyzed_at
		FROM songs
		WHERE play_count > 0
		ORDER BY play_count DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return d.scanSongsWithMood(rows)
}

// GetUnderplayedSongs returns songs with low play counts that match criteria.
func (d *DB) GetUnderplayedSongs(genreNames []string, maxPlayCount int, limit int) ([]Song, error) {
	query := `
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, duration, file_path, cover_path, added_at,
		       play_count, last_played, skip_count, file_hash,
		       mood, energy, tempo, bpm, instrumental, mood_analyzed_at
		FROM songs
		WHERE play_count <= ?
	`
	args := []any{maxPlayCount}

	if len(genreNames) > 0 {
		genreConditions := []string{}
		for _, g := range genreNames {
			genreConditions = append(genreConditions, "genre LIKE ?")
			args = append(args, "%\""+g+"\"%")
		}
		query += " AND (" + strings.Join(genreConditions, " OR ") + ")"
	}

	query += " ORDER BY RANDOM() LIMIT ?"
	args = append(args, limit)

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return d.scanSongsWithMood(rows)
}

// GetFavoriteArtists returns artist names ordered by total play count across all songs.
// Used by the AI DJ to identify user preferences for artist-aware recommendations.
func (d *DB) GetFavoriteArtists(limit int) ([]string, error) {
	rows, err := d.conn.Query(`
		SELECT artist, SUM(play_count) as total_plays
		FROM songs
		WHERE play_count > 0
		GROUP BY artist
		ORDER BY total_plays DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artists []string
	for rows.Next() {
		var artist string
		var totalPlays int
		if err := rows.Scan(&artist, &totalPlays); err != nil {
			return nil, err
		}
		artists = append(artists, artist)
	}
	return artists, rows.Err()
}

// ArtistPlayStats holds aggregated play statistics for an artist.
// Used to calculate artist affinity bonuses in playlist scoring.
type ArtistPlayStats struct {
	Artist     string // Artist name
	TotalPlays int    // Sum of play_count across all songs
	SongCount  int    // Number of songs by this artist
}

// GetArtistPlayStats returns play statistics for all artists that have been played.
// Returns a map keyed by artist name for O(1) lookups during playlist generation.
func (d *DB) GetArtistPlayStats() (map[string]ArtistPlayStats, error) {
	rows, err := d.conn.Query(`
		SELECT artist, SUM(play_count) as total_plays, COUNT(*) as song_count
		FROM songs
		WHERE play_count > 0
		GROUP BY artist
		ORDER BY total_plays DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]ArtistPlayStats)
	for rows.Next() {
		var s ArtistPlayStats
		if err := rows.Scan(&s.Artist, &s.TotalPlays, &s.SongCount); err != nil {
			return nil, err
		}
		stats[s.Artist] = s
	}
	return stats, rows.Err()
}

// GetSongsByArtist returns all songs by a specific artist (case-insensitive match).
// Used for "more like [artist]" prompts to seed artist-based playlists.
func (d *DB) GetSongsByArtist(artistName string) ([]Song, error) {
	rows, err := d.conn.Query(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, duration, file_path, cover_path, added_at,
		       play_count, last_played, skip_count, file_hash,
		       mood, energy, tempo, bpm, instrumental, mood_analyzed_at
		FROM songs
		WHERE LOWER(artist) = LOWER(?)
		ORDER BY album, track_number
	`, artistName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return d.scanSongsWithMood(rows)
}

// GetSimilarArtists finds other artists in the library that share genres with
// the given artist. Used to expand artist-based playlists with similar music.
// Returns up to 'limit' artists, ordered by play count (most played first).
func (d *DB) GetSimilarArtists(artistName string, limit int) ([]string, error) {
	// First get genres for the given artist
	var genreJSON sql.NullString
	err := d.conn.QueryRow(`
		SELECT genre FROM songs 
		WHERE LOWER(artist) = LOWER(?) AND genre IS NOT NULL 
		LIMIT 1
	`, artistName).Scan(&genreJSON)
	if err != nil {
		return nil, err
	}

	if !genreJSON.Valid || genreJSON.String == "" {
		return nil, nil
	}

	var genres []string
	if err := json.Unmarshal([]byte(genreJSON.String), &genres); err != nil {
		return nil, nil
	}

	if len(genres) == 0 {
		return nil, nil
	}

	// Find other artists with similar genres (with LIKE wildcard escaping for security)
	genreConditions := []string{}
	args := []any{}
	for _, g := range genres {
		genreConditions = append(genreConditions, "genre LIKE ? ESCAPE '\\'")
		args = append(args, buildGenreLikePattern(g))
	}
	args = append(args, artistName, limit)

	query := fmt.Sprintf(`
		SELECT DISTINCT artist 
		FROM songs 
		WHERE (%s) AND LOWER(artist) != LOWER(?)
		ORDER BY play_count DESC
		LIMIT ?
	`, strings.Join(genreConditions, " OR "))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artists []string
	for rows.Next() {
		var artist string
		if err := rows.Scan(&artist); err != nil {
			return nil, err
		}
		artists = append(artists, artist)
	}
	return artists, rows.Err()
}

// scanSongsWithMood is a helper to scan song rows including mood fields.
func (d *DB) scanSongsWithMood(rows *sql.Rows) ([]Song, error) {
	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON sql.NullString
		var albumArtist, coverPath, fileHash sql.NullString
		var trackNum, discNum, year, playCount, skipCount sql.NullInt64
		var lastPlayed sql.NullInt64
		var mood, energy, tempo sql.NullString
		var bpm, instrumental, moodAnalyzedAt sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist,
			&trackNum, &discNum, &genreJSON, &year,
			&s.Duration, &s.FilePath, &coverPath, &s.AddedAt,
			&playCount, &lastPlayed, &skipCount, &fileHash,
			&mood, &energy, &tempo, &bpm, &instrumental, &moodAnalyzedAt,
		)
		if err != nil {
			return nil, err
		}

		if albumArtist.Valid {
			s.AlbumArtist = albumArtist.String
		}
		if trackNum.Valid {
			s.TrackNumber = int(trackNum.Int64)
		}
		if discNum.Valid {
			s.DiscNumber = int(discNum.Int64)
		}
		if genreJSON.Valid && genreJSON.String != "" {
			json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		if year.Valid {
			s.Year = int(year.Int64)
		}
		if coverPath.Valid {
			s.CoverPath = coverPath.String
		}
		if playCount.Valid {
			s.PlayCount = int(playCount.Int64)
		}
		if lastPlayed.Valid {
			s.LastPlayed = lastPlayed.Int64
		}
		if skipCount.Valid {
			s.SkipCount = int(skipCount.Int64)
		}
		if fileHash.Valid {
			s.FileHash = fileHash.String
		}
		if mood.Valid {
			s.Mood = mood.String
		}
		if energy.Valid {
			s.Energy = energy.String
		}
		if tempo.Valid {
			s.Tempo = tempo.String
		}
		if bpm.Valid {
			s.BPM = int(bpm.Int64)
		}
		if instrumental.Valid {
			s.Instrumental = instrumental.Int64 == 1
		}
		if moodAnalyzedAt.Valid {
			s.MoodAnalyzedAt = moodAnalyzedAt.Int64
		}

		songs = append(songs, s)
	}

	return songs, rows.Err()
}

// Playlist operations

// GetAllPlaylists returns all persisted playlists.
func (d *DB) GetAllPlaylists() ([]Playlist, error) {
	rows, err := d.conn.Query(`SELECT id, name, song_ids, cover_path, created_at FROM playlists ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var playlists []Playlist
	for rows.Next() {
		var p Playlist
		var songIDsJSON string
		var coverPath sql.NullString

		err := rows.Scan(&p.ID, &p.Name, &songIDsJSON, &coverPath, &p.CreatedAt)
		if err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(songIDsJSON), &p.SongIDs)
		if coverPath.Valid {
			p.CoverPath = coverPath.String
		}

		playlists = append(playlists, p)
	}

	return playlists, rows.Err()
}

// SavePlaylist creates or updates a playlist record.
func (d *DB) SavePlaylist(p *Playlist) error {
	songIDsJSON, _ := json.Marshal(p.SongIDs)

	_, err := d.conn.Exec(`
		INSERT INTO playlists (id, name, song_ids, cover_path, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			song_ids = excluded.song_ids,
			cover_path = excluded.cover_path
	`, p.ID, p.Name, string(songIDsJSON), p.CoverPath, p.CreatedAt)
	return err
}

// DeletePlaylist deletes a playlist by ID.
func (d *DB) DeletePlaylist(id string) error {
	_, err := d.conn.Exec("DELETE FROM playlists WHERE id = ?", id)
	return err
}

// Scan folder operations

// GetScanFolders returns the configured scan folders for the library.
func (d *DB) GetScanFolders() ([]ScanFolder, error) {
	rows, err := d.conn.Query(`SELECT id, path, added_at, last_scan, song_count FROM scan_folders ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []ScanFolder
	for rows.Next() {
		var f ScanFolder
		var lastScan sql.NullInt64

		err := rows.Scan(&f.ID, &f.Path, &f.AddedAt, &lastScan, &f.SongCount)
		if err != nil {
			return nil, err
		}

		if lastScan.Valid {
			f.LastScan = lastScan.Int64
		}

		folders = append(folders, f)
	}

	return folders, rows.Err()
}

// AddScanFolder adds a new folder to be scanned for music files.
func (d *DB) AddScanFolder(f *ScanFolder) error {
	_, err := d.conn.Exec(`
		INSERT INTO scan_folders (id, path, added_at, song_count)
		VALUES (?, ?, ?, 0)
		ON CONFLICT(path) DO NOTHING
	`, f.ID, f.Path, f.AddedAt)
	return err
}

// UpdateScanFolder updates the scan timestamp and song count for a folder.
func (d *DB) UpdateScanFolder(id string, lastScan int64, songCount int) error {
	_, err := d.conn.Exec(`
		UPDATE scan_folders SET last_scan = ?, song_count = ? WHERE id = ?
	`, lastScan, songCount, id)
	return err
}

// RemoveScanFolder removes a configured scan folder.
func (d *DB) RemoveScanFolder(id string) error {
	_, err := d.conn.Exec("DELETE FROM scan_folders WHERE id = ?", id)
	return err
}

// Spotify Download operations

// SpotifyDownload represents an item in the Spotify download queue persisted
// in the database. It contains metadata about the requested download and
// its current progress/status.
type SpotifyDownload struct {
	ID          string `json:"id"`
	SpotifyID   string `json:"spotifyId"`
	SpotifyURI  string `json:"spotifyUri"`
	Type        string `json:"type"` // "track", "album", "playlist"
	Title       string `json:"title"`
	Artist      string `json:"artist,omitempty"`
	Album       string `json:"album,omitempty"`
	Status      string `json:"status"` // "queued", "downloading", "completed", "failed"
	Progress    int    `json:"progress"`
	Error       string `json:"error,omitempty"`
	FilePath    string `json:"filePath,omitempty"`
	AddedAt     int64  `json:"addedAt"`
	StartedAt   int64  `json:"startedAt,omitempty"`
	CompletedAt int64  `json:"completedAt,omitempty"`
	Metadata    string `json:"metadata,omitempty"` // JSON string for additional data
}

// AddDownload inserts a new SpotifyDownload into the download queue.
// AddDownload inserts a new SpotifyDownload into the download queue table.
func (d *DB) AddDownload(download *SpotifyDownload) error {
	_, err := d.conn.Exec(`
		INSERT INTO spotify_downloads (
			id, spotify_id, spotify_uri, type, title, artist, album,
			status, progress, error, file_path, added_at, started_at,
			completed_at, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, download.ID, download.SpotifyID, download.SpotifyURI, download.Type,
		download.Title, download.Artist, download.Album, download.Status,
		download.Progress, download.Error, download.FilePath, download.AddedAt,
		download.StartedAt, download.CompletedAt, download.Metadata)
	return err
}

// GetDownload retrieves a single SpotifyDownload by ID.
// GetDownload retrieves a SpotifyDownload by ID.
func (d *DB) GetDownload(id string) (*SpotifyDownload, error) {
	var dl SpotifyDownload
	var artist, album, errorMsg, filePath, metadata sql.NullString
	var startedAt, completedAt sql.NullInt64

	err := d.conn.QueryRow(`
		SELECT id, spotify_id, spotify_uri, type, title, artist, album,
		       status, progress, error, file_path, added_at, started_at,
		       completed_at, metadata
		FROM spotify_downloads WHERE id = ?
	`, id).Scan(
		&dl.ID, &dl.SpotifyID, &dl.SpotifyURI, &dl.Type, &dl.Title,
		&artist, &album, &dl.Status, &dl.Progress, &errorMsg, &filePath,
		&dl.AddedAt, &startedAt, &completedAt, &metadata,
	)

	if err != nil {
		return nil, err
	}

	if artist.Valid {
		dl.Artist = artist.String
	}
	if album.Valid {
		dl.Album = album.String
	}
	if errorMsg.Valid {
		dl.Error = errorMsg.String
	}
	if filePath.Valid {
		dl.FilePath = filePath.String
	}
	if startedAt.Valid {
		dl.StartedAt = startedAt.Int64
	}
	if completedAt.Valid {
		dl.CompletedAt = completedAt.Int64
	}
	if metadata.Valid {
		dl.Metadata = metadata.String
	}

	return &dl, nil
}

// GetAllDownloads returns all Spotify downloads (completed, queued, failed).
// GetAllDownloads returns all Spotify downloads stored in the DB.
func (d *DB) GetAllDownloads() ([]SpotifyDownload, error) {
	rows, err := d.conn.Query(`
		SELECT id, spotify_id, spotify_uri, type, title, artist, album,
		       status, progress, error, file_path, added_at, started_at,
		       completed_at, metadata
		FROM spotify_downloads
		ORDER BY added_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	downloads := []SpotifyDownload{}
	for rows.Next() {
		var dl SpotifyDownload
		var artist, album, errorMsg, filePath, metadata sql.NullString
		var startedAt, completedAt sql.NullInt64

		err := rows.Scan(
			&dl.ID, &dl.SpotifyID, &dl.SpotifyURI, &dl.Type, &dl.Title,
			&artist, &album, &dl.Status, &dl.Progress, &errorMsg, &filePath,
			&dl.AddedAt, &startedAt, &completedAt, &metadata,
		)
		if err != nil {
			return nil, err
		}

		if artist.Valid {
			dl.Artist = artist.String
		}
		if album.Valid {
			dl.Album = album.String
		}
		if errorMsg.Valid {
			dl.Error = errorMsg.String
		}
		if filePath.Valid {
			dl.FilePath = filePath.String
		}
		if startedAt.Valid {
			dl.StartedAt = startedAt.Int64
		}
		if completedAt.Valid {
			dl.CompletedAt = completedAt.Int64
		}
		if metadata.Valid {
			dl.Metadata = metadata.String
		}

		downloads = append(downloads, dl)
	}

	return downloads, rows.Err()
}

// GetQueuedDownloads returns queued Spotify downloads ordered by added time.
func (d *DB) GetQueuedDownloads() ([]SpotifyDownload, error) {
	rows, err := d.conn.Query(`
		SELECT id, spotify_id, spotify_uri, type, title, artist, album,
		       status, progress, error, file_path, added_at, started_at,
		       completed_at, metadata
		FROM spotify_downloads
		WHERE status = 'queued'
		ORDER BY added_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	downloads := []SpotifyDownload{}
	for rows.Next() {
		var dl SpotifyDownload
		var artist, album, errorMsg, filePath, metadata sql.NullString
		var startedAt, completedAt sql.NullInt64

		err := rows.Scan(
			&dl.ID, &dl.SpotifyID, &dl.SpotifyURI, &dl.Type, &dl.Title,
			&artist, &album, &dl.Status, &dl.Progress, &errorMsg, &filePath,
			&dl.AddedAt, &startedAt, &completedAt, &metadata,
		)
		if err != nil {
			return nil, err
		}

		if artist.Valid {
			dl.Artist = artist.String
		}
		if album.Valid {
			dl.Album = album.String
		}
		if errorMsg.Valid {
			dl.Error = errorMsg.String
		}
		if filePath.Valid {
			dl.FilePath = filePath.String
		}
		if startedAt.Valid {
			dl.StartedAt = startedAt.Int64
		}
		if completedAt.Valid {
			dl.CompletedAt = completedAt.Int64
		}
		if metadata.Valid {
			dl.Metadata = metadata.String
		}

		downloads = append(downloads, dl)
	}

	return downloads, rows.Err()
}

// UpdateDownloadStatus sets the status, progress, and optional error message
// for the given SpotifyDownload ID.
func (d *DB) UpdateDownloadStatus(id string, status string, progress int, errorMsg string) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = ?, progress = ?, error = ?
		WHERE id = ?
	`, status, progress, errorMsg, id)
	return err
}

// UpdateDownloadProgress updates the progress percentage for a download.
func (d *DB) UpdateDownloadProgress(id string, progress int) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads SET progress = ? WHERE id = ?
	`, progress, id)
	return err
}

// MarkDownloadStarted marks a download as started and records a timestamp.
func (d *DB) MarkDownloadStarted(id string) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'downloading', started_at = ?
		WHERE id = ?
	`, time.Now().Unix(), id)
	return err
}

// MarkDownloadCompleted marks a download as complete and stores the file path.
func (d *DB) MarkDownloadCompleted(id string, filePath string) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'completed', progress = 100, file_path = ?, completed_at = ?
		WHERE id = ?
	`, filePath, time.Now().Unix(), id)
	return err
}

// MarkDownloadFailed marks a download as failed and records an error message.
func (d *DB) MarkDownloadFailed(id string, errorMsg string) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'failed', error = ?, completed_at = ?
		WHERE id = ?
	`, errorMsg, time.Now().Unix(), id)
	return err
}

// DeleteDownload removes a download record from the database.
func (d *DB) DeleteDownload(id string) error {
	_, err := d.conn.Exec("DELETE FROM spotify_downloads WHERE id = ?", id)
	return err
}

// ResetDownloadForRetry resets a failed download back to queued status
func (d *DB) ResetDownloadForRetry(id string) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'queued', progress = 0, error = NULL, completed_at = NULL, added_at = ?
		WHERE id = ? AND status = 'failed'
	`, time.Now().Unix(), id)
	return err
}

// DeleteCompletedDownloads removes all downloads with status 'completed'
func (d *DB) DeleteCompletedDownloads() (int64, error) {
	result, err := d.conn.Exec("DELETE FROM spotify_downloads WHERE status = 'completed'")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Settings operations

// GetSetting retrieves a value for a configuration key from the settings table.
//
// Encryption Handling:
// Sensitive keys (spotify_credentials, gemini_api_key) are automatically decrypted
// using the machine-bound encryption key. If decryption fails (e.g., database was
// moved to a different machine), an error is returned to prevent exposing
// corrupted or encrypted data.
//
// Returns:
//   - The setting value (decrypted if sensitive)
//   - Empty string if the key doesn't exist
//   - Error if decryption fails or database error occurs
func (d *DB) GetSetting(key string) (string, error) {
	var value string
	err := d.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	// Decrypt sensitive settings
	if crypto.IsSensitiveKey(key) {
		decrypted, err := crypto.Decrypt(value)
		if err != nil {
			// Log error but return empty to avoid exposing encrypted data
			return "", fmt.Errorf("failed to decrypt setting %s: %w", key, err)
		}
		return decrypted, nil
	}

	return value, nil
}

// SetSetting sets a value for a configuration key in the settings table.
//
// Encryption Handling:
// Sensitive keys (spotify_credentials, gemini_api_key) are automatically encrypted
// using AES-256-GCM before storage. The encryption key is derived from machine-
// specific identifiers, binding the encrypted data to this installation.
//
// Upsert Behavior:
// Uses INSERT ... ON CONFLICT to either insert a new setting or update an
// existing one. This is an atomic operation.
//
// Empty Value Handling:
// Empty strings are stored without encryption to avoid unnecessary processing.
func (d *DB) SetSetting(key, value string) error {
	storeValue := value

	// Encrypt sensitive settings
	if crypto.IsSensitiveKey(key) && value != "" {
		encrypted, err := crypto.Encrypt(value)
		if err != nil {
			return fmt.Errorf("failed to encrypt setting %s: %w", key, err)
		}
		storeValue = encrypted
	}

	_, err := d.conn.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, storeValue)
	return err
}

// AlbumMetadata operations

// AlbumMetadata represents cached metadata for an album from Spotify
type AlbumMetadata struct {
	AlbumKey       string `json:"albumKey"` // "{album}::{artist}" format
	AlbumName      string `json:"albumName"`
	ArtistName     string `json:"artistName"`
	SpotifyID      string `json:"spotifyId,omitempty"`
	CoverURL       string `json:"coverUrl,omitempty"`
	LocalCoverPath string `json:"localCoverPath,omitempty"`
	Description    string `json:"description,omitempty"`
	Genre          string `json:"genre,omitempty"`
	ReleaseDate    string `json:"releaseDate,omitempty"`
	SpotifyURL     string `json:"spotifyUrl,omitempty"`
	Copyright      string `json:"copyright,omitempty"`
	SpotifyChecked bool   `json:"spotifyChecked"` // True if we've checked Spotify (even if not found)
	SpotifyFound   bool   `json:"spotifyFound"`   // True if Spotify returned results
	FetchedAt      int64  `json:"fetchedAt,omitempty"`
	UpdatedAt      int64  `json:"updatedAt,omitempty"`
}

// GetAlbumMetadata retrieves cached metadata for an album
func (d *DB) GetAlbumMetadata(albumKey string) (*AlbumMetadata, error) {
	var m AlbumMetadata
	var spotifyID, coverURL, localCoverPath, description, genre, releaseDate, spotifyURL, copyright sql.NullString
	var fetchedAt, updatedAt sql.NullInt64
	var spotifyChecked, spotifyFound sql.NullInt64

	err := d.conn.QueryRow(`
		SELECT album_key, album_name, artist_name, spotify_id, cover_url, local_cover_path,
		       description, genre, release_date, spotify_url, copyright,
		       spotify_checked, spotify_found, fetched_at, updated_at
		FROM album_metadata WHERE album_key = ?
	`, albumKey).Scan(
		&m.AlbumKey, &m.AlbumName, &m.ArtistName, &spotifyID, &coverURL, &localCoverPath,
		&description, &genre, &releaseDate, &spotifyURL, &copyright,
		&spotifyChecked, &spotifyFound, &fetchedAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if spotifyID.Valid {
		m.SpotifyID = spotifyID.String
	}
	if coverURL.Valid {
		m.CoverURL = coverURL.String
	}
	if localCoverPath.Valid {
		m.LocalCoverPath = localCoverPath.String
	}
	if description.Valid {
		m.Description = description.String
	}
	if genre.Valid {
		m.Genre = genre.String
	}
	if releaseDate.Valid {
		m.ReleaseDate = releaseDate.String
	}
	if spotifyURL.Valid {
		m.SpotifyURL = spotifyURL.String
	}
	if copyright.Valid {
		m.Copyright = copyright.String
	}
	if spotifyChecked.Valid {
		m.SpotifyChecked = spotifyChecked.Int64 == 1
	}
	if spotifyFound.Valid {
		m.SpotifyFound = spotifyFound.Int64 == 1
	}
	if fetchedAt.Valid {
		m.FetchedAt = fetchedAt.Int64
	}
	if updatedAt.Valid {
		m.UpdatedAt = updatedAt.Int64
	}

	return &m, nil
}

// GetAllAlbumMetadata retrieves all cached album metadata
func (d *DB) GetAllAlbumMetadata() ([]AlbumMetadata, error) {
	rows, err := d.conn.Query(`
		SELECT album_key, album_name, artist_name, spotify_id, cover_url, local_cover_path,
		       description, genre, release_date, spotify_url, copyright,
		       spotify_checked, spotify_found, fetched_at, updated_at
		FROM album_metadata
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AlbumMetadata
	for rows.Next() {
		var m AlbumMetadata
		var spotifyID, coverURL, localCoverPath, description, genre, releaseDate, spotifyURL, copyright sql.NullString
		var fetchedAt, updatedAt sql.NullInt64
		var spotifyChecked, spotifyFound sql.NullInt64

		err := rows.Scan(
			&m.AlbumKey, &m.AlbumName, &m.ArtistName, &spotifyID, &coverURL, &localCoverPath,
			&description, &genre, &releaseDate, &spotifyURL, &copyright,
			&spotifyChecked, &spotifyFound, &fetchedAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if spotifyID.Valid {
			m.SpotifyID = spotifyID.String
		}
		if coverURL.Valid {
			m.CoverURL = coverURL.String
		}
		if localCoverPath.Valid {
			m.LocalCoverPath = localCoverPath.String
		}
		if description.Valid {
			m.Description = description.String
		}
		if genre.Valid {
			m.Genre = genre.String
		}
		if releaseDate.Valid {
			m.ReleaseDate = releaseDate.String
		}
		if spotifyURL.Valid {
			m.SpotifyURL = spotifyURL.String
		}
		if copyright.Valid {
			m.Copyright = copyright.String
		}
		if spotifyChecked.Valid {
			m.SpotifyChecked = spotifyChecked.Int64 == 1
		}
		if spotifyFound.Valid {
			m.SpotifyFound = spotifyFound.Int64 == 1
		}
		if fetchedAt.Valid {
			m.FetchedAt = fetchedAt.Int64
		}
		if updatedAt.Valid {
			m.UpdatedAt = updatedAt.Int64
		}

		results = append(results, m)
	}

	return results, nil
}

// SaveAlbumMetadata saves or updates album metadata
func (d *DB) SaveAlbumMetadata(m *AlbumMetadata) error {
	now := time.Now().Unix()
	spotifyChecked := 0
	spotifyFound := 0
	if m.SpotifyChecked {
		spotifyChecked = 1
	}
	if m.SpotifyFound {
		spotifyFound = 1
	}

	_, err := d.conn.Exec(`
		INSERT INTO album_metadata (
			album_key, album_name, artist_name, spotify_id, cover_url, local_cover_path,
			description, genre, release_date, spotify_url, copyright,
			spotify_checked, spotify_found, fetched_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(album_key) DO UPDATE SET
			spotify_id = excluded.spotify_id,
			cover_url = excluded.cover_url,
			local_cover_path = COALESCE(excluded.local_cover_path, album_metadata.local_cover_path),
			description = excluded.description,
			genre = excluded.genre,
			release_date = excluded.release_date,
			spotify_url = excluded.spotify_url,
			copyright = excluded.copyright,
			spotify_checked = excluded.spotify_checked,
			spotify_found = excluded.spotify_found,
			fetched_at = COALESCE(excluded.fetched_at, album_metadata.fetched_at),
			updated_at = excluded.updated_at
	`, m.AlbumKey, m.AlbumName, m.ArtistName, m.SpotifyID, m.CoverURL, m.LocalCoverPath,
		m.Description, m.Genre, m.ReleaseDate, m.SpotifyURL, m.Copyright,
		spotifyChecked, spotifyFound, m.FetchedAt, now)
	return err
}

// UpdateAlbumLocalCover updates just the local cover path for an album
func (d *DB) UpdateAlbumLocalCover(albumKey, localCoverPath string) error {
	_, err := d.conn.Exec(`
		UPDATE album_metadata SET local_cover_path = ?, updated_at = ?
		WHERE album_key = ?
	`, localCoverPath, time.Now().Unix(), albumKey)
	return err
}

// GetAlbumsNeedingMetadata returns albums that haven't been checked yet
func (d *DB) GetAlbumsNeedingMetadata() ([]AlbumMetadata, error) {
	rows, err := d.conn.Query(`
		SELECT DISTINCT album_key, album_name, artist_name
		FROM album_metadata
		WHERE spotify_checked = 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AlbumMetadata
	for rows.Next() {
		var m AlbumMetadata
		err := rows.Scan(&m.AlbumKey, &m.AlbumName, &m.ArtistName)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}

	return results, nil
}

// ResetAlbumSpotifyCheck resets the spotify_checked flag for an album to force re-fetch
func (d *DB) ResetAlbumSpotifyCheck(albumKey string) error {
	_, err := d.conn.Exec(`
		UPDATE album_metadata 
		SET spotify_checked = 0, spotify_found = 0, fetched_at = NULL, updated_at = ?
		WHERE album_key = ?
	`, time.Now().Unix(), albumKey)
	return err
}

// GetAlbumMetadataBatch returns metadata for multiple albums at once
func (d *DB) GetAlbumMetadataBatch(albumKeys []string) ([]AlbumMetadata, error) {
	if len(albumKeys) == 0 {
		return nil, nil
	}

	// Build query with placeholders
	placeholders := make([]string, len(albumKeys))
	args := make([]interface{}, len(albumKeys))
	for i, key := range albumKeys {
		placeholders[i] = "?"
		args[i] = key
	}

	query := fmt.Sprintf(`
		SELECT album_key, album_name, artist_name, spotify_id, cover_url, local_cover_path,
		       description, genre, release_date, spotify_url, copyright,
		       spotify_checked, spotify_found, fetched_at, updated_at
		FROM album_metadata
		WHERE album_key IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AlbumMetadata
	for rows.Next() {
		var m AlbumMetadata
		var spotifyID, coverURL, localCoverPath, description, genre, releaseDate, spotifyURL, copyright sql.NullString
		var fetchedAt, updatedAt sql.NullInt64
		var spotifyChecked, spotifyFound sql.NullInt64

		err := rows.Scan(
			&m.AlbumKey, &m.AlbumName, &m.ArtistName, &spotifyID, &coverURL, &localCoverPath,
			&description, &genre, &releaseDate, &spotifyURL, &copyright,
			&spotifyChecked, &spotifyFound, &fetchedAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if spotifyID.Valid {
			m.SpotifyID = spotifyID.String
		}
		if coverURL.Valid {
			m.CoverURL = coverURL.String
		}
		if localCoverPath.Valid {
			m.LocalCoverPath = localCoverPath.String
		}
		if description.Valid {
			m.Description = description.String
		}
		if genre.Valid {
			m.Genre = genre.String
		}
		if releaseDate.Valid {
			m.ReleaseDate = releaseDate.String
		}
		if spotifyURL.Valid {
			m.SpotifyURL = spotifyURL.String
		}
		if copyright.Valid {
			m.Copyright = copyright.String
		}
		if spotifyChecked.Valid {
			m.SpotifyChecked = spotifyChecked.Int64 == 1
		}
		if spotifyFound.Valid {
			m.SpotifyFound = spotifyFound.Int64 == 1
		}
		if fetchedAt.Valid {
			m.FetchedAt = fetchedAt.Int64
		}
		if updatedAt.Valid {
			m.UpdatedAt = updatedAt.Int64
		}

		results = append(results, m)
	}

	return results, nil
}

// GetExpiredAlbumMetadata returns albums that need re-checking
// (checked more than X days ago AND spotify was not found)
func (d *DB) GetExpiredAlbumMetadata(expirationDays int) ([]AlbumMetadata, error) {
	expirationTime := time.Now().AddDate(0, 0, -expirationDays).Unix()

	rows, err := d.conn.Query(`
		SELECT album_key, album_name, artist_name, local_cover_path
		FROM album_metadata
		WHERE spotify_checked = 1 
		  AND spotify_found = 0 
		  AND fetched_at IS NOT NULL 
		  AND fetched_at < ?
	`, expirationTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AlbumMetadata
	for rows.Next() {
		var m AlbumMetadata
		var localCoverPath sql.NullString
		err := rows.Scan(&m.AlbumKey, &m.AlbumName, &m.ArtistName, &localCoverPath)
		if err != nil {
			return nil, err
		}
		if localCoverPath.Valid {
			m.LocalCoverPath = localCoverPath.String
		}
		results = append(results, m)
	}

	return results, nil
}

// ArtistMetadata operations

// ArtistMetadata represents cached metadata for an artist from Spotify
type ArtistMetadata struct {
	ArtistName     string `json:"artistName"`
	SpotifyID      string `json:"spotifyId,omitempty"`
	ImageURL       string `json:"imageUrl,omitempty"`
	LocalImagePath string `json:"localImagePath,omitempty"`
	SpotifyURL     string `json:"spotifyUrl,omitempty"`
	SpotifyChecked bool   `json:"spotifyChecked"` // True if we've checked Spotify (even if not found)
	SpotifyFound   bool   `json:"spotifyFound"`   // True if Spotify returned results
	FetchedAt      int64  `json:"fetchedAt,omitempty"`
	UpdatedAt      int64  `json:"updatedAt,omitempty"`
}

// GetArtistMetadata retrieves cached metadata for an artist
func (d *DB) GetArtistMetadata(artistName string) (*ArtistMetadata, error) {
	var m ArtistMetadata
	var spotifyID, imageURL, localImagePath, spotifyURL sql.NullString
	var fetchedAt, updatedAt sql.NullInt64
	var spotifyChecked, spotifyFound sql.NullInt64

	err := d.conn.QueryRow(`
		SELECT artist_name, spotify_id, image_url, local_image_path, spotify_url,
		       spotify_checked, spotify_found, fetched_at, updated_at
		FROM artist_metadata WHERE artist_name = ?
	`, artistName).Scan(
		&m.ArtistName, &spotifyID, &imageURL, &localImagePath, &spotifyURL,
		&spotifyChecked, &spotifyFound, &fetchedAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if spotifyID.Valid {
		m.SpotifyID = spotifyID.String
	}
	if imageURL.Valid {
		m.ImageURL = imageURL.String
	}
	if localImagePath.Valid {
		m.LocalImagePath = localImagePath.String
	}
	if spotifyURL.Valid {
		m.SpotifyURL = spotifyURL.String
	}
	if spotifyChecked.Valid {
		m.SpotifyChecked = spotifyChecked.Int64 == 1
	}
	if spotifyFound.Valid {
		m.SpotifyFound = spotifyFound.Int64 == 1
	}
	if fetchedAt.Valid {
		m.FetchedAt = fetchedAt.Int64
	}
	if updatedAt.Valid {
		m.UpdatedAt = updatedAt.Int64
	}

	return &m, nil
}

// GetAllArtistMetadata retrieves all cached artist metadata
func (d *DB) GetAllArtistMetadata() ([]ArtistMetadata, error) {
	rows, err := d.conn.Query(`
		SELECT artist_name, spotify_id, image_url, local_image_path, spotify_url,
		       spotify_checked, spotify_found, fetched_at, updated_at
		FROM artist_metadata
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ArtistMetadata
	for rows.Next() {
		var m ArtistMetadata
		var spotifyID, imageURL, localImagePath, spotifyURL sql.NullString
		var fetchedAt, updatedAt sql.NullInt64
		var spotifyChecked, spotifyFound sql.NullInt64

		err := rows.Scan(
			&m.ArtistName, &spotifyID, &imageURL, &localImagePath, &spotifyURL,
			&spotifyChecked, &spotifyFound, &fetchedAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if spotifyID.Valid {
			m.SpotifyID = spotifyID.String
		}
		if imageURL.Valid {
			m.ImageURL = imageURL.String
		}
		if localImagePath.Valid {
			m.LocalImagePath = localImagePath.String
		}
		if spotifyURL.Valid {
			m.SpotifyURL = spotifyURL.String
		}
		if spotifyChecked.Valid {
			m.SpotifyChecked = spotifyChecked.Int64 == 1
		}
		if spotifyFound.Valid {
			m.SpotifyFound = spotifyFound.Int64 == 1
		}
		if fetchedAt.Valid {
			m.FetchedAt = fetchedAt.Int64
		}
		if updatedAt.Valid {
			m.UpdatedAt = updatedAt.Int64
		}

		results = append(results, m)
	}

	return results, nil
}

// SaveArtistMetadata saves or updates artist metadata
func (d *DB) SaveArtistMetadata(m *ArtistMetadata) error {
	now := time.Now().Unix()
	spotifyChecked := 0
	spotifyFound := 0
	if m.SpotifyChecked {
		spotifyChecked = 1
	}
	if m.SpotifyFound {
		spotifyFound = 1
	}

	_, err := d.conn.Exec(`
		INSERT INTO artist_metadata (
			artist_name, spotify_id, image_url, local_image_path, spotify_url,
			spotify_checked, spotify_found, fetched_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(artist_name) DO UPDATE SET
			spotify_id = excluded.spotify_id,
			image_url = excluded.image_url,
			local_image_path = COALESCE(excluded.local_image_path, artist_metadata.local_image_path),
			spotify_url = excluded.spotify_url,
			spotify_checked = excluded.spotify_checked,
			spotify_found = excluded.spotify_found,
			fetched_at = COALESCE(excluded.fetched_at, artist_metadata.fetched_at),
			updated_at = excluded.updated_at
	`, m.ArtistName, m.SpotifyID, m.ImageURL, m.LocalImagePath, m.SpotifyURL,
		spotifyChecked, spotifyFound, m.FetchedAt, now)
	return err
}

// UpdateArtistLocalImage updates just the local image path for an artist
func (d *DB) UpdateArtistLocalImage(artistName, localImagePath string) error {
	_, err := d.conn.Exec(`
		UPDATE artist_metadata SET local_image_path = ?, updated_at = ?
		WHERE artist_name = ?
	`, localImagePath, time.Now().Unix(), artistName)
	return err
}

// ResetArtistSpotifyCheck resets the spotify_checked flag for an artist
func (d *DB) ResetArtistSpotifyCheck(artistName string) error {
	_, err := d.conn.Exec(`
		UPDATE artist_metadata 
		SET spotify_checked = 0, spotify_found = 0, fetched_at = NULL, updated_at = ?
		WHERE artist_name = ?
	`, time.Now().Unix(), artistName)
	return err
}

// GetArtistsNeedingMetadata returns artists that haven't been checked yet
func (d *DB) GetArtistsNeedingMetadata() ([]ArtistMetadata, error) {
	rows, err := d.conn.Query(`
		SELECT artist_name
		FROM artist_metadata
		WHERE spotify_checked = 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ArtistMetadata
	for rows.Next() {
		var m ArtistMetadata
		err := rows.Scan(&m.ArtistName)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}

	return results, nil
}

// GetExpiredArtistMetadata returns artists that need re-checking
func (d *DB) GetExpiredArtistMetadata(expirationDays int) ([]ArtistMetadata, error) {
	expirationTime := time.Now().AddDate(0, 0, -expirationDays).Unix()

	rows, err := d.conn.Query(`
		SELECT artist_name, local_image_path
		FROM artist_metadata
		WHERE spotify_checked = 1 
		  AND spotify_found = 0 
		  AND fetched_at IS NOT NULL 
		  AND fetched_at < ?
	`, expirationTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ArtistMetadata
	for rows.Next() {
		var m ArtistMetadata
		var localImagePath sql.NullString
		err := rows.Scan(&m.ArtistName, &localImagePath)
		if err != nil {
			return nil, err
		}
		if localImagePath.Valid {
			m.LocalImagePath = localImagePath.String
		}
		results = append(results, m)
	}

	return results, nil
}

// GetSongsForEnrichment returns a list of songs for genre enrichment.
// If force is true, it returns all songs (paginated by offset).
// If force is false, it returns only songs with missing genres.
func (d *DB) GetSongsForEnrichment(limit int, force bool, offset int) ([]Song, error) {
	var query string
	var args []interface{}

	baseQuery := `
		SELECT id, title, artist, album, album_artist, track_number, disc_number, genre, year, duration, file_path, cover_path, added_at, play_count, last_played, skip_count, file_hash
		FROM songs
	`

	if force {
		query = baseQuery + ` ORDER BY added_at DESC LIMIT ? OFFSET ?`
		args = []interface{}{limit, offset}
	} else {
		query = baseQuery + `
			WHERE genre IS NULL OR genre = '' OR genre = '[]' OR genre = 'null'
			LIMIT ?
		`
		args = []interface{}{limit}
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query songs for enrichment: %w", err)
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON sql.NullString
		var trackNum, discNum sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &s.AlbumArtist,
			&trackNum, &discNum, &genreJSON, &s.Year, &s.Duration,
			&s.FilePath, &s.CoverPath, &s.AddedAt, &s.PlayCount,
			&s.LastPlayed, &s.SkipCount, &s.FileHash,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan song row: %w", err)
		}

		if trackNum.Valid {
			s.TrackNumber = int(trackNum.Int64)
		}
		if discNum.Valid {
			s.DiscNumber = int(discNum.Int64)
		}
		if genreJSON.Valid && genreJSON.String != "" {
			json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}

		songs = append(songs, s)
	}

	return songs, nil
}

// UpdateSongGenres updates the genre list for a specific song.
func (d *DB) UpdateSongGenres(songID string, genres []string) error {
	genreJSON, err := json.Marshal(genres)
	if err != nil {
		return fmt.Errorf("failed to marshal genres: %w", err)
	}

	query := `UPDATE songs SET genre = ? WHERE id = ?`
	_, err = d.conn.Exec(query, string(genreJSON), songID)
	if err != nil {
		return fmt.Errorf("failed to update song genres: %w", err)
	}

	return nil
}

// GetSongsWithMissingGenres returns songs that have no genre information.
func (d *DB) GetSongsWithMissingGenres(limit int) ([]Song, error) {
	query := `SELECT id, title, artist, album, genre FROM songs WHERE genre IS NULL OR genre = '[]' OR genre = '' OR genre = 'null' LIMIT ?`
	rows, err := d.conn.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON sql.NullString
		if err := rows.Scan(&s.ID, &s.Title, &s.Artist, &s.Album, &genreJSON); err != nil {
			return nil, err
		}
		if genreJSON.Valid && genreJSON.String != "" {
			json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		songs = append(songs, s)
	}
	return songs, rows.Err()
}

// GetSongsBySmartFilter returns songs matching the given criteria.
func (d *DB) GetSongsBySmartFilter(genres []string, artists []string, minYear, maxYear int) ([]Song, error) {
	query := "SELECT id, title, artist, album, genre, year, duration, file_path, cover_path, added_at, play_count, last_played FROM songs WHERE 1=1"
	var args []interface{}

	if len(genres) > 0 {
		query += " AND ("
		for i, g := range genres {
			if i > 0 {
				query += " OR "
			}
			query += "genre LIKE ?"
			args = append(args, "%"+g+"%")
		}
		query += ")"
	}

	if len(artists) > 0 {
		query += " AND ("
		for i, a := range artists {
			if i > 0 {
				query += " OR "
			}
			query += "artist LIKE ?"
			args = append(args, "%"+a+"%")
		}
		query += ")"
	}

	if minYear > 0 {
		query += " AND year >= ?"
		args = append(args, minYear)
	}
	if maxYear > 0 {
		query += " AND year <= ?"
		args = append(args, maxYear)
	}

	query += " ORDER BY RANDOM() LIMIT 50"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON sql.NullString
		var coverPath sql.NullString
		var lastPlayed sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &genreJSON, &s.Year, &s.Duration,
			&s.FilePath, &coverPath, &s.AddedAt, &s.PlayCount, &lastPlayed,
		)
		if err != nil {
			return nil, err
		}

		if genreJSON.Valid && genreJSON.String != "" {
			json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		if coverPath.Valid {
			s.CoverPath = coverPath.String
		}
		if lastPlayed.Valid {
			s.LastPlayed = lastPlayed.Int64
		}
		songs = append(songs, s)
	}
	return songs, rows.Err()
}

// ==============================================================================
// Fast Scan: Directory Signature Methods
// ==============================================================================

// SaveDirectorySignature saves or updates a directory signature for fast change detection.
func (d *DB) SaveDirectorySignature(sig DirectorySignature) error {
	_, err := d.conn.Exec(`
		INSERT INTO directory_signatures (path, file_count, total_size, latest_mtime, content_hash, last_verified)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			file_count = excluded.file_count,
			total_size = excluded.total_size,
			latest_mtime = excluded.latest_mtime,
			content_hash = excluded.content_hash,
			last_verified = excluded.last_verified
	`, sig.Path, sig.FileCount, sig.TotalSize, sig.LatestMtime, sig.ContentHash, sig.LastVerified)
	return err
}

// SaveDirectorySignaturesBatch saves multiple directory signatures in a single transaction.
func (d *DB) SaveDirectorySignaturesBatch(sigs []DirectorySignature) error {
	if len(sigs) == 0 {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO directory_signatures (path, file_count, total_size, latest_mtime, content_hash, last_verified)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			file_count = excluded.file_count,
			total_size = excluded.total_size,
			latest_mtime = excluded.latest_mtime,
			content_hash = excluded.content_hash,
			last_verified = excluded.last_verified
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sig := range sigs {
		_, err = stmt.Exec(sig.Path, sig.FileCount, sig.TotalSize, sig.LatestMtime, sig.ContentHash, sig.LastVerified)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetDirectorySignature retrieves a directory signature by path.
func (d *DB) GetDirectorySignature(path string) (*DirectorySignature, error) {
	var sig DirectorySignature
	err := d.conn.QueryRow(`
		SELECT path, file_count, total_size, latest_mtime, content_hash, last_verified
		FROM directory_signatures
		WHERE path = ?
	`, path).Scan(&sig.Path, &sig.FileCount, &sig.TotalSize, &sig.LatestMtime, &sig.ContentHash, &sig.LastVerified)
	if err != nil {
		return nil, err
	}
	return &sig, nil
}

// GetAllDirectorySignatures retrieves all stored directory signatures.
func (d *DB) GetAllDirectorySignatures() ([]DirectorySignature, error) {
	rows, err := d.conn.Query(`
		SELECT path, file_count, total_size, latest_mtime, content_hash, last_verified
		FROM directory_signatures
		ORDER BY path
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sigs []DirectorySignature
	for rows.Next() {
		var sig DirectorySignature
		err := rows.Scan(&sig.Path, &sig.FileCount, &sig.TotalSize, &sig.LatestMtime, &sig.ContentHash, &sig.LastVerified)
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, sig)
	}
	return sigs, rows.Err()
}

// DeleteDirectorySignature removes a directory signature by path.
func (d *DB) DeleteDirectorySignature(path string) error {
	_, err := d.conn.Exec(`DELETE FROM directory_signatures WHERE path = ?`, path)
	return err
}

// DeleteDirectorySignaturesWithPrefix removes all signatures for paths under a given prefix.
// Useful for removing signatures for an entire folder tree when a scan folder is removed.
// Example: DeleteDirectorySignaturesWithPrefix("/music") removes /music, /music/albums, etc.
func (d *DB) DeleteDirectorySignaturesWithPrefix(prefix string) error {
	_, err := d.conn.Exec(`DELETE FROM directory_signatures WHERE path LIKE ?`, prefix+"%")
	return err
}

// DeleteDirectorySignatures removes multiple directory signatures by path in a single
// transaction. This is used by CleanupStaleSignatures() to efficiently remove
// signatures for directories that no longer exist.
//
// Returns the number of signatures actually deleted.
// The operation is atomic - either all deletions succeed or none are committed.
func (d *DB) DeleteDirectorySignatures(paths []string) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`DELETE FROM directory_signatures WHERE path = ?`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	deleted := 0
	for _, path := range paths {
		result, err := stmt.Exec(path)
		if err != nil {
			return deleted, err
		}
		affected, _ := result.RowsAffected()
		deleted += int(affected)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// ==============================================================================
// Fast Scan: Scan State Methods
// ==============================================================================

// SaveScanState saves or updates the global scan state.
// The scan state is stored with a fixed ID (1) so there's always only one row.
// Uses upsert (INSERT ... ON CONFLICT UPDATE) for efficiency.
func (d *DB) SaveScanState(state ScanState) error {
	_, err := d.conn.Exec(`
		INSERT INTO scan_state (id, last_scan_time, windows_usn, macos_event_id, linux_last_mtime, scan_duration_ms, files_scanned, files_changed)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_scan_time = excluded.last_scan_time,
			windows_usn = excluded.windows_usn,
			macos_event_id = excluded.macos_event_id,
			linux_last_mtime = excluded.linux_last_mtime,
			scan_duration_ms = excluded.scan_duration_ms,
			files_scanned = excluded.files_scanned,
			files_changed = excluded.files_changed
	`, state.LastScanTime, state.WindowsUSN, state.MacOSEventID, state.LinuxLastMtime, state.ScanDurationMs, state.FilesScanned, state.FilesChanged)
	return err
}

// GetScanState retrieves the global scan state.
func (d *DB) GetScanState() (*ScanState, error) {
	var state ScanState
	var windowsUSN, macosEventID, linuxLastMtime, scanDurationMs sql.NullInt64
	var filesScanned, filesChanged sql.NullInt64

	err := d.conn.QueryRow(`
		SELECT last_scan_time, windows_usn, macos_event_id, linux_last_mtime, scan_duration_ms, files_scanned, files_changed
		FROM scan_state
		WHERE id = 1
	`).Scan(&state.LastScanTime, &windowsUSN, &macosEventID, &linuxLastMtime, &scanDurationMs, &filesScanned, &filesChanged)
	if err != nil {
		return nil, err
	}

	if windowsUSN.Valid {
		state.WindowsUSN = windowsUSN.Int64
	}
	if macosEventID.Valid {
		state.MacOSEventID = macosEventID.Int64
	}
	if linuxLastMtime.Valid {
		state.LinuxLastMtime = linuxLastMtime.Int64
	}
	if scanDurationMs.Valid {
		state.ScanDurationMs = scanDurationMs.Int64
	}
	if filesScanned.Valid {
		state.FilesScanned = int(filesScanned.Int64)
	}
	if filesChanged.Valid {
		state.FilesChanged = int(filesChanged.Int64)
	}

	return &state, nil
}

// ==============================================================================
// Fast Scan: File Metadata Cache Methods
// ==============================================================================

// SaveFileMetadataCache saves or updates a file metadata cache entry.
func (d *DB) SaveFileMetadataCache(cache FileMetadataCache) error {
	_, err := d.conn.Exec(`
		INSERT INTO file_metadata_cache (file_path, file_size, mtime, metadata_hash, last_verified)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			file_size = excluded.file_size,
			mtime = excluded.mtime,
			metadata_hash = excluded.metadata_hash,
			last_verified = excluded.last_verified
	`, cache.FilePath, cache.FileSize, cache.Mtime, cache.MetadataHash, cache.LastVerified)
	return err
}

// SaveFileMetadataCacheBatch saves multiple file metadata cache entries in a single transaction.
func (d *DB) SaveFileMetadataCacheBatch(caches []FileMetadataCache) error {
	if len(caches) == 0 {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO file_metadata_cache (file_path, file_size, mtime, metadata_hash, last_verified)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			file_size = excluded.file_size,
			mtime = excluded.mtime,
			metadata_hash = excluded.metadata_hash,
			last_verified = excluded.last_verified
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, cache := range caches {
		_, err = stmt.Exec(cache.FilePath, cache.FileSize, cache.Mtime, cache.MetadataHash, cache.LastVerified)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetFileMetadataCache retrieves a file metadata cache entry by path.
func (d *DB) GetFileMetadataCache(path string) (*FileMetadataCache, error) {
	var cache FileMetadataCache
	var metadataHash sql.NullString

	err := d.conn.QueryRow(`
		SELECT file_path, file_size, mtime, metadata_hash, last_verified
		FROM file_metadata_cache
		WHERE file_path = ?
	`, path).Scan(&cache.FilePath, &cache.FileSize, &cache.Mtime, &metadataHash, &cache.LastVerified)
	if err != nil {
		return nil, err
	}

	if metadataHash.Valid {
		cache.MetadataHash = metadataHash.String
	}

	return &cache, nil
}

// GetAllFileMetadataCache retrieves all file metadata cache entries.
func (d *DB) GetAllFileMetadataCache() ([]FileMetadataCache, error) {
	rows, err := d.conn.Query(`
		SELECT file_path, file_size, mtime, metadata_hash, last_verified
		FROM file_metadata_cache
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var caches []FileMetadataCache
	for rows.Next() {
		var cache FileMetadataCache
		var metadataHash sql.NullString

		err := rows.Scan(&cache.FilePath, &cache.FileSize, &cache.Mtime, &metadataHash, &cache.LastVerified)
		if err != nil {
			return nil, err
		}

		if metadataHash.Valid {
			cache.MetadataHash = metadataHash.String
		}

		caches = append(caches, cache)
	}
	return caches, rows.Err()
}

// GetFileMetadataCacheMap returns a map of file path to FileMetadataCache for fast lookup.
func (d *DB) GetFileMetadataCacheMap() (map[string]FileMetadataCache, error) {
	caches, err := d.GetAllFileMetadataCache()
	if err != nil {
		return nil, err
	}

	result := make(map[string]FileMetadataCache, len(caches))
	for _, cache := range caches {
		result[cache.FilePath] = cache
	}
	return result, nil
}

// DeleteFileMetadataCache removes a file metadata cache entry by path.
func (d *DB) DeleteFileMetadataCache(path string) error {
	_, err := d.conn.Exec(`DELETE FROM file_metadata_cache WHERE file_path = ?`, path)
	return err
}

// DeleteFileMetadataCacheBatch removes multiple file metadata cache entries.
func (d *DB) DeleteFileMetadataCacheBatch(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`DELETE FROM file_metadata_cache WHERE file_path = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, path := range paths {
		_, err = stmt.Exec(path)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetRandomFileSample retrieves a random sample of file metadata cache entries.
func (d *DB) GetRandomFileSample(count int) ([]FileMetadataCache, error) {
	rows, err := d.conn.Query(`
		SELECT file_path, file_size, mtime, metadata_hash, last_verified
		FROM file_metadata_cache
		ORDER BY RANDOM()
		LIMIT ?
	`, count)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var caches []FileMetadataCache
	for rows.Next() {
		var cache FileMetadataCache
		var metadataHash sql.NullString

		err := rows.Scan(&cache.FilePath, &cache.FileSize, &cache.Mtime, &metadataHash, &cache.LastVerified)
		if err != nil {
			return nil, err
		}

		if metadataHash.Valid {
			cache.MetadataHash = metadataHash.String
		}

		caches = append(caches, cache)
	}
	return caches, rows.Err()
}
