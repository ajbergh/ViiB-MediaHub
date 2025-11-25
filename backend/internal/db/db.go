package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
