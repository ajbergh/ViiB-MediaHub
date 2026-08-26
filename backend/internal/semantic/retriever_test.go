package semantic

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestSearchSemanticDocumentsUsesOneCachedQueryEmbeddingAndPreservesEntitySets(t *testing.T) {
	database := newServiceTestDB(t)
	for _, song := range []db.Song{
		{ID: "first", Title: "First", Artist: "Artist One", Album: "Album One", FilePath: filepath.Join(t.TempDir(), "first.flac"), AddedAt: 1},
		{ID: "second", Title: "Second", Artist: "Artist Two", Album: "Album Two", FilePath: filepath.Join(t.TempDir(), "second.flac"), AddedAt: 1},
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
	result, err := service.SearchSemanticDocuments(context.Background(), "atmospheric drive")
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity.Provider != "fake" || len(result.Tracks) != 2 || len(result.Albums) != 2 || len(result.Artists) != 2 || result.Tracks[0].Document.SongID != "first" {
		t.Fatalf("result=%#v", result)
	}
	if _, err := service.SearchSemanticDocuments(context.Background(), "atmospheric drive"); err != nil {
		t.Fatal(err)
	}
	if provider.queryCallCount() != 1 {
		t.Fatalf("query calls=%d", provider.queryCallCount())
	}
	retrieval, err := service.RetrieveSemanticCandidates(context.Background(), "atmospheric drive", SemanticRetrievalOptions{Source: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if len(retrieval.Candidates) != 2 || retrieval.Candidates[0].Evidence.TrackSimilarity == 0 || retrieval.Candidates[0].Evidence.BestSimilarity < retrieval.Candidates[0].Evidence.TrackSimilarity {
		t.Fatalf("retrieval=%#v", retrieval)
	}
	if provider.queryCallCount() != 1 {
		t.Fatalf("cached retrieval query calls=%d", provider.queryCallCount())
	}
	filtered, err := service.RetrieveSemanticCandidates(context.Background(), "atmospheric drive", SemanticRetrievalOptions{Source: "all", ExcludeArtists: []string{"Artist One"}})
	if err != nil || len(filtered.Candidates) != 1 || filtered.Candidates[0].Song.ID != "second" {
		t.Fatalf("filtered=%#v err=%v", filtered, err)
	}
}

func TestSearchSemanticDocumentsFallsBackWithoutReadyIndexes(t *testing.T) {
	service, err := NewService(newServiceTestDB(t), &fakeEmbeddingProvider{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if _, err := service.SearchSemanticDocuments(context.Background(), "anything"); !errors.Is(err, ErrNoSearchableSemanticIndex) {
		t.Fatalf("err=%v", err)
	}
}
