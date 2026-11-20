// library_sync_schema.go defines the revision log and search-index schema.
package db

import "fmt"

const (
	defaultSnapshotLimit = 500
	maxSnapshotLimit     = 2000
	defaultChangeLimit   = 500
	maxChangeLimit       = 2000
)

// LibraryChange records one durable song upsert or deletion at a revision.
type LibraryChange struct {
	Revision        int64  `json:"revision"`
	SongID          string `json:"songId"`
	Operation       string `json:"operation"`
	ChangedAt       int64  `json:"changedAt"`
	AnalysisChanged bool   `json:"analysisChanged"`
}

// LibrarySnapshotPage is one ID-cursor page plus the revision it observed.
type LibrarySnapshotPage struct {
	Revision   int64  `json:"revision"`
	Songs      []Song `json:"songs"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

// LibraryChangePage contains ordered revisions and payloads for upserts only.
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

// LibrarySearchPlaylist is the lightweight playlist result returned by search.
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

// EnsureLibrarySyncSchema installs the revision and search schema once per DB
// handle. Its initial backfill is insert-only, so routine requests never
// rewrite the complete search index.
func (d *DB) EnsureLibrarySyncSchema() error {
	d.librarySyncOnce.Do(func() {
		if err := d.ConfigureRuntime(); err != nil {
			d.librarySyncInitErr = err
			return
		}
		d.librarySyncInitErr = d.ensureLibrarySyncSchema()
	})
	return d.librarySyncInitErr
}

func (d *DB) ensureLibrarySyncSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS library_state (id INTEGER PRIMARY KEY CHECK (id = 1), revision INTEGER NOT NULL DEFAULT 0);
	INSERT OR IGNORE INTO library_state (id, revision) VALUES (1, 0);
	CREATE TABLE IF NOT EXISTS library_changes (revision INTEGER PRIMARY KEY, song_id TEXT NOT NULL, operation TEXT NOT NULL CHECK (operation IN ('upsert', 'delete')), changed_at INTEGER NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_library_changes_song ON library_changes(song_id, revision);
	CREATE TABLE IF NOT EXISTS song_search (song_id TEXT PRIMARY KEY, title TEXT NOT NULL, artist TEXT NOT NULL, album TEXT NOT NULL, album_artist TEXT NOT NULL, genre TEXT NOT NULL, file_path TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_song_search_title ON song_search(title);
	CREATE INDEX IF NOT EXISTS idx_song_search_artist ON song_search(artist);
	CREATE INDEX IF NOT EXISTS idx_song_search_album ON song_search(album);
	CREATE INDEX IF NOT EXISTS idx_song_search_album_artist ON song_search(album_artist);
	CREATE INDEX IF NOT EXISTS idx_song_search_genre ON song_search(genre);
	CREATE INDEX IF NOT EXISTS idx_song_search_file_path ON song_search(file_path);
	CREATE INDEX IF NOT EXISTS idx_playlists_search_name ON playlists(lower(name));
	CREATE VIRTUAL TABLE IF NOT EXISTS song_artist_fts USING fts5(song_id UNINDEXED, artist, album_artist, tokenize='trigram');
	CREATE TABLE IF NOT EXISTS song_search_meta (id INTEGER PRIMARY KEY CHECK (id = 1), artist_fts_ready INTEGER NOT NULL DEFAULT 0);
	INSERT OR IGNORE INTO song_search_meta (id, artist_fts_ready) VALUES (1, 0);
	CREATE TRIGGER IF NOT EXISTS song_search_fts_insert AFTER INSERT ON song_search BEGIN
		INSERT INTO song_artist_fts(rowid, song_id, artist, album_artist)
		VALUES (NEW.rowid, NEW.song_id, NEW.artist, NEW.album_artist);
	END;
	DROP TRIGGER IF EXISTS song_search_fts_update;
	CREATE TRIGGER IF NOT EXISTS song_search_fts_update_v2 AFTER UPDATE OF song_id, artist, album_artist ON song_search
	WHEN OLD.song_id IS NOT NEW.song_id OR OLD.artist IS NOT NEW.artist OR OLD.album_artist IS NOT NEW.album_artist BEGIN
		DELETE FROM song_artist_fts WHERE rowid = OLD.rowid;
		INSERT INTO song_artist_fts(rowid, song_id, artist, album_artist)
		VALUES (NEW.rowid, NEW.song_id, NEW.artist, NEW.album_artist);
	END;
	CREATE TRIGGER IF NOT EXISTS song_search_fts_delete AFTER DELETE ON song_search BEGIN
		DELETE FROM song_artist_fts WHERE rowid = OLD.rowid;
	END;
	CREATE TRIGGER IF NOT EXISTS songs_sync_insert AFTER INSERT ON songs BEGIN
		UPDATE library_state SET revision = revision + 1 WHERE id = 1;
		INSERT INTO library_changes(revision, song_id, operation, changed_at)
			SELECT revision, NEW.id, 'upsert', CAST(strftime('%s','now') AS INTEGER) * 1000 FROM library_state WHERE id = 1;
		INSERT INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
		VALUES(NEW.id, lower(COALESCE(NEW.title, '')), lower(COALESCE(NEW.artist, '')), lower(COALESCE(NEW.album, '')), lower(COALESCE(NEW.album_artist, '')), lower(COALESCE(NEW.genre, '')), lower(COALESCE(NEW.file_path, '')))
		ON CONFLICT(song_id) DO UPDATE SET title = excluded.title, artist = excluded.artist, album = excluded.album, album_artist = excluded.album_artist, genre = excluded.genre, file_path = excluded.file_path;
	END;
	CREATE TRIGGER IF NOT EXISTS songs_sync_delete AFTER DELETE ON songs BEGIN
		UPDATE library_state SET revision = revision + 1 WHERE id = 1;
		INSERT INTO library_changes(revision, song_id, operation, changed_at) SELECT revision, OLD.id, 'delete', CAST(strftime('%s','now') AS INTEGER) * 1000 FROM library_state WHERE id = 1;
		DELETE FROM song_search WHERE song_id = OLD.id;
	END;
	`
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("create library sync schema: %w", err)
	}
	var hasAnalysisChanged bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM pragma_table_info('library_changes') WHERE name = 'analysis_changed')`).Scan(&hasAnalysisChanged); err != nil {
		return err
	}
	if !hasAnalysisChanged {
		if _, err := tx.Exec(`ALTER TABLE library_changes ADD COLUMN analysis_changed INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	// Playback and preference writes leave source identity alone. Other updates
	// remain conservative because a scan can observe a changed file timestamp.
	if _, err := tx.Exec(`
		DROP TRIGGER IF EXISTS songs_sync_update;
		CREATE TRIGGER IF NOT EXISTS songs_sync_update_v2 AFTER UPDATE ON songs BEGIN
			UPDATE library_state SET revision = revision + 1 WHERE id = 1;
			INSERT INTO library_changes(revision, song_id, operation, changed_at, analysis_changed)
			SELECT revision, NEW.id, 'upsert', CAST(strftime('%s','now') AS INTEGER) * 1000,
				CASE WHEN (OLD.play_count IS NOT NEW.play_count OR OLD.last_played IS NOT NEW.last_played OR OLD.skip_count IS NOT NEW.skip_count OR OLD.liked IS NOT NEW.liked OR OLD.liked_at IS NOT NEW.liked_at)
				AND OLD.file_path IS NEW.file_path AND OLD.file_hash IS NEW.file_hash AND OLD.ignored IS NEW.ignored
				AND OLD.duration IS NEW.duration AND OLD.bpm IS NEW.bpm THEN 0 ELSE 1 END
			FROM library_state WHERE id = 1;
			INSERT INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
			VALUES(NEW.id, lower(COALESCE(NEW.title, '')), lower(COALESCE(NEW.artist, '')), lower(COALESCE(NEW.album, '')), lower(COALESCE(NEW.album_artist, '')), lower(COALESCE(NEW.genre, '')), lower(COALESCE(NEW.file_path, '')))
			ON CONFLICT(song_id) DO UPDATE SET title = excluded.title, artist = excluded.artist, album = excluded.album,
				album_artist = excluded.album_artist, genre = excluded.genre, file_path = excluded.file_path
			WHERE song_search.title IS NOT excluded.title OR song_search.artist IS NOT excluded.artist
				OR song_search.album IS NOT excluded.album OR song_search.album_artist IS NOT excluded.album_artist
				OR song_search.genre IS NOT excluded.genre OR song_search.file_path IS NOT excluded.file_path;
		END;
	`); err != nil {
		return fmt.Errorf("update library sync triggers: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	_, err = d.conn.Exec(`
		INSERT OR IGNORE INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
		SELECT id, lower(COALESCE(title, '')), lower(COALESCE(artist, '')), lower(COALESCE(album, '')), lower(COALESCE(album_artist, '')), lower(COALESCE(genre, '')), lower(COALESCE(file_path, ''))
		FROM songs
	`)
	if err != nil {
		return fmt.Errorf("backfill library search index: %w", err)
	}

	var ftsReady int
	if err := d.conn.QueryRow(`SELECT artist_fts_ready FROM song_search_meta WHERE id = 1`).Scan(&ftsReady); err != nil {
		return fmt.Errorf("read library search fts state: %w", err)
	}
	if ftsReady == 0 {
		tx, err := d.conn.Begin()
		if err != nil {
			return fmt.Errorf("begin library search fts backfill: %w", err)
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`DELETE FROM song_artist_fts`); err != nil {
			return fmt.Errorf("clear library search fts: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO song_artist_fts(rowid, song_id, artist, album_artist)
			SELECT rowid, song_id, artist, album_artist FROM song_search
		`); err != nil {
			return fmt.Errorf("backfill library search fts: %w", err)
		}
		if _, err := tx.Exec(`UPDATE song_search_meta SET artist_fts_ready = 1 WHERE id = 1`); err != nil {
			return fmt.Errorf("mark library search fts ready: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit library search fts backfill: %w", err)
		}
	}
	return nil
}

// RebuildLibrarySearchIndex is the explicit repair path. Routine API requests
// only ensure the schema once and never rewrite the complete search table.
func (d *DB) RebuildLibrarySearchIndex() error {
	if err := d.EnsureLibrarySyncSchema(); err != nil {
		return err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM song_search`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM song_artist_fts`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO song_search(song_id, title, artist, album, album_artist, genre, file_path)
		SELECT id, lower(COALESCE(title, '')), lower(COALESCE(artist, '')),
		       lower(COALESCE(album, '')), lower(COALESCE(album_artist, '')),
		       lower(COALESCE(genre, '')), lower(COALESCE(file_path, ''))
		FROM songs
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE song_search_meta SET artist_fts_ready = 1 WHERE id = 1`); err != nil {
		return err
	}
	return tx.Commit()
}
