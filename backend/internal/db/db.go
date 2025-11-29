// Package db provides SQLite database access for ViiB MediaHub.
//
// Schema includes tables for:
//   - songs: Audio file metadata with paths and usage stats
//   - playlists: User-created playlists with song references
//   - scan_folders: Configured directories to scan for music
//   - settings: Key-value store for application configuration
//   - spotify_downloads: Download queue with status tracking
//   - album_metadata: Cached Spotify album metadata
//   - artist_metadata: Cached Spotify artist metadata
//
// Uses SQLite with WAL mode for concurrent access and foreign keys enabled.
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

type Song struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Artist      string   `json:"artist"`
	Album       string   `json:"album"`
	AlbumArtist string   `json:"albumArtist,omitempty"`
	TrackNumber int      `json:"trackNumber,omitempty"`
	DiscNumber  int      `json:"discNumber,omitempty"`
	Genre       []string `json:"genre,omitempty"`
	Year        int      `json:"year,omitempty"`
	Duration    float64  `json:"duration"`
	FilePath    string   `json:"filePath"`
	CoverPath   string   `json:"coverPath,omitempty"`
	AddedAt     int64    `json:"addedAt"`
	PlayCount   int      `json:"playCount,omitempty"`
	LastPlayed  int64    `json:"lastPlayed,omitempty"`
	SkipCount   int      `json:"skipCount,omitempty"`
	FileHash    string   `json:"fileHash,omitempty"`
}

type Playlist struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	SongIDs   []string `json:"songIds"`
	CoverPath string   `json:"coverPath,omitempty"`
	CreatedAt int64    `json:"createdAt"`
}

type ScanFolder struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	AddedAt   int64  `json:"addedAt"`
	LastScan  int64  `json:"lastScan,omitempty"`
	SongCount int    `json:"songCount"`
}

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
		file_hash TEXT
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
	`

	_, err := d.conn.Exec(schema)
	return err
}

// Song operations

func (d *DB) GetAllSongs() ([]Song, error) {
	rows, err := d.conn.Query(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, duration, file_path, cover_path, added_at,
		       play_count, last_played, skip_count, file_hash
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

		err := rows.Scan(
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

		songs = append(songs, s)
	}

	return songs, rows.Err()
}

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
			genre = excluded.genre,
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

func (d *DB) DeleteSong(id string) error {
	_, err := d.conn.Exec("DELETE FROM songs WHERE id = ?", id)
	return err
}

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

func (d *DB) UpdatePlayCount(id string) error {
	_, err := d.conn.Exec(`
		UPDATE songs 
		SET play_count = play_count + 1, last_played = ?
		WHERE id = ?
	`, time.Now().UnixMilli(), id)
	return err
}

// Playlist operations

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

func (d *DB) DeletePlaylist(id string) error {
	_, err := d.conn.Exec("DELETE FROM playlists WHERE id = ?", id)
	return err
}

// Scan folder operations

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

func (d *DB) AddScanFolder(f *ScanFolder) error {
	_, err := d.conn.Exec(`
		INSERT INTO scan_folders (id, path, added_at, song_count)
		VALUES (?, ?, ?, 0)
		ON CONFLICT(path) DO NOTHING
	`, f.ID, f.Path, f.AddedAt)
	return err
}

func (d *DB) UpdateScanFolder(id string, lastScan int64, songCount int) error {
	_, err := d.conn.Exec(`
		UPDATE scan_folders SET last_scan = ?, song_count = ? WHERE id = ?
	`, lastScan, songCount, id)
	return err
}

func (d *DB) RemoveScanFolder(id string) error {
	_, err := d.conn.Exec("DELETE FROM scan_folders WHERE id = ?", id)
	return err
}

// Spotify Download operations

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

func (d *DB) UpdateDownloadStatus(id string, status string, progress int, errorMsg string) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = ?, progress = ?, error = ?
		WHERE id = ?
	`, status, progress, errorMsg, id)
	return err
}

func (d *DB) UpdateDownloadProgress(id string, progress int) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads SET progress = ? WHERE id = ?
	`, progress, id)
	return err
}

func (d *DB) MarkDownloadStarted(id string) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'downloading', started_at = ?
		WHERE id = ?
	`, time.Now().Unix(), id)
	return err
}

func (d *DB) MarkDownloadCompleted(id string, filePath string) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'completed', progress = 100, file_path = ?, completed_at = ?
		WHERE id = ?
	`, filePath, time.Now().Unix(), id)
	return err
}

func (d *DB) MarkDownloadFailed(id string, errorMsg string) error {
	_, err := d.conn.Exec(`
		UPDATE spotify_downloads
		SET status = 'failed', error = ?, completed_at = ?
		WHERE id = ?
	`, errorMsg, time.Now().Unix(), id)
	return err
}

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

func (d *DB) GetSetting(key string) (string, error) {
	var value string
	err := d.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.conn.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
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
