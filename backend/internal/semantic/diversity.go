package semantic

import (
	"context"
	"fmt"
	"strings"
)

const semanticMMRWorkingSetLimit = 120

// DiversityOptions controls the bounded MMR pass after hybrid ranking.
type DiversityOptions struct {
	DiscoverMode string
	Limit        int
	AlbumFocused bool
}

// ApplyMMRDiversity applies MMR only to the highest-ranked bounded working
// set. It returns selection order, preserving the original hybrid score on
// each item for diagnostics and downstream DJ scoring.
func (service *Service) ApplyMMRDiversity(ctx context.Context, candidates []RankedSemanticCandidate, options DiversityOptions) ([]RankedSemanticCandidate, error) {
	if len(candidates) == 0 {
		return []RankedSemanticCandidate{}, nil
	}
	workingCount := min(len(candidates), semanticMMRWorkingSetLimit)
	working := append([]RankedSemanticCandidate(nil), candidates[:workingCount]...)
	_, dimensions, hasReadyIndex, err := service.searchableIndexes()
	if err != nil {
		return nil, err
	}
	if !hasReadyIndex {
		return nil, ErrNoSearchableSemanticIndex
	}
	songIDs := make([]string, 0, len(working))
	for _, candidate := range working {
		songIDs = append(songIDs, candidate.Candidate.Song.ID)
	}
	stored, err := service.database.GetReadySemanticTrackEmbeddingsBySongIDs(ctx, songIDs)
	if err != nil {
		return nil, fmt.Errorf("load diversity candidate vectors: %w", err)
	}
	vectors := make(map[string][]float32, len(stored))
	for _, item := range stored {
		vector, decodeErr := DecodeVector(item.Embedding, dimensions)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode diversity track vector for %q: %w", item.SongID, decodeErr)
		}
		vectors[item.SongID] = vector
	}
	selected, remaining := selectMMRCandidates(working, vectors, options)
	if options.Limit <= 0 {
		selected = append(selected, remaining...)
		selected = append(selected, candidates[workingCount:]...)
	}
	return selected, nil
}

func selectMMRCandidates(candidates []RankedSemanticCandidate, vectors map[string][]float32, options DiversityOptions) ([]RankedSemanticCandidate, []RankedSemanticCandidate) {
	remaining := append([]RankedSemanticCandidate(nil), candidates...)
	limit := options.Limit
	if limit <= 0 || limit > len(remaining) {
		limit = len(remaining)
	}
	selected := make([]RankedSemanticCandidate, 0, limit)
	albumSelections := make(map[string]int)
	lambda := mmrLambda(options.DiscoverMode)
	for len(remaining) > 0 && len(selected) < limit {
		best := -1
		bestScore := -2.0
		for index := range remaining {
			candidate := remaining[index]
			maxSimilarity := maxSimilarityToSelected(candidate, selected, vectors)
			if !options.AlbumFocused && albumSelections[normalizedAlbum(candidate.Candidate.Song.Album, candidate.Candidate.Song.AlbumArtist, candidate.Candidate.Song.Artist)] >= 2 {
				maxSimilarity = max(maxSimilarity, 0.50)
			}
			mmrScore := lambda*candidate.Score - (1-lambda)*maxSimilarity
			if best == -1 || mmrScore > bestScore || (mmrScore == bestScore && candidate.Candidate.Song.ID < remaining[best].Candidate.Song.ID) {
				best = index
				bestScore = mmrScore
			}
		}
		choice := remaining[best]
		selected = append(selected, choice)
		album := normalizedAlbum(choice.Candidate.Song.Album, choice.Candidate.Song.AlbumArtist, choice.Candidate.Song.Artist)
		if album != "" {
			albumSelections[album]++
		}
		remaining = append(remaining[:best], remaining[best+1:]...)
	}
	return selected, remaining
}

func maxSimilarityToSelected(candidate RankedSemanticCandidate, selected []RankedSemanticCandidate, vectors map[string][]float32) float64 {
	vector, exists := vectors[candidate.Candidate.Song.ID]
	if !exists {
		return 0
	}
	maxSimilarity := 0.0
	for _, previous := range selected {
		if previousVector, found := vectors[previous.Candidate.Song.ID]; found {
			maxSimilarity = max(maxSimilarity, cosineSimilarity(vector, previousVector))
		}
	}
	return maxSimilarity
}

func mmrLambda(discoverMode string) float64 {
	switch strings.ToLower(strings.TrimSpace(discoverMode)) {
	case "favorites":
		return 0.85
	case "discover":
		return 0.60
	default:
		return 0.75
	}
}

func normalizedAlbum(album, albumArtist, artist string) string {
	album = strings.ToLower(strings.TrimSpace(album))
	if album == "" {
		return ""
	}
	albumArtist = strings.ToLower(strings.TrimSpace(albumArtist))
	if albumArtist == "" {
		albumArtist = strings.ToLower(strings.TrimSpace(artist))
	}
	return albumArtist + "\x00" + album
}
