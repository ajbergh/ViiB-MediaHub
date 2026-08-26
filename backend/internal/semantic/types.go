// Package semantic contains the pure-Go semantic retrieval primitives. It has
// no provider or HTTP dependency in the foundation phase.
package semantic

import (
	"context"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const (
	// SemanticDocumentVersion is part of every document hash. Increment it when
	// the deterministic document template changes in a material way.
	SemanticDocumentVersion = 1

	vectorParallelThreshold = 25000
	maxVectorScanShards     = 8
)

type Document = db.SemanticDocument
type StoredEmbedding = db.StoredEmbedding

// VectorIndex is deliberately provider- and storage-independent so an
// approximate implementation can replace the exact scan only when measured
// corpus size or latency requires it.
type VectorIndex interface {
	Upsert(id int64, vector []float32) error
	Delete(id int64) error
	Search(vector []float32, k int) ([]VectorMatch, error)
	Len() int
	Dimensions() int
	Rebuild(ctx context.Context, items []StoredEmbedding) error
	Close() error
}

// VectorMatch is sorted by descending similarity, then ascending document ID.
// Similarity is clamped to [0, 1] before it leaves the index.
type VectorMatch struct {
	ID         int64
	Similarity float64
}

// ArtistContext contains optional enrichment fetched in bulk by a later
// indexer. The builder is still useful with an empty context map.
type ArtistContext struct {
	Tags           []string
	Bio            string
	SimilarArtists []string
	ActiveYears    string
}

// AlbumContext contains optional album-level enrichment fetched in bulk.
type AlbumContext struct {
	Tags []string
}

// DocumentContext holds lookup maps keyed by CanonicalArtistKey and
// CanonicalAlbumKey. It avoids document generation doing per-song database
// reads or per-song Last.fm parsing.
type DocumentContext struct {
	Artists map[string]ArtistContext
	Albums  map[string]AlbumContext
}
