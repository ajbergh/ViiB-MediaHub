package semantic

import "testing"

func TestQueryEmbeddingCacheIsBoundedLRUAndCopiesVectors(t *testing.T) {
	cache := newQueryEmbeddingCache(2)
	vector := []float32{1, 2}
	cache.Put("first", vector)
	vector[0] = 99
	loaded, ok := cache.Get("first")
	if !ok || loaded[0] != 1 {
		t.Fatalf("loaded=%v ok=%v", loaded, ok)
	}
	loaded[1] = 77
	cache.Put("second", []float32{2})
	cache.Get("first")
	cache.Put("third", []float32{3})
	if _, ok := cache.Get("second"); ok {
		t.Fatal("least recently used entry remained")
	}
	loaded, ok = cache.Get("first")
	if !ok || loaded[1] != 2 {
		t.Fatalf("cache returned aliased vector: %v", loaded)
	}
}
