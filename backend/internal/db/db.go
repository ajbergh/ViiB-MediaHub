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
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/crypto"
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
	conn               *sql.DB
	librarySyncOnce    sync.Once
	librarySyncInitErr error
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
	OriginalYear   int      `json:"originalYear,omitempty"`   // Original release year (for remasters)
	YearUncertain  bool     `json:"yearUncertain,omitempty"`  // True if year may be remaster date
	YearAnalyzedAt int64    `json:"yearAnalyzedAt,omitempty"` // Timestamp of year analysis
	Duration       float64  `json:"duration"`
	ReplayGainDB   float64  `json:"replayGainDb,omitempty"`
	ReplayPeak     float64  `json:"replayPeak,omitempty"`
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
	Liked          bool     `json:"liked,omitempty"`          // true if user has liked this song
	LikedAt        int64    `json:"likedAt,omitempty"`        // Timestamp when song was liked
	// Last.FM enrichment data
	LastFMListeners  int    `json:"lastfmListeners,omitempty"`  // Global listener count
	LastFMPlaycount  int    `json:"lastfmPlaycount,omitempty"`  // Global play count
	LastFMTags       string `json:"lastfmTags,omitempty"`       // JSON array of tags
	LastFMURL        string `json:"lastfmUrl,omitempty"`        // Last.FM track page URL
	LastFMMBID       string `json:"lastfmMbid,omitempty"`       // MusicBrainz track ID
	LastFMEnrichedAt int64  `json:"lastfmEnrichedAt,omitempty"` // Timestamp of Last.FM enrichment
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
	registerSQLiteRuntimeDriver()
	conn, err := sql.Open(sqliteRuntimeDriverName, sqliteRuntimeDSN(dbPath))
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

	// Check for liked column (added for likes feature) and add if missing
	err = d.conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('songs') WHERE name='liked'`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		likeMigrations := []string{
			`ALTER TABLE songs ADD COLUMN liked INTEGER DEFAULT 0`,
			`ALTER TABLE songs ADD COLUMN liked_at INTEGER`,
		}
		for _, m := range likeMigrations {
			if _, err := d.conn.Exec(m); err != nil {
				if !strings.Contains(err.Error(), "duplicate column") {
					return err
				}
			}
		}
		// Create index for fast liked song queries
		if _, err := d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_songs_liked ON songs(liked) WHERE liked = 1`); err != nil {
			// Ignore if index already exists
			if !strings.Contains(err.Error(), "already exists") {
				return err
			}
		}
	}

	// Migration: Add liked columns to album_metadata
	albumLikeMigrations := []string{
		`ALTER TABLE album_metadata ADD COLUMN liked INTEGER DEFAULT 0`,
		`ALTER TABLE album_metadata ADD COLUMN liked_at INTEGER`,
	}
	for _, m := range albumLikeMigrations {
		if _, err := d.conn.Exec(m); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				// Ignore duplicate column errors
			}
		}
	}
	// Create index for fast liked album queries
	if _, err := d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_album_metadata_liked ON album_metadata(liked) WHERE liked = 1`); err != nil {
		// Ignore if index already exists
	}

	// Migration: Create listening_events table for preference learning (Phase 5.1)
	// This table tracks detailed listening behavior: plays, skips, and timing.
	// Used by the AI DJ to personalize recommendations based on user preferences.
	listeningEventsTable := `
	CREATE TABLE IF NOT EXISTS listening_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		song_id TEXT NOT NULL,
		event_type TEXT NOT NULL,  -- 'play_complete', 'skip_early' (<10s), 'skip_mid' (10-30s), 'skip_late' (>30s)
		play_duration REAL,        -- How many seconds played before skip/complete
		song_duration REAL,        -- Total song duration
		timestamp INTEGER NOT NULL,
		genre TEXT,                -- Cached genre for aggregation
		mood TEXT,                 -- Cached mood for aggregation
		energy TEXT,               -- Cached energy for aggregation
		context TEXT               -- 'ai_dj', 'album', 'playlist', 'queue', 'search'
	);
	CREATE INDEX IF NOT EXISTS idx_listening_events_song ON listening_events(song_id);
	CREATE INDEX IF NOT EXISTS idx_listening_events_type ON listening_events(event_type);
	CREATE INDEX IF NOT EXISTS idx_listening_events_timestamp ON listening_events(timestamp);
	`
	// Execute table creation (ignore errors if exists)
	for _, stmt := range strings.Split(listeningEventsTable, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := d.conn.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				// Log but don't fail
			}
		}
	}

	// Migration: Create genre_preferences table for AI DJ preference learning
	// Aggregates listening events into per-genre preference scores.
	genrePreferencesTable := `
	CREATE TABLE IF NOT EXISTS genre_preferences (
		genre TEXT PRIMARY KEY,
		play_count INTEGER DEFAULT 0,
		skip_count INTEGER DEFAULT 0,
		complete_rate REAL DEFAULT 0.5,  -- 0.0-1.0, ratio of plays that complete
		skip_early_rate REAL DEFAULT 0,  -- 0.0-1.0, ratio of skips < 10s
		affinity_score REAL DEFAULT 0.5, -- 0.0-1.0, overall preference score
		last_updated INTEGER
	);
	`
	if _, err := d.conn.Exec(genrePreferencesTable); err != nil {
		// Ignore if exists
	}

	// Migrate unencrypted sensitive settings to encrypted format
	if err := d.migrateUnencryptedSettings(); err != nil {
		return err
	}

	// Migration: Add original_year column for remaster detection
	// This allows the AI DJ to correctly filter by decade even for remastered songs.
	// The original_year stores the actual release year of the song (not the remaster date).
	originalYearMigrations := []string{
		`ALTER TABLE songs ADD COLUMN original_year INTEGER`,
		`ALTER TABLE songs ADD COLUMN year_uncertain INTEGER DEFAULT 0`,
		`ALTER TABLE songs ADD COLUMN year_analyzed_at INTEGER`,
	}
	for _, m := range originalYearMigrations {
		if _, err := d.conn.Exec(m); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				// Ignore duplicate column errors
			}
		}
	}
	// Create indexes for efficient querying
	if _, err := d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_songs_original_year ON songs(original_year)`); err != nil {
		// Ignore if index already exists
	}
	if _, err := d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_songs_year_uncertain ON songs(year_uncertain) WHERE year_uncertain = 1`); err != nil {
		// Ignore if index already exists
	}

	// Migration: Add ReplayGain metadata used by the normalization audio stage.
	replayGainMigrations := []string{
		`ALTER TABLE songs ADD COLUMN replay_gain_db REAL`,
		`ALTER TABLE songs ADD COLUMN replay_peak REAL`,
		`ALTER TABLE songs ADD COLUMN ignored INTEGER DEFAULT 0`,
	}
	for _, m := range replayGainMigrations {
		if _, err := d.conn.Exec(m); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	if _, err := d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_songs_ignored ON songs(ignored)`); err != nil {
		return err
	}

	// Migration: Add Last.FM enrichment columns to songs table
	// These columns store metadata from Last.FM API for AI DJ and playlist generation.
	lastfmSongMigrations := []string{
		`ALTER TABLE songs ADD COLUMN lastfm_listeners INTEGER`,
		`ALTER TABLE songs ADD COLUMN lastfm_playcount INTEGER`,
		`ALTER TABLE songs ADD COLUMN lastfm_tags TEXT`,
		`ALTER TABLE songs ADD COLUMN lastfm_url TEXT`,
		`ALTER TABLE songs ADD COLUMN lastfm_mbid TEXT`,
		`ALTER TABLE songs ADD COLUMN lastfm_enriched_at INTEGER`,
	}
	for _, m := range lastfmSongMigrations {
		if _, err := d.conn.Exec(m); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				// Ignore duplicate column errors
			}
		}
	}

	// Migration: Create lastfm_similar_tracks table for track similarity data
	lastfmSimilarTracksTable := `
	CREATE TABLE IF NOT EXISTS lastfm_similar_tracks (
		song_id TEXT NOT NULL,
		similar_artist TEXT NOT NULL,
		similar_track TEXT NOT NULL,
		match_score REAL NOT NULL,
		similar_song_id TEXT,
		FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE,
		PRIMARY KEY (song_id, similar_artist, similar_track)
	);
	CREATE INDEX IF NOT EXISTS idx_similar_tracks_song ON lastfm_similar_tracks(song_id);
	CREATE INDEX IF NOT EXISTS idx_similar_tracks_similar_song ON lastfm_similar_tracks(similar_song_id);
	`
	for _, stmt := range strings.Split(lastfmSimilarTracksTable, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := d.conn.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				// Log but don't fail
			}
		}
	}

	// Migration: Create lastfm_similar_artists table for artist similarity data
	lastfmSimilarArtistsTable := `
	CREATE TABLE IF NOT EXISTS lastfm_similar_artists (
		artist_name TEXT NOT NULL,
		similar_artist TEXT NOT NULL,
		match_score REAL NOT NULL,
		PRIMARY KEY (artist_name, similar_artist)
	);
	CREATE INDEX IF NOT EXISTS idx_similar_artists ON lastfm_similar_artists(artist_name);
	`
	for _, stmt := range strings.Split(lastfmSimilarArtistsTable, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := d.conn.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				// Log but don't fail
			}
		}
	}

	// Migration: Add Last.FM columns to artist_metadata table
	lastfmArtistMigrations := []string{
		`ALTER TABLE artist_metadata ADD COLUMN lastfm_listeners INTEGER`,
		`ALTER TABLE artist_metadata ADD COLUMN lastfm_playcount INTEGER`,
		`ALTER TABLE artist_metadata ADD COLUMN lastfm_tags TEXT`,
		`ALTER TABLE artist_metadata ADD COLUMN lastfm_bio TEXT`,
		`ALTER TABLE artist_metadata ADD COLUMN lastfm_url TEXT`,
		`ALTER TABLE artist_metadata ADD COLUMN lastfm_mbid TEXT`,
		`ALTER TABLE artist_metadata ADD COLUMN lastfm_enriched_at INTEGER`,
	}
	for _, m := range lastfmArtistMigrations {
		if _, err := d.conn.Exec(m); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				// Ignore duplicate column errors
			}
		}
	}

	// Migration: Create DJ waveform cache table
	djWaveformTable := `
	CREATE TABLE IF NOT EXISTS dj_waveform_cache (
		song_id TEXT PRIMARY KEY,
		duration REAL NOT NULL,
		sample_rate INTEGER NOT NULL,
		resolution INTEGER NOT NULL,
		peaks_data BLOB NOT NULL,
		peak_count INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE
	);
	`
	if _, err := d.conn.Exec(djWaveformTable); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			// Log but don't fail
		}
	}

	// Migration: Create DJ hot cues table
	djHotCuesTable := `
	CREATE TABLE IF NOT EXISTS dj_hot_cues (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		song_id TEXT NOT NULL,
		slot INTEGER NOT NULL,
		position REAL NOT NULL,
		label TEXT,
		color TEXT DEFAULT '#FF5500',
		created_at INTEGER NOT NULL,
		FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE,
		UNIQUE(song_id, slot)
	);
	CREATE INDEX IF NOT EXISTS idx_dj_hot_cues_song ON dj_hot_cues(song_id);
	`
	for _, stmt := range strings.Split(djHotCuesTable, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := d.conn.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				// Log but don't fail
			}
		}
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
	sensitiveKeys := []string{"spotify_credentials", "gemini_api_key", "llm_api_key", "lastfm_api_key", "lastfm_shared_secret", "lastfm_session_key"}

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
		       genre, year, original_year, year_uncertain, year_analyzed_at,
		       duration, replay_gain_db, replay_peak, file_path, cover_path, added_at, play_count, last_played,
		       skip_count, file_hash, mood, energy, tempo, bpm, instrumental,
		       mood_analyzed_at, liked, liked_at, lastfm_listeners, lastfm_playcount,
		       lastfm_tags, lastfm_url, lastfm_mbid, lastfm_enriched_at
		FROM songs
		WHERE COALESCE(ignored, 0) = 0
		ORDER BY album, album_artist, disc_number, track_number, title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON, albumArtist, coverPath, fileHash, mood, energy, tempo sql.NullString
		var lastFMTags, lastFMURL, lastFMMBID sql.NullString
		var trackNum, discNum, year, originalYear, yearUncertain, yearAnalyzedAt sql.NullInt64
		var playCount, lastPlayed, skipCount, bpm, instrumental, moodAnalyzedAt sql.NullInt64
		var liked, likedAt, lastFMListeners, lastFMPlaycount, lastFMEnrichedAt sql.NullInt64
		var replayGainDB, replayPeak sql.NullFloat64
		if err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist, &trackNum, &discNum,
			&genreJSON, &year, &originalYear, &yearUncertain, &yearAnalyzedAt,
			&s.Duration, &replayGainDB, &replayPeak, &s.FilePath, &coverPath, &s.AddedAt, &playCount, &lastPlayed,
			&skipCount, &fileHash, &mood, &energy, &tempo, &bpm, &instrumental,
			&moodAnalyzedAt, &liked, &likedAt, &lastFMListeners, &lastFMPlaycount,
			&lastFMTags, &lastFMURL, &lastFMMBID, &lastFMEnrichedAt,
		); err != nil {
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
			_ = json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		if year.Valid {
			s.Year = int(year.Int64)
		}
		if originalYear.Valid {
			s.OriginalYear = int(originalYear.Int64)
		}
		s.YearUncertain = yearUncertain.Valid && yearUncertain.Int64 == 1
		if yearAnalyzedAt.Valid {
			s.YearAnalyzedAt = yearAnalyzedAt.Int64
		}
		if replayGainDB.Valid {
			s.ReplayGainDB = replayGainDB.Float64
		}
		if replayPeak.Valid {
			s.ReplayPeak = replayPeak.Float64
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
		s.Instrumental = instrumental.Valid && instrumental.Int64 == 1
		if moodAnalyzedAt.Valid {
			s.MoodAnalyzedAt = moodAnalyzedAt.Int64
		}
		s.Liked = liked.Valid && liked.Int64 == 1
		if likedAt.Valid {
			s.LikedAt = likedAt.Int64
		}
		if lastFMListeners.Valid {
			s.LastFMListeners = int(lastFMListeners.Int64)
		}
		if lastFMPlaycount.Valid {
			s.LastFMPlaycount = int(lastFMPlaycount.Int64)
		}
		if lastFMTags.Valid {
			s.LastFMTags = lastFMTags.String
		}
		if lastFMURL.Valid {
			s.LastFMURL = lastFMURL.String
		}
		if lastFMMBID.Valid {
			s.LastFMMBID = lastFMMBID.String
		}
		if lastFMEnrichedAt.Valid {
			s.LastFMEnrichedAt = lastFMEnrichedAt.Int64
		}
		songs = append(songs, s)
	}
	return songs, rows.Err()
}

