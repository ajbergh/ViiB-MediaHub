package semantic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const (
	semanticTrackSearchLimit  = 300
	semanticAlbumSearchLimit  = 40
	semanticArtistSearchLimit = 30
)

// ErrNoSearchableSemanticIndex tells callers to use their existing fallback
// path without issuing a query-embedding request against an empty index.
var ErrNoSearchableSemanticIndex = errors.New("semantic index has no ready documents")

// SemanticDocumentMatch is an internal retrieval result. It includes no API
// serialization tags so raw document text cannot accidentally surface in the
// normal Smart Playlist response.
type SemanticDocumentMatch struct {
	Document   db.SemanticDocument
	Similarity float64
}

// SemanticSearchResult preserves entity-specific matches so a later expansion
// step can distinguish direct track evidence from album/artist rescue evidence.
type SemanticSearchResult struct {
	Query    string
	Identity EmbeddingIdentity
	Tracks   []SemanticDocumentMatch
	Albums   []SemanticDocumentMatch
	Artists  []SemanticDocumentMatch
}

// SearchSemanticDocuments embeds one query once, then exact-searches each
// active arena at the named Phase 1 quotas. It never scans SQLite vectors.
func (service *Service) SearchSemanticDocuments(ctx context.Context, query string) (SemanticSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SemanticSearchResult{}, errors.New("semantic query is required")
	}
	indexes, dimensions, hasReadyIndex, err := service.searchableIndexes()
	if err != nil {
		return SemanticSearchResult{}, err
	}
	if !hasReadyIndex {
		return SemanticSearchResult{}, ErrNoSearchableSemanticIndex
	}
	vector, identity, err := service.embedQuery(ctx, query, dimensions)
	if err != nil {
		return SemanticSearchResult{}, err
	}
	result := SemanticSearchResult{Query: query, Identity: identity}
	if result.Tracks, err = service.searchDocuments(ctx, indexes[db.SemanticEntityTrack], db.SemanticEntityTrack, vector, semanticTrackSearchLimit); err != nil {
		return SemanticSearchResult{}, err
	}
	if result.Albums, err = service.searchDocuments(ctx, indexes[db.SemanticEntityAlbum], db.SemanticEntityAlbum, vector, semanticAlbumSearchLimit); err != nil {
		return SemanticSearchResult{}, err
	}
	if result.Artists, err = service.searchDocuments(ctx, indexes[db.SemanticEntityArtist], db.SemanticEntityArtist, vector, semanticArtistSearchLimit); err != nil {
		return SemanticSearchResult{}, err
	}
	return result, nil
}

func (service *Service) searchableIndexes() (map[string]VectorIndex, int, bool, error) {
	service.indexesMu.RLock()
	defer service.indexesMu.RUnlock()
	indexes := make(map[string]VectorIndex, len(semanticEntityOrder))
	dimensions := 0
	hasReadyIndex := false
	for _, entityType := range semanticEntityOrder {
		index := service.indexes[entityType]
		if index == nil || index.Len() == 0 {
			continue
		}
		indexDimensions := index.Dimensions()
		if indexDimensions <= 0 {
			return nil, 0, false, fmt.Errorf("semantic %s index has invalid dimensions", entityType)
		}
		if dimensions != 0 && dimensions != indexDimensions {
			return nil, 0, false, fmt.Errorf("semantic indexes disagree on dimensions: %d and %d", dimensions, indexDimensions)
		}
		dimensions = indexDimensions
		indexes[entityType] = index
		hasReadyIndex = true
	}
	return indexes, dimensions, hasReadyIndex, nil
}

func (service *Service) embedQuery(ctx context.Context, query string, expectedDimensions int) ([]float32, EmbeddingIdentity, error) {
	identity := EmbeddingIdentity{
		Provider:       service.provider.Name(),
		Model:          service.provider.Model(),
		Dimensions:     expectedDimensions,
		DocumentPrefix: service.provider.DocumentPrefix(),
		QueryPrefix:    service.provider.QueryPrefix(),
	}
	if err := identity.Valid(); err != nil {
		return nil, EmbeddingIdentity{}, err
	}
	cacheKey := strings.Join([]string{identity.Provider, identity.Model, identity.DocumentPrefix, identity.QueryPrefix, identity.QueryPrefix + query}, "\x00")
	if vector, exists := service.queryCache.Get(cacheKey); exists {
		return vector, identity, nil
	}
	vector, err := service.provider.EmbedQuery(ctx, query)
	if err != nil {
		return nil, EmbeddingIdentity{}, fmt.Errorf("embed semantic query: %w", err)
	}
	normalized, err := NormalizeEmbeddingBatch([][]float32{vector}, 1, expectedDimensions)
	if err != nil {
		return nil, EmbeddingIdentity{}, err
	}
	service.queryCache.Put(cacheKey, normalized[0])
	return normalized[0], identity, nil
}

func (service *Service) searchDocuments(ctx context.Context, index VectorIndex, entityType string, vector []float32, limit int) ([]SemanticDocumentMatch, error) {
	if index == nil || index.Len() == 0 {
		return []SemanticDocumentMatch{}, nil
	}
	matches, err := index.Search(vector, limit)
	if err != nil {
		return nil, fmt.Errorf("search semantic %s index: %w", entityType, err)
	}
	ids := make([]int64, len(matches))
	for position, match := range matches {
		ids[position] = match.ID
	}
	documents, err := service.database.GetSemanticDocumentsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load semantic %s matches: %w", entityType, err)
	}
	byID := make(map[int64]db.SemanticDocument, len(documents))
	for _, document := range documents {
		byID[document.ID] = document
	}
	result := make([]SemanticDocumentMatch, 0, len(matches))
	for _, match := range matches {
		document, exists := byID[match.ID]
		if !exists || document.EntityType != entityType || document.Status != "ready" {
			continue
		}
		result = append(result, SemanticDocumentMatch{Document: document, Similarity: match.Similarity})
	}
	return result, nil
}
