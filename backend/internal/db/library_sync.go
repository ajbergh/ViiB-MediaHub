package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	defaultSnapshotLimit = 500
	maxSnapshotLimit     = 2000
	defaultChangeLimit   = 500
	maxChangeLimit       = 2000
)

// LibraryChange is one durable mutation from the songs table.
type LibraryChange struct {
	Revision  int64  `json:"revision"`
	SongID    string `json:"songId"`
	Operation string `json:"operation"`
	ChangedAt int64  `json:"changedAt"`
}

// LibrarySnapshotPage is a cursor-based page from a consistent library view.
type LibrarySnapshotPage struct {
	Revision   int64  `json:"revision"`
	Songs      []Song `json:"songs"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

// LibraryChangePage contains ordered mutations and the current head revision.
type LibraryChangePage struct {
	FromRevision int64           `json:"fromRevision"`
	ToRevision   int64           `json:"toRevision"`
	Changes      []LibraryChange `json:"changes"`
	Songs        []Song          `json:"songs"`
	HasMore      bool            `json:"hasMore"`
}

type LibrarySearchAlbum struct {
	Name      string `json:"name"`
	Artist    string `json:"artist"`
	SongCount int    `json:"songCount"`
	CoverPath string `json:"coverPath,omitempty"`
}

type LibrarySearchArtist struct {
	Name       string `json:"name"`
	SongCount  int    `json:"songCount"`
	AlbumCount int    `json:"albumCount"`
}

type LibrarySearchPlaylist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SongCount int    `json:"songCount"`
}

type LibrarySearchResult struct {
	Query     string                  `json:"query"`
	Tracks    []Song                  `json:"tracks"`
	Albums    []LibrarySearchAlbum    `json:"albums"`
	Artists   []LibrarySearchArtist   `json:"artists"`
	Playlists []LibrarySearchPlaylist `json:"playlists"`
}

// EnsureLibrarySyncSchema installs additive revision, change-log, and search
// structures. Triggers guarantee that every song mutation is captured no matter
// which scanner or API path performed it.
func (d *DB) EnsureLibrarySyncSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS library_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		revision INTEGER NOT NULL DEFAULT 0
	);
	INSERT OR IGNORE INTO library_state (id, revision) VALUES (1, 0);

	CREATE TABLE IF NOT EXISTS library_changes (
		revision INTEGER PRIMARY KEY,
		song_id TEXT NOT NULL,
		operation TEXT NOT NULL CHECK (operation IN ('upsert', 'delete')),
		changed_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_library_changes_song ON library_changes(song_id, revision);

	CREATE TABLE IF NOT EXISTS song_search (
		song_id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		artist TEXT NOT NULL,
		album TEXT NOT NULL,
		album_artist TEXT NOT NULL,
		genre TEXT NOT NULL,
		file_path TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_song_search_title ON song_search(title);
	CREATE INDEX IF NOT EXISTS idx_song_search_artist ON song_search(artist);
	CREATE INDEX IF NOT EXISTS idx_song_search_album ON song_search(album);
	CREATE INDEX IF NOT EXISTS idx_song_search_album_artist ON song_search(album_artist);

	CREATE TRIGGER IF NOT EXISTS songs_sync_insert AFTER INSERT ON songs BEGIN
		UPDATE library_state SET revision = revision + 1 WHERE id = 1;
		INSERT INTO library_changes(revision, song_id, operation, changed_at)
			SELECT revision, NEW.id, 'upsert', CAST(strftime('%s','now') AS INTEGER) * 1000
			FROM library_state WHERE id = 1;
		INSERT INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
		VALUES(
			NEW.id,
			lower(COALESCE(NEW.title, '')),
			lower(COALESCE(NEW.artist, '')),
			lower(COALESCE(NEW.album, '')),
			lower(COALESCE(NEW.album_artist, '')),
			lower(COALESCE(NEW.genre, '')),
			lower(COALESCE(NEW.file_path, ''))
		)
		ON CONFLICT(song_id) DO UPDATE SET
			title = excluded.title,
			artist = excluded.artist,
			album = excluded.album,
			album_artist = excluded.album_artist,
			genre = excluded.genre,
			file_path = excluded.file_path;
	END;

	CREATE TRIGGER IF NOT EXISTS songs_sync_update AFTER UPDATE ON songs BEGIN
		UPDATE library_state SET revision = revision + 1 WHERE id = 1;
		INSERT INTO library_changes(revision, song_id, operation, changed_at)
			SELECT revision, NEW.id, 'upsert', CAST(strftime('%s','now') AS INTEGER) * 1000
			FROM library_state WHERE id = 1;
		INSERT INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
		VALUES(
			NEW.id,
			lower(COALESCE(NEW.title, '')),
			lower(COALESCE(NEW.artist, '')),
			lower(COALESCE(NEW.album, '')),
			lower(COALESCE(NEW.album_artist, '')),
			lower(COALESCE(NEW.genre, '')),
			lower(COALESCE(NEW.file_path, ''))
		)
		ON CONFLICT(song_id) DO UPDATE SET
			title = excluded.title,
			artist = excluded.artist,
			album = excluded.album,
			album_artist = excluded.album_artist,
			genre = excluded.genre,
			file_path = excluded.file_path;
	END;

	CREATE TRIGGER IF NOT EXISTS songs_sync_delete AFTER DELETE ON songs BEGIN
		UPDATE library_state SET revision = revision + 1 WHERE id = 1;
		INSERT INTO library_changes(revision, song_id, operation, changed_at)
			SELECT revision, OLD.id, 'delete', CAST(strftime('%s','now') AS INTEGER) * 1000
			FROM library_state WHERE id = 1;
		DELETE FROM song_search WHERE song_id = OLD.id;
	END;
	`
	if _, err := d.conn.Exec(schema); err != nil {
		return fmt.Errorf("create library sync schema: %w", err)
	}

	_, err := d.conn.Exec(`
		INSERT INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
		SELECT id, lower(COALESCE(title, '')), lower(COALESCE(artist, '')),
		       lower(COALESCE(album, '')), lower(COALESCE(album_artist, '')),
		       lower(COALESCE(genre, '')), lower(COALESCE(file_path, ''))
		FROM songs
		ON CONFLICT(song_id) DO UPDATE SET
			title = excluded.title,
			artist = excluded.artist,
			album = excluded.album,
			album_artist = excluded.album_artist,
			genre = excluded.genre,
			file_path = excluded.file_path
	`)
	if err != nil {
		return fmt.Errorf("backfill library search index: %w", err)
	}
	return nil
}

