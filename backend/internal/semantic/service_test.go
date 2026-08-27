package semantic

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestServiceIndexesExistingLibraryInDocumentOrderAndSkipsUnchangedContent(t *testing.T) {
	database := newServiceTestDB(t)
	// Populate the normal library before the semantic service is constructed to
	// cover adding Phase 1 to an existing user database with no semantic rows.
	for _, song := range []db.Song{
		{ID: "song-a", Title: "A", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "a.flac"), AddedAt: 1},
		{ID: "song-b", Title: "B", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "b.flac"), AddedAt: 1},
	} {
		if err := database.SaveSong(&song); err != nil {
			t.Fatal(err)
		}
	}
	provider := &fakeEmbeddingProvider{}
	service, err := NewService(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.callCount() != 3 {
		t.Fatalf("provider calls=%d, want artist, album, track batches", provider.callCount())
	}
	if service.Index(db.SemanticEntityArtist).Len() != 1 || service.Index(db.SemanticEntityAlbum).Len() != 1 || service.Index(db.SemanticEntityTrack).Len() != 2 {
		t.Fatalf("index lengths artist=%d album=%d track=%d", service.Index(db.SemanticEntityArtist).Len(), service.Index(db.SemanticEntityAlbum).Len(), service.Index(db.SemanticEntityTrack).Len())
	}
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.callCount() != 3 {
		t.Fatalf("unchanged documents made %d provider calls", provider.callCount())
	}
	if got := service.Status(); got.State != serviceStateReady || got.DocumentsIndexed != 0 {
		t.Fatalf("status=%#v", got)
	}
}

func TestServiceRetriesThenRecordsAndRecoversProviderFailures(t *testing.T) {
	database := newServiceTestDB(t)
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "song.flac"), AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	provider := &fakeEmbeddingProvider{failCalls: semanticEmbeddingRetryLimit * 3}
	service, err := NewService(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := service.Status(); got.State != serviceStateError || got.FailedDocuments != 3 {
		t.Fatalf("status=%#v", got)
	}
	if retried, err := service.RetryErrors(context.Background()); err != nil || retried != 3 {
		t.Fatalf("retried=%d err=%v", retried, err)
	}
	if got := service.Status(); got.State != serviceStateReady || service.Index(db.SemanticEntityTrack).Len() != 1 {
		t.Fatalf("status=%#v track=%d", got, service.Index(db.SemanticEntityTrack).Len())
	}
}

func TestServiceRecoversFromZeroMagnitudeProviderVectors(t *testing.T) {
	database := newServiceTestDB(t)
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "song.flac"), AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	provider := &fakeEmbeddingProvider{zeroVectors: true}
	service, err := NewService(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := service.Status(); status.State != serviceStateError || status.FailedDocuments != 3 {
		t.Fatalf("status=%#v", status)
	}
	stats, err := database.GetSemanticIndexStats(context.Background())
	if err != nil || stats.DocumentsByStatus["error"] != 3 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
	if service.Index(db.SemanticEntityTrack).Len() != 0 {
		t.Fatal("zero-magnitude vectors entered the track index")
	}

	provider.zeroVectors = false
	if retried, err := service.RetryErrors(context.Background()); err != nil || retried != 3 {
		t.Fatalf("retried=%d err=%v", retried, err)
	}
	if status := service.Status(); status.State != serviceStateReady || service.Index(db.SemanticEntityTrack).Len() != 1 {
		t.Fatalf("status=%#v track=%d", status, service.Index(db.SemanticEntityTrack).Len())
	}
}

func TestServiceRecoversFromProviderDimensionMismatchWithoutMixingArenas(t *testing.T) {
	database := newServiceTestDB(t)
	song := db.Song{ID: "song", Title: "Original", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "song.flac"), AddedAt: 1}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	provider := &fakeEmbeddingProvider{}
	service, err := NewService(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}

	song.Title = "Renamed"
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	songs, err := database.GetAllSongs()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.reconcileCatalogDocuments(context.Background(), songs, true); err != nil {
		t.Fatal(err)
	}
	provider.dimensions = 3
	indexed, failed, err := service.processPending(context.Background(), true)
	if err != nil || indexed != 0 || failed == 0 {
		t.Fatalf("indexed=%d failed=%d err=%v", indexed, failed, err)
	}
	trackState, err := database.GetSemanticIndexState(context.Background(), db.SemanticEntityTrack)
	if err != nil || trackState.Dimensions != 0 || trackState.ItemCount != 0 {
		t.Fatalf("track state=%#v err=%v", trackState, err)
	}
	artistState, err := database.GetSemanticIndexState(context.Background(), db.SemanticEntityArtist)
	if err != nil || artistState.Dimensions != 2 {
		t.Fatalf("artist state=%#v err=%v", artistState, err)
	}
	if service.Index(db.SemanticEntityTrack).Len() != 0 {
		t.Fatal("mismatched vectors entered the track index")
	}

	provider.dimensions = 2
	if retried, err := service.RetryErrors(context.Background()); err != nil || retried != failed {
		t.Fatalf("retried=%d failed=%d err=%v", retried, failed, err)
	}
	if status := service.Status(); status.State != serviceStateReady || service.Index(db.SemanticEntityTrack).Len() != 1 {
		t.Fatalf("status=%#v track=%d", status, service.Index(db.SemanticEntityTrack).Len())
	}
}

