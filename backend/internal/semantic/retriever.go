package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const (
	semanticTrackSearchLimit     = 300
	semanticAlbumSearchLimit     = 40
	semanticArtistSearchLimit    = 30
	semanticAlbumExpansionLimit  = 8
	semanticArtistExpansionLimit = 10
	semanticCandidatePoolLimit   = 900
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

// SemanticEvidence records why a local song entered the candidate pool.
// Track matches are intentionally weighted above album and artist rescue.
type SemanticEvidence struct {
	TrackSimilarity    float64
	AlbumSimilarity    float64
	ArtistSimilarity   float64
	BestSimilarity     float64
	NegativeSimilarity float64
	AdjustedSimilarity float64
	NegativeApplied    bool
	MatchedAlbum       string
	MatchedArtist      string
}

// Relevance returns the final semantic score for ranking. A negative query is
// optional; without one, the original direct/album/artist evidence is used.
func (evidence SemanticEvidence) Relevance() float64 {
	if evidence.NegativeApplied {
		return evidence.AdjustedSimilarity
	}
	return evidence.BestSimilarity
}

// SemanticCandidate pairs a valid local ViiB catalog song with durable
// retrieval evidence. It never carries raw embedding or document content.
type SemanticCandidate struct {
	Song     db.Song
	Evidence SemanticEvidence
}

// SemanticRetrievalOptions contains only deterministic eligibility filters.
// Behavioural ranking and diversity are intentionally separate concerns.
type SemanticRetrievalOptions struct {
	Source                string
	IncludeArtists        []string
	ExcludeArtists        []string
	MinYear               int
	MaxYear               int
	YearConstraintHard    bool
	InstrumentalOnly      bool
	NegativeSemanticQuery string
	RequiredStyles        []string
	ExcludedTerms         []string
}

type SemanticFilterDiagnostics struct {
	HardExcluded     int
	StyleMismatches  int
	NegativeRejected int
}

// SemanticRetrievalResult is the expanded, deduplicated candidate pool ready
// for ranking. CandidateCount is capped before hard filters by design.
type SemanticRetrievalResult struct {
	Search      SemanticSearchResult
	Candidates  []SemanticCandidate
	Diagnostics SemanticFilterDiagnostics
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

// RetrieveSemanticCandidates expands exact document matches into local songs,
// applies source availability plus hard constraints, and retains evidence for
// the hybrid ranker. It never falls back to loading the entire catalog.
func (service *Service) RetrieveSemanticCandidates(ctx context.Context, query string, options SemanticRetrievalOptions) (SemanticRetrievalResult, error) {
	search, err := service.SearchSemanticDocuments(ctx, query)
	if err != nil {
		return SemanticRetrievalResult{}, err
	}
	candidates := make(map[string]SemanticCandidate)
	trackIDs := make([]string, 0, len(search.Tracks))
	trackScores := make(map[string]float64, len(search.Tracks))
	for _, match := range search.Tracks {
		if songID := strings.TrimSpace(match.Document.SongID); songID != "" {
			if match.Similarity > trackScores[songID] {
				trackScores[songID] = match.Similarity
			}
			trackIDs = append(trackIDs, songID)
		}
	}
	tracks, err := service.database.GetSongsByIDsContext(ctx, uniqueStrings(trackIDs))
	if err != nil {
		return SemanticRetrievalResult{}, fmt.Errorf("expand semantic track matches: %w", err)
	}
	for _, song := range tracks {
		service.addSemanticCandidate(candidates, song, SemanticEvidence{TrackSimilarity: trackScores[song.ID]})
	}
	for _, match := range search.Albums {
		if len(candidates) >= semanticCandidatePoolLimit {
			break
		}
		songs, expansionErr := service.database.GetSongsForSemanticAlbum(ctx, match.Document.Artist, match.Document.Album, semanticAlbumExpansionLimit)
		if expansionErr != nil {
			return SemanticRetrievalResult{}, fmt.Errorf("expand semantic album %q: %w", match.Document.DisplayName, expansionErr)
		}
		for _, song := range songs {
			service.addSemanticCandidate(candidates, song, SemanticEvidence{AlbumSimilarity: match.Similarity, MatchedAlbum: match.Document.Album})
		}
	}
	for _, match := range search.Artists {
		if len(candidates) >= semanticCandidatePoolLimit {
			break
		}
		songs, expansionErr := service.database.GetSongsForSemanticArtist(ctx, match.Document.Artist, semanticArtistExpansionLimit)
		if expansionErr != nil {
			return SemanticRetrievalResult{}, fmt.Errorf("expand semantic artist %q: %w", match.Document.DisplayName, expansionErr)
		}
		for _, song := range songs {
			service.addSemanticCandidate(candidates, song, SemanticEvidence{ArtistSimilarity: match.Similarity, MatchedArtist: match.Document.Artist})
		}
	}
	songs := make([]db.Song, 0, len(candidates))
	for _, candidate := range candidates {
		songs = append(songs, candidate.Song)
	}
	songs, err = service.database.FilterSongsForAIDJ(songs, options.Source)
	if err != nil {
		return SemanticRetrievalResult{}, fmt.Errorf("filter semantic candidate sources: %w", err)
	}
	result := SemanticRetrievalResult{Search: search, Candidates: make([]SemanticCandidate, 0, len(songs))}
	for _, song := range songs {
		candidate := candidates[song.ID]
		candidate.Song = song
		allowed, reason := semanticCandidateAllowed(candidate, options)
		if allowed {
			result.Candidates = append(result.Candidates, candidate)
		} else if reason == "hard_exclusion" {
			result.Diagnostics.HardExcluded++
		} else if reason == "style_mismatch" {
			result.Diagnostics.StyleMismatches++
		}
	}
	if strings.TrimSpace(options.NegativeSemanticQuery) != "" {
		adjusted, adjustErr := service.ApplyNegativeSemanticPenalty(ctx, result.Candidates, options.NegativeSemanticQuery)
		if adjustErr != nil {
			return SemanticRetrievalResult{}, adjustErr
		}
		result.Diagnostics.NegativeRejected += len(result.Candidates) - len(adjusted)
		result.Candidates = adjusted
	}
	sort.Slice(result.Candidates, func(left, right int) bool {
		if result.Candidates[left].Evidence.Relevance() == result.Candidates[right].Evidence.Relevance() {
			return result.Candidates[left].Song.ID < result.Candidates[right].Song.ID
		}
		return result.Candidates[left].Evidence.Relevance() > result.Candidates[right].Evidence.Relevance()
	})
	return result, nil
}

// ApplyNegativeSemanticPenalty embeds the negative intent once, then compares
// it only with vectors belonging to the already-bounded positive candidate
// pool. It deliberately does not run a second corpus search.
func (service *Service) ApplyNegativeSemanticPenalty(ctx context.Context, candidates []SemanticCandidate, negativeQuery string) ([]SemanticCandidate, error) {
	negativeQuery = strings.TrimSpace(negativeQuery)
	if negativeQuery == "" || len(candidates) == 0 {
		return candidates, nil
	}
	_, dimensions, hasReadyIndex, err := service.searchableIndexes()
	if err != nil {
		return nil, err
	}
	if !hasReadyIndex {
		return nil, ErrNoSearchableSemanticIndex
	}
	negativeVector, _, err := service.embedQuery(ctx, negativeQuery, dimensions)
	if err != nil {
		return nil, fmt.Errorf("embed negative semantic query: %w", err)
	}
	songIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		songIDs = append(songIDs, candidate.Song.ID)
	}
	stored, err := service.database.GetReadySemanticTrackEmbeddingsBySongIDs(ctx, songIDs)
	if err != nil {
		return nil, fmt.Errorf("load positive candidate vectors: %w", err)
	}
	vectors := make(map[string][]float32, len(stored))
	for _, item := range stored {
		vector, decodeErr := DecodeVector(item.Embedding, dimensions)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode semantic track vector for %q: %w", item.SongID, decodeErr)
		}
		vectors[item.SongID] = vector
	}
	adjusted := make([]SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		vector, exists := vectors[candidate.Song.ID]
		if !exists {
			adjusted = append(adjusted, candidate)
			continue
		}
		negativeSimilarity := cosineSimilarity(vector, negativeVector)
		candidate.Evidence.NegativeSimilarity = negativeSimilarity
		candidate.Evidence.AdjustedSimilarity = clampSemanticScore(candidate.Evidence.BestSimilarity - 0.65*negativeSimilarity)
		candidate.Evidence.NegativeApplied = true
		margin := candidate.Evidence.BestSimilarity - negativeSimilarity
		if negativeSimilarity >= 0.78 || (negativeSimilarity >= 0.60 && margin <= 0.08) {
			continue
		}
		adjusted = append(adjusted, candidate)
	}
	return adjusted, nil
}

