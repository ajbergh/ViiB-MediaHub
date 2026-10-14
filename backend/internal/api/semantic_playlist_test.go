// Tests and fixtures for semantic playlist behavior.

package api

import (
	"context"
	"fmt"
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

type countingPlaylistEmbeddingProvider struct {
	apiEmbeddingProvider
	queries int
}

func (p *countingPlaylistEmbeddingProvider) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	p.queries++
	if text == "avoid" {
		return []float32{-1, -1}, nil
	}
	return []float32{1, 1}, nil
}

func adaptivePlaylistFixture(t *testing.T, count int, modify func(int, *db.Song)) (*API, *countingPlaylistEmbeddingProvider) {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "adaptive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	for i := 0; i < count; i++ {
		song := db.Song{ID: fmt.Sprintf("song-%04d", i), Title: fmt.Sprintf("Song %04d", i), Artist: "Artist", Album: "Album", Genre: []string{"Jazz"}, Year: 2005, Instrumental: true, FilePath: fmt.Sprintf("music-%04d.flac", i)}
		if modify != nil {
			modify(i, &song)
		}
		if err := database.SaveSong(&song); err != nil {
			t.Fatal(err)
		}
		if err := database.UpdateSongMood(song.ID, song.Mood, song.Energy, song.Tempo, song.Instrumental); err != nil {
			t.Fatal(err)
		}
	}
	provider := &countingPlaylistEmbeddingProvider{}
	service, err := semantic.NewService(database, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	return &API{db: database, semanticService: service}, provider
}

func acceptingPlaylistAudit(_ context.Context, _ string, _ llm.PlaylistIntent, songs []db.Song) ([]db.Song, playlistValidationDiagnostics) {
	return songs, playlistValidationDiagnostics{AuditStatus: "passed", AuditReviewed: len(songs)}
}

func TestAdaptivePlaylistWidensPastRecentAndHardFilteredTracks(t *testing.T) {
	api, provider := adaptivePlaylistFixture(t, 800, func(i int, song *db.Song) {
		if i >= 350 && i < 700 {
			song.Year = 1975
			song.Instrumental = false
		}
	})
	recent := make(map[string]bool)
	for i := 0; i < 350; i++ {
		recent[fmt.Sprintf("song-%04d", i)] = true
	}
	result, handled, err := api.retrieveSemanticPlaylistWithAudit(context.Background(), llm.PlaylistIntent{SemanticQuery: "jazz", NegativeSemanticQuery: "avoid", MinYear: 2000, YearConstraintHard: true, InstrumentalOnly: true, RequiredStyles: []string{"jazz"}}, semanticPlaylistRequest{TargetSongs: 50, Source: "local", RecentlyPlayedIDs: recent}, acceptingPlaylistAudit)
	if err != nil || !handled || len(result.Songs) != 50 {
		t.Fatalf("handled=%v count=%d err=%v diagnostics=%+v", handled, len(result.Songs), err, result.Diagnostics)
	}
	if result.Diagnostics.SearchPasses != 2 || result.Diagnostics.Validation.Shortened {
		t.Fatalf("diagnostics=%+v", result.Diagnostics)
	}
	if provider.queries != 2 {
		t.Fatalf("query embeddings=%d, want one positive and one negative", provider.queries)
	}
	for _, song := range result.Songs {
		if recent[song.ID] || song.Year < 2000 || !song.Instrumental {
			t.Fatalf("constraint lost: %+v", song)
		}
	}
}

func TestAdaptivePlaylistRefillsAuditRejectionsBeyondInitialSearch(t *testing.T) {
	api, provider := adaptivePlaylistFixture(t, 420, nil)
	seen := make(map[string]bool)
	audit := func(_ context.Context, _ string, _ llm.PlaylistIntent, songs []db.Song) ([]db.Song, playlistValidationDiagnostics) {
		if len(songs) > 100 {
			t.Fatal("unbounded audit batch")
		}
		var accepted []db.Song
		for _, song := range songs {
			if seen[song.ID] {
				t.Fatalf("repeated audit of %s", song.ID)
			}
			seen[song.ID] = true
			if song.ID >= "song-0325" {
				accepted = append(accepted, song)
			}
		}
		return accepted, playlistValidationDiagnostics{AuditStatus: "passed", AuditReviewed: len(songs), AuditRejected: len(songs) - len(accepted)}
	}
	result, handled, err := api.retrieveSemanticPlaylistWithAudit(context.Background(), llm.PlaylistIntent{SemanticQuery: "jazz"}, semanticPlaylistRequest{TargetSongs: 50, Source: "local"}, audit)
	if err != nil || !handled || len(result.Songs) != 50 {
		t.Fatalf("count=%d handled=%v err=%v", len(result.Songs), handled, err)
	}
	if provider.queries != 1 || result.Diagnostics.SearchPasses != 2 || result.Diagnostics.Validation.AuditRejected != 325 {
		t.Fatalf("queries=%d diagnostics=%+v", provider.queries, result.Diagnostics)
	}
	for _, song := range result.Songs {
		if song.ID < "song-0325" {
			t.Fatalf("rejected track returned: %s", song.ID)
		}
	}
}