func TestServiceCloseCancelsBackgroundIndexingAndPreventsRestart(t *testing.T) {
	database := newServiceTestDB(t)
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "song.flac"), AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	provider := newBlockingEmbeddingProvider()
	service, err := NewService(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("background indexing did not reach the provider")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- service.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("service close did not cancel background indexing")
	}
	select {
	case <-provider.closed:
	default:
		t.Fatal("provider was not closed")
	}
	if service.Index(db.SemanticEntityTrack) != nil {
		t.Fatal("service close retained track arena")
	}
	if err := service.Start(context.Background()); err == nil {
		t.Fatal("closed service restarted")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServiceResetsWhenProviderIdentityChanges(t *testing.T) {
	database := newServiceTestDB(t)
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "song.flac"), AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	first := &fakeEmbeddingProvider{model: "one"}
	service, err := NewService(database, first)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = service.Close()
	second := &fakeEmbeddingProvider{model: "two"}
	service, err = NewService(database, second)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if second.callCount() != 3 {
		t.Fatalf("new identity calls=%d, want full reindex", second.callCount())
	}
}

func TestServiceTailsLibraryChangesWithoutReembeddingBehavior(t *testing.T) {
	database := newServiceTestDB(t)
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		t.Fatal(err)
	}
	song := db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "song.flac"), AddedAt: 1}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	provider := &fakeEmbeddingProvider{}
	service, err := NewService(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.callCount() != 3 {
		t.Fatalf("initial calls=%d", provider.callCount())
	}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	if err := service.SyncChanges(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.callCount() != 3 {
		t.Fatalf("behavior-only change re-embedded documents: calls=%d", provider.callCount())
	}
	state, err := database.GetSemanticIndexState(context.Background(), db.SemanticEntityTrack)
	if err != nil || state.CatalogCursor != 2 {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	if err := database.DeleteSong(song.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.SyncChanges(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.Index(db.SemanticEntityTrack).Len() != 0 || service.Index(db.SemanticEntityAlbum).Len() != 0 || service.Index(db.SemanticEntityArtist).Len() != 0 {
		t.Fatalf("deleted catalog remains in indexes: track=%d album=%d artist=%d", service.Index(db.SemanticEntityTrack).Len(), service.Index(db.SemanticEntityAlbum).Len(), service.Index(db.SemanticEntityArtist).Len())
	}
}

func TestServiceSyncUpdatesOnlyInvalidatedArenaEntries(t *testing.T) {
	database := newServiceTestDB(t)
	song := db.Song{ID: "song", Title: "Original", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "song.flac"), AddedAt: 1}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	provider := &fakeEmbeddingProvider{}
	service, err := NewService(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}

	song.Title = "Renamed"
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	songs, err := database.GetAllSongs()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.reconcileCatalogDocuments(context.Background(), songs, true); err != nil {
		t.Fatal(err)
	}
	if service.Index(db.SemanticEntityTrack).Len() != 0 || service.Index(db.SemanticEntityAlbum).Len() != 0 || service.Index(db.SemanticEntityArtist).Len() != 1 {
		t.Fatalf("only the content-hash-invalidated track and album should leave their arenas: track=%d album=%d artist=%d", service.Index(db.SemanticEntityTrack).Len(), service.Index(db.SemanticEntityAlbum).Len(), service.Index(db.SemanticEntityArtist).Len())
	}
	if indexed, failed, err := service.processPending(context.Background(), true); err != nil || indexed != 2 || failed != 0 {
		t.Fatalf("incremental pending processing indexed=%d failed=%d err=%v", indexed, failed, err)
	}
	if service.Index(db.SemanticEntityTrack).Len() != 1 || service.Index(db.SemanticEntityAlbum).Len() != 1 || service.Index(db.SemanticEntityArtist).Len() != 1 {
		t.Fatalf("replacement embeddings were not incrementally restored: track=%d album=%d artist=%d", service.Index(db.SemanticEntityTrack).Len(), service.Index(db.SemanticEntityAlbum).Len(), service.Index(db.SemanticEntityArtist).Len())
	}
}

func TestServiceRefreshesDocumentsAfterMetadataChanges(t *testing.T) {
	database := newServiceTestDB(t)
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		t.Fatal(err)
	}
	song := db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "song.flac"), AddedAt: 1}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	provider := &fakeEmbeddingProvider{}
	service, err := NewService(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateArtistLastFM("Artist", db.LastFMArtistUpdate{Tags: []string{"dream pop"}, Bio: "A detailed and atmospheric artist biography."}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveAlbumMetadata(&db.AlbumMetadata{AlbumKey: "Album::Artist", AlbumName: "Album", ArtistName: "Artist", Genre: "shoegaze, dream pop"}); err != nil {
		t.Fatal(err)
	}
	changes, err := database.GetSemanticMetadataChanges(context.Background(), 10)
	if err != nil || len(changes) != 2 {
		t.Fatalf("metadata changes=%#v err=%v", changes, err)
	}
	if err := service.SyncChanges(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.callCount() != 6 {
		t.Fatalf("metadata refresh calls=%d, want one refreshed artist, album, and track batch", provider.callCount())
	}
	changes, err = database.GetSemanticMetadataChanges(context.Background(), 10)
	if err != nil || len(changes) != 0 {
		t.Fatalf("metadata changes after sync=%#v err=%v", changes, err)
	}
}

type fakeEmbeddingProvider struct {
	mu          sync.Mutex
	calls       int
	queryCalls  int
	failCalls   int
	model       string
	dimensions  int
	zeroVectors bool
}

func (provider *fakeEmbeddingProvider) Name() string { return "fake" }
func (provider *fakeEmbeddingProvider) Model() string {
	if provider.model == "" {
		return "test"
	}
	return provider.model
}
func (provider *fakeEmbeddingProvider) DocumentPrefix() string { return "fake_document: " }
func (provider *fakeEmbeddingProvider) QueryPrefix() string    { return "fake_query: " }
func (provider *fakeEmbeddingProvider) MaxBatchSize() int      { return 32 }
func (provider *fakeEmbeddingProvider) Close() error           { return nil }

func (provider *fakeEmbeddingProvider) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.calls++
	if provider.calls <= provider.failCalls {
		return nil, errors.New("temporary provider failure")
	}
	dimensions := provider.dimensions
	if dimensions == 0 {
		dimensions = 2
	}
	vectors := make([][]float32, len(texts))
	for index := range texts {
		vectors[index] = make([]float32, dimensions)
		if !provider.zeroVectors {
			vectors[index][0] = float32(index + 1)
			if dimensions > 1 {
				vectors[index][1] = 1
			}
		}
	}
	return vectors, nil
}

func (provider *fakeEmbeddingProvider) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	provider.mu.Lock()
	provider.queryCalls++
	provider.mu.Unlock()
	return []float32{1, 1}, nil
}

func (provider *fakeEmbeddingProvider) callCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

func (provider *fakeEmbeddingProvider) queryCallCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.queryCalls
}

type blockingEmbeddingProvider struct {
	started   chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

func newBlockingEmbeddingProvider() *blockingEmbeddingProvider {
	return &blockingEmbeddingProvider{started: make(chan struct{}, 1), closed: make(chan struct{})}
}

func (provider *blockingEmbeddingProvider) Name() string           { return "blocking" }
func (provider *blockingEmbeddingProvider) Model() string          { return "test" }
func (provider *blockingEmbeddingProvider) DocumentPrefix() string { return "" }
func (provider *blockingEmbeddingProvider) QueryPrefix() string    { return "" }
func (provider *blockingEmbeddingProvider) MaxBatchSize() int      { return 32 }
func (provider *blockingEmbeddingProvider) Close() error {
	provider.closeOnce.Do(func() { close(provider.closed) })
	return nil
}
func (provider *blockingEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	select {
	case provider.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}
func (provider *blockingEmbeddingProvider) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return []float32{1, 1}, nil
}

func newServiceTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