func cosineSimilarity(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	similarity := 0.0
	for index, value := range left {
		similarity += float64(value) * float64(right[index])
	}
	return clampSemanticScore(similarity)
}

func (service *Service) addSemanticCandidate(candidates map[string]SemanticCandidate, song db.Song, evidence SemanticEvidence) {
	if song.ID == "" {
		return
	}
	candidate, exists := candidates[song.ID]
	if !exists && len(candidates) >= semanticCandidatePoolLimit {
		return
	}
	if !exists {
		candidate.Song = song
	}
	candidate.Evidence.TrackSimilarity = max(candidate.Evidence.TrackSimilarity, evidence.TrackSimilarity)
	candidate.Evidence.AlbumSimilarity = max(candidate.Evidence.AlbumSimilarity, evidence.AlbumSimilarity)
	candidate.Evidence.ArtistSimilarity = max(candidate.Evidence.ArtistSimilarity, evidence.ArtistSimilarity)
	if evidence.MatchedAlbum != "" {
		candidate.Evidence.MatchedAlbum = evidence.MatchedAlbum
	}
	if evidence.MatchedArtist != "" {
		candidate.Evidence.MatchedArtist = evidence.MatchedArtist
	}
	candidate.Evidence.BestSimilarity = max(
		candidate.Evidence.TrackSimilarity,
		candidate.Evidence.AlbumSimilarity*0.94,
		candidate.Evidence.ArtistSimilarity*0.88,
	)
	candidates[song.ID] = candidate
}

