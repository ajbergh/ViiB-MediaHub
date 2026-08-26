package semantic

import (
	"context"
	"math/rand"
	"runtime"
	"sync"
	"testing"
)

func TestScanIndexUpsertDeleteAndExactSearch(t *testing.T) {
	index := newScanIndex()
	for id, vector := range map[int64][]float32{1: {1, 0}, 2: {0.8, 0.2}, 3: {-1, 0}} {
		if err := index.Upsert(id, vector); err != nil {
			t.Fatal(err)
		}
	}
	if err := index.Upsert(2, []float32{0, 1}); err != nil {
		t.Fatal(err)
	}
	if err := index.Delete(3); err != nil {
		t.Fatal(err)
	}
	if got := index.Len(); got != 2 {
		t.Fatalf("Len = %d, want 2", got)
	}
	matches, err := index.Search([]float32{1, 0}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].ID != 1 || matches[1].ID != 2 {
		t.Fatalf("matches = %#v, deleted/upserted rows were not handled", matches)
	}
	if _, err := index.Search([]float32{1}, 1); err == nil {
		t.Fatal("dimension mismatch did not return an error")
	}
}

func TestScanIndexMatchesNaiveReference(t *testing.T) {
	random := rand.New(rand.NewSource(9))
	index := newScanIndex()
	vectors := make(map[int64][]float32)
	for id := int64(1); id <= 600; id++ {
		vector := randomVector(random, 12)
		vectors[id] = vector
		if err := index.Upsert(id, vector); err != nil {
			t.Fatal(err)
		}
	}
	query := randomVector(random, 12)
	got, err := index.Search(query, 40)
	if err != nil {
		t.Fatal(err)
	}
	want := naiveMatches(t, vectors, query, 40)
	assertMatchesEqual(t, got, want)
}

func TestScanIndexShardedAndSingleThreadedAgree(t *testing.T) {
	previous := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(previous)
	random := rand.New(rand.NewSource(15))
	index := newScanIndex()
	vectors := make(map[int64][]float32, vectorParallelThreshold+1)
	for id := int64(1); id <= vectorParallelThreshold+1; id++ {
		vector := randomVector(random, 3)
		vectors[id] = vector
		if err := index.Upsert(id, vector); err != nil {
			t.Fatal(err)
		}
	}
	query := randomVector(random, 3)
	got, err := index.Search(query, 25)
	if err != nil {
		t.Fatal(err)
	}
	want := naiveMatches(t, vectors, query, 25)
	assertMatchesEqual(t, got, want)
}

func TestScanIndexRebuildAndConcurrentAccess(t *testing.T) {
	items := make([]StoredEmbedding, 0, 64)
	for id := int64(1); id <= 64; id++ {
		blob, err := EncodeVector([]float32{float32(id), 1})
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, StoredEmbedding{ID: id, Embedding: blob})
	}
	index := newScanIndex()
	if err := index.Rebuild(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if index.Len() != 64 || index.Dimensions() != 2 {
		t.Fatalf("rebuilt index = len %d, dimensions %d", index.Len(), index.Dimensions())
	}
	bad := append([]StoredEmbedding(nil), items...)
	bad[3].Embedding = []byte{1}
	if err := index.Rebuild(context.Background(), bad); err == nil {
		t.Fatal("invalid rebuild was accepted")
	}
	if index.Len() != 64 {
		t.Fatal("failed rebuild replaced live arena")
	}

	var group sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		group.Add(1)
		go func(offset int64) {
			defer group.Done()
			for iteration := int64(0); iteration < 100; iteration++ {
				if _, err := index.Search([]float32{1, 0}, 5); err != nil {
					t.Errorf("search: %v", err)
				}
				if err := index.Upsert(1000+offset, []float32{1, float32(iteration)}); err != nil {
					t.Errorf("upsert: %v", err)
				}
			}
		}(int64(worker))
	}
	group.Wait()
}

func randomVector(random *rand.Rand, dimensions int) []float32 {
	vector := make([]float32, dimensions)
	for dimension := range vector {
		vector[dimension] = random.Float32()*2 - 1
	}
	return vector
}

func naiveMatches(t *testing.T, vectors map[int64][]float32, query []float32, limit int) []VectorMatch {
	t.Helper()
	normalizedQuery, err := NormalizeL2(query)
	if err != nil {
		t.Fatal(err)
	}
	matches := make([]scoredVector, 0, len(vectors))
	for id, vector := range vectors {
		normalized, err := NormalizeL2(vector)
		if err != nil {
			t.Fatal(err)
		}
		var score float64
		for dimension := range normalized {
			score += float64(normalized[dimension]) * float64(normalizedQuery[dimension])
		}
		matches = append(matches, scoredVector{id: id, score: score})
	}
	return matchesFromHeap(topNaive(matches, limit))
}

func topNaive(matches []scoredVector, limit int) []scoredVector {
	for index := 0; index < len(matches); index++ {
		for other := index + 1; other < len(matches); other++ {
			if isBetter(matches[other], matches[index]) {
				matches[index], matches[other] = matches[other], matches[index]
			}
		}
	}
	return matches[:limit]
}

func assertMatchesEqual(t *testing.T, got, want []VectorMatch) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("result length = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].ID != want[index].ID || got[index].Similarity != want[index].Similarity {
			t.Fatalf("result %d = %#v, want %#v", index, got[index], want[index])
		}
	}
}
