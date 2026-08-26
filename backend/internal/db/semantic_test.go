package db

import (
	"context"
	"encoding/binary"
	"math"
	"path/filepath"
	"testing"
)

func TestEnsureSemanticSchemaPreservesCatalogAndInitializesState(t *testing.T) {
	database := newSemanticTestDB(t)
	if err := database.SaveSong(&Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "song.flac"), AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureSemanticSchema(); err != nil {
		t.Fatal(err)
	}
	if song, err := database.GetSongByID("song"); err != nil || song == nil || song.Title != "Song" {
		t.Fatalf("catalog data changed while installing schema: song=%#v err=%v", song, err)
	}
	stats, err := database.GetSemanticIndexStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.State) != 3 {
		t.Fatalf("state rows = %d, want one per entity type", len(stats.State))
	}
}

func TestSemanticDocumentEmbeddingLifecycle(t *testing.T) {
	database := newSemanticTestDB(t)
	ctx := context.Background()
	doc := SemanticDocument{EntityType: SemanticEntityTrack, EntityKey: "song", DisplayName: "Song", SongID: "song", Artist: "Artist", Album: "Album", Content: "Track: Song.", ContentHash: "hash-one", DocumentVersion: 1}
	if err := database.UpsertSemanticDocuments(ctx, []SemanticDocument{doc}); err != nil {
		t.Fatal(err)
	}
	pending, err := database.GetPendingSemanticDocuments(ctx, SemanticEntityTrack, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending documents = %#v, err=%v", pending, err)
	}
	if err := database.StoreSemanticEmbeddings(ctx, []EmbeddingUpdate{{DocumentID: pending[0].ID, EntityType: SemanticEntityTrack, EmbeddingProvider: "test", EmbeddingModel: "unit", EmbeddingDimensions: 2, Embedding: normalizedBlob(t, []float32{3, 4})}}); err != nil {
		t.Fatal(err)
	}
	state, err := database.GetSemanticIndexState(ctx, SemanticEntityTrack)
	if err != nil {
		t.Fatal(err)
	}
	if state.ItemCount != 1 || state.Dimensions != 2 || state.DocumentRevision != 1 {
		t.Fatalf("ready state = %#v", state)
	}
	if err := database.UpsertSemanticDocuments(ctx, []SemanticDocument{doc}); err != nil {
		t.Fatal(err)
	}
	stateAfterSame, err := database.GetSemanticIndexState(ctx, SemanticEntityTrack)
	if err != nil {
		t.Fatal(err)
	}
	if stateAfterSame.DocumentRevision != state.DocumentRevision {
		t.Fatalf("unchanged content bumped revision: %d -> %d", state.DocumentRevision, stateAfterSame.DocumentRevision)
	}
	doc.Content = "Track: Renamed Song."
	doc.ContentHash = "hash-two"
	if err := database.UpsertSemanticDocuments(ctx, []SemanticDocument{doc}); err != nil {
		t.Fatal(err)
	}
	pending, err = database.GetPendingSemanticDocuments(ctx, SemanticEntityTrack, 10)
	if err != nil || len(pending) != 1 || pending[0].Embedding != nil {
		t.Fatalf("changed document was not reset to pending: %#v, err=%v", pending, err)
	}
	stateAfterChange, err := database.GetSemanticIndexState(ctx, SemanticEntityTrack)
	if err != nil {
		t.Fatal(err)
	}
	if stateAfterChange.DocumentRevision != state.DocumentRevision+1 || stateAfterChange.ItemCount != 0 {
		t.Fatalf("ready invalidation state = %#v, want revision %d and zero ready items", stateAfterChange, state.DocumentRevision+1)
	}
}

func TestSemanticDocumentOperationsChunkAndRemoveOrphans(t *testing.T) {
	database := newSemanticTestDB(t)
	ctx := context.Background()
	if err := database.SaveSong(&Song{ID: "live", Title: "Live", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "live.flac"), AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	docs := make([]SemanticDocument, 0, 902)
	for index := 0; index < 902; index++ {
		entityType, songID := SemanticEntityArtist, ""
		if index == 0 {
			entityType, songID = SemanticEntityTrack, "live"
		}
		docs = append(docs, SemanticDocument{EntityType: entityType, EntityKey: "key-" + itoa(index), DisplayName: "Document", SongID: songID, Content: "Content", ContentHash: "hash-" + itoa(index), DocumentVersion: 1})
	}
	if err := database.UpsertSemanticDocuments(ctx, docs); err != nil {
		t.Fatal(err)
	}
	pending, err := database.GetPendingSemanticDocuments(ctx, SemanticEntityArtist, 200)
	if err != nil || len(pending) != 200 {
		t.Fatalf("bounded pending batch = %d, err=%v", len(pending), err)
	}
	ids := make([]int64, 0, len(pending))
	for _, doc := range pending {
		ids = append(ids, doc.ID)
	}
	loaded, err := database.GetSemanticDocumentsByIDs(ctx, ids)
	if err != nil || len(loaded) != len(ids) {
		t.Fatalf("loaded documents = %d, want %d, err=%v", len(loaded), len(ids), err)
	}
	track, err := database.GetPendingSemanticDocuments(ctx, SemanticEntityTrack, 1)
	if err != nil || len(track) != 1 {
		t.Fatalf("track pending = %#v, err=%v", track, err)
	}
	if err := database.StoreSemanticEmbeddings(ctx, []EmbeddingUpdate{{DocumentID: track[0].ID, EntityType: SemanticEntityTrack, EmbeddingProvider: "test", EmbeddingModel: "unit", EmbeddingDimensions: 2, Embedding: normalizedBlob(t, []float32{1, 0})}}); err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteSong("live"); err != nil {
		t.Fatal(err)
	}
	deleted, err := database.DeleteSemanticDocumentsForMissingSongs(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("deleted orphan documents = %d, err=%v", deleted, err)
	}
	ready, err := database.ListReadySemanticEmbeddings(ctx, SemanticEntityTrack)
	if err != nil || len(ready) != 0 {
		t.Fatalf("orphaned ready rows = %#v, err=%v", ready, err)
	}
}

func newSemanticTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := New(filepath.Join(t.TempDir(), "semantic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func normalizedBlob(t *testing.T, vector []float32) []byte {
	t.Helper()
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	if sum == 0 {
		t.Fatal("test vector has zero magnitude")
	}
	encoded := make([]byte, len(vector)*4)
	magnitude := math.Sqrt(sum)
	for index, value := range vector {
		binary.LittleEndian.PutUint32(encoded[index*4:], math.Float32bits(float32(float64(value)/magnitude)))
	}
	return encoded
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	return result
}
