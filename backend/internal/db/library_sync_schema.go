package db

import "fmt"

const (
	defaultSnapshotLimit = 500
	maxSnapshotLimit     = 2000
	defaultChangeLimit   = 500
	maxChangeLimit       = 2000
)

type LibraryChange struct {
	Revision  int64  `json:"revision"`
	SongID    string `json:"songId"`
	Operation string `json:"operation"`
	ChangedAt int64  `json:"changedAt"`
}

type LibrarySnapshotPage struct {
	Revision   int64  `json:"revision"`
	Songs      []Song `json:"songs"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

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
			SELECT revision, NEW.id, 'upsert', CAST(strftime('%s','now') AS INTEGER) * 1000 FROM library_state WHERE id = 1;
		INSERT INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
		VALUES(NEW.id, lower(COALESCE(NEW.title, '')), lower(COALESCE(NEW.artist, '')), lower(COALESCE(NEW.album, '')), lower(COALESCE(NEW.album_artist, '')), lower(COALESCE(NEW.genre, '')), lower(COALESCE(NEW.file_path, '')))
		ON CONFLICT(song_id) DO UPDATE SET title = excluded.title, artist = excluded.artist, album = excluded.album, album_artist = excluded.album_artist, genre = excluded.genre, file_path = excluded.file_path;
	END;
	CREATE TRIGGER IF NOT EXISTS songs_sync_update AFTER UPDATE ON songs BEGIN
		UPDATE library_state SET revision = revision + 1 WHERE id = 1;
		INSERT INTO library_changes(revision, song_id, operation, changed_at)
			SELECT revision, NEW.id, 'upsert', CAST(strftime('%s','now') AS INTEGER) * 1000 FROM library_state WHERE id = 1;
		INSERT INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
		VALUES(NEW.id, lower(COALESCE(NEW.title, '')), lower(COALESCE(NEW.artist, '')), lower(COALESCE(NEW.album, '')), lower(COALESCE(NEW.album_artist, '')), lower(COALESCE(NEW.genre, '')), lower(COALESCE(NEW.file_path, '')))
		ON CONFLICT(song_id) DO UPDATE SET title = excluded.title, artist = excluded.artist, album = excluded.album, album_artist = excluded.album_artist, genre = excluded.genre, file_path = excluded.file_path;
	END;
	CREATE TRIGGER IF NOT EXISTS songs_sync_delete AFTER DELETE ON songs BEGIN
		UPDATE library_state SET revision = revision + 1 WHERE id = 1;
		INSERT INTO library_changes(revision, song_id, operation, changed_at)
			SELECT revision, OLD.id, 'delete', CAST(strftime('%s','now') AS INTEGER) * 1000 FROM library_state WHERE id = 1;
		DELETE FROM song_search WHERE song_id = OLD.id;
	END;
	`
	if _, err := d.conn.Exec(schema); err != nil { return fmt.Errorf("create library sync schema: %w", err) }

	_, err := d.conn.Exec(`
		INSERT INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
		SELECT id, lower(COALESCE(title, '')), lower(COALESCE(artist, '')),
		       lower(COALESCE(album, '')), lower(COALESCE(album_artist, '')),
		       lower(COALESCE(genre, '')), lower(COALESCE(file_path, ''))
		FROM songs
		WHERE true
		ON CONFLICT(song_id) DO UPDATE SET
			title = excluded.title, artist = excluded.artist, album = excluded.album,
			album_artist = excluded.album_artist, genre = excluded.genre,
			file_path = excluded.file_path
	`)
	if err != nil { return fmt.Errorf("backfill library search index: %w", err) }
	return nil
}
