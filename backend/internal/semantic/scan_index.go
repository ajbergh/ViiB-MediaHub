package semantic

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
)

// scanIndex is an exact cosine index over a contiguous, L2-normalized arena.
// It is intentionally private: callers depend only on VectorIndex.
type scanIndex struct {
	mu      sync.RWMutex
	dims    int
	keys    []int64
	vectors []float32
	live    []bool
	offset  map[int64]int
	free    []int
}

func newScanIndex() *scanIndex { return &scanIndex{offset: make(map[int64]int)} }

func (index *scanIndex) Upsert(id int64, vector []float32) error {
	if id <= 0 {
		return errors.New("vector ID must be positive")
	}
	normalized, err := NormalizeL2(vector)
	if err != nil {
		return err
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.dims != 0 && len(normalized) != index.dims {
		return fmt.Errorf("vector dimensions %d do not match index dimensions %d", len(normalized), index.dims)
	}
	if index.dims == 0 {
		index.dims = len(normalized)
	}
	if row, exists := index.offset[id]; exists {
		copy(index.vectors[row*index.dims:(row+1)*index.dims], normalized)
		return nil
	}
	row := len(index.keys)
	if len(index.free) > 0 {
		row = index.free[len(index.free)-1]
		index.free = index.free[:len(index.free)-1]
		index.keys[row] = id
		index.live[row] = true
		copy(index.vectors[row*index.dims:(row+1)*index.dims], normalized)
	} else {
		index.keys = append(index.keys, id)
		index.vectors = append(index.vectors, normalized...)
		index.live = append(index.live, true)
	}
	index.offset[id] = row
	return nil
}

func (index *scanIndex) Delete(id int64) error {
	index.mu.Lock()
	defer index.mu.Unlock()
	row, exists := index.offset[id]
	if !exists {
		return nil
	}
	delete(index.offset, id)
	index.live[row] = false
	index.free = append(index.free, row)
	return nil
}

func (index *scanIndex) Search(vector []float32, k int) ([]VectorMatch, error) {
	if k <= 0 {
		return []VectorMatch{}, nil
	}
	index.mu.RLock()
	defer index.mu.RUnlock()
	if index.dims == 0 || len(index.offset) == 0 {
		return []VectorMatch{}, nil
	}
	if len(vector) != index.dims {
		return nil, fmt.Errorf("query dimensions %d do not match index dimensions %d", len(vector), index.dims)
	}
	query, err := NormalizeL2(vector)
	if err != nil {
		return nil, err
	}
	if k > len(index.offset) {
		k = len(index.offset)
	}
	if len(index.offset) < vectorParallelThreshold {
		return matchesFromHeap(index.searchRange(query, k, 0, len(index.keys))), nil
	}
	shards := min(runtime.GOMAXPROCS(0), maxVectorScanShards)
	if shards < 2 {
		return matchesFromHeap(index.searchRange(query, k, 0, len(index.keys))), nil
	}
	shards = min(shards, len(index.keys))
	partial := make([][]scoredVector, shards)
	var group sync.WaitGroup
	for shard := 0; shard < shards; shard++ {
		start := shard * len(index.keys) / shards
		end := (shard + 1) * len(index.keys) / shards
		group.Add(1)
		go func(slot, from, to int) {
			defer group.Done()
			partial[slot] = index.searchRange(query, k, from, to)
		}(shard, start, end)
	}
	group.Wait()
	merged := &scoredMinHeap{}
	heap.Init(merged)
	for _, matches := range partial {
		for _, match := range matches {
			pushTopK(merged, match, k)
		}
	}
	return matchesFromHeap(*merged), nil
}

func (index *scanIndex) searchRange(query []float32, k, start, end int) []scoredVector {
	matches := &scoredMinHeap{}
	heap.Init(matches)
	for row := start; row < end; row++ {
		if !index.live[row] {
			continue
		}
		base := row * index.dims
		var score float64
		for dimension, value := range query {
			score += float64(index.vectors[base+dimension]) * float64(value)
		}
		pushTopK(matches, scoredVector{id: index.keys[row], score: score}, k)
	}
	return append([]scoredVector(nil), (*matches)...)
}

func (index *scanIndex) Len() int {
	index.mu.RLock()
	defer index.mu.RUnlock()
	return len(index.offset)
}

func (index *scanIndex) Dimensions() int {
	index.mu.RLock()
	defer index.mu.RUnlock()
	return index.dims
}

// Rebuild validates every ready blob before replacing the arena. A failure
// leaves the currently searchable arena untouched.
func (index *scanIndex) Rebuild(ctx context.Context, items []StoredEmbedding) error {
	replacement := newScanIndex()
	if len(items) == 0 {
		index.mu.Lock()
		index.dims, index.keys, index.vectors, index.live, index.offset, index.free = 0, nil, nil, nil, make(map[int64]int), nil
		index.mu.Unlock()
		return nil
	}
	first, err := DecodeVector(items[0].Embedding, 0)
	if err != nil {
		return fmt.Errorf("decode first embedding: %w", err)
	}
	if items[0].ID <= 0 {
		return errors.New("stored embedding ID must be positive")
	}
	replacement.dims = len(first)
	replacement.keys = make([]int64, 0, len(items))
	replacement.vectors = make([]float32, 0, len(items)*replacement.dims)
	replacement.live = make([]bool, 0, len(items))
	if err := replacement.addNormalized(items[0].ID, first); err != nil {
		return err
	}
	for itemIndex := 1; itemIndex < len(items); itemIndex++ {
		if itemIndex%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		item := items[itemIndex]
		if item.ID <= 0 {
			return errors.New("stored embedding ID must be positive")
		}
		vector, err := DecodeVector(item.Embedding, replacement.dims)
		if err != nil {
			return fmt.Errorf("decode embedding %d: %w", item.ID, err)
		}
		if err := replacement.addNormalized(item.ID, vector); err != nil {
			return err
		}
	}
	index.mu.Lock()
	index.dims = replacement.dims
	index.keys = replacement.keys
	index.vectors = replacement.vectors
	index.live = replacement.live
	index.offset = replacement.offset
	index.free = replacement.free
	index.mu.Unlock()
	return nil
}

func (index *scanIndex) addNormalized(id int64, vector []float32) error {
	if len(vector) != index.dims {
		return fmt.Errorf("vector dimensions %d do not match index dimensions %d", len(vector), index.dims)
	}
	if row, exists := index.offset[id]; exists {
		copy(index.vectors[row*index.dims:(row+1)*index.dims], vector)
		return nil
	}
	row := len(index.keys)
	index.keys = append(index.keys, id)
	index.vectors = append(index.vectors, vector...)
	index.live = append(index.live, true)
	index.offset[id] = row
	return nil
}

func (index *scanIndex) Close() error {
	index.mu.Lock()
	defer index.mu.Unlock()
	index.dims = 0
	index.keys, index.vectors, index.live, index.free = nil, nil, nil, nil
	index.offset = make(map[int64]int)
	return nil
}

type scoredVector struct {
	id    int64
	score float64
}

// scoredMinHeap keeps the worst retained candidate at index zero. A tie with a
// larger ID is worse, yielding deterministic exact result order.
type scoredMinHeap []scoredVector

func (heap scoredMinHeap) Len() int { return len(heap) }
func (heap scoredMinHeap) Less(i, j int) bool {
	if heap[i].score == heap[j].score {
		return heap[i].id > heap[j].id
	}
	return heap[i].score < heap[j].score
}
func (heap scoredMinHeap) Swap(i, j int)   { heap[i], heap[j] = heap[j], heap[i] }
func (heap *scoredMinHeap) Push(value any) { *heap = append(*heap, value.(scoredVector)) }
func (heap *scoredMinHeap) Pop() any {
	old := *heap
	last := old[len(old)-1]
	*heap = old[:len(old)-1]
	return last
}

func isBetter(candidate, current scoredVector) bool {
	return candidate.score > current.score || (candidate.score == current.score && candidate.id < current.id)
}

func pushTopK(matches *scoredMinHeap, candidate scoredVector, k int) {
	if matches.Len() < k {
		heap.Push(matches, candidate)
		return
	}
	if isBetter(candidate, (*matches)[0]) {
		(*matches)[0] = candidate
		heap.Fix(matches, 0)
	}
}

func matchesFromHeap(matches []scoredVector) []VectorMatch {
	sort.Slice(matches, func(i, j int) bool { return isBetter(matches[i], matches[j]) })
	result := make([]VectorMatch, len(matches))
	for index, match := range matches {
		result[index] = VectorMatch{ID: match.id, Similarity: math.Max(0, math.Min(1, match.score))}
	}
	return result
}

var _ VectorIndex = (*scanIndex)(nil)
