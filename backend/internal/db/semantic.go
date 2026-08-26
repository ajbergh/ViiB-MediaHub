// semantic.go owns the durable, provider-neutral semantic library records.
// SQLite is authoritative; the in-memory vector arenas are rebuilt from these rows.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	SemanticEntityTrack  = "track"
	SemanticEntityAlbum  = "album"
	SemanticEntityArtist = "artist"

	semanticWriteRowLimit  = 200
	semanticSQLiteParamCap = 900
	semanticWriteBlobLimit = 2 << 20
)

// SemanticDocument is the durable description and embedding state for one catalog entity.
// Content deliberately excludes embedding-provider task prefixes and user behaviour.
type SemanticDocument struct {
	ID                  int64
	EntityType          string
	EntityKey           string
	DisplayName         string
	SongID              string
	Artist              string
	Album               string
	Content             string
	ContentHash         string
	DocumentVersion     int
	EmbeddingProvider   string
	EmbeddingModel      string
	EmbeddingDimensions int
	Embedding           []byte
	Status              string
	RetryCount          int
	LastError           string
	EmbeddedAt          int64
	CreatedAt           int64
	UpdatedAt           int64
}

// EmbeddingUpdate is a validated, already L2-normalized embedding to persist.
type EmbeddingUpdate struct {
	DocumentID           int64
	EntityType           string
	EmbeddingProvider    string
	EmbeddingModel       string
	EmbeddingDimensions  int
	EmbeddingInputPrefix string
	Embedding            []byte
	EmbeddedAt           int64
}

// StoredEmbedding is the compact form streamed into an in-memory vector arena.
type StoredEmbedding struct {
	ID        int64
	Embedding []byte
}

// SemanticIndexState describes one entity-type arena and its provider identity.
type SemanticIndexState struct {
	EntityType           string
	DocumentRevision     int64
	ItemCount            int
	Dimensions           int
	EmbeddingProvider    string
	EmbeddingModel       string
	EmbeddingInputPrefix string
	CatalogCursor        int64
	LastFullRebuildAt    int64
	LastError            string
}

// SemanticStats is intentionally small enough for a local status endpoint.
type SemanticStats struct {
	DocumentsByStatus map[string]int
	State             []SemanticIndexState
}

// SemanticArtistEnrichment is the small, stable artist-metadata subset used
// while assembling deterministic semantic documents.
type SemanticArtistEnrichment struct {
	Artist         string
	LastFMTags     string
	LastFMBio      string
	SimilarArtists []string
}

// SemanticAlbumEnrichment is the album-metadata subset that can materially
// improve document context without copying artwork or provider identifiers.
type SemanticAlbumEnrichment struct {
	Artist string
	Album  string
	Genre  string
}

// SemanticMetadataChange identifies metadata writes that do not pass through
// the songs triggers and therefore need a semantic reconciliation pass.
type SemanticMetadataChange struct {
	ID         int64
	EntityType string
	EntityKey  string
	ChangedAt  int64
}

func validSemanticEntityType(entityType string) bool {
	switch entityType {
	case SemanticEntityTrack, SemanticEntityAlbum, SemanticEntityArtist:
		return true
	default:
		return false
	}
}

// EnsureSemanticSchema installs the semantic tables once per database handle.
// It deliberately does not extend db.migrate(): this feature is additive and can
// start independently of the existing catalog migration lifecycle.
func (d *DB) EnsureSemanticSchema() error {
	d.semanticOnce.Do(func() {
		if err := d.ConfigureRuntime(); err != nil {
			d.semanticInitErr = err
			return
		}
		d.semanticInitErr = d.ensureSemanticSchema()
	})
	return d.semanticInitErr
}

