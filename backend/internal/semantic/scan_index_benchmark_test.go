package semantic

import (
	"math/rand"
	"testing"
)

// BenchmarkScanIndexExactSearch20K is the reproducible 20k-track fixture for
// the Phase 1 exact-scan budget. Setup stays outside the timed region so the
// result represents retrieval after query embedding and arena load.
func BenchmarkScanIndexExactSearch20K(b *testing.B) {
	const (
		tracks     = 20_000
		dimensions = 512
		limit      = 300
	)
	random := rand.New(rand.NewSource(20_000))
	index := newScanIndex()
	for id := int64(1); id <= tracks; id++ {
		if err := index.Upsert(id, randomVector(random, dimensions)); err != nil {
			b.Fatal(err)
		}
	}
	query := randomVector(random, dimensions)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		matches, err := index.Search(query, limit)
		if err != nil {
			b.Fatal(err)
		}
		if len(matches) != limit {
			b.Fatalf("matches=%d, want %d", len(matches), limit)
		}
	}
}
