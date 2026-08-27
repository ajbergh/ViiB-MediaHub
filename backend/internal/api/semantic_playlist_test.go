package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

func TestRetrieveSemanticPlaylistUsesBoundedRankAndPreservesResponseFilter(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "semantic-playlist.db"))
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
	result, handled, err := api.retrieveSemanticPlaylist(context.Background(), llm.PlaylistIntent{
		IntentSummary:   "atmospheric driving music",
		SemanticQuery:   "atmospheric driving music",
		PreferredGenres: []string{"Electronic"},
	}, semanticPlaylistRequest{
		TargetSongs:  1,
		DiscoverMode: "balanced",
		Source:       "local",
	})
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if len(result.Songs) != 1 || result.Filter.Description != "atmospheric driving music" || result.Filter.Source != "local" {
		t.Fatalf("result=%#v", result)
	}
	if result.Diagnostics.Mode != "semantic" || result.Diagnostics.CandidateCount != 2 || result.Diagnostics.ReturnedCount != 1 {
		t.Fatalf("diagnostics=%#v", result.Diagnostics)
	}
}

func TestRetrieveSemanticPlaylistFallsBackOnlyWithoutSearchableIndex(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "semantic-playlist-empty.db"))
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
	if _, handled, err := api.retrieveSemanticPlaylist(context.Background(), llm.FallbackPlaylistIntent("anything"), semanticPlaylistRequest{TargetSongs: 5, Source: "local"}); err != nil || handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
}