func (d *DB) LibraryRevision() (int64, error) {
	var revision int64
	if err := d.conn.QueryRow(`SELECT revision FROM library_state WHERE id = 1`).Scan(&revision); err != nil {
		return 0, err
	}
	return revision, nil
}

func clampPageLimit(limit, fallback, maximum int) int {
	if limit <= 0 {
		return fallback
	}
	if limit > maximum {
		return maximum
	}
	return limit
}

func (d *DB) ListSongsPage(afterID string, limit int) (LibrarySnapshotPage, error) {
	limit = clampPageLimit(limit, defaultSnapshotLimit, maxSnapshotLimit)
	tx, err := d.conn.BeginTx(nil, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return LibrarySnapshotPage{}, err
	}
	defer tx.Rollback()

	var revision int64
	if err := tx.QueryRow(`SELECT revision FROM library_state WHERE id = 1`).Scan(&revision); err != nil {
		return LibrarySnapshotPage{}, err
	}

	rows, err := tx.Query(songSelect+`
		WHERE COALESCE(ignored, 0) = 0 AND id > ?
		ORDER BY id
		LIMIT ?`, afterID, limit+1)
	if err != nil {
		return LibrarySnapshotPage{}, err
	}
	songs, err := scanLibrarySongRows(rows)
	if err != nil {
		return LibrarySnapshotPage{}, err
	}

	page := LibrarySnapshotPage{Revision: revision, Songs: songs}
	if len(page.Songs) > limit {
		page.HasMore = true
		page.Songs = page.Songs[:limit]
	}
	if page.HasMore && len(page.Songs) > 0 {
		page.NextCursor = page.Songs[len(page.Songs)-1].ID
	}
	if err := tx.Commit(); err != nil {
		return LibrarySnapshotPage{}, err
	}
	return page, nil
}