func (d *DB) ensureSemanticSchema() error {
	const schema = `
		CREATE TABLE IF NOT EXISTS semantic_documents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_type TEXT NOT NULL CHECK(entity_type IN ('track','album','artist')),
			entity_key TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			song_id TEXT,
			artist TEXT NOT NULL DEFAULT '',
			album TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			document_version INTEGER NOT NULL,
			embedding_provider TEXT NOT NULL DEFAULT '',
			embedding_model TEXT NOT NULL DEFAULT '',
			embedding_dimensions INTEGER NOT NULL DEFAULT 0,
			embedding BLOB,
			status TEXT NOT NULL DEFAULT 'pending'
				CHECK(status IN ('pending','embedding','ready','error')),
			retry_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			embedded_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(entity_type, entity_key)
		);
		CREATE INDEX IF NOT EXISTS idx_semantic_documents_status
			ON semantic_documents(entity_type, status);
		CREATE INDEX IF NOT EXISTS idx_semantic_documents_song_id
			ON semantic_documents(song_id);
		CREATE INDEX IF NOT EXISTS idx_semantic_documents_artist
			ON semantic_documents(artist);
		CREATE TABLE IF NOT EXISTS semantic_index_state (
			entity_type TEXT PRIMARY KEY CHECK(entity_type IN ('track','album','artist')),
			document_revision INTEGER NOT NULL DEFAULT 0,
			item_count INTEGER NOT NULL DEFAULT 0,
			dimensions INTEGER NOT NULL DEFAULT 0,
			embedding_provider TEXT NOT NULL DEFAULT '',
			embedding_model TEXT NOT NULL DEFAULT '',
			embedding_input_prefix TEXT NOT NULL DEFAULT '',
			catalog_cursor INTEGER NOT NULL DEFAULT 0,
			last_full_rebuild_at INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS semantic_metadata_changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_type TEXT NOT NULL CHECK(entity_type IN ('artist','album')),
			entity_key TEXT NOT NULL,
			changed_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_semantic_metadata_changes_id
			ON semantic_metadata_changes(id);
	`
	if _, err := d.conn.Exec(schema); err != nil {
		return fmt.Errorf("create semantic schema: %w", err)
	}
	for _, entityType := range []string{SemanticEntityTrack, SemanticEntityAlbum, SemanticEntityArtist} {
		if _, err := d.conn.Exec(`INSERT OR IGNORE INTO semantic_index_state(entity_type) VALUES (?)`, entityType); err != nil {
			return fmt.Errorf("initialize semantic index state: %w", err)
		}
	}
	return nil
}

// RecordSemanticMetadataChange records metadata writes that are invisible to
// library_changes. The background semantic worker folds these into a
// content-hash reconciliation; recording never embeds synchronously.
func (d *DB) RecordSemanticMetadataChange(ctx context.Context, entityType, entityKey string) error {
	if entityType != SemanticEntityArtist && entityType != SemanticEntityAlbum {
		return fmt.Errorf("invalid semantic metadata entity type %q", entityType)
	}
	if strings.TrimSpace(entityKey) == "" {
		return errors.New("semantic metadata entity key is required")
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		return err
	}
	_, err := d.conn.ExecContext(ctx, `INSERT INTO semantic_metadata_changes(entity_type, entity_key, changed_at) VALUES (?, ?, ?)`, entityType, entityKey, time.Now().UnixMilli())
	return err
}

