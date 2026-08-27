package semantic

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestRankSemanticCandidatesPrioritizesSemanticRelevance(t *testing.T) {
	ranked := RankSemanticCandidates([]SemanticCandidate{
		{Song: db.Song{ID: "favorite-low", Artist: "Favorite", Liked: true, PlayCount: 100}, Evidence: SemanticEvidence{BestSimilarity: 0.35}},
		{Song: db.Song{ID: "relevant", Artist: "Discovery"}, Evidence: SemanticEvidence{BestSimilarity: 0.92}},
	}, HybridRankingOptions{DiscoverMode: "balanced"})
	if len(ranked) != 2 || ranked[0].Candidate.Song.ID != "relevant" {
		t.Fatalf("ranking=%#v", ranked)
	}
}

func TestRankSemanticCandidatesAppliesRecencyAndArtistPolicies(t *testing.T) {
	ranked := RankSemanticCandidates([]SemanticCandidate{
		{Song: db.Song{ID: "recent", Artist: "Recent"}, Evidence: SemanticEvidence{BestSimilarity: 0.99}},
		{Song: db.Song{ID: "first", Artist: "Repeated"}, Evidence: SemanticEvidence{BestSimilarity: 0.95}},
		{Song: db.Song{ID: "second", Artist: "Repeated"}, Evidence: SemanticEvidence{BestSimilarity: 0.94}},
		{Song: db.Song{ID: "other", Artist: "Other"}, Evidence: SemanticEvidence{BestSimilarity: 0.80}},
	}, HybridRankingOptions{
		RecentlyPlayedIDs: map[string]bool{"recent": true},
		OnePerArtist:      true,
	})
	if len(ranked) != 2 || ranked[0].Candidate.Song.ID != "first" || ranked[1].Candidate.Song.ID != "other" {
		t.Fatalf("ranking=%#v", ranked)
	}
}

func TestRankSemanticCandidatesRespectsDiscoveryAndFavoritesModes(t *testing.T) {
	candidates := []SemanticCandidate{
		{Song: db.Song{ID: "familiar", Artist: "Familiar", Liked: true, PlayCount: 25}, Evidence: SemanticEvidence{BestSimilarity: 0.7}},
		{Song: db.Song{ID: "deep-cut", Artist: "Deep Cut"}, Evidence: SemanticEvidence{BestSimilarity: 0.7}},
	}
	if ranked := RankSemanticCandidates(candidates, HybridRankingOptions{DiscoverMode: "favorites"}); ranked[0].Candidate.Song.ID != "familiar" {
		t.Fatalf("favorites ranking=%#v", ranked)
	}
	if ranked := RankSemanticCandidates(candidates, HybridRankingOptions{DiscoverMode: "discover"}); ranked[0].Candidate.Song.ID != "deep-cut" {
		t.Fatalf("discover ranking=%#v", ranked)
	}
}
