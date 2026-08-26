package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/dj"
	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

func TestRetrieveSemanticDJPhasePoolsBuildsBoundedPerPhaseScores(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "semantic-dj.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, song := range []db.Song{
		{ID: "first", Title: "First", Artist: "Artist One", Album: "Album One", FilePath: filepath.Join(t.TempDir(), "first.flac"), AddedAt: 1},
		{ID: "second", Title: "Second", Artist: "Artist Two", Album: "Album Two", FilePath: filepath.Join(t.TempDir(), "second.flac"), AddedAt: 1},
	} {
		if err := database.SaveSong(&song); err != nil {
			t.Fatal(err)
		}
	}
	service, err := semantic.NewService(database, apiEmbeddingProvider{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	api := &API{db: database, semanticService: service}
	result, handled, err := api.retrieveSemanticDJPhasePools(context.Background(), &dj.DJSetPlan{
		IntentSummary: "night drive",
		Phases: []dj.DJPhase{
			{Name: "Warm-up", SemanticQuery: "quiet night drive", TargetCount: 1},
			{Name: "Peak", SemanticQuery: "energized night drive", TargetCount: 1},
		},
	}, semanticDJRequest{Source: "local", DiscoverMode: "balanced", Intent: llm.FallbackPlaylistIntent("night drive")})
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if len(result.Pools) != 2 || len(result.Pools[0].Songs) == 0 || len(result.Pools[1].SemanticScores) == 0 || result.CandidateCount != 4 {
		t.Fatalf("result=%#v", result)
	}
}

func TestRetrieveSemanticDJPhasePoolsFallsBackWhenIndexIsEmpty(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "semantic-dj-empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service, err := semantic.NewService(database, apiEmbeddingProvider{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	api := &API{db: database, semanticService: service}
	if _, handled, err := api.retrieveSemanticDJPhasePools(context.Background(), &dj.DJSetPlan{IntentSummary: "anything", Phases: []dj.DJPhase{{Name: "Warm-up", TargetCount: 1}}}, semanticDJRequest{Source: "local"}); err != nil || handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
}
