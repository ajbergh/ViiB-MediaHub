package db

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (d *DB) LibraryRevision() (int64, error) {
	var revision int64
	if err := d.conn.QueryRow(`SELECT revision FROM library_state WHERE id = 1`).Scan(&revision); err != nil {
		return 0, err
	}
	return revision, nil
}

func clampPageLimit(limit, fallback, maximum int) int {
	if limit <= 0 { return fallback }
	if limit > maximum { return maximum }
	return limit
}

func (d *DB) ListSongsPage(afterID string, limit int) (LibrarySnapshotPage, error) {
	limit = clampPageLimit(limit, defaultSnapshotLimit, maxSnapshotLimit)
	tx, err := d.conn.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil { return LibrarySnapshotPage{}, err }
	defer tx.Rollback()

	var revision int64
	if err := tx.QueryRow(`SELECT revision FROM library_state WHERE id = 1`).Scan(&revision); err != nil {
		return LibrarySnapshotPage{}, err
	}
	rows, err := tx.Query(songSelect+`
		WHERE COALESCE(ignored, 0) = 0 AND id > ?
		ORDER BY id LIMIT ?`, afterID, limit+1)
	if err != nil { return LibrarySnapshotPage{}, err }
	songs, err := scanLibrarySongRows(rows)
	if err != nil { return LibrarySnapshotPage{}, err }

	page := LibrarySnapshotPage{Revision: revision, Songs: songs}
	if len(page.Songs) > limit {
		page.HasMore = true
		page.Songs = page.Songs[:limit]
		page.NextCursor = page.Songs[len(page.Songs)-1].ID
	}
	if err := tx.Commit(); err != nil { return LibrarySnapshotPage{}, err }
	return page, nil
}

func (d *DB) GetLibraryChanges(since int64, limit int) (LibraryChangePage, error) {
	limit = clampPageLimit(limit, defaultChangeLimit, maxChangeLimit)
	rows, err := d.conn.Query(`
		SELECT revision, song_id, operation, changed_at
		FROM library_changes WHERE revision > ?
		ORDER BY revision LIMIT ?`, since, limit+1)
	if err != nil { return LibraryChangePage{}, err }
	defer rows.Close()

	changes := make([]LibraryChange, 0, limit+1)
	for rows.Next() {
		var change LibraryChange
		if err := rows.Scan(&change.Revision, &change.SongID, &change.Operation, &change.ChangedAt); err != nil {
			return LibraryChangePage{}, err
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil { return LibraryChangePage{}, err }

	page := LibraryChangePage{FromRevision: since, Changes: changes}
	if len(page.Changes) > limit {
		page.HasMore = true
		page.Changes = page.Changes[:limit]
	}
	current, err := d.LibraryRevision()
	if err != nil { return LibraryChangePage{}, err }
	if len(page.Changes) > 0 { page.ToRevision = page.Changes[len(page.Changes)-1].Revision } else { page.ToRevision = current }

	upsertIDs := make([]string, 0, len(page.Changes))
	seen := make(map[string]struct{}, len(page.Changes))
	for _, change := range page.Changes {
		if change.Operation != "upsert" { continue }
		if _, exists := seen[change.SongID]; exists { continue }
		seen[change.SongID] = struct{}{}
		upsertIDs = append(upsertIDs, change.SongID)
	}
	page.Songs, err = d.GetSongsByIDs(upsertIDs)
	if err != nil { return LibraryChangePage{}, err }
	return page, nil
}

func (d *DB) GetSongsByIDs(ids []string) ([]Song, error) {
	if len(ids) == 0 { return []Song{}, nil }
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids { args = append(args, id) }
	rows, err := d.conn.Query(songSelect+`
		WHERE COALESCE(ignored, 0) = 0 AND id IN (`+placeholders+`)`, args...)
	if err != nil { return nil, err }
	return scanLibrarySongRows(rows)
}

func (d *DB) PruneLibraryChanges(retain int64) error {
	if retain <= 0 { retain = 100000 }
	revision, err := d.LibraryRevision()
	if err != nil { return err }
	cutoff := revision - retain
	if cutoff <= 0 { return nil }
	_, err = d.conn.Exec(`DELETE FROM library_changes WHERE revision < ?`, cutoff)
	return err
}

func (d *DB) LibrarySyncStats() (revision, retainedChanges int64, err error) {
	if err = d.conn.QueryRow(`SELECT revision FROM library_state WHERE id = 1`).Scan(&revision); err != nil { return 0, 0, err }
	if err = d.conn.QueryRow(`SELECT COUNT(*) FROM library_changes`).Scan(&retainedChanges); err != nil { return 0, 0, err }
	return revision, retainedChanges, nil
}

func LibraryChangeRetentionAge(change LibraryChange) time.Duration {
	return time.Since(time.UnixMilli(change.ChangedAt))
}
