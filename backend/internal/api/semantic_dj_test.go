// Tests and fixtures for semantic dj behavior.

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

func TestRetrieveSemanticDJPhasePoolsKeepsSemanticShortageWhenPoolsLackDistinctSongs(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "semantic-dj-small.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	song := db.Song{ID: "only", Title: "Only", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "only.flac"), AddedAt: 1}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
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
	if _, handled, err := api.retrieveSemanticDJPhasePools(context.Background(), &dj.DJSetPlan{
		IntentSummary: "anything",
		Phases: []dj.DJPhase{
			{Name: "Warm-up", SemanticQuery: "quiet", TargetCount: 1},
			{Name: "Peak", SemanticQuery: "loud", TargetCount: 1},
		},
	}, semanticDJRequest{Source: "local"}); err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
}

func TestAdaptiveDJRefillsAuditRejectionsOnceAcrossOverlappingPhases(t *testing.T) {
	api, provider := adaptivePlaylistFixture(t, 420, nil)
	seen := make(map[string]bool)
	audit := func(_ context.Context, _ string, _ llm.PlaylistIntent, songs []db.Song) ([]db.Song, playlistValidationDiagnostics) {
		var accepted []db.Song
		for _, song := range songs {
			if seen[song.ID] {
				t.Fatalf("duplicate audit across phases: %s", song.ID)
			}
			seen[song.ID] = true
			if song.ID >= "song-0325" {
				accepted = append(accepted, song)
			}
		}
		return accepted, playlistValidationDiagnostics{AuditStatus: "passed", AuditReviewed: len(songs), AuditRejected: len(songs) - len(accepted)}
	}
	plan := &dj.DJSetPlan{IntentSummary: "jazz", Phases: []dj.DJPhase{{Name: "Warm-up", SemanticQuery: "jazz", TargetCount: 10}, {Name: "Peak", SemanticQuery: "jazz", TargetCount: 10}}}
	result, handled, err := api.retrieveSemanticDJPhasePoolsWithAudit(context.Background(), plan, semanticDJRequest{Source: "local"}, audit)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if len(result.Pools[0].Songs) != 50 || len(result.Pools[1].Songs) != 50 || provider.queries != 1 {
		t.Fatalf("pools=%v query embeds=%d", result.PhaseCandidateCount, provider.queries)
	}
	queue, _, err := sequenceAvailableSemanticPhases(result.Pools, plan, dj.GetPersona(dj.PersonaFlowMaster), dj.NewScoreContext())
	if err != nil || len(queue) != 20 {
		t.Fatalf("queue=%d err=%v", len(queue), err)
	}
	unique := make(map[string]bool)
	for _, song := range queue {
		if unique[song.ID] || song.ID < "song-0325" {
			t.Fatalf("invalid or repeated track %s", song.ID)
		}
		unique[song.ID] = true
	}
	if result.Audit.AuditReviewed != len(seen) || result.Audit.AuditRejected != 325 {
		t.Fatalf("audit=%+v unique=%d", result.Audit, len(seen))
	}
}

func TestSequenceAvailableSemanticPhasesPreservesEmptyConstrainedPhase(t *testing.T) {
	plan := &dj.DJSetPlan{Phases: []dj.DJPhase{{Name: "Empty", TargetCount: 2}, {Name: "Available", TargetCount: 1}}}
	queue, phases, err := sequenceAvailableSemanticPhases([]dj.PhaseCandidatePool{{}, {Songs: []db.Song{{ID: "only", Artist: "Artist"}}}}, plan, dj.GetPersona(dj.PersonaFlowMaster), dj.NewScoreContext())
	if err != nil || len(queue) != 1 || len(phases) != 2 || phases[0].Name != "Empty" || phases[0].SongCount != 0 || phases[1].SongCount != 1 {
		t.Fatalf("queue=%v phases=%v err=%v", queue, phases, err)
	}
}