func semanticCandidateAllowed(candidate SemanticCandidate, options SemanticRetrievalOptions) (bool, string) {
	song := candidate.Song
	artist := strings.TrimSpace(song.Artist)
	if semanticArtistListed(artist, options.ExcludeArtists) {
		return false, "artist"
	}
	if len(options.IncludeArtists) > 0 && !semanticArtistListed(artist, options.IncludeArtists) {
		return false, "artist"
	}
	if options.InstrumentalOnly && !song.Instrumental {
		return false, "instrumental"
	}
	if options.YearConstraintHard {
		year := song.OriginalYear
		if year == 0 {
			year = song.Year
		}
		if options.MinYear > 0 && year < options.MinYear {
			return false, "year"
		}
		if options.MaxYear > 0 && year > options.MaxYear {
			return false, "year"
		}
	}
	if songMatchesExcludedTerms(song, options.ExcludedTerms) {
		return false, "hard_exclusion"
	}
	if !songMatchesRequiredStyles(candidate, options.RequiredStyles) {
		return false, "style_mismatch"
	}
	return true, ""
}

// FilterSongsByConstraints applies the same deterministic AI DJ policy to
// legacy catalog paths that cannot use semantic candidate evidence.
func FilterSongsByConstraints(songs []db.Song, options SemanticRetrievalOptions) ([]db.Song, SemanticFilterDiagnostics) {
	filtered := make([]db.Song, 0, len(songs))
	diagnostics := SemanticFilterDiagnostics{}
	for _, song := range songs {
		allowed, reason := semanticCandidateAllowed(SemanticCandidate{Song: song}, options)
		if allowed {
			filtered = append(filtered, song)
		} else if reason == "hard_exclusion" {
			diagnostics.HardExcluded++
		} else if reason == "style_mismatch" {
			diagnostics.StyleMismatches++
		}
	}
	return filtered, diagnostics
}