// GetSemanticMetadataChanges returns a bounded FIFO page. It deliberately
// retains rows until the semantic worker has durably reconciled their content.
func (d *DB) GetSemanticMetadataChanges(ctx context.Context, limit int) ([]SemanticMetadataChange, error) {
	if err := d.EnsureSemanticSchema(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > semanticWriteRowLimit {
		limit = semanticWriteRowLimit
	}
	rows, err := d.conn.QueryContext(ctx, `SELECT id, entity_type, entity_key, changed_at FROM semantic_metadata_changes ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	changes := make([]SemanticMetadataChange, 0, limit)
	for rows.Next() {
		var change SemanticMetadataChange
		if err := rows.Scan(&change.ID, &change.EntityType, &change.EntityKey, &change.ChangedAt); err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, rows.Err()
}

// DeleteSemanticMetadataChangesThrough acknowledges an already reconciled
// FIFO page. Newer changes remain intact if metadata is updated mid-run.
func (d *DB) DeleteSemanticMetadataChangesThrough(ctx context.Context, id int64) error {
	if id <= 0 {
		return nil
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		return err
	}
	_, err := d.conn.ExecContext(ctx, `DELETE FROM semantic_metadata_changes WHERE id <= ?`, id)
	return err
}

// GetSemanticEnrichments loads artist and album metadata in bounded bulk
// queries so document generation never does N+1 catalog reads.
func (d *DB) GetSemanticEnrichments(ctx context.Context, artists []string) ([]SemanticArtistEnrichment, []SemanticAlbumEnrichment, error) {
	artistNames := make([]string, 0, len(artists))
	seen := make(map[string]struct{}, len(artists))
	for _, artist := range artists {
		artist = strings.TrimSpace(artist)
		if artist == "" {
			continue
		}
		if _, exists := seen[artist]; exists {
			continue
		}
		seen[artist] = struct{}{}
		artistNames = append(artistNames, artist)
	}
	if len(artistNames) == 0 {
		return []SemanticArtistEnrichment{}, []SemanticAlbumEnrichment{}, nil
	}
	artistByName := make(map[string]*SemanticArtistEnrichment)
	albums := make([]SemanticAlbumEnrichment, 0)
	for start := 0; start < len(artistNames); start += semanticSQLiteParamCap {
		end := start + semanticSQLiteParamCap
		if end > len(artistNames) {
			end = len(artistNames)
		}
		chunk := artistNames[start:end]
		placeholders := strings.TrimRight(strings.Repeat("?,", len(chunk)), ",")
		args := make([]any, len(chunk))
		for index, artist := range chunk {
			args[index] = artist
		}
		rows, err := d.conn.QueryContext(ctx, `SELECT artist_name, COALESCE(lastfm_tags, ''), COALESCE(lastfm_bio, '') FROM artist_metadata WHERE artist_name IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			item := &SemanticArtistEnrichment{}
			if err := rows.Scan(&item.Artist, &item.LastFMTags, &item.LastFMBio); err != nil {
				rows.Close()
				return nil, nil, err
			}
			artistByName[item.Artist] = item
		}
		if err := rows.Close(); err != nil {
			return nil, nil, err
		}
		similarRows, err := d.conn.QueryContext(ctx, `SELECT artist_name, similar_artist FROM lastfm_similar_artists WHERE artist_name IN (`+placeholders+`) ORDER BY artist_name, match_score DESC, similar_artist`, args...)
		if err != nil {
			return nil, nil, err
		}
		for similarRows.Next() {
			var artist, similar string
			if err := similarRows.Scan(&artist, &similar); err != nil {
				similarRows.Close()
				return nil, nil, err
			}
			item := artistByName[artist]
			if item == nil {
				item = &SemanticArtistEnrichment{Artist: artist}
				artistByName[artist] = item
			}
			item.SimilarArtists = append(item.SimilarArtists, similar)
		}
		if err := similarRows.Close(); err != nil {
			return nil, nil, err
		}
		albumRows, err := d.conn.QueryContext(ctx, `SELECT album_name, artist_name, COALESCE(genre, '') FROM album_metadata WHERE artist_name IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, nil, err
		}
		for albumRows.Next() {
			var album SemanticAlbumEnrichment
			if err := albumRows.Scan(&album.Album, &album.Artist, &album.Genre); err != nil {
				albumRows.Close()
				return nil, nil, err
			}
			albums = append(albums, album)
		}
		if err := albumRows.Close(); err != nil {
			return nil, nil, err
		}
	}
	artistResults := make([]SemanticArtistEnrichment, 0, len(artistByName))
	for _, artist := range artistByName {
		artistResults = append(artistResults, *artist)
	}
	return artistResults, albums, nil
}

func semanticDocumentSelect() string {
	return `SELECT id, entity_type, entity_key, display_name, song_id, artist, album,
		content, content_hash, document_version, embedding_provider, embedding_model,
		embedding_dimensions, embedding, status, retry_count, last_error, embedded_at,
		created_at, updated_at FROM semantic_documents`
}

func scanSemanticDocument(scanner interface{ Scan(...any) error }) (SemanticDocument, error) {
	var doc SemanticDocument
	var songID sql.NullString
	var embedding []byte
	if err := scanner.Scan(&doc.ID, &doc.EntityType, &doc.EntityKey, &doc.DisplayName, &songID,
		&doc.Artist, &doc.Album, &doc.Content, &doc.ContentHash, &doc.DocumentVersion,
		&doc.EmbeddingProvider, &doc.EmbeddingModel, &doc.EmbeddingDimensions, &embedding,
		&doc.Status, &doc.RetryCount, &doc.LastError, &doc.EmbeddedAt, &doc.CreatedAt, &doc.UpdatedAt); err != nil {
		return SemanticDocument{}, err
	}
	if songID.Valid {
		doc.SongID = songID.String
	}
	if embedding != nil {
		doc.Embedding = append([]byte(nil), embedding...)
	}
	return doc, nil
}

// UpsertSemanticDocuments writes document changes in short transactions. An
// unchanged content hash retains its embedding, while a changed ready document
// is atomically returned to pending and advances its arena revision.
func (d *DB) UpsertSemanticDocuments(ctx context.Context, docs []SemanticDocument) error {
	if err := d.EnsureSemanticSchema(); err != nil {
		return err
	}
	for start := 0; start < len(docs); start += semanticWriteRowLimit {
		end := start + semanticWriteRowLimit
		if end > len(docs) {
			end = len(docs)
		}
		if err := d.upsertSemanticDocumentBatch(ctx, docs[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) upsertSemanticDocumentBatch(ctx context.Context, docs []SemanticDocument) error {
	if len(docs) == 0 {
		return nil
	}
	for _, doc := range docs {
		if !validSemanticEntityType(doc.EntityType) || strings.TrimSpace(doc.EntityKey) == "" || strings.TrimSpace(doc.Content) == "" || strings.TrimSpace(doc.ContentHash) == "" || doc.DocumentVersion <= 0 {
			return errors.New("semantic document has invalid required fields")
		}
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	changedReady, err := semanticReadyRevisionDeltas(ctx, tx, docs)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO semantic_documents(
			entity_type, entity_key, display_name, song_id, artist, album, content, content_hash,
			document_version, status, created_at, updated_at
		) VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, 'pending', ?, ?)
		ON CONFLICT(entity_type, entity_key) DO UPDATE SET
			display_name = excluded.display_name,
			song_id = excluded.song_id,
			artist = excluded.artist,
			album = excluded.album,
			content = excluded.content,
			content_hash = excluded.content_hash,
			document_version = excluded.document_version,
			status = CASE WHEN semantic_documents.content_hash != excluded.content_hash
				OR semantic_documents.document_version != excluded.document_version THEN 'pending' ELSE semantic_documents.status END,
			embedding_provider = CASE WHEN semantic_documents.content_hash != excluded.content_hash
				OR semantic_documents.document_version != excluded.document_version THEN '' ELSE semantic_documents.embedding_provider END,
			embedding_model = CASE WHEN semantic_documents.content_hash != excluded.content_hash
				OR semantic_documents.document_version != excluded.document_version THEN '' ELSE semantic_documents.embedding_model END,
			embedding_dimensions = CASE WHEN semantic_documents.content_hash != excluded.content_hash
				OR semantic_documents.document_version != excluded.document_version THEN 0 ELSE semantic_documents.embedding_dimensions END,
			embedding = CASE WHEN semantic_documents.content_hash != excluded.content_hash
				OR semantic_documents.document_version != excluded.document_version THEN NULL ELSE semantic_documents.embedding END,
			retry_count = CASE WHEN semantic_documents.content_hash != excluded.content_hash
				OR semantic_documents.document_version != excluded.document_version THEN 0 ELSE semantic_documents.retry_count END,
			last_error = CASE WHEN semantic_documents.content_hash != excluded.content_hash
				OR semantic_documents.document_version != excluded.document_version THEN '' ELSE semantic_documents.last_error END,
			embedded_at = CASE WHEN semantic_documents.content_hash != excluded.content_hash
				OR semantic_documents.document_version != excluded.document_version THEN 0 ELSE semantic_documents.embedded_at END,
			updated_at = excluded.updated_at
		WHERE semantic_documents.display_name IS NOT excluded.display_name
			OR semantic_documents.song_id IS NOT excluded.song_id
			OR semantic_documents.artist IS NOT excluded.artist
			OR semantic_documents.album IS NOT excluded.album
			OR semantic_documents.content_hash != excluded.content_hash
			OR semantic_documents.document_version != excluded.document_version`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UnixMilli()
	for _, doc := range docs {
		createdAt := doc.CreatedAt
		if createdAt == 0 {
			createdAt = now
		}
		updatedAt := doc.UpdatedAt
		if updatedAt == 0 {
			updatedAt = now
		}
		if _, err := stmt.ExecContext(ctx, doc.EntityType, doc.EntityKey, doc.DisplayName, doc.SongID, doc.Artist, doc.Album, doc.Content, doc.ContentHash, doc.DocumentVersion, createdAt, updatedAt); err != nil {
			return fmt.Errorf("upsert semantic document: %w", err)
		}
	}
	if err := invalidateReadySemanticDocuments(ctx, tx, changedReady); err != nil {
		return err
	}
	return tx.Commit()
}

func semanticReadyRevisionDeltas(ctx context.Context, tx *sql.Tx, docs []SemanticDocument) (map[string]int64, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	values := make([]string, 0, len(docs))
	args := make([]any, 0, len(docs)*4)
	for _, doc := range docs {
		values = append(values, "(?, ?, ?, ?)")
		args = append(args, doc.EntityType, doc.EntityKey, doc.ContentHash, doc.DocumentVersion)
	}
	rows, err := tx.QueryContext(ctx, `WITH incoming(entity_type, entity_key, content_hash, document_version) AS (VALUES `+strings.Join(values, ",")+`)
		SELECT d.entity_type, COUNT(*)
		FROM semantic_documents d JOIN incoming i ON d.entity_type = i.entity_type AND d.entity_key = i.entity_key
		WHERE d.status = 'ready' AND (d.content_hash != i.content_hash OR d.document_version != i.document_version)
		GROUP BY d.entity_type`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deltas := make(map[string]int64)
	for rows.Next() {
		var entityType string
		var count int64
		if err := rows.Scan(&entityType, &count); err != nil {
			return nil, err
		}
		deltas[entityType] = count
	}
	return deltas, rows.Err()
}

// invalidateReadySemanticDocuments keeps the durable state count truthful while
// a changed or orphaned ready row is absent from the live arena. StoreSemanticEmbeddings
// later increments the count again only after the replacement vector is ready.
func invalidateReadySemanticDocuments(ctx context.Context, tx *sql.Tx, deltas map[string]int64) error {
	for entityType, delta := range deltas {
		if delta == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE semantic_index_state
			SET document_revision = document_revision + ?,
				item_count = MAX(0, item_count - ?),
				dimensions = CASE WHEN item_count <= ? THEN 0 ELSE dimensions END,
				embedding_provider = CASE WHEN item_count <= ? THEN '' ELSE embedding_provider END,
				embedding_model = CASE WHEN item_count <= ? THEN '' ELSE embedding_model END,
				embedding_input_prefix = CASE WHEN item_count <= ? THEN '' ELSE embedding_input_prefix END
			WHERE entity_type = ?`, delta, delta, delta, delta, delta, delta, entityType); err != nil {
			return err
		}
	}
	return nil
}

// GetPendingSemanticDocuments returns a bounded embedding queue. Error rows
// intentionally require an explicit retry path instead of silently looping.
func (d *DB) GetPendingSemanticDocuments(ctx context.Context, entityType string, limit int) ([]SemanticDocument, error) {
	if !validSemanticEntityType(entityType) {
		return nil, fmt.Errorf("invalid semantic entity type %q", entityType)
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > semanticWriteRowLimit {
		limit = semanticWriteRowLimit
	}
	rows, err := d.conn.QueryContext(ctx, semanticDocumentSelect()+` WHERE entity_type = ? AND status = 'pending' ORDER BY id LIMIT ?`, entityType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	docs := make([]SemanticDocument, 0, limit)
	for rows.Next() {
		doc, err := scanSemanticDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

// StoreSemanticEmbeddings persists a bounded batch and advances revision only
// for rows that became ready. Callers must validate and L2-normalize blobs.
func (d *DB) StoreSemanticEmbeddings(ctx context.Context, updates []EmbeddingUpdate) error {
	if err := d.EnsureSemanticSchema(); err != nil {
		return err
	}
	byType := make(map[string][]EmbeddingUpdate)
	for _, update := range updates {
		if update.DocumentID <= 0 || !validSemanticEntityType(update.EntityType) || update.EmbeddingDimensions <= 0 || len(update.Embedding) == 0 || len(update.Embedding)%4 != 0 {
			return errors.New("semantic embedding update has invalid required fields")
		}
		byType[update.EntityType] = append(byType[update.EntityType], update)
	}
	for entityType, entityUpdates := range byType {
		for start := 0; start < len(entityUpdates); {
			end, bytes := start, 0
			for end < len(entityUpdates) && end-start < semanticWriteRowLimit {
				next := len(entityUpdates[end].Embedding)
				if end > start && bytes+next > semanticWriteBlobLimit {
					break
				}
				bytes += next
				end++
			}
			if err := d.storeSemanticEmbeddingBatch(ctx, entityType, entityUpdates[start:end]); err != nil {
				return err
			}
			start = end
		}
	}
	return nil
}

func (d *DB) storeSemanticEmbeddingBatch(ctx context.Context, entityType string, updates []EmbeddingUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	identity := updates[0]
	for _, update := range updates[1:] {
		if update.EmbeddingDimensions != identity.EmbeddingDimensions || update.EmbeddingProvider != identity.EmbeddingProvider || update.EmbeddingModel != identity.EmbeddingModel || update.EmbeddingInputPrefix != identity.EmbeddingInputPrefix {
			return errors.New("semantic embedding batch mixes provider identities")
		}
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `UPDATE semantic_documents SET
		embedding_provider = ?, embedding_model = ?, embedding_dimensions = ?, embedding = ?,
		status = 'ready', retry_count = 0, last_error = '', embedded_at = ?, updated_at = ?
		WHERE id = ? AND entity_type = ? AND status IN ('pending', 'embedding', 'error')`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	var changed int64
	now := time.Now().UnixMilli()
	for _, update := range updates {
		embeddedAt := update.EmbeddedAt
		if embeddedAt == 0 {
			embeddedAt = now
		}
		result, err := stmt.ExecContext(ctx, update.EmbeddingProvider, update.EmbeddingModel, update.EmbeddingDimensions, update.Embedding, embeddedAt, now, update.DocumentID, entityType)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		changed += count
	}
	if changed > 0 {
		first := updates[0]
		var itemCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_documents WHERE entity_type = ? AND status = 'ready'`, entityType).Scan(&itemCount); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE semantic_index_state SET
			document_revision = document_revision + ?, item_count = ?, dimensions = ?,
			embedding_provider = ?, embedding_model = ?, embedding_input_prefix = ?
			WHERE entity_type = ?`, changed, itemCount, first.EmbeddingDimensions, first.EmbeddingProvider, first.EmbeddingModel, first.EmbeddingInputPrefix, entityType); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MarkSemanticDocumentError records a retryable provider failure without
// touching a ready vector or the arena revision.
func (d *DB) MarkSemanticDocumentError(ctx context.Context, id int64, err error) error {
	if id <= 0 || err == nil {
		return errors.New("semantic document id and error are required")
	}
	if schemaErr := d.EnsureSemanticSchema(); schemaErr != nil {
		return schemaErr
	}
	_, updateErr := d.conn.ExecContext(ctx, `UPDATE semantic_documents SET status = 'error', retry_count = retry_count + 1, last_error = ?, updated_at = ?
		WHERE id = ? AND status IN ('pending', 'embedding', 'error')`, err.Error(), time.Now().UnixMilli(), id)
	return updateErr
}

// ResetSemanticEmbeddings invalidates every durable vector when the embedding
// identity changes. Documents remain available for re-embedding, while an
// already-loaded in-memory arena can continue serving the prior identity until
// the caller swaps in a complete replacement.
func (d *DB) ResetSemanticEmbeddings(ctx context.Context) error {
	if err := d.EnsureSemanticSchema(); err != nil {
		return err
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE semantic_documents SET
		embedding_provider = '', embedding_model = '', embedding_dimensions = 0, embedding = NULL,
		status = 'pending', retry_count = 0, last_error = '', embedded_at = 0, updated_at = ?`, time.Now().UnixMilli()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE semantic_index_state SET
		document_revision = document_revision + 1, item_count = 0, dimensions = 0,
		embedding_provider = '', embedding_model = '', embedding_input_prefix = '', last_error = ''`); err != nil {
		return err
	}
	return tx.Commit()
}

// RetrySemanticDocumentErrors puts explicitly retried provider failures back
// onto the pending queue. It deliberately does not touch ready embeddings.
func (d *DB) RetrySemanticDocumentErrors(ctx context.Context) (int, error) {
	if err := d.EnsureSemanticSchema(); err != nil {
		return 0, err
	}
	result, err := d.conn.ExecContext(ctx, `UPDATE semantic_documents SET
		status = 'pending', retry_count = 0, last_error = '', updated_at = ?
		WHERE status = 'error'`, time.Now().UnixMilli())
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	return int(count), err
}

// DeleteSemanticDocumentsForMissingSongs removes orphaned track identities.
func (d *DB) DeleteSemanticDocumentsForMissingSongs(ctx context.Context) (int, error) {
	if err := d.EnsureSemanticSchema(); err != nil {
		return 0, err
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	const missing = `entity_type = 'track' AND song_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM songs WHERE songs.id = semantic_documents.song_id)`
	var ready int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_documents WHERE status = 'ready' AND `+missing).Scan(&ready); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM semantic_documents WHERE `+missing)
	if err != nil {
		return 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if ready > 0 {
		if err := invalidateReadySemanticDocuments(ctx, tx, map[string]int64{SemanticEntityTrack: ready}); err != nil {
			return 0, err
		}
	}
	return int(deleted), tx.Commit()
}

// ListSemanticDocumentKeys returns durable entity keys so the indexer can
// remove artist/album aggregates that disappeared after a catalog change.
func (d *DB) ListSemanticDocumentKeys(ctx context.Context, entityType string) ([]string, error) {
	if !validSemanticEntityType(entityType) {
		return nil, fmt.Errorf("invalid semantic entity type %q", entityType)
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.QueryContext(ctx, `SELECT entity_key FROM semantic_documents WHERE entity_type = ? ORDER BY entity_key`, entityType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// DeleteSemanticDocumentsByKeys removes obsolete entities in bounded
// transactions and keeps their arena state truthful until the next rebuild.
func (d *DB) DeleteSemanticDocumentsByKeys(ctx context.Context, entityType string, keys []string) (int, error) {
	if !validSemanticEntityType(entityType) {
		return 0, fmt.Errorf("invalid semantic entity type %q", entityType)
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		return 0, err
	}
	deleted := 0
	for start := 0; start < len(keys); start += semanticSQLiteParamCap {
		end := start + semanticSQLiteParamCap
		if end > len(keys) {
			end = len(keys)
		}
		count, err := d.deleteSemanticDocumentKeyBatch(ctx, entityType, keys[start:end])
		if err != nil {
			return deleted, err
		}
		deleted += count
	}
	return deleted, nil
}

func (d *DB) deleteSemanticDocumentKeyBatch(ctx context.Context, entityType string, keys []string) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	placeholders := strings.TrimRight(strings.Repeat("?,", len(keys)), ",")
	args := make([]any, 0, len(keys)+1)
	args = append(args, entityType)
	for _, key := range keys {
		args = append(args, key)
	}
	var ready int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_documents WHERE entity_type = ? AND status = 'ready' AND entity_key IN (`+placeholders+`)`, args...).Scan(&ready); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM semantic_documents WHERE entity_type = ? AND entity_key IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if ready > 0 {
		if err := invalidateReadySemanticDocuments(ctx, tx, map[string]int64{entityType: ready}); err != nil {
			return 0, err
		}
	}
	return int(count), tx.Commit()
}