func (d *DB) GetLibraryChanges(since int64, limit int) (LibraryChangePage, error) {
	limit = clampPageLimit(limit, defaultChangeLimit, maxChangeLimit)
	rows, err := d.conn.Query(`
		SELECT revision, song_id, operation, changed_at
		FROM library_changes
		WHERE revision > ?
		ORDER BY revision
		LIMIT ?`, since, limit+1)
	if err != nil {
		return LibraryChangePage{}, err
	}
	defer rows.Close()

	changes := make([]LibraryChange, 0, limit+1)
	for rows.Next() {
		var change LibraryChange
		if err := rows.Scan(&change.Revision, &change.SongID, &change.Operation, &change.ChangedAt); err != nil {
			return LibraryChangePage{}, err
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return LibraryChangePage{}, err
	}

	page := LibraryChangePage{FromRevision: since, Changes: changes}
	if len(page.Changes) > limit {
		page.HasMore = true
		page.Changes = page.Changes[:limit]
	}

	toRevision, err := d.LibraryRevision()
	if err != nil {
		return LibraryChangePage{}, err
	}
	if len(page.Changes) > 0 {
		page.ToRevision = page.Changes[len(page.Changes)-1].Revision
	} else {
		page.ToRevision = toRevision
	}

	upsertIDs := make([]string, 0, len(page.Changes))
	seen := make(map[string]struct{}, len(page.Changes))
	for _, change := range page.Changes {
		if change.Operation == "upsert" {
			if _, exists := seen[change.SongID]; !exists {
				seen[change.SongID] = struct{}{}
				upsertIDs = append(upsertIDs, change.SongID)
			}
		}
	}
	page.Songs, err = d.GetSongsByIDs(upsertIDs)
	if err != nil {
		return LibraryChangePage{}, err
	}
	return page, nil
}

func (d *DB) GetSongsByIDs(ids []string) ([]Song, error) {
	if len(ids) == 0 {
		return []Song{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := d.conn.Query(songSelect+`
		WHERE COALESCE(ignored, 0) = 0 AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	return scanLibrarySongRows(rows)
}

func (d *DB) SearchLibrary(query string, limit int) (LibrarySearchResult, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	limit = clampPageLimit(limit, 50, 200)
	result := LibrarySearchResult{Query: query, Tracks: []Song{}, Albums: []LibrarySearchAlbum{}, Artists: []LibrarySearchArtist{}, Playlists: []LibrarySearchPlaylist{}}
	if query == "" {
		return result, nil
	}

	escaped := escapeLikePattern(query)
	prefix := escaped + "%"
	contains := "%" + escaped + "%"

	rows, err := d.conn.Query(songSelect+`
		JOIN song_search ss ON ss.song_id = songs.id
		WHERE COALESCE(songs.ignored, 0) = 0
		  AND (ss.title LIKE ? ESCAPE '\' OR ss.artist LIKE ? ESCAPE '\'
		       OR ss.album LIKE ? ESCAPE '\' OR ss.album_artist LIKE ? ESCAPE '\'
		       OR ss.genre LIKE ? ESCAPE '\' OR ss.file_path LIKE ? ESCAPE '\')
		ORDER BY CASE
			WHEN ss.title LIKE ? ESCAPE '\' THEN 0
			WHEN ss.artist LIKE ? ESCAPE '\' THEN 1
			WHEN ss.album LIKE ? ESCAPE '\' THEN 2
			ELSE 3 END,
			COALESCE(songs.play_count, 0) DESC, songs.title
		LIMIT ?`, contains, contains, contains, contains, contains, contains, prefix, prefix, prefix, limit)
	if err != nil {
		return result, err
	}
	result.Tracks, err = scanLibrarySongRows(rows)
	if err != nil {
		return result, err
	}

	albumRows, err := d.conn.Query(`
		SELECT songs.album, COALESCE(NULLIF(songs.album_artist, ''), songs.artist) AS artist,
		       COUNT(*), COALESCE(MAX(songs.cover_path), '')
		FROM songs JOIN song_search ss ON ss.song_id = songs.id
		WHERE COALESCE(songs.ignored, 0) = 0
		  AND (ss.album LIKE ? ESCAPE '\' OR ss.album_artist LIKE ? ESCAPE '\' OR ss.artist LIKE ? ESCAPE '\')
		GROUP BY songs.album, artist
		ORDER BY CASE WHEN lower(songs.album) LIKE ? ESCAPE '\' THEN 0 ELSE 1 END,
		         COUNT(*) DESC, songs.album
		LIMIT ?`, contains, contains, contains, prefix, limit)
	if err != nil {
		return result, err
	}
	for albumRows.Next() {
		var album LibrarySearchAlbum
		if err := albumRows.Scan(&album.Name, &album.Artist, &album.SongCount, &album.CoverPath); err != nil {
			albumRows.Close()
			return result, err
		}
		result.Albums = append(result.Albums, album)
	}
	if err := albumRows.Close(); err != nil {
		return result, err
	}

	artistRows, err := d.conn.Query(`
		SELECT songs.artist, COUNT(*), COUNT(DISTINCT songs.album)
		FROM songs JOIN song_search ss ON ss.song_id = songs.id
		WHERE COALESCE(songs.ignored, 0) = 0 AND ss.artist LIKE ? ESCAPE '\'
		GROUP BY songs.artist
		ORDER BY CASE WHEN ss.artist LIKE ? ESCAPE '\' THEN 0 ELSE 1 END,
		         COUNT(*) DESC, songs.artist
		LIMIT ?`, contains, prefix, limit)
	if err != nil {
		return result, err
	}
	for artistRows.Next() {
		var artist LibrarySearchArtist
		if err := artistRows.Scan(&artist.Name, &artist.SongCount, &artist.AlbumCount); err != nil {
			artistRows.Close()
			return result, err
		}
		result.Artists = append(result.Artists, artist)
	}
	if err := artistRows.Close(); err != nil {
		return result, err
	}

	playlistRows, err := d.conn.Query(`
		SELECT id, name, json_array_length(song_ids)
		FROM playlists
		WHERE lower(name) LIKE ? ESCAPE '\'
		ORDER BY CASE WHEN lower(name) LIKE ? ESCAPE '\' THEN 0 ELSE 1 END, name
		LIMIT ?`, contains, prefix, limit)
	if err != nil {
		return result, err
	}
	for playlistRows.Next() {
		var playlist LibrarySearchPlaylist
		if err := playlistRows.Scan(&playlist.ID, &playlist.Name, &playlist.SongCount); err != nil {
			playlistRows.Close()
			return result, err
		}
		result.Playlists = append(result.Playlists, playlist)
	}
	if err := playlistRows.Close(); err != nil {
		return result, err
	}
	return result, nil
}

const songSelect = `
	SELECT songs.id, songs.title, songs.artist, songs.album, songs.album_artist,
	       songs.track_number, songs.disc_number, songs.genre, songs.year,
	       songs.original_year, songs.year_uncertain, songs.year_analyzed_at,
	       songs.duration, songs.replay_gain_db, songs.replay_peak, songs.file_path,
	       songs.cover_path, songs.added_at, songs.play_count, songs.last_played,
	       songs.skip_count, songs.file_hash, songs.mood, songs.energy, songs.tempo,
	       songs.bpm, songs.instrumental, songs.mood_analyzed_at, songs.liked,
	       songs.liked_at, songs.lastfm_listeners, songs.lastfm_playcount,
	       songs.lastfm_tags, songs.lastfm_url, songs.lastfm_mbid, songs.lastfm_enriched_at
	FROM songs`

type libraryRowScanner interface {
	Scan(dest ...any) error
}

func scanLibrarySong(row libraryRowScanner) (Song, error) {
	var song Song
	var genreJSON, albumArtist, coverPath, fileHash, mood, energy, tempo sql.NullString
	var lastFMTags, lastFMURL, lastFMMBID sql.NullString
	var trackNum, discNum, year, originalYear, yearUncertain, yearAnalyzedAt sql.NullInt64
	var playCount, lastPlayed, skipCount, bpm, instrumental, moodAnalyzedAt sql.NullInt64
	var liked, likedAt, lastFMListeners, lastFMPlaycount, lastFMEnrichedAt sql.NullInt64
	var replayGainDB, replayPeak sql.NullFloat64

	err := row.Scan(
		&song.ID, &song.Title, &song.Artist, &song.Album, &albumArtist,
		&trackNum, &discNum, &genreJSON, &year, &originalYear, &yearUncertain,
		&yearAnalyzedAt, &song.Duration, &replayGainDB, &replayPeak, &song.FilePath,
		&coverPath, &song.AddedAt, &playCount, &lastPlayed, &skipCount, &fileHash,
		&mood, &energy, &tempo, &bpm, &instrumental, &moodAnalyzedAt, &liked,
		&likedAt, &lastFMListeners, &lastFMPlaycount, &lastFMTags, &lastFMURL,
		&lastFMMBID, &lastFMEnrichedAt,
	)
	if err != nil {
		return Song{}, err
	}

	if albumArtist.Valid { song.AlbumArtist = albumArtist.String }
	if trackNum.Valid { song.TrackNumber = int(trackNum.Int64) }
	if discNum.Valid { song.DiscNumber = int(discNum.Int64) }
	if genreJSON.Valid && genreJSON.String != "" { _ = json.Unmarshal([]byte(genreJSON.String), &song.Genre) }
	if year.Valid { song.Year = int(year.Int64) }
	if originalYear.Valid { song.OriginalYear = int(originalYear.Int64) }
	song.YearUncertain = yearUncertain.Valid && yearUncertain.Int64 == 1
	if yearAnalyzedAt.Valid { song.YearAnalyzedAt = yearAnalyzedAt.Int64 }
	if replayGainDB.Valid { song.ReplayGainDB = replayGainDB.Float64 }
	if replayPeak.Valid { song.ReplayPeak = replayPeak.Float64 }
	if coverPath.Valid { song.CoverPath = coverPath.String }
	if playCount.Valid { song.PlayCount = int(playCount.Int64) }
	if lastPlayed.Valid { song.LastPlayed = lastPlayed.Int64 }
	if skipCount.Valid { song.SkipCount = int(skipCount.Int64) }
	if fileHash.Valid { song.FileHash = fileHash.String }
	if mood.Valid { song.Mood = mood.String }
	if energy.Valid { song.Energy = energy.String }
	if tempo.Valid { song.Tempo = tempo.String }
	if bpm.Valid { song.BPM = int(bpm.Int64) }
	song.Instrumental = instrumental.Valid && instrumental.Int64 == 1
	if moodAnalyzedAt.Valid { song.MoodAnalyzedAt = moodAnalyzedAt.Int64 }
	song.Liked = liked.Valid && liked.Int64 == 1
	if likedAt.Valid { song.LikedAt = likedAt.Int64 }
	if lastFMListeners.Valid { song.LastFMListeners = int(lastFMListeners.Int64) }
	if lastFMPlaycount.Valid { song.LastFMPlaycount = int(lastFMPlaycount.Int64) }
	if lastFMTags.Valid { song.LastFMTags = lastFMTags.String }
	if lastFMURL.Valid { song.LastFMURL = lastFMURL.String }
	if lastFMMBID.Valid { song.LastFMMBID = lastFMMBID.String }
	if lastFMEnrichedAt.Valid { song.LastFMEnrichedAt = lastFMEnrichedAt.Int64 }
	return song, nil
}

func scanLibrarySongRows(rows *sql.Rows) ([]Song, error) {
	defer rows.Close()
	songs := make([]Song, 0)
	for rows.Next() {
		song, err := scanLibrarySong(rows)
		if err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}
	return songs, rows.Err()
}

// PruneLibraryChanges bounds the durable replay log after clients have had a
// generous recovery window.
func (d *DB) PruneLibraryChanges(retain int64) error {
	if retain <= 0 {
		retain = 100000
	}
	revision, err := d.LibraryRevision()
	if err != nil {
		return err
	}
	cutoff := revision - retain
	if cutoff <= 0 {
		return nil
	}
	_, err = d.conn.Exec(`DELETE FROM library_changes WHERE revision < ?`, cutoff)
	return err
}

func (d *DB) LibrarySyncStats() (revision, retainedChanges int64, err error) {
	if err = d.conn.QueryRow(`SELECT revision FROM library_state WHERE id = 1`).Scan(&revision); err != nil {
		return 0, 0, err
	}
	if err = d.conn.QueryRow(`SELECT COUNT(*) FROM library_changes`).Scan(&retainedChanges); err != nil {
		return 0, 0, err
	}
	return revision, retainedChanges, nil
}

// LibraryChangeRetentionAge is used only for diagnostics and tests.
func LibraryChangeRetentionAge(change LibraryChange) time.Duration {
	return time.Since(time.UnixMilli(change.ChangedAt))
}
