package semantic

import (
	"container/list"
	"sync"
)

const semanticQueryCacheCapacity = 128

type queryEmbeddingCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[string]*list.Element
	order    *list.List
}

type queryEmbeddingCacheEntry struct {
	key    string
	vector []float32
}

func newQueryEmbeddingCache(capacity int) *queryEmbeddingCache {
	if capacity <= 0 {
		capacity = semanticQueryCacheCapacity
	}
	return &queryEmbeddingCache{capacity: capacity, entries: make(map[string]*list.Element), order: list.New()}
}

func (cache *queryEmbeddingCache) Get(key string) ([]float32, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element, exists := cache.entries[key]
	if !exists {
		return nil, false
	}
	cache.order.MoveToFront(element)
	entry := element.Value.(queryEmbeddingCacheEntry)
	return append([]float32(nil), entry.vector...), true
}

func (cache *queryEmbeddingCache) Put(key string, vector []float32) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	copyVector := append([]float32(nil), vector...)
	if element, exists := cache.entries[key]; exists {
		element.Value = queryEmbeddingCacheEntry{key: key, vector: copyVector}
		cache.order.MoveToFront(element)
		return
	}
	element := cache.order.PushFront(queryEmbeddingCacheEntry{key: key, vector: copyVector})
	cache.entries[key] = element
	if cache.order.Len() <= cache.capacity {
		return
	}
	oldest := cache.order.Back()
	entry := oldest.Value.(queryEmbeddingCacheEntry)
	delete(cache.entries, entry.key)
	cache.order.Remove(oldest)
}