// ListReadySemanticEmbeddings returns the corpus in ID order for sequential
// preallocation into a contiguous in-memory arena.
func (d *DB) ListReadySemanticEmbeddings(ctx context.Context, entityType string) ([]StoredEmbedding, error) {
	if !validSemanticEntityType(entityType) {
		return nil, fmt.Errorf("invalid semantic entity type %q", entityType)
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.QueryContext(ctx, `SELECT id, embedding FROM semantic_documents WHERE entity_type = ? AND status = 'ready' ORDER BY id`, entityType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]StoredEmbedding, 0)
	for rows.Next() {
		var item StoredEmbedding
		if err := rows.Scan(&item.ID, &item.Embedding); err != nil {
			return nil, err
		}
		item.Embedding = append([]byte(nil), item.Embedding...)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) GetSemanticDocumentsByIDs(ctx context.Context, ids []int64) ([]SemanticDocument, error) {
	if err := d.EnsureSemanticSchema(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []SemanticDocument{}, nil
	}
	docs := make([]SemanticDocument, 0, len(ids))
	for start := 0; start < len(ids); start += semanticSQLiteParamCap {
		end := start + semanticSQLiteParamCap
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := strings.TrimRight(strings.Repeat("?,", end-start), ",")
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
			args = append(args, id)
		}
		rows, err := d.conn.QueryContext(ctx, semanticDocumentSelect()+` WHERE id IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			doc, scanErr := scanSemanticDocument(rows)
			if scanErr != nil {
				rows.Close()
				return nil, scanErr
			}
			docs = append(docs, doc)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return docs, nil
}

// GetSongsByIDsContext is the context-aware, chunked variant used by semantic
// retrieval. GetSongsByIDs remains for existing callers until they migrate.
func (d *DB) GetSongsByIDsContext(ctx context.Context, ids []string) ([]Song, error) {
	if len(ids) == 0 {
		return []Song{}, nil
	}
	songs := make([]Song, 0, len(ids))
	for start := 0; start < len(ids); start += semanticSQLiteParamCap {
		end := start + semanticSQLiteParamCap
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := strings.TrimRight(strings.Repeat("?,", end-start), ",")
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
			args = append(args, id)
		}
		rows, err := d.conn.QueryContext(ctx, songSelect+` WHERE COALESCE(ignored, 0) = 0 AND id IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		chunk, err := scanLibrarySongRows(rows)
		if err != nil {
			return nil, err
		}
		songs = append(songs, chunk...)
	}
	return songs, nil
}

func (d *DB) GetSongsForSemanticAlbum(ctx context.Context, artist, album string, limit int) ([]Song, error) {
	if limit <= 0 || limit > semanticSQLiteParamCap {
		limit = semanticSQLiteParamCap
	}
	rows, err := d.conn.QueryContext(ctx, songSelect+` WHERE COALESCE(ignored, 0) = 0 AND lower(COALESCE(album_artist, artist)) = lower(?) AND lower(album) = lower(?) ORDER BY disc_number, track_number, title LIMIT ?`, artist, album, limit)
	if err != nil {
		return nil, err
	}
	return scanLibrarySongRows(rows)
}

func (d *DB) GetSongsForSemanticArtist(ctx context.Context, artist string, limit int) ([]Song, error) {
	if limit <= 0 || limit > semanticSQLiteParamCap {
		limit = semanticSQLiteParamCap
	}
	rows, err := d.conn.QueryContext(ctx, songSelect+` WHERE COALESCE(ignored, 0) = 0 AND lower(artist) = lower(?) ORDER BY album, disc_number, track_number, title LIMIT ?`, artist, limit)
	if err != nil {
		return nil, err
	}
	return scanLibrarySongRows(rows)
}

func (d *DB) GetSemanticIndexState(ctx context.Context, entityType string) (SemanticIndexState, error) {
	if !validSemanticEntityType(entityType) {
		return SemanticIndexState{}, fmt.Errorf("invalid semantic entity type %q", entityType)
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		return SemanticIndexState{}, err
	}
	return scanSemanticIndexState(d.conn.QueryRowContext(ctx, `SELECT entity_type, document_revision, item_count, dimensions, embedding_provider, embedding_model, embedding_input_prefix, catalog_cursor, last_full_rebuild_at, last_error FROM semantic_index_state WHERE entity_type = ?`, entityType))
}

func scanSemanticIndexState(scanner interface{ Scan(...any) error }) (SemanticIndexState, error) {
	var state SemanticIndexState
	err := scanner.Scan(&state.EntityType, &state.DocumentRevision, &state.ItemCount, &state.Dimensions, &state.EmbeddingProvider, &state.EmbeddingModel, &state.EmbeddingInputPrefix, &state.CatalogCursor, &state.LastFullRebuildAt, &state.LastError)
	return state, err
}

func (d *DB) IncrementSemanticDocumentRevision(ctx context.Context, entityType string) error {
	if !validSemanticEntityType(entityType) {
		return fmt.Errorf("invalid semantic entity type %q", entityType)
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		return err
	}
	_, err := d.conn.ExecContext(ctx, `UPDATE semantic_index_state SET document_revision = document_revision + 1 WHERE entity_type = ?`, entityType)
	return err
}

func (d *DB) SetSemanticCatalogCursor(ctx context.Context, entityType string, revision int64) error {
	if !validSemanticEntityType(entityType) || revision < 0 {
		return errors.New("semantic entity type and non-negative cursor are required")
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		return err
	}
	_, err := d.conn.ExecContext(ctx, `UPDATE semantic_index_state SET catalog_cursor = ? WHERE entity_type = ?`, revision, entityType)
	return err
}

func (d *DB) GetSemanticIndexStats(ctx context.Context) (SemanticStats, error) {
	if err := d.EnsureSemanticSchema(); err != nil {
		return SemanticStats{}, err
	}
	stats := SemanticStats{DocumentsByStatus: make(map[string]int)}
	rows, err := d.conn.QueryContext(ctx, `SELECT status, COUNT(*) FROM semantic_documents GROUP BY status`)
	if err != nil {
		return SemanticStats{}, err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return SemanticStats{}, err
		}
		stats.DocumentsByStatus[status] = count
	}
	if err := rows.Close(); err != nil {
		return SemanticStats{}, err
	}
	stateRows, err := d.conn.QueryContext(ctx, `SELECT entity_type, document_revision, item_count, dimensions, embedding_provider, embedding_model, embedding_input_prefix, catalog_cursor, last_full_rebuild_at, last_error FROM semantic_index_state ORDER BY entity_type`)
	if err != nil {
		return SemanticStats{}, err
	}
	defer stateRows.Close()
	for stateRows.Next() {
		state, err := scanSemanticIndexState(stateRows)
		if err != nil {
			return SemanticStats{}, err
		}
		stats.State = append(stats.State, state)
	}
	return stats, stateRows.Err()
}
