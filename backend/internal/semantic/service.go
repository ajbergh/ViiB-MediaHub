package semantic

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const (
	semanticEmbeddingRetryLimit = 3
	semanticRetryBaseDelay      = 50 * time.Millisecond
	semanticChangePollInterval  = 30 * time.Second
)

const (
	serviceStateIdle     = "idle"
	serviceStateIndexing = "indexing"
	serviceStateReady    = "ready"
	serviceStateError    = "error"
)

// ServiceStatus is intentionally small enough for a local status endpoint.
// It contains no document text, file paths, or secret settings.
type ServiceStatus struct {
	State            string    `json:"state"`
	CurrentEntity    string    `json:"currentEntity,omitempty"`
	DocumentsIndexed int       `json:"documentsIndexed"`
	FailedDocuments  int       `json:"failedDocuments"`
	LastError        string    `json:"lastError,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// Service owns the durable document-to-vector pipeline. SQLite is the source
// of truth; vector arenas are replaceable read-only snapshots rebuilt only
// after a complete successful pass.
type Service struct {
	database *db.DB
	provider EmbeddingProvider

	runMu     sync.Mutex
	statusMu  sync.RWMutex
	indexesMu sync.RWMutex
	indexes   map[string]VectorIndex
	status    ServiceStatus

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
	closed      bool
}

func NewService(database *db.DB, provider EmbeddingProvider) (*Service, error) {
	if database == nil {
		return nil, errors.New("semantic database is required")
	}
	if provider == nil {
		return nil, errors.New("semantic embedding provider is required")
	}
	return &Service{
		database: database,
		provider: provider,
		indexes: map[string]VectorIndex{
			db.SemanticEntityArtist: newScanIndex(),
			db.SemanticEntityAlbum:  newScanIndex(),
			db.SemanticEntityTrack:  newScanIndex(),
		},
		status: ServiceStatus{State: serviceStateIdle, UpdatedAt: time.Now().UTC()},
	}, nil
}

// Start loads any previously durable arenas then begins a content-hash-gated
// pass in the background. Backend startup never waits for provider calls.
func (service *Service) Start(parent context.Context) error {
	if err := service.LoadReadyIndexes(parent); err != nil {
		return err
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	if service.closed {
		return errors.New("semantic service is closed")
	}
	if service.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	service.cancel = cancel
	service.done = make(chan struct{})
	go func() {
		defer close(service.done)
		if err := service.Reindex(ctx); err != nil && !errors.Is(err, context.Canceled) {
			service.setStatus(ServiceStatus{State: serviceStateError, LastError: err.Error()})
			return
		}
		ticker := time.NewTicker(semanticChangePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := service.SyncChanges(ctx); err != nil && !errors.Is(err, context.Canceled) {
					service.setStatus(ServiceStatus{State: serviceStateError, LastError: err.Error()})
				}
			}
		}
	}()
	return nil
}

// Reindex regenerates all deterministic documents, but content hashes make
// unchanged rows retain their durable embeddings and skip provider work.
func (service *Service) Reindex(ctx context.Context) error {
	service.runMu.Lock()
	defer service.runMu.Unlock()
	return service.reindexLocked(ctx)
}

func (service *Service) reindexLocked(ctx context.Context) (err error) {
	defer func() {
		if err != nil && !errors.Is(err, context.Canceled) {
			service.setStatus(ServiceStatus{State: serviceStateError, LastError: err.Error()})
		}
	}()
	service.setStatus(ServiceStatus{State: serviceStateIndexing})
	if err := service.database.EnsureSemanticSchema(); err != nil {
		return err
	}
	if err := service.database.EnsureLibrarySyncSchema(); err != nil {
		return fmt.Errorf("initialize library change log: %w", err)
	}
	if err := service.resetForIdentityChange(ctx); err != nil {
		return err
	}
	catalogRevision, err := service.database.LibraryRevision()
	if err != nil {
		return fmt.Errorf("read catalog revision: %w", err)
	}
	songs, err := service.database.GetAllSongs()
	if err != nil {
		return fmt.Errorf("load catalog for semantic indexing: %w", err)
	}
	removed, err := service.reconcileCatalogDocuments(ctx, songs)
	if err != nil {
		return err
	}
	if err := service.setCatalogCursor(ctx, catalogRevision); err != nil {
		return err
	}
	indexed, failed, err := service.processPending(ctx)
	if err != nil {
		return err
	}
	if err := service.database.CheckpointWAL(); err != nil {
		return fmt.Errorf("checkpoint semantic WAL: %w", err)
	}
	if failed > 0 {
		service.setStatus(ServiceStatus{State: serviceStateError, DocumentsIndexed: indexed, FailedDocuments: failed, LastError: "some semantic documents could not be embedded"})
		return nil
	}
	if err := service.loadReadyIndexesLocked(ctx); err != nil {
		return err
	}
	service.setStatus(ServiceStatus{State: serviceStateReady, DocumentsIndexed: indexed + removed})
	return nil
}

// SyncChanges tails the durable songs change log. It performs a catalog
// content-hash reconciliation for the changed revision window, which keeps
// behavioral updates cheap while correctly refreshing affected aggregates.
func (service *Service) SyncChanges(ctx context.Context) error {
	service.runMu.Lock()
	defer service.runMu.Unlock()
	return service.syncChangesLocked(ctx)
}

func (service *Service) syncChangesLocked(ctx context.Context) (err error) {
	defer func() {
		if err != nil && !errors.Is(err, context.Canceled) {
			service.setStatus(ServiceStatus{State: serviceStateError, LastError: err.Error()})
		}
	}()
	if err := service.database.EnsureLibrarySyncSchema(); err != nil {
		return fmt.Errorf("initialize library change log: %w", err)
	}
	metadataChanges, err := service.database.GetSemanticMetadataChanges(ctx, 200)
	if err != nil {
		return fmt.Errorf("read semantic metadata changes: %w", err)
	}
	metadataChangeID := int64(0)
	if len(metadataChanges) > 0 {
		metadataChangeID = metadataChanges[len(metadataChanges)-1].ID
	}
	cursor, err := service.catalogCursor(ctx)
	if err != nil {
		return err
	}
	currentRevision, err := service.database.LibraryRevision()
	if err != nil {
		return fmt.Errorf("read catalog revision: %w", err)
	}
	hasCatalogChanges := currentRevision > cursor
	if !hasCatalogChanges && metadataChangeID == 0 {
		return nil
	}
	if hasCatalogChanges {
		oldest, retained, err := service.database.OldestLibraryChangeRevision()
		if err != nil {
			return fmt.Errorf("read oldest library change: %w", err)
		}
		if cursor == 0 || (retained && oldest > cursor+1) {
			if err := service.reindexLocked(ctx); err != nil {
				return err
			}
			return service.database.DeleteSemanticMetadataChangesThrough(ctx, metadataChangeID)
		}
		page, err := service.database.GetLibraryChanges(cursor, 1)
		if err != nil {
			return fmt.Errorf("read library changes: %w", err)
		}
		if len(page.Changes) == 0 && metadataChangeID == 0 {
			return service.setCatalogCursor(ctx, currentRevision)
		}
	}
	service.setStatus(ServiceStatus{State: serviceStateIndexing})
	songs, err := service.database.GetAllSongs()
	if err != nil {
		return fmt.Errorf("load catalog for semantic change reconciliation: %w", err)
	}
	removed, err := service.reconcileCatalogDocuments(ctx, songs)
	if err != nil {
		return err
	}
	if hasCatalogChanges {
		if err := service.setCatalogCursor(ctx, currentRevision); err != nil {
			return err
		}
	}
	indexed, failed, err := service.processPending(ctx)
	if err != nil {
		return err
	}
	if err := service.database.CheckpointWAL(); err != nil {
		return fmt.Errorf("checkpoint semantic WAL: %w", err)
	}
	if failed > 0 {
		service.setStatus(ServiceStatus{State: serviceStateError, DocumentsIndexed: indexed, FailedDocuments: failed, LastError: "some semantic documents could not be embedded"})
		return nil
	}
	if indexed > 0 || removed > 0 {
		if err := service.loadReadyIndexesLocked(ctx); err != nil {
			return err
		}
	}
	if err := service.database.DeleteSemanticMetadataChangesThrough(ctx, metadataChangeID); err != nil {
		return fmt.Errorf("acknowledge semantic metadata changes: %w", err)
	}
	service.setStatus(ServiceStatus{State: serviceStateReady, DocumentsIndexed: indexed + removed})
	return nil
}

// RetryErrors is the explicit recovery path for documents left in error after
// bounded provider retries. It never silently retries while serving requests.
func (service *Service) RetryErrors(ctx context.Context) (int, error) {
	service.runMu.Lock()
	defer service.runMu.Unlock()
	count, err := service.database.RetrySemanticDocumentErrors(ctx)
	if err != nil || count == 0 {
		return count, err
	}
	service.setStatus(ServiceStatus{State: serviceStateIndexing})
	indexed, failed, err := service.processPending(ctx)
	if err != nil {
		return count, err
	}
	if failed > 0 {
		service.setStatus(ServiceStatus{State: serviceStateError, DocumentsIndexed: indexed, FailedDocuments: failed, LastError: "some semantic documents could not be embedded"})
		return count, nil
	}
	if err := service.loadReadyIndexesLocked(ctx); err != nil {
		return count, err
	}
	service.setStatus(ServiceStatus{State: serviceStateReady, DocumentsIndexed: indexed})
	return count, nil
}

// LoadReadyIndexes restores the last complete durable vectors without issuing
// provider requests. It is safe to call before a background rebuild starts.
func (service *Service) LoadReadyIndexes(ctx context.Context) error {
	service.runMu.Lock()
	defer service.runMu.Unlock()
	return service.loadReadyIndexesLocked(ctx)
}

func (service *Service) loadReadyIndexesLocked(ctx context.Context) error {
	replacement := make(map[string]VectorIndex, 3)
	for _, entityType := range semanticEntityOrder {
		items, err := service.database.ListReadySemanticEmbeddings(ctx, entityType)
		if err != nil {
			return fmt.Errorf("load %s semantic embeddings: %w", entityType, err)
		}
		index := newScanIndex()
		if err := index.Rebuild(ctx, items); err != nil {
			return fmt.Errorf("rebuild %s semantic index: %w", entityType, err)
		}
		state, err := service.database.GetSemanticIndexState(ctx, entityType)
		if err != nil {
			return err
		}
		if state.ItemCount != index.Len() || (state.Dimensions != 0 && state.Dimensions != index.Dimensions()) {
			return fmt.Errorf("%s semantic index state does not match durable embeddings", entityType)
		}
		replacement[entityType] = index
	}
	service.indexesMu.Lock()
	previous := service.indexes
	service.indexes = replacement
	service.indexesMu.Unlock()
	for _, index := range previous {
		_ = index.Close()
	}
	return nil
}

func (service *Service) resetForIdentityChange(ctx context.Context) error {
	for _, entityType := range semanticEntityOrder {
		state, err := service.database.GetSemanticIndexState(ctx, entityType)
		if err != nil {
			return err
		}
		if state.ItemCount == 0 || state.EmbeddingProvider == "" {
			continue
		}
		if state.EmbeddingProvider != service.provider.Name() || state.EmbeddingModel != service.provider.Model() || state.EmbeddingInputPrefix != service.provider.DocumentPrefix() {
			return service.database.ResetSemanticEmbeddings(ctx)
		}
	}
	return nil
}

func (service *Service) reconcileCatalogDocuments(ctx context.Context, songs []db.Song) (int, error) {
	documentContext, err := service.documentContext(ctx, songs)
	if err != nil {
		return 0, err
	}
	documents := BuildDocuments(songs, documentContext)
	if err := service.database.UpsertSemanticDocuments(ctx, documents); err != nil {
		return 0, fmt.Errorf("upsert semantic documents: %w", err)
	}
	expected := make(map[string]map[string]struct{}, len(semanticEntityOrder))
	for _, entityType := range semanticEntityOrder {
		expected[entityType] = make(map[string]struct{})
	}
	for _, document := range documents {
		expected[document.EntityType][document.EntityKey] = struct{}{}
	}
	removed := 0
	for _, entityType := range semanticEntityOrder {
		keys, err := service.database.ListSemanticDocumentKeys(ctx, entityType)
		if err != nil {
			return removed, fmt.Errorf("list %s semantic document keys: %w", entityType, err)
		}
		obsolete := make([]string, 0)
		for _, key := range keys {
			if _, exists := expected[entityType][key]; !exists {
				obsolete = append(obsolete, key)
			}
		}
		deleted, err := service.database.DeleteSemanticDocumentsByKeys(ctx, entityType, obsolete)
		if err != nil {
			return removed, fmt.Errorf("remove obsolete %s semantic documents: %w", entityType, err)
		}
		removed += deleted
	}
	return removed, nil
}

func (service *Service) documentContext(ctx context.Context, songs []db.Song) (DocumentContext, error) {
	artists := make([]string, 0, len(songs))
	for _, song := range songs {
		artists = append(artists, song.Artist)
		if song.AlbumArtist != "" {
			artists = append(artists, song.AlbumArtist)
		}
	}
	artistEnrichments, albumEnrichments, err := service.database.GetSemanticEnrichments(ctx, artists)
	if err != nil {
		return DocumentContext{}, fmt.Errorf("load semantic document enrichment: %w", err)
	}
	result := DocumentContext{Artists: make(map[string]ArtistContext), Albums: make(map[string]AlbumContext)}
	for _, enrichment := range artistEnrichments {
		result.Artists[CanonicalArtistKey(enrichment.Artist)] = ArtistContext{
			Tags:           parseTags(enrichment.LastFMTags),
			Bio:            enrichment.LastFMBio,
			SimilarArtists: enrichment.SimilarArtists,
		}
	}
	for _, enrichment := range albumEnrichments {
		result.Albums[CanonicalAlbumKey(enrichment.Artist, enrichment.Album)] = AlbumContext{Tags: splitMetadataTags(enrichment.Genre)}
	}
	return result, nil
}

func splitMetadataTags(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '/'
	})
	return normalizedValues(parts)
}

func (service *Service) catalogCursor(ctx context.Context) (int64, error) {
	var cursor int64
	for index, entityType := range semanticEntityOrder {
		state, err := service.database.GetSemanticIndexState(ctx, entityType)
		if err != nil {
			return 0, err
		}
		if index == 0 || state.CatalogCursor < cursor {
			cursor = state.CatalogCursor
		}
	}
	return cursor, nil
}

func (service *Service) setCatalogCursor(ctx context.Context, revision int64) error {
	for _, entityType := range semanticEntityOrder {
		if err := service.database.SetSemanticCatalogCursor(ctx, entityType, revision); err != nil {
			return fmt.Errorf("set %s semantic catalog cursor: %w", entityType, err)
		}
	}
	return nil
}

func (service *Service) processPending(ctx context.Context) (int, int, error) {
	indexed, failed := 0, 0
	for _, entityType := range semanticEntityOrder {
		service.setStatus(ServiceStatus{State: serviceStateIndexing, CurrentEntity: entityType, DocumentsIndexed: indexed, FailedDocuments: failed})
		for {
			if err := ctx.Err(); err != nil {
				return indexed, failed, err
			}
			documents, err := service.database.GetPendingSemanticDocuments(ctx, entityType, service.provider.MaxBatchSize())
			if err != nil {
				return indexed, failed, fmt.Errorf("load pending %s semantic documents: %w", entityType, err)
			}
			if len(documents) == 0 {
				break
			}
			if err := service.embedBatch(ctx, documents); err != nil {
				failed += len(documents)
				for _, document := range documents {
					if markErr := service.database.MarkSemanticDocumentError(ctx, document.ID, err); markErr != nil {
						return indexed, failed, fmt.Errorf("record semantic provider failure: %w", markErr)
					}
				}
				continue
			}
			indexed += len(documents)
		}
	}
	return indexed, failed, nil
}

func (service *Service) embedBatch(ctx context.Context, documents []db.SemanticDocument) error {
	texts := make([]string, len(documents))
	for index, document := range documents {
		texts[index] = document.Content
	}
	vectors, err := service.embedDocumentsWithRetry(ctx, texts)
	if err != nil {
		return err
	}
	expectedDimensions := 0
	if state, stateErr := service.database.GetSemanticIndexState(ctx, documents[0].EntityType); stateErr == nil {
		expectedDimensions = state.Dimensions
	} else {
		return stateErr
	}
	normalized, err := NormalizeEmbeddingBatch(vectors, len(documents), expectedDimensions)
	if err != nil {
		return err
	}
	updates := make([]db.EmbeddingUpdate, len(documents))
	for index, vector := range normalized {
		encoded, encodeErr := EncodeVector(vector)
		if encodeErr != nil {
			return encodeErr
		}
		updates[index] = db.EmbeddingUpdate{
			DocumentID:           documents[index].ID,
			EntityType:           documents[index].EntityType,
			EmbeddingProvider:    service.provider.Name(),
			EmbeddingModel:       service.provider.Model(),
			EmbeddingDimensions:  len(vector),
			EmbeddingInputPrefix: service.provider.DocumentPrefix(),
			Embedding:            encoded,
		}
	}
	return service.database.StoreSemanticEmbeddings(ctx, updates)
}

func (service *Service) embedDocumentsWithRetry(ctx context.Context, texts []string) ([][]float32, error) {
	var lastErr error
	for attempt := 0; attempt < semanticEmbeddingRetryLimit; attempt++ {
		vectors, err := service.provider.EmbedDocuments(ctx, texts)
		if err == nil {
			return vectors, nil
		}
		lastErr = err
		if attempt+1 == semanticEmbeddingRetryLimit {
			break
		}
		delay := semanticRetryBaseDelay << attempt
		jitter := time.Duration(time.Now().UnixNano() % int64(semanticRetryBaseDelay/2))
		timer := time.NewTimer(delay + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, fmt.Errorf("embed semantic documents after %d attempts: %w", semanticEmbeddingRetryLimit, lastErr)
}

func (service *Service) Index(entityType string) VectorIndex {
	service.indexesMu.RLock()
	defer service.indexesMu.RUnlock()
	return service.indexes[entityType]
}

func (service *Service) Status() ServiceStatus {
	service.statusMu.RLock()
	defer service.statusMu.RUnlock()
	return service.status
}

func (service *Service) setStatus(status ServiceStatus) {
	status.UpdatedAt = time.Now().UTC()
	service.statusMu.Lock()
	service.status = status
	service.statusMu.Unlock()
}

// Close cancels a background run, releases the provider, and makes all
// existing arena memory eligible for collection.
func (service *Service) Close() error {
	service.lifecycleMu.Lock()
	if service.closed {
		service.lifecycleMu.Unlock()
		return nil
	}
	service.closed = true
	cancel, done := service.cancel, service.done
	service.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	service.indexesMu.Lock()
	indexes := service.indexes
	service.indexes = map[string]VectorIndex{}
	service.indexesMu.Unlock()
	for _, index := range indexes {
		_ = index.Close()
	}
	return service.provider.Close()
}

var semanticEntityOrder = []string{db.SemanticEntityArtist, db.SemanticEntityAlbum, db.SemanticEntityTrack}
