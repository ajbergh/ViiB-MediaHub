package semantic

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestSelectMMRCandidatesBalancesRelevanceAndDiversityByMode(t *testing.T) {
	candidates := []RankedSemanticCandidate{
		{Candidate: SemanticCandidate{Song: db.Song{ID: "first", Artist: "A"}}, Score: 0.90},
		{Candidate: SemanticCandidate{Song: db.Song{ID: "near-duplicate", Artist: "B"}}, Score: 0.89},
		{Candidate: SemanticCandidate{Song: db.Song{ID: "different", Artist: "C"}}, Score: 0.70},
	}
	vectors := map[string][]float32{
		"first":          {1, 0},
		"near-duplicate": {1, 0},
		"different":      {0, 1},
	}
	discover, _ := selectMMRCandidates(candidates, vectors, DiversityOptions{DiscoverMode: "discover", Limit: 2})
	if len(discover) != 2 || discover[0].Candidate.Song.ID != "first" || discover[1].Candidate.Song.ID != "different" {
		t.Fatalf("discover selection=%#v", discover)
	}
	favorites, _ := selectMMRCandidates(candidates, vectors, DiversityOptions{DiscoverMode: "favorites", Limit: 2})
	if len(favorites) != 2 || favorites[0].Candidate.Song.ID != "first" || favorites[1].Candidate.Song.ID != "near-duplicate" {
		t.Fatalf("favorites selection=%#v", favorites)
	}
}

func TestSelectMMRCandidatesAvoidsLongAlbumRuns(t *testing.T) {
	candidates := []RankedSemanticCandidate{
		{Candidate: SemanticCandidate{Song: db.Song{ID: "one", Album: "Record", Artist: "Artist"}}, Score: 0.90},
		{Candidate: SemanticCandidate{Song: db.Song{ID: "two", Album: "Record", Artist: "Artist"}}, Score: 0.89},
		{Candidate: SemanticCandidate{Song: db.Song{ID: "three", Album: "Record", Artist: "Artist"}}, Score: 0.88},
		{Candidate: SemanticCandidate{Song: db.Song{ID: "other", Album: "Elsewhere", Artist: "Other"}}, Score: 0.72},
	}
	vectors := map[string][]float32{
		"one":   {1, 0},
		"two":   {0, 1},
		"three": {0, 1},
		"other": {-1, 0},
	}
	selected, _ := selectMMRCandidates(candidates, vectors, DiversityOptions{DiscoverMode: "favorites", Limit: 4})
	if len(selected) != 4 || selected[2].Candidate.Song.ID != "other" {
		t.Fatalf("album-aware selection=%#v", selected)
	}
}