// SaveSong inserts or updates a single Song record in the database.
// Genres are normalized to consistent Title Case capitalization.
func (d *DB) SaveSong(s *Song) error {
	// Normalize genres for consistent capitalization
	normalizedGenres := NormalizeGenres(s.Genre)
	genreJSON, _ := json.Marshal(normalizedGenres)

	_, err := d.conn.Exec(`
		INSERT INTO songs (
			id, title, artist, album, album_artist, track_number, disc_number,
			genre, year, duration, replay_gain_db, replay_peak, file_path, cover_path, added_at,
			play_count, last_played, skip_count, file_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			replay_gain_db = excluded.replay_gain_db,
			replay_peak = excluded.replay_peak,
			cover_path = excluded.cover_path,
			file_path = excluded.file_path,
			play_count = excluded.play_count,
			last_played = excluded.last_played,
			skip_count = excluded.skip_count,
			file_hash = excluded.file_hash
	`,
		s.ID, s.Title, s.Artist, s.Album, s.AlbumArtist, s.TrackNumber, s.DiscNumber,
		string(genreJSON), s.Year, s.Duration, s.ReplayGainDB, s.ReplayPeak, s.FilePath, s.CoverPath, s.AddedAt,
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
			genre, year, duration, replay_gain_db, replay_peak, file_path, cover_path, added_at,
			play_count, last_played, skip_count, file_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO UPDATE SET
			file_path = excluded.file_path,
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
			replay_gain_db = excluded.replay_gain_db,
			replay_peak = excluded.replay_peak,
			cover_path = excluded.cover_path,
			file_hash = excluded.file_hash
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range songs {
		// Normalize genres for consistent capitalization
		normalizedGenres := NormalizeGenres(s.Genre)
		genreJSON, _ := json.Marshal(normalizedGenres)
		_, err = stmt.Exec(
			s.ID, s.Title, s.Artist, s.Album, s.AlbumArtist, s.TrackNumber, s.DiscNumber,
			string(genreJSON), s.Year, s.Duration, s.ReplayGainDB, s.ReplayPeak, s.FilePath, s.CoverPath, s.AddedAt,
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

// UpdateSongDuration updates the duration for a song.
// This is useful when the actual audio duration differs from metadata.
func (d *DB) UpdateSongDuration(songID string, duration float64) error {
	_, err := d.conn.Exec(`UPDATE songs SET duration = ? WHERE id = ?`, duration, songID)
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
		       mood, energy, tempo, bpm, instrumental, mood_analyzed_at,
		       liked, liked_at
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
	var liked, likedAt sql.NullInt64
	var mood, energy, tempo sql.NullString

	err := d.conn.QueryRow(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, duration, file_path, cover_path, added_at,
		       play_count, last_played, skip_count, file_hash,
		       liked, liked_at, mood, energy, tempo
		FROM songs WHERE id = ?
	`, id).Scan(
		&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist,
		&trackNum, &discNum, &genreJSON, &year,
		&s.Duration, &s.FilePath, &coverPath, &s.AddedAt,
		&playCount, &lastPlayed, &skipCount, &fileHash,
		&liked, &likedAt, &mood, &energy, &tempo,
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
	if liked.Valid {
		s.Liked = liked.Int64 == 1
	}
	if likedAt.Valid {
		s.LikedAt = likedAt.Int64
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

// UpdateSkipCount increments the skip count for the given song ID.
// This is used for preference learning in the AI DJ.
func (d *DB) UpdateSkipCount(id string) error {
	_, err := d.conn.Exec(`
		UPDATE songs 
		SET skip_count = skip_count + 1
		WHERE id = ?
	`, id)
	return err
}

// ListeningEvent represents a detailed listening event for preference learning.
// Used by the AI DJ to understand user preferences based on listening behavior.
type ListeningEvent struct {
	SongID       string  `json:"songId"`
	EventType    string  `json:"eventType"`    // 'play_complete', 'skip_early', 'skip_mid', 'skip_late'
	PlayDuration float64 `json:"playDuration"` // Seconds played before event
	SongDuration float64 `json:"songDuration"` // Total song duration
	Genre        string  `json:"genre,omitempty"`
	Mood         string  `json:"mood,omitempty"`
	Energy       string  `json:"energy,omitempty"`
	Context      string  `json:"context,omitempty"` // 'ai_dj', 'album', 'playlist', 'queue', 'search'
}

// RecordListeningEvent records a listening event for preference learning.
// Event types:
//   - 'play_complete': Song played to completion
//   - 'skip_early': Skipped within first 10 seconds
//   - 'skip_mid': Skipped between 10-30 seconds
//   - 'skip_late': Skipped after 30 seconds
//
// This data is used to build genre preferences and improve AI DJ recommendations.
func (d *DB) RecordListeningEvent(event ListeningEvent) error {
	_, err := d.conn.Exec(`
		INSERT INTO listening_events 
		(song_id, event_type, play_duration, song_duration, timestamp, genre, mood, energy, context)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.SongID, event.EventType, event.PlayDuration, event.SongDuration,
		time.Now().UnixMilli(), event.Genre, event.Mood, event.Energy, event.Context)
	return err
}

// UpdateGenrePreferences recalculates preference scores for a genre based on listening events.
// This is called after recording a listening event to update aggregate statistics.
func (d *DB) UpdateGenrePreferences(genre string) error {
	if genre == "" {
		return nil
	}

	// Calculate aggregates from listening_events
	var playCount, skipCount int
	var completeRate, skipEarlyRate float64

	// Count events by type for this genre
	row := d.conn.QueryRow(`
		SELECT 
			COUNT(*) FILTER (WHERE event_type = 'play_complete'),
			COUNT(*) FILTER (WHERE event_type IN ('skip_early', 'skip_mid', 'skip_late')),
			COALESCE(AVG(CASE WHEN event_type = 'play_complete' THEN 1.0 ELSE 0.0 END), 0.5),
			COALESCE(AVG(CASE WHEN event_type = 'skip_early' THEN 1.0 ELSE 0.0 END), 0.0)
		FROM listening_events
		WHERE genre = ?
	`, genre)
	err := row.Scan(&playCount, &skipCount, &completeRate, &skipEarlyRate)
	if err != nil {
		// SQLite doesn't support FILTER clause - use fallback
		row = d.conn.QueryRow(`
			SELECT 
				SUM(CASE WHEN event_type = 'play_complete' THEN 1 ELSE 0 END),
				SUM(CASE WHEN event_type IN ('skip_early', 'skip_mid', 'skip_late') THEN 1 ELSE 0 END)
			FROM listening_events
			WHERE genre = ?
		`, genre)
		var playCountNull, skipCountNull sql.NullInt64
		if err := row.Scan(&playCountNull, &skipCountNull); err != nil {
			return err
		}
		if playCountNull.Valid {
			playCount = int(playCountNull.Int64)
		}
		if skipCountNull.Valid {
			skipCount = int(skipCountNull.Int64)
		}

		total := playCount + skipCount
		if total > 0 {
			completeRate = float64(playCount) / float64(total)
		} else {
			completeRate = 0.5
		}

		// Calculate early skip rate
		var earlySkips sql.NullInt64
		d.conn.QueryRow(`
			SELECT COUNT(*) FROM listening_events 
			WHERE genre = ? AND event_type = 'skip_early'
		`, genre).Scan(&earlySkips)
		if skipCount > 0 && earlySkips.Valid {
			skipEarlyRate = float64(earlySkips.Int64) / float64(skipCount)
		}
	}

	// Calculate affinity score: higher complete rate = higher affinity
	// Early skips hurt more than late skips
	affinityScore := completeRate - (skipEarlyRate * 0.3)
	if affinityScore < 0 {
		affinityScore = 0
	}
	if affinityScore > 1 {
		affinityScore = 1
	}

	// Upsert genre preferences
	_, err = d.conn.Exec(`
		INSERT INTO genre_preferences (genre, play_count, skip_count, complete_rate, skip_early_rate, affinity_score, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(genre) DO UPDATE SET
			play_count = excluded.play_count,
			skip_count = excluded.skip_count,
			complete_rate = excluded.complete_rate,
			skip_early_rate = excluded.skip_early_rate,
			affinity_score = excluded.affinity_score,
			last_updated = excluded.last_updated
	`, genre, playCount, skipCount, completeRate, skipEarlyRate, affinityScore, time.Now().UnixMilli())
	return err
}

// GetGenrePreferences returns the preference data for a genre.
// Returns nil if no preferences exist for this genre.
func (d *DB) GetGenrePreferences(genre string) (*GenrePreference, error) {
	var pref GenrePreference
	err := d.conn.QueryRow(`
		SELECT genre, play_count, skip_count, complete_rate, skip_early_rate, affinity_score, last_updated
		FROM genre_preferences
		WHERE genre = ?
	`, genre).Scan(&pref.Genre, &pref.PlayCount, &pref.SkipCount, &pref.CompleteRate,
		&pref.SkipEarlyRate, &pref.AffinityScore, &pref.LastUpdated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

// GenrePreference represents aggregated listening preferences for a genre.
type GenrePreference struct {
	Genre         string  `json:"genre"`
	PlayCount     int     `json:"playCount"`
	SkipCount     int     `json:"skipCount"`
	CompleteRate  float64 `json:"completeRate"`  // 0.0-1.0
	SkipEarlyRate float64 `json:"skipEarlyRate"` // 0.0-1.0
	AffinityScore float64 `json:"affinityScore"` // 0.0-1.0
	LastUpdated   int64   `json:"lastUpdated"`
}

// GetAllGenrePreferences returns all genre preferences ordered by affinity score.
func (d *DB) GetAllGenrePreferences() ([]GenrePreference, error) {
	rows, err := d.conn.Query(`
		SELECT genre, play_count, skip_count, complete_rate, skip_early_rate, affinity_score, last_updated
		FROM genre_preferences
		ORDER BY affinity_score DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prefs []GenrePreference
	for rows.Next() {
		var p GenrePreference
		if err := rows.Scan(&p.Genre, &p.PlayCount, &p.SkipCount, &p.CompleteRate,
			&p.SkipEarlyRate, &p.AffinityScore, &p.LastUpdated); err != nil {
			return nil, err
		}
		prefs = append(prefs, p)
	}
	return prefs, rows.Err()
}

// ToggleLike toggles the liked status of a song and returns the new liked state.
// If the song is currently liked, it will be unliked (and likedAt cleared).
// If the song is not liked, it will be liked (and likedAt set to current time).
func (d *DB) ToggleLike(songID string) (bool, int64, error) {
	// First get current state
	var liked sql.NullInt64
	err := d.conn.QueryRow(`SELECT liked FROM songs WHERE id = ?`, songID).Scan(&liked)
	if err != nil {
		return false, 0, err
	}

	isCurrentlyLiked := liked.Valid && liked.Int64 == 1
	newLiked := !isCurrentlyLiked
	var likedAt int64

	if newLiked {
		likedAt = time.Now().UnixMilli()
		_, err = d.conn.Exec(`UPDATE songs SET liked = 1, liked_at = ? WHERE id = ?`, likedAt, songID)
	} else {
		likedAt = 0
		_, err = d.conn.Exec(`UPDATE songs SET liked = 0, liked_at = NULL WHERE id = ?`, songID)
	}

	if err != nil {
		return false, 0, err
	}
	return newLiked, likedAt, nil
}

// SetLike explicitly sets the liked status of a song.
// Pass liked=true to like, liked=false to unlike.
func (d *DB) SetLike(songID string, liked bool) (int64, error) {
	var likedAt int64
	var err error

	if liked {
		likedAt = time.Now().UnixMilli()
		_, err = d.conn.Exec(`UPDATE songs SET liked = 1, liked_at = ? WHERE id = ?`, likedAt, songID)
	} else {
		likedAt = 0
		_, err = d.conn.Exec(`UPDATE songs SET liked = 0, liked_at = NULL WHERE id = ?`, songID)
	}

	return likedAt, err
}

// BulkSetLike sets the liked status for multiple songs at once.
// Returns the number of songs updated.
func (d *DB) BulkSetLike(songIDs []string, liked bool) (int, error) {
	if len(songIDs) == 0 {
		return 0, nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var updated int
	likedAt := time.Now().UnixMilli()

	for _, id := range songIDs {
		var result sql.Result
		if liked {
			result, err = tx.Exec(`UPDATE songs SET liked = 1, liked_at = ? WHERE id = ?`, likedAt, id)
		} else {
			result, err = tx.Exec(`UPDATE songs SET liked = 0, liked_at = NULL WHERE id = ?`, id)
		}
		if err != nil {
			return 0, err
		}
		affected, _ := result.RowsAffected()
		updated += int(affected)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}

// GetLikedSongIDs returns the IDs of all liked songs, ordered by when they were liked (newest first).
func (d *DB) GetLikedSongIDs() ([]string, error) {
	rows, err := d.conn.Query(`
		SELECT id FROM songs 
		WHERE liked = 1
		ORDER BY liked_at DESC
	`)
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

// Album Like Operations

// ToggleAlbumLike toggles the liked status of an album and returns the new liked state.
// If the album doesn't exist in album_metadata, it creates an entry for it.
func (d *DB) ToggleAlbumLike(albumKey string) (bool, int64, error) {
	// First check if album metadata exists
	var liked sql.NullInt64
	err := d.conn.QueryRow(`SELECT liked FROM album_metadata WHERE album_key = ?`, albumKey).Scan(&liked)

	if err == sql.ErrNoRows {
		// Album doesn't exist in metadata table - create a basic entry
		// Parse album_key format: "AlbumName::ArtistName"
		parts := strings.SplitN(albumKey, "::", 2)
		albumName := albumKey
		artistName := ""
		if len(parts) == 2 {
			albumName = parts[0]
			artistName = parts[1]
		}

		likedAt := time.Now().UnixMilli()
		_, err = d.conn.Exec(`
			INSERT INTO album_metadata (album_key, album_name, artist_name, liked, liked_at)
			VALUES (?, ?, ?, 1, ?)
		`, albumKey, albumName, artistName, likedAt)
		if err != nil {
			return false, 0, err
		}
		return true, likedAt, nil
	}
	if err != nil {
		return false, 0, err
	}

	isCurrentlyLiked := liked.Valid && liked.Int64 == 1
	newLiked := !isCurrentlyLiked
	var likedAt int64

	if newLiked {
		likedAt = time.Now().UnixMilli()
		_, err = d.conn.Exec(`UPDATE album_metadata SET liked = 1, liked_at = ? WHERE album_key = ?`, likedAt, albumKey)
	} else {
		likedAt = 0
		_, err = d.conn.Exec(`UPDATE album_metadata SET liked = 0, liked_at = NULL WHERE album_key = ?`, albumKey)
	}

	if err != nil {
		return false, 0, err
	}
	return newLiked, likedAt, nil
}

// GetLikedAlbumKeys returns the album_keys of all liked albums, ordered by when they were liked (newest first).
func (d *DB) GetLikedAlbumKeys() ([]string, error) {
	rows, err := d.conn.Query(`
		SELECT album_key FROM album_metadata 
		WHERE liked = 1
		ORDER BY liked_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// GetLikedAlbums returns all liked albums with their metadata.
func (d *DB) GetLikedAlbums() ([]AlbumMetadata, error) {
	rows, err := d.conn.Query(`
		SELECT album_key, album_name, artist_name, spotify_id, cover_url, local_cover_path,
		       description, genre, release_date, spotify_url, copyright,
		       spotify_checked, spotify_found, fetched_at, updated_at, liked, liked_at
		FROM album_metadata
		WHERE liked = 1
		ORDER BY liked_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AlbumMetadata
	for rows.Next() {
		var m AlbumMetadata
		var spotifyID, coverURL, localCoverPath, description, genre, releaseDate, spotifyURL, copyright sql.NullString
		var fetchedAt, updatedAt, liked, likedAt sql.NullInt64
		var spotifyChecked, spotifyFound sql.NullInt64

		err := rows.Scan(
			&m.AlbumKey, &m.AlbumName, &m.ArtistName, &spotifyID, &coverURL, &localCoverPath,
			&description, &genre, &releaseDate, &spotifyURL, &copyright,
			&spotifyChecked, &spotifyFound, &fetchedAt, &updatedAt, &liked, &likedAt,
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
		if liked.Valid {
			m.Liked = liked.Int64 == 1
		}
		if likedAt.Valid {
			m.LikedAt = likedAt.Int64
		}

		results = append(results, m)
	}

	return results, rows.Err()
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
		       mood, energy, tempo, bpm, instrumental, mood_analyzed_at,
		       liked, liked_at
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
		       mood, energy, tempo, bpm, instrumental, mood_analyzed_at,
		       liked, liked_at
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
		       mood, energy, tempo, bpm, instrumental, mood_analyzed_at,
		       liked, liked_at
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

// scanSongsWithMood is a helper to scan song rows including mood and like fields.
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
		var liked, likedAt sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist,
			&trackNum, &discNum, &genreJSON, &year,
			&s.Duration, &s.FilePath, &coverPath, &s.AddedAt,
			&playCount, &lastPlayed, &skipCount, &fileHash,
			&mood, &energy, &tempo, &bpm, &instrumental, &moodAnalyzedAt,
			&liked, &likedAt,
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
		if liked.Valid {
			s.Liked = liked.Int64 == 1
		}
		if likedAt.Valid {
			s.LikedAt = likedAt.Int64
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

// AddDownloads inserts a logical album or playlist as one transaction. Either
// every track is visible to the dispatcher or none are.
func (d *DB) AddDownloads(downloads []*SpotifyDownload) ([]string, error) {
	if len(downloads) == 0 {
		return []string{}, nil
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	statement, err := tx.Prepare(`
		INSERT INTO spotify_downloads (
			id, spotify_id, spotify_uri, type, title, artist, album,
			status, progress, error, file_path, added_at, started_at,
			completed_at, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, err
	}
	defer statement.Close()
	ids := make([]string, 0, len(downloads))
	for _, download := range downloads {
		var existingID string
		err := tx.QueryRow(`
			SELECT id FROM spotify_downloads
			WHERE spotify_id = ? AND metadata = ? AND status IN ('queued', 'downloading')
			ORDER BY added_at ASC LIMIT 1
		`, download.SpotifyID, download.Metadata).Scan(&existingID)
		if err == nil {
			ids = append(ids, existingID)
			continue
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
		if _, err := statement.Exec(download.ID, download.SpotifyID, download.SpotifyURI,
			download.Type, download.Title, download.Artist, download.Album, download.Status,
			download.Progress, download.Error, download.FilePath, download.AddedAt,
			download.StartedAt, download.CompletedAt, download.Metadata); err != nil {
			return nil, err
		}
		ids = append(ids, download.ID)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
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
func (d *DB) GetAllDownloads(limit, offset int) ([]SpotifyDownload, error) {
	if limit <= 0 || limit > 500 {
		limit = 250
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := d.conn.Query(`
		SELECT id, spotify_id, spotify_uri, type, title, artist, album,
		       status, progress, error, file_path, added_at, started_at,
		       completed_at, metadata
		FROM spotify_downloads
		ORDER BY added_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
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

// CountActiveDownloads returns the exact number of queued and in-progress
// downloads without applying the history-list pagination limit.
func (d *DB) CountActiveDownloads() (int, error) {
	var count int
	err := d.conn.QueryRow(`
		SELECT COUNT(*)
		FROM spotify_downloads
		WHERE status IN ('queued', 'downloading')
	`).Scan(&count)
	return count, err
}

// GetQueuedDownloads returns queued Spotify downloads ordered by added time.
func (d *DB) GetQueuedDownloads(limit int) ([]SpotifyDownload, error) {
	if limit <= 0 {
		return []SpotifyDownload{}, nil
	}
	rows, err := d.conn.Query(`
		SELECT id, spotify_id, spotify_uri, type, title, artist, album,
		       status, progress, error, file_path, added_at, started_at,
		       completed_at, metadata
		FROM spotify_downloads
		WHERE status = 'queued'
		ORDER BY added_at ASC, rowid ASC
		LIMIT ?
	`, limit)
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
func (d *DB) MarkDownloadStarted(id string) (bool, error) {
	result, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'downloading', started_at = ?, completed_at = NULL, error = NULL
		WHERE id = ? AND status = 'queued'
	`, time.Now().Unix(), id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// MarkDownloadCompleted marks a download as complete and stores the file path.
func (d *DB) MarkDownloadCompleted(id string, filePath string) (bool, error) {
	result, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'completed', progress = 100, file_path = ?, completed_at = ?
		WHERE id = ? AND status = 'downloading'
	`, filePath, time.Now().Unix(), id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// MarkDownloadFailed marks a download as failed and records an error message.
func (d *DB) MarkDownloadFailed(id string, errorMsg string) (bool, error) {
	result, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'failed', error = ?, completed_at = ?
		WHERE id = ? AND status = 'downloading'
	`, errorMsg, time.Now().Unix(), id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// DeleteDownload removes a download record from the database.
func (d *DB) DeleteDownload(id string) error {
	_, err := d.conn.Exec("DELETE FROM spotify_downloads WHERE id = ?", id)
	return err
}

// ResetDownloadForRetry resets a failed download back to queued status
func (d *DB) ResetDownloadForRetry(id string) error {
	result, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'queued', progress = 0, error = NULL, completed_at = NULL, added_at = ?
		WHERE id = ? AND status = 'failed'
	`, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("download is not failed or does not exist")
	}
	return nil
}

// ResetDownloadForForceRestart makes any non-deleted download eligible for a
// fresh attempt. An active worker remains registered in memory until it exits,
// preventing the dispatcher from starting the replacement concurrently.
func (d *DB) ResetDownloadForForceRestart(id string) error {
	result, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'queued', progress = 0, error = NULL, file_path = NULL,
			started_at = NULL, completed_at = NULL, added_at = ?
		WHERE id = ?
	`, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("download does not exist")
	}
	return nil
}

// RequeueDownloading returns an active item to the queue without allowing a
// stale worker to overwrite a newer state.
func (d *DB) RequeueDownloading(id string) (bool, error) {
	result, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'queued', progress = 0, error = NULL, started_at = NULL
		WHERE id = ? AND status = 'downloading'
	`, id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// ResetStuckDownloads resets all downloads that are stuck in 'downloading' status
// back to 'queued'. This is called on startup to recover downloads that were
// interrupted by application crashes or restarts.
func (d *DB) ResetStuckDownloads() (int64, error) {
	result, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'queued', progress = 0
		WHERE status = 'downloading'
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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

// SetSettingsBatch atomically saves multiple settings in a single transaction.
func (d *DB) SetSettingsBatch(settings map[string]string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for key, value := range settings {
		storeValue := value
		if crypto.IsSensitiveKey(key) && value != "" {
			encrypted, err := crypto.Encrypt(value)
			if err != nil {
				return fmt.Errorf("failed to encrypt setting %s: %w", key, err)
			}
			storeValue = encrypted
		}
		if _, err := tx.Exec(`
			INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value
		`, key, storeValue); err != nil {
			return fmt.Errorf("failed to save setting %s: %w", key, err)
		}
	}

	return tx.Commit()
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
	Liked          bool   `json:"liked,omitempty"`   // True if user has liked this album
	LikedAt        int64  `json:"likedAt,omitempty"` // Timestamp when album was liked
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
		       spotify_checked, spotify_found, fetched_at, updated_at, liked, liked_at
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
		var fetchedAt, updatedAt, liked, likedAt sql.NullInt64
		var spotifyChecked, spotifyFound sql.NullInt64

		err := rows.Scan(
			&m.AlbumKey, &m.AlbumName, &m.ArtistName, &spotifyID, &coverURL, &localCoverPath,
			&description, &genre, &releaseDate, &spotifyURL, &copyright,
			&spotifyChecked, &spotifyFound, &fetchedAt, &updatedAt, &liked, &likedAt,
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
		if liked.Valid {
			m.Liked = liked.Int64 == 1
		}
		if likedAt.Valid {
			m.LikedAt = likedAt.Int64
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

// BackfillSongYearsFromAlbumMetadata populates missing song year values from album_metadata.release_date.
// This enables the AI DJ to correctly filter by decade (e.g., "90s hip hop") even when songs
// don't have year metadata embedded in their audio files.
// Returns the count of songs updated.
func (d *DB) BackfillSongYearsFromAlbumMetadata() (int64, error) {
	// Update songs where year is 0 or NULL, joining on album_metadata to get release_date
	// The album_key format is "album::artist" but we need to match on album + artist separately
	result, err := d.conn.Exec(`
		UPDATE songs SET year = CAST(SUBSTR(am.release_date, 1, 4) AS INTEGER)
		FROM album_metadata am
		WHERE songs.year IS NULL OR songs.year = 0
		AND am.release_date IS NOT NULL 
		AND LENGTH(am.release_date) >= 4
		AND CAST(SUBSTR(am.release_date, 1, 4) AS INTEGER) > 1900
		AND am.album_name = songs.album
		AND (am.artist_name = songs.artist OR am.artist_name = songs.album_artist)
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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
// If force is false, it returns only songs with 0 or 1 genres (needs enrichment).
// Songs with 0-1 genres are considered "under-enriched" and benefit from AI analysis.
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
		// Include songs with no genres OR only 1 genre
		// JSON array length check: count commas (0 commas = 0-1 elements)
		// Empty/null: genre IS NULL OR genre = '' OR genre = '[]' OR genre = 'null'
		// Single genre: genre LIKE '["%"]' AND genre NOT LIKE '%,%' (one element, no comma)
		query = baseQuery + `
			WHERE genre IS NULL 
			   OR genre = '' 
			   OR genre = '[]' 
			   OR genre = 'null'
			   OR (genre LIKE '["%"]' AND genre NOT LIKE '%,%')
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

// GetSongsForAIEnrichment returns songs with at least one field that still
// benefits from AI enrichment. Unlike the older genre-only query, this also
// includes tracks without mood data and flagged remasters without an original
// release year. Results are stable and can be paged without a 10,000-song cap.
func (d *DB) GetSongsForAIEnrichment(limit int, force bool, offset int) ([]Song, error) {
	baseQuery := `
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, original_year, year_uncertain, mood, energy, tempo, bpm, instrumental
		FROM songs WHERE COALESCE(ignored, 0) = 0`
	args := []interface{}{limit, offset}
	if !force {
		baseQuery += ` AND (
			genre IS NULL OR genre = '' OR genre = '[]' OR genre = 'null'
			OR (genre LIKE '["%"]' AND genre NOT LIKE '%,%')
			OR mood IS NULL OR mood = ''
			OR (COALESCE(year_uncertain, 0) = 1 AND COALESCE(original_year, 0) = 0)
		)`
	}
	rows, err := d.conn.Query(baseQuery+` ORDER BY id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query songs for AI enrichment: %w", err)
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON, albumArtist, mood, energy, tempo sql.NullString
		var trackNum, discNum, originalYear, bpm sql.NullInt64
		var yearUncertain, instrumental sql.NullBool
		if err := rows.Scan(&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist, &trackNum, &discNum,
			&genreJSON, &s.Year, &originalYear, &yearUncertain, &mood, &energy, &tempo, &bpm, &instrumental); err != nil {
			return nil, fmt.Errorf("failed to scan AI enrichment song: %w", err)
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
		if originalYear.Valid {
			s.OriginalYear = int(originalYear.Int64)
		}
		if yearUncertain.Valid {
			s.YearUncertain = yearUncertain.Bool
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
			s.Instrumental = instrumental.Bool
		}
		if genreJSON.Valid && genreJSON.String != "" {
			_ = json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		songs = append(songs, s)
	}
	return songs, rows.Err()
}

// AIEnrichmentUpdate is a validated AI result that can be committed atomically.
type AIEnrichmentUpdate struct {
	SongID       string
	Genres       []string
	Mood         string
	Energy       string
	Tempo        string
	BPM          int
	Instrumental bool
	OriginalYear int
}

// AIEnrichmentApplyResult reports the number of fields changed in one batch.
type AIEnrichmentApplyResult struct{ Genres, Mood, Years int }

// ApplyAIEnrichmentBatch applies a batch transactionally and only fills fields
// that are missing or low-detail unless force is requested.
func (d *DB) ApplyAIEnrichmentBatch(updates []AIEnrichmentUpdate, force bool) (AIEnrichmentApplyResult, error) {
	var result AIEnrichmentApplyResult
	tx, err := d.conn.Begin()
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, update := range updates {
		var genreJSON, mood, energy, tempo sql.NullString
		var originalYear sql.NullInt64
		var yearUncertain sql.NullBool
		if err := tx.QueryRow(`SELECT genre, mood, energy, tempo, original_year, year_uncertain FROM songs WHERE id = ?`, update.SongID).
			Scan(&genreJSON, &mood, &energy, &tempo, &originalYear, &yearUncertain); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return result, fmt.Errorf("read enrichment target: %w", err)
		}

		currentGenre := genreJSON.String
		applyGenres := len(update.Genres) > 0 && (force || genreNeedsEnrichment(currentGenre))
		applyMood := update.Mood != "" && (force || !mood.Valid || strings.TrimSpace(mood.String) == "")
		applyYear := update.OriginalYear > 0 && (force || (yearUncertain.Valid && yearUncertain.Bool && (!originalYear.Valid || originalYear.Int64 == 0)))
		if !applyGenres && !applyMood && !applyYear {
			continue
		}

		newGenre, newMood, newEnergy, newTempo := currentGenre, mood.String, energy.String, tempo.String
		newOriginalYear, newYearUncertain := originalYear.Int64, yearUncertain.Bool
		newBPM, newInstrumental := 0, false
		if applyGenres {
			normalized := NormalizeGenres(update.Genres)
			if encoded, err := json.Marshal(normalized); err != nil {
				return result, err
			} else {
				newGenre = string(encoded)
			}
			result.Genres++
		}
		if applyMood {
			newMood, newEnergy, newTempo = update.Mood, update.Energy, update.Tempo
			newBPM, newInstrumental = update.BPM, update.Instrumental
			result.Mood++
		}
		if applyYear {
			newOriginalYear, newYearUncertain = int64(update.OriginalYear), false
			result.Years++
		}
		now := time.Now().Unix()
		if _, err := tx.Exec(`UPDATE songs SET genre = ?, mood = ?, energy = ?, tempo = ?, bpm = CASE WHEN ? THEN ? ELSE bpm END, instrumental = CASE WHEN ? THEN ? ELSE instrumental END, mood_analyzed_at = CASE WHEN ? THEN ? ELSE mood_analyzed_at END, original_year = ?, year_uncertain = ?, year_analyzed_at = CASE WHEN ? THEN ? ELSE year_analyzed_at END WHERE id = ?`,
			newGenre, newMood, newEnergy, newTempo, applyMood, newBPM, applyMood, newInstrumental, applyMood, now, newOriginalYear, newYearUncertain, applyYear, now, update.SongID); err != nil {
			return result, fmt.Errorf("apply enrichment update: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func genreNeedsEnrichment(genreJSON string) bool {
	genreJSON = strings.TrimSpace(genreJSON)
	return genreJSON == "" || genreJSON == "[]" || genreJSON == "null" ||
		(strings.HasPrefix(genreJSON, `["`) && !strings.Contains(genreJSON, ","))
}

// UpdateSongGenres updates the genre list for a specific song.
// Genres are normalized to consistent Title Case capitalization before saving.
func (d *DB) UpdateSongGenres(songID string, genres []string) error {
	// Normalize genres for consistent capitalization
	normalized := NormalizeGenres(genres)

	genreJSON, err := json.Marshal(normalized)
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
// This function supports the AI DJ feature by querying songs based on:
// - Genre matching (partial, case-insensitive)
// - Artist matching (partial, case-insensitive)
// - Year range filtering (minYear to maxYear)
// - Mood/Energy/Tempo filtering (exact match when provided)
//
// The mood/energy/tempo fields are populated by Gemini AI analysis via AnalyzeSongMood().
// When these filters are provided, only songs with matching analyzed metadata are returned.
// Songs without mood analysis are excluded when mood filters are active.
//
// Results are randomized and limited to 50 songs by default.
func (d *DB) GetSongsBySmartFilter(genres []string, artists []string, minYear, maxYear int, mood, energy, tempo string) ([]Song, error) {
	query := "SELECT id, title, artist, album, genre, year, duration, file_path, cover_path, added_at, play_count, last_played, mood, energy, tempo FROM songs WHERE 1=1"
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

	// Year filters using COALESCE to prefer original_year (for remasters) over embedded year
	if minYear > 0 {
		query += " AND COALESCE(original_year, year) >= ?"
		args = append(args, minYear)
	}
	if maxYear > 0 {
		query += " AND COALESCE(original_year, year) <= ?"
		args = append(args, maxYear)
	}

	// Mood/Energy/Tempo filters - exact match when provided
	// These fields are populated by Gemini AI analysis via AnalyzeSongMood()
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
		var moodVal, energyVal, tempoVal sql.NullString

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &genreJSON, &s.Year, &s.Duration,
			&s.FilePath, &coverPath, &s.AddedAt, &s.PlayCount, &lastPlayed,
			&moodVal, &energyVal, &tempoVal,
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
		if moodVal.Valid {
			s.Mood = moodVal.String
		}
		if energyVal.Valid {
			s.Energy = energyVal.String
		}
		if tempoVal.Valid {
			s.Tempo = tempoVal.String
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

// ============================================================================
// Last.FM Integration Types and Methods
// ============================================================================

// LastFMSongUpdate contains Last.FM enrichment data for a song
type LastFMSongUpdate struct {
	Listeners    int      `json:"listeners"`
	Playcount    int      `json:"playcount"`
	Tags         []string `json:"tags"`
	URL          string   `json:"url"`
	MBID         string   `json:"mbid"`
	Mood         string   `json:"mood,omitempty"`
	Energy       string   `json:"energy,omitempty"`
	Tempo        string   `json:"tempo,omitempty"`
	Genres       []string `json:"genres,omitempty"`
	Instrumental bool     `json:"instrumental,omitempty"`
}

// LastFMArtistUpdate contains Last.FM enrichment data for an artist
type LastFMArtistUpdate struct {
	Listeners int      `json:"listeners"`
	Playcount int      `json:"playcount"`
	Tags      []string `json:"tags"`
	Bio       string   `json:"bio"`
	URL       string   `json:"url"`
	MBID      string   `json:"mbid"`
}

// LastFMSimilarTrack represents a similar track from Last.FM
type LastFMSimilarTrack struct {
	SongID        string  `json:"songId"`
	SimilarArtist string  `json:"similarArtist"`
	SimilarTrack  string  `json:"similarTrack"`
	MatchScore    float64 `json:"matchScore"`
	SimilarSongID string  `json:"similarSongId,omitempty"` // If we have this song in our library
}

// LastFMSimilarArtist represents a similar artist from Last.FM
type LastFMSimilarArtist struct {
	ArtistName    string  `json:"artistName"`
	SimilarArtist string  `json:"similarArtist"`
	MatchScore    float64 `json:"matchScore"`
}

// UpdateSongLastFM updates a song with Last.FM enrichment data
func (d *DB) UpdateSongLastFM(songID string, update LastFMSongUpdate) error {
	tagsJSON, err := json.Marshal(update.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	now := time.Now().Unix()

	// Build dynamic update query based on what data we have
	query := `UPDATE songs SET 
		lastfm_listeners = ?,
		lastfm_playcount = ?,
		lastfm_tags = ?,
		lastfm_url = ?,
		lastfm_mbid = ?,
		lastfm_enriched_at = ?`

	args := []interface{}{
		update.Listeners,
		update.Playcount,
		string(tagsJSON),
		update.URL,
		update.MBID,
		now,
	}

	// Also update mood/energy/tempo if provided and not already set
	if update.Mood != "" {
		query += `, mood = COALESCE(NULLIF(mood, ''), ?)`
		args = append(args, update.Mood)
	}
	if update.Energy != "" {
		query += `, energy = COALESCE(NULLIF(energy, ''), ?)`
		args = append(args, update.Energy)
	}
	if update.Tempo != "" {
		query += `, tempo = COALESCE(NULLIF(tempo, ''), ?)`
		args = append(args, update.Tempo)
	}
	if len(update.Genres) > 0 {
		genresJSON, _ := json.Marshal(update.Genres)
		query += `, genre = COALESCE(NULLIF(genre, ''), NULLIF(genre, '[]'), ?)`
		args = append(args, string(genresJSON))
	}
	if update.Instrumental {
		query += `, instrumental = 1`
	}

	query += ` WHERE id = ?`
	args = append(args, songID)

	_, err = d.conn.Exec(query, args...)
	return err
}

// StoreSimilarTracks stores similar track relationships from Last.FM
func (d *DB) StoreSimilarTracks(songID string, tracks []LastFMSimilarTrack) error {
	if len(tracks) == 0 {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing similar tracks for this song
	_, err = tx.Exec(`DELETE FROM lastfm_similar_tracks WHERE song_id = ?`, songID)
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO lastfm_similar_tracks (song_id, similar_artist, similar_track, match_score, similar_song_id)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, track := range tracks {
		var similarSongID interface{}
		if track.SimilarSongID != "" {
			similarSongID = track.SimilarSongID
		}
		_, err = stmt.Exec(songID, track.SimilarArtist, track.SimilarTrack, track.MatchScore, similarSongID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetSimilarTracks retrieves similar tracks for a song
func (d *DB) GetSimilarTracks(songID string) ([]LastFMSimilarTrack, error) {
	rows, err := d.conn.Query(`
		SELECT song_id, similar_artist, similar_track, match_score, similar_song_id
		FROM lastfm_similar_tracks
		WHERE song_id = ?
		ORDER BY match_score DESC
	`, songID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []LastFMSimilarTrack
	for rows.Next() {
		var t LastFMSimilarTrack
		var similarSongID sql.NullString
		err := rows.Scan(&t.SongID, &t.SimilarArtist, &t.SimilarTrack, &t.MatchScore, &similarSongID)
		if err != nil {
			return nil, err
		}
		if similarSongID.Valid {
			t.SimilarSongID = similarSongID.String
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

// StoreSimilarArtists stores similar artist relationships from Last.FM
func (d *DB) StoreSimilarArtists(artistName string, similar []LastFMSimilarArtist) error {
	if len(similar) == 0 {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing similar artists
	_, err = tx.Exec(`DELETE FROM lastfm_similar_artists WHERE artist_name = ?`, artistName)
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO lastfm_similar_artists (artist_name, similar_artist, match_score)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, artist := range similar {
		_, err = stmt.Exec(artistName, artist.SimilarArtist, artist.MatchScore)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetLastFMSimilarArtists retrieves similar artists from Last.FM data
func (d *DB) GetLastFMSimilarArtists(artistName string) ([]LastFMSimilarArtist, error) {
	rows, err := d.conn.Query(`
		SELECT artist_name, similar_artist, match_score
		FROM lastfm_similar_artists
		WHERE artist_name = ?
		ORDER BY match_score DESC
	`, artistName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artists []LastFMSimilarArtist
	for rows.Next() {
		var a LastFMSimilarArtist
		err := rows.Scan(&a.ArtistName, &a.SimilarArtist, &a.MatchScore)
		if err != nil {
			return nil, err
		}
		artists = append(artists, a)
	}
	return artists, rows.Err()
}

// GetSongsWithoutLastFM retrieves songs that haven't been enriched with Last.FM data
func (d *DB) GetSongsWithoutLastFM(limit int) ([]Song, error) {
	rows, err := d.conn.Query(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number, genre,
		       year, original_year, year_uncertain, year_analyzed_at, duration, file_path, cover_path,
		       added_at, play_count, last_played, skip_count, file_hash, mood, energy, tempo,
		       bpm, instrumental, mood_analyzed_at, liked, liked_at,
		       lastfm_listeners, lastfm_playcount, lastfm_tags, lastfm_url, lastfm_mbid, lastfm_enriched_at
		FROM songs
		WHERE lastfm_enriched_at IS NULL OR lastfm_enriched_at = 0
		ORDER BY play_count DESC, added_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return d.scanSongsWithLastFM(rows)
}

// scanSongsWithLastFM is a helper to scan songs including Last.FM fields
func (d *DB) scanSongsWithLastFM(rows *sql.Rows) ([]Song, error) {
	var songs []Song
	for rows.Next() {
		var s Song
		var albumArtist, coverPath, fileHash, mood, energy, tempo, genreStr sql.NullString
		var trackNum, discNum, year, originalYear, bpm, playCount, skipCount sql.NullInt64
		var lastPlayed, moodAnalyzedAt, likedAt, yearAnalyzedAt sql.NullInt64
		var instrumental, liked, yearUncertain sql.NullInt64
		var lastfmListeners, lastfmPlaycount sql.NullInt64
		var lastfmTags, lastfmURL, lastfmMBID sql.NullString
		var lastfmEnrichedAt sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist, &trackNum, &discNum, &genreStr,
			&year, &originalYear, &yearUncertain, &yearAnalyzedAt, &s.Duration, &s.FilePath, &coverPath,
			&s.AddedAt, &playCount, &lastPlayed, &skipCount, &fileHash, &mood, &energy, &tempo,
			&bpm, &instrumental, &moodAnalyzedAt, &liked, &likedAt,
			&lastfmListeners, &lastfmPlaycount, &lastfmTags, &lastfmURL, &lastfmMBID, &lastfmEnrichedAt,
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
		if genreStr.Valid && genreStr.String != "" {
			_ = json.Unmarshal([]byte(genreStr.String), &s.Genre)
		}
		if year.Valid {
			s.Year = int(year.Int64)
		}
		if originalYear.Valid {
			s.OriginalYear = int(originalYear.Int64)
		}
		if yearUncertain.Valid {
			s.YearUncertain = yearUncertain.Int64 == 1
		}
		if yearAnalyzedAt.Valid {
			s.YearAnalyzedAt = yearAnalyzedAt.Int64
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
		if liked.Valid {
			s.Liked = liked.Int64 == 1
		}
		if likedAt.Valid {
			s.LikedAt = likedAt.Int64
		}
		if lastfmListeners.Valid {
			s.LastFMListeners = int(lastfmListeners.Int64)
		}
		if lastfmPlaycount.Valid {
			s.LastFMPlaycount = int(lastfmPlaycount.Int64)
		}
		if lastfmTags.Valid {
			s.LastFMTags = lastfmTags.String
		}
		if lastfmURL.Valid {
			s.LastFMURL = lastfmURL.String
		}
		if lastfmMBID.Valid {
			s.LastFMMBID = lastfmMBID.String
		}
		if lastfmEnrichedAt.Valid {
			s.LastFMEnrichedAt = lastfmEnrichedAt.Int64
		}

		songs = append(songs, s)
	}
	return songs, rows.Err()
}

// UpdateArtistLastFM updates artist metadata with Last.FM enrichment data
func (d *DB) UpdateArtistLastFM(artistName string, update LastFMArtistUpdate) error {
	tagsJSON, err := json.Marshal(update.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	now := time.Now().Unix()

	// First check if artist exists in metadata table
	var exists int
	err = d.conn.QueryRow(`SELECT COUNT(*) FROM artist_metadata WHERE artist_name = ?`, artistName).Scan(&exists)
	if err != nil {
		return err
	}

	if exists == 0 {
		// Insert new artist metadata record
		_, err = d.conn.Exec(`
			INSERT INTO artist_metadata (artist_name, lastfm_listeners, lastfm_playcount, lastfm_tags, lastfm_bio, lastfm_url, lastfm_mbid, lastfm_enriched_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, artistName, update.Listeners, update.Playcount, string(tagsJSON), update.Bio, update.URL, update.MBID, now, now)
	} else {
		// Update existing record
		_, err = d.conn.Exec(`
			UPDATE artist_metadata SET
				lastfm_listeners = ?,
				lastfm_playcount = ?,
				lastfm_tags = ?,
				lastfm_bio = ?,
				lastfm_url = ?,
				lastfm_mbid = ?,
				lastfm_enriched_at = ?,
				updated_at = ?
			WHERE artist_name = ?
		`, update.Listeners, update.Playcount, string(tagsJSON), update.Bio, update.URL, update.MBID, now, now, artistName)
	}

	return err
}

// GetArtistsWithoutLastFM retrieves artists that haven't been enriched with Last.FM data
func (d *DB) GetArtistsWithoutLastFM(limit int) ([]string, error) {
	// Get unique artists from songs that don't have Last.FM data in artist_metadata
	rows, err := d.conn.Query(`
		SELECT DISTINCT s.artist
		FROM songs s
		LEFT JOIN artist_metadata am ON s.artist = am.artist_name
		WHERE am.lastfm_enriched_at IS NULL OR am.lastfm_enriched_at = 0
		ORDER BY (SELECT COUNT(*) FROM songs WHERE artist = s.artist) DESC
		LIMIT ?
	`, limit)
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

// GetLastFMEnrichmentStats returns statistics about Last.FM enrichment
func (d *DB) GetLastFMEnrichmentStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count songs with Last.FM data
	var enrichedSongs, totalSongs int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM songs WHERE lastfm_enriched_at > 0`).Scan(&enrichedSongs)
	if err != nil {
		return nil, err
	}
	err = d.conn.QueryRow(`SELECT COUNT(*) FROM songs`).Scan(&totalSongs)
	if err != nil {
		return nil, err
	}

	// Count artists with Last.FM data
	var enrichedArtists, totalArtists int
	err = d.conn.QueryRow(`SELECT COUNT(*) FROM artist_metadata WHERE lastfm_enriched_at > 0`).Scan(&enrichedArtists)
	if err != nil {
		return nil, err
	}
	err = d.conn.QueryRow(`SELECT COUNT(DISTINCT artist) FROM songs`).Scan(&totalArtists)
	if err != nil {
		return nil, err
	}

	// Count similar track/artist relationships
	var similarTracks, similarArtists int
	err = d.conn.QueryRow(`SELECT COUNT(*) FROM lastfm_similar_tracks`).Scan(&similarTracks)
	if err != nil {
		return nil, err
	}
	err = d.conn.QueryRow(`SELECT COUNT(*) FROM lastfm_similar_artists`).Scan(&similarArtists)
	if err != nil {
		return nil, err
	}

	stats["enrichedSongs"] = enrichedSongs
	stats["totalSongs"] = totalSongs
	stats["enrichedArtists"] = enrichedArtists
	stats["totalArtists"] = totalArtists
	stats["similarTracks"] = similarTracks
	stats["similarArtists"] = similarArtists

	return stats, nil
}

// FindSongByArtistAndTitle finds a song in the library by artist and title (case-insensitive)
func (d *DB) FindSongByArtistAndTitle(artist, title string) (*Song, error) {
	row := d.conn.QueryRow(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number, genre,
		       year, original_year, year_uncertain, year_analyzed_at, duration, file_path, cover_path,
		       added_at, play_count, last_played, skip_count, file_hash, mood, energy, tempo,
		       bpm, instrumental, mood_analyzed_at, liked, liked_at,
		       lastfm_listeners, lastfm_playcount, lastfm_tags, lastfm_url, lastfm_mbid, lastfm_enriched_at
		FROM songs
		WHERE LOWER(artist) = LOWER(?) AND LOWER(title) = LOWER(?)
		LIMIT 1
	`, artist, title)

	var s Song
	var albumArtist, coverPath, fileHash, mood, energy, tempo, genreStr sql.NullString
	var trackNum, discNum, year, originalYear, bpm, playCount, skipCount sql.NullInt64
	var lastPlayed, moodAnalyzedAt, likedAt, yearAnalyzedAt sql.NullInt64
	var instrumental, liked, yearUncertain sql.NullInt64
	var lastfmListeners, lastfmPlaycount sql.NullInt64
	var lastfmTags, lastfmURL, lastfmMBID sql.NullString
	var lastfmEnrichedAt sql.NullInt64

	err := row.Scan(
		&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist, &trackNum, &discNum, &genreStr,
		&year, &originalYear, &yearUncertain, &yearAnalyzedAt, &s.Duration, &s.FilePath, &coverPath,
		&s.AddedAt, &playCount, &lastPlayed, &skipCount, &fileHash, &mood, &energy, &tempo,
		&bpm, &instrumental, &moodAnalyzedAt, &liked, &likedAt,
		&lastfmListeners, &lastfmPlaycount, &lastfmTags, &lastfmURL, &lastfmMBID, &lastfmEnrichedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Populate optional fields
	if albumArtist.Valid {
		s.AlbumArtist = albumArtist.String
	}
	if trackNum.Valid {
		s.TrackNumber = int(trackNum.Int64)
	}
	if discNum.Valid {
		s.DiscNumber = int(discNum.Int64)
	}
	if genreStr.Valid && genreStr.String != "" {
		_ = json.Unmarshal([]byte(genreStr.String), &s.Genre)
	}
	if year.Valid {
		s.Year = int(year.Int64)
	}
	if originalYear.Valid {
		s.OriginalYear = int(originalYear.Int64)
	}
	if yearUncertain.Valid {
		s.YearUncertain = yearUncertain.Int64 == 1
	}
	if yearAnalyzedAt.Valid {
		s.YearAnalyzedAt = yearAnalyzedAt.Int64
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
	if liked.Valid {
		s.Liked = liked.Int64 == 1
	}
	if likedAt.Valid {
		s.LikedAt = likedAt.Int64
	}
	if lastfmListeners.Valid {
		s.LastFMListeners = int(lastfmListeners.Int64)
	}
	if lastfmPlaycount.Valid {
		s.LastFMPlaycount = int(lastfmPlaycount.Int64)
	}
	if lastfmTags.Valid {
		s.LastFMTags = lastfmTags.String
	}
	if lastfmURL.Valid {
		s.LastFMURL = lastfmURL.String
	}
	if lastfmMBID.Valid {
		s.LastFMMBID = lastfmMBID.String
	}
	if lastfmEnrichedAt.Valid {
		s.LastFMEnrichedAt = lastfmEnrichedAt.Int64
	}

	return &s, nil
}

// ============================================================================
// DJ Waveform Cache
// ============================================================================

// DJWaveform represents cached waveform data for DJ mode.
type DJWaveform struct {
	Duration   float64   // Track duration in seconds
	SampleRate int       // Source sample rate
	Resolution int       // Samples per peak
	Peaks      []float64 // Normalized peak values (0-1)
}

// GetDJWaveform retrieves cached waveform data for a track.
func (d *DB) GetDJWaveform(songID string) (*DJWaveform, error) {
	var duration float64
	var sampleRate, resolution, peakCount int
	var peaksData []byte

	err := d.conn.QueryRow(`
		SELECT duration, sample_rate, resolution, peaks_data, peak_count
		FROM dj_waveform_cache
		WHERE song_id = ?
	`, songID).Scan(&duration, &sampleRate, &resolution, &peaksData, &peakCount)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Decompress peaks
	peaks, err := decompressWaveformPeaks(peaksData, peakCount)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress waveform: %w", err)
	}

	return &DJWaveform{
		Duration:   duration,
		SampleRate: sampleRate,
		Resolution: resolution,
		Peaks:      peaks,
	}, nil
}

// SaveDJWaveform stores waveform data for a track.
func (d *DB) SaveDJWaveform(songID string, waveform *DJWaveform) error {
	// Compress peaks for storage
	peaksData, err := compressWaveformPeaks(waveform.Peaks)
	if err != nil {
		return fmt.Errorf("failed to compress waveform: %w", err)
	}

	_, err = d.conn.Exec(`
		INSERT OR REPLACE INTO dj_waveform_cache 
		(song_id, duration, sample_rate, resolution, peaks_data, peak_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, songID, waveform.Duration, waveform.SampleRate, waveform.Resolution,
		peaksData, len(waveform.Peaks), time.Now().Unix())

	return err
}

// DeleteDJWaveform removes cached waveform data for a track.
func (d *DB) DeleteDJWaveform(songID string) error {
	_, err := d.conn.Exec("DELETE FROM dj_waveform_cache WHERE song_id = ?", songID)
	return err
}

// compressWaveformPeaks compresses float peaks to bytes using gzip.
func compressWaveformPeaks(peaks []float64) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)

	for _, p := range peaks {
		// Store as float32 to save space
		if err := binary.Write(gz, binary.LittleEndian, float32(p)); err != nil {
			gz.Close()
			return nil, err
		}
	}

	if err := gz.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompressWaveformPeaks decompresses peaks from stored bytes.
func decompressWaveformPeaks(data []byte, peakCount int) ([]float64, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	peaks := make([]float64, 0, peakCount)
	for {
		var p float32
		if err := binary.Read(gz, binary.LittleEndian, &p); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		peaks = append(peaks, float64(p))
	}

	return peaks, nil
}

// ============================================================================
// DJ Hot Cues
// ============================================================================

// DJHotCue represents a hot cue point.
type DJHotCue struct {
	Slot     int
	Position float64
	Label    string
	Color    string
}

// GetDJHotCues retrieves hot cues for a track.
func (d *DB) GetDJHotCues(songID string) ([]DJHotCue, error) {
	rows, err := d.conn.Query(`
		SELECT slot, position, label, color
		FROM dj_hot_cues
		WHERE song_id = ?
		ORDER BY slot
	`, songID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hotCues []DJHotCue
	for rows.Next() {
		var hc DJHotCue
		var label sql.NullString
		if err := rows.Scan(&hc.Slot, &hc.Position, &label, &hc.Color); err != nil {
			return nil, err
		}
		if label.Valid {
			hc.Label = label.String
		}
		hotCues = append(hotCues, hc)
	}

	return hotCues, rows.Err()
}

// SaveDJHotCues saves hot cues for a track, replacing any existing.
func (d *DB) SaveDJHotCues(songID string, hotCues []DJHotCue) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing hot cues
	_, err = tx.Exec("DELETE FROM dj_hot_cues WHERE song_id = ?", songID)
	if err != nil {
		return err
	}

	// Insert new hot cues
	for _, hc := range hotCues {
		_, err = tx.Exec(`
			INSERT INTO dj_hot_cues (song_id, slot, position, label, color, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, songID, hc.Slot, hc.Position, hc.Label, hc.Color, time.Now().Unix())
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteDJHotCue deletes a specific hot cue.
func (d *DB) DeleteDJHotCue(songID string, slot int) error {
	_, err := d.conn.Exec("DELETE FROM dj_hot_cues WHERE song_id = ? AND slot = ?", songID, slot)
	return err
}