func TestAdaptivePlaylistReportsExhaustionAndAuditBudget(t *testing.T) {
	for _, count := range []int{3, 1100} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			api, _ := adaptivePlaylistFixture(t, count, nil)
			audit := acceptingPlaylistAudit
			if count > 1000 {
				audit = func(_ context.Context, _ string, _ llm.PlaylistIntent, songs []db.Song) ([]db.Song, playlistValidationDiagnostics) {
					return nil, playlistValidationDiagnostics{AuditStatus: "passed", AuditReviewed: len(songs), AuditRejected: len(songs)}
				}
			}
			result, handled, err := api.retrieveSemanticPlaylistWithAudit(context.Background(), llm.PlaylistIntent{SemanticQuery: "jazz"}, semanticPlaylistRequest{TargetSongs: 50, Source: "local"}, audit)
			if err != nil || !handled || !result.Diagnostics.Validation.Shortened {
				t.Fatalf("handled=%v err=%v diagnostics=%+v", handled, err, result.Diagnostics)
			}
			if count == 3 && (len(result.Songs) != 3 || result.Diagnostics.SearchPasses != 1 || result.Diagnostics.Validation.ShortfallReason != "matching_tracks_exhausted") {
				t.Fatalf("exhaustion=%+v", result.Diagnostics)
			}
			if count > 1000 && (len(result.Songs) != 0 || result.Diagnostics.Validation.AuditReviewed != 1000 || result.Diagnostics.Validation.ShortfallReason != "audit_limit_reached") {
				t.Fatalf("audit budget=%+v", result.Diagnostics)
			}
		})
	}
}

func TestAdaptivePlaylistReplacesRejectedArtistRepresentative(t *testing.T) {
	api, _ := adaptivePlaylistFixture(t, 4, func(i int, song *db.Song) { song.Artist = fmt.Sprintf("Artist %d", i/2) })
	audit := func(_ context.Context, _ string, _ llm.PlaylistIntent, songs []db.Song) ([]db.Song, playlistValidationDiagnostics) {
		var accepted []db.Song
		for _, song := range songs {
			if song.ID == "song-0001" || song.ID == "song-0003" {
				accepted = append(accepted, song)
			}
		}
		return accepted, playlistValidationDiagnostics{AuditStatus: "passed", AuditReviewed: len(songs), AuditRejected: len(songs) - len(accepted)}
	}
	result, _, err := api.retrieveSemanticPlaylistWithAudit(context.Background(), llm.PlaylistIntent{SemanticQuery: "jazz"}, semanticPlaylistRequest{TargetSongs: 2, Source: "local", OnePerArtist: true}, audit)
	if err != nil || len(result.Songs) != 2 || result.Songs[0].Artist == result.Songs[1].Artist {
		t.Fatalf("count=%d err=%v", len(result.Songs), err)
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

func TestAdaptivePlaylistWidensAfterLocalSourceFiltering(t *testing.T) {
	api, _ := adaptivePlaylistFixture(t, 400, nil)
	if err := api.db.SavePlexSource(db.PlexSource{ID: "remote", MachineIdentifier: "machine", BaseURL: "https://plex.invalid", Available: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	tracks := make([]db.PlexCatalogTrack, 350)
	for i := range tracks {
		tracks[i] = db.PlexCatalogTrack{SongID: fmt.Sprintf("song-%04d", i), SourceID: "remote", LibraryID: "music", MachineID: "machine", RatingKey: fmt.Sprint(i), MediaKey: fmt.Sprintf("/media/%d.flac", i), Title: fmt.Sprintf("AAA %04d", i), Artist: "Artist", Album: "Album"}
	}
	if _, _, _, err := api.db.SyncPlexLibrary("remote", "music", tracks); err != nil {
		t.Fatal(err)
	}
	if err := api.semanticService.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, handled, err := api.retrieveSemanticPlaylistWithAudit(context.Background(), llm.PlaylistIntent{SemanticQuery: "music"}, semanticPlaylistRequest{TargetSongs: 50, Source: "local"}, acceptingPlaylistAudit)
	if err != nil || !handled || len(result.Songs) != 50 || result.Diagnostics.SearchPasses != 2 {
		t.Fatalf("count=%d err=%v diagnostics=%+v", len(result.Songs), err, result.Diagnostics)
	}
	for _, song := range result.Songs {
		if song.Source != "local" {
			t.Fatalf("wrong source: %+v", song)
		}
	}
}

func TestAdaptivePlaylistLargeReserveIsNotCappedByMMRWindow(t *testing.T) {
	api, _ := adaptivePlaylistFixture(t, 180, nil)
	result, handled, err := api.retrieveSemanticPlaylistWithAudit(context.Background(), llm.PlaylistIntent{SemanticQuery: "jazz"}, semanticPlaylistRequest{TargetSongs: 150, Source: "local"}, func(ctx context.Context, prompt string, intent llm.PlaylistIntent, songs []db.Song) ([]db.Song, playlistValidationDiagnostics) {
		if len(songs) > maximumPlaylistAuditCandidates {
			t.Fatal("audit batch exceeded 100")
		}
		return acceptingPlaylistAudit(ctx, prompt, intent, songs)
	})
	if err != nil || !handled || len(result.Songs) != 150 {
		t.Fatalf("count=%d err=%v", len(result.Songs), err)
	}
}