var semanticStyleAliases = map[string][]string{
	"jazz":       {"jazz", "nu jazz", "acid jazz", "jazz fusion", "electro jazz", "contemporary jazz"},
	"rock":       {"rock"},
	"hip hop":    {"hip hop", "hip-hop", "rap"},
	"electronic": {"electronic", "electronica", "edm", "techno", "house"},
	"classical":  {"classical"},
	"country":    {"country"},
	"metal":      {"metal"},
	"blues":      {"blues"},
	"reggae":     {"reggae"},
	"folk":       {"folk"},
	"soul":       {"soul"},
	"funk":       {"funk"},
	"r&b":        {"r&b", "rnb", "rhythm and blues"},
	"latin":      {"latin"},
	"pop":        {"pop"},
}

var semanticExclusionAliases = map[string][]string{
	"christmas": {"christmas", "xmas", "yuletide", "noel", "noël", "carol", "carols", "holiday", "holidays", "auld lang syne"},
	"holiday":   {"christmas", "xmas", "yuletide", "noel", "noël", "carol", "carols", "holiday", "holidays", "auld lang syne"},
}

func songMatchesExcludedTerms(song db.Song, excludedTerms []string) bool {
	if len(excludedTerms) == 0 {
		return false
	}
	haystack := normalizedMetadataText(song)
	for _, term := range excludedTerms {
		normalizedTerm := normalizeMatchText(term)
		aliases := semanticExclusionAliases[normalizedTerm]
		if len(aliases) == 0 {
			for canonical, family := range semanticExclusionAliases {
				if containsNormalizedPhrase(" "+normalizedTerm+" ", canonical) {
					aliases = family
					break
				}
			}
		}
		if len(aliases) == 0 {
			aliases = []string{term}
		}
		for _, alias := range aliases {
			if containsNormalizedPhrase(haystack, alias) {
				return true
			}
		}
	}
	return false
}

func songMatchesRequiredStyles(candidate SemanticCandidate, requiredStyles []string) bool {
	if len(requiredStyles) == 0 {
		return true
	}
	metadata := normalizedMetadataText(candidate.Song)
	for _, required := range requiredStyles {
		aliases := semanticStyleAliases[normalizeMatchText(required)]
		if len(aliases) == 0 {
			aliases = []string{required}
		}
		for _, alias := range aliases {
			if containsNormalizedPhrase(metadata, alias) {
				return true
			}
		}
	}
	// Poorly tagged tracks may still be rescued by strong direct track evidence;
	// album/artist expansion alone is not sufficient to override an explicit style.
	return len(candidate.Song.Genre) == 0 && strings.TrimSpace(candidate.Song.LastFMTags) == "" && candidate.Evidence.TrackSimilarity >= 0.62
}

func normalizedMetadataText(song db.Song) string {
	values := []string{song.Title, song.Artist, song.Album}
	values = append(values, song.Genre...)
	var tags []string
	if json.Unmarshal([]byte(song.LastFMTags), &tags) == nil {
		values = append(values, tags...)
	}
	return " " + normalizeMatchText(strings.Join(values, " ")) + " "
}

func containsNormalizedPhrase(haystack, phrase string) bool {
	phrase = normalizeMatchText(phrase)
	return phrase != "" && strings.Contains(haystack, " "+phrase+" ")
}

func normalizeMatchText(value string) string {
	var builder strings.Builder
	lastSpace := true
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '&' {
			builder.WriteRune(character)
			lastSpace = false
		} else if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(builder.String())
}

func semanticArtistListed(artist string, artists []string) bool {
	for _, candidate := range artists {
		if strings.EqualFold(strings.TrimSpace(candidate), artist) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
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
