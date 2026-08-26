package semantic

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestServiceIndexesInDocumentOrderAndSkipsUnchangedContent(t *testing.T) {
	database := newServiceTestDB(t)
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

type fakeEmbeddingProvider struct {
	mu        sync.Mutex
	calls     int
	failCalls int
	model     string
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
	vectors := make([][]float32, len(texts))
	for index := range texts {
		vectors[index] = []float32{float32(index + 1), 1}
	}
	return vectors, nil
}

func (provider *fakeEmbeddingProvider) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return []float32{1, 1}, nil
}

func (provider *fakeEmbeddingProvider) callCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
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
