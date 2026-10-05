// Tests and fixtures for spotify match behavior.

package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
)

func matchCandidate(id, title, artist, album string, duration int) *catalog.Track {
	return &catalog.Track{ID: id, Name: title, Artists: []catalog.SimpleArtist{{Name: artist}}, Album: catalog.AlbumSummary{Name: album}, DurationMS: duration}
}

func TestSpotifyRecordingMatcherRanksReleasesAndRejectsWrongVersions(t *testing.T) {
	song := db.Song{Title: "Song", Artist: "Artist", Album: "Album", Duration: 180}
	good := matchCandidate(cachedSpotifyID, "SONG", "artist", "Album", 181000)
	duplicate := matchCandidate("0123456789012345678901", "Song", "Artist", "Compilation", 180000)
	for _, test := range []struct {
		name       string
		candidates []*catalog.Track
		want       string
	}{
		{"exact normalized", []*catalog.Track{good}, cachedSpotifyID},
		{"wrong artist", []*catalog.Track{matchCandidate(cachedSpotifyID, "Song", "Cover Artist", "Album", 180000)}, ""},
		{"live version", []*catalog.Track{matchCandidate(cachedSpotifyID, "Song (Live)", "Artist", "Album", 180000)}, ""},
		{"remix", []*catalog.Track{matchCandidate(cachedSpotifyID, "Song - Remix", "Artist", "Album", 180000)}, ""},
		{"duration mismatch", []*catalog.Track{matchCandidate(cachedSpotifyID, "Song", "Artist", "Album", 190000)}, ""},
		{"album disambiguates", []*catalog.Track{duplicate, good}, cachedSpotifyID},
		{"duplicates same ID", []*catalog.Track{good, good}, cachedSpotifyID},
		{"same album closest duration", []*catalog.Track{good, matchCandidate(duplicate.ID, "Song", "Artist", "Album", 180000)}, duplicate.ID},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := selectSpotifyRecording(song, test.candidates)
			if test.want == "" {
				if got != nil {
					t.Fatalf("unexpected match: %+v", got)
				}
			} else if got == nil || got.ID != test.want {
				t.Fatalf("match: %+v", got)
			}
		})
	}
	song.Duration = 0
	if selectSpotifyRecording(song, []*catalog.Track{good}) != nil {
		t.Fatal("matched without duration")
	}
	song.Duration = 180
	duet := matchCandidate(cachedSpotifyID, "Song", "Other", "Album", 180000)
	duet.Artists = append(duet.Artists, catalog.SimpleArtist{Name: "Artist"})
	if selectSpotifyRecording(song, []*catalog.Track{duet}) != nil {
		t.Fatal("secondary credit matched a different primary artist")
	}
	song.Artist = "Other & Artist"
	if selectSpotifyRecording(song, []*catalog.Track{duet}) == nil {
		t.Fatal("combined artist credits did not match")
	}
}

func TestSpotifySearchEnrichmentBindsAndReusesRecording(t *testing.T) {
	for _, scenario := range []string{"match", "changed source", "changed during features", "search failure", "rate limited", "multiple releases", "feature failure"} {
		t.Run(scenario, func(t *testing.T) {
			logDirectory := t.TempDir()
			if err := logger.Init(logDirectory); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(logger.Close)
			a, path := newBPMRouteTestAPI(t, false)
			song, _ := a.db.GetSongByID("song")
			song.Duration = 180
			if err := a.db.SaveSong(song); err != nil {
				t.Fatal(err)
			}
			source, _ := analysis.ResolveLocalSource(a.db, "song")
			provider := &spotifyRefreshFixtureProvider{fn: func(_ context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
				if scenario == "changed during features" {
					if err := os.WriteFile(path, []byte("replacement audio during feature lookup"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "feature failure" {
					return spotifyanalysis.Observation{}, &spotifyanalysis.Error{Code: spotifyanalysis.AnalysisUnavailable}
				}
				bpm := 123.456
				return spotifyanalysis.Observation{TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now().UTC(), BPM: &bpm}, nil
			}}
			service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			if err := a.InstallSpotifyAnalysisService(service); err != nil {
				t.Fatal(err)
			}
			searches := 0
			a.spotifyTrackSearch = func(_ context.Context, query string) ([]*catalog.Track, error) {
				searches++
				if query != "Song Artist" {
					t.Fatal(query)
				}
				if scenario == "rate limited" {
					return nil, &spotifyauth.WebPlayerHTTPError{Status: 429, RetryAfter: 90 * time.Second}
				}
				if scenario == "search failure" {
					return nil, errors.New("unavailable")
				}
				if scenario == "changed source" {
					if err := os.WriteFile(path, []byte("different source audio"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				candidates := []*catalog.Track{matchCandidate(cachedSpotifyID, "Song", "Artist", "Album", 180000)}
				if scenario == "multiple releases" {
					candidates = append(candidates, matchCandidate("0123456789012345678901", "Song", "Artist", "Album", 180000))
				}
				return candidates, nil
			}
			o := a.spotifyFeaturesForSource(t.Context(), source)
			contents, err := os.ReadFile(filepath.Join(logDirectory, "scan.log"))
			if err != nil {
				t.Fatal(err)
			}
			expected := map[string]string{"match": "reason=automatic_match", "changed source": "reason=source_changed_or_unavailable", "changed during features": "reason=source_changed_or_unavailable", "search failure": "reason=search_failed", "rate limited": "reason=search_rate_limited http_status=429 retry_after_seconds=90", "multiple releases": "reason=album_match_ranked", "feature failure": "spotify_" + string(spotifyanalysis.AnalysisUnavailable)}[scenario]
			if !strings.Contains(string(contents), expected) {
				t.Fatalf("missing reason %q: %s", expected, contents)
			}
			if scenario != "match" && scenario != "multiple releases" {
				if o != nil {
					t.Fatal("unsafe/unavailable result")
				}
				return
			}
			if o == nil || *o.BPM != 123.456 {
				t.Fatalf("features: %+v", o)
			}
			link, err := a.db.GetSpotifyRecording("song", source.Fingerprint)
			if err != nil || link == nil || link.LinkOrigin != "automatic_search" {
				t.Fatalf("identity: %+v %v", link, err)
			}
			if a.spotifyFeaturesForSource(t.Context(), source) == nil || searches != 1 || provider.calls.Load() != 1 {
				t.Fatal("match/cache not reused")
			}
			if err := a.db.DeleteSpotifyRecording("song"); err != nil {
				t.Fatal(err)
			}
			if a.spotifyFeaturesForSource(t.Context(), source) != nil || searches != 1 {
				t.Fatal("removed match searched again")
			}
		})
	}
}

func TestSpotifyRecordingMatcherCompilationReleases(t *testing.T) {
	song := db.Song{Title: "Better Off Alone", Artist: "Alice Deejay", Album: "100 Most Iconic EDM Songs", Duration: 214.883}
	canonical := matchCandidate("5XVjNRubJUW0iPhhSWpLCj", "Better Off Alone", "Alice Deejay", "Who Needs Guitars Anyway?", 214880)
	duplicate := matchCandidate("0123456789012345678901", song.Title, song.Artist, "Dance Hits", 216000)
	for _, candidates := range [][]*catalog.Track{{duplicate, canonical}, {canonical, duplicate}} {
		got, details := selectSpotifyRecordingWithDetails(song, candidates)
		if got != canonical || details.eligible != 2 || details.reason != "matching_release_ranked" {
			t.Fatalf("compilation match: %+v %+v", got, details)
		}
	}
	// Equivalent release ties retain Spotify's search ranking.
	duplicate.DurationMS = canonical.DurationMS
	if got := selectSpotifyRecording(song, []*catalog.Track{canonical, duplicate}); got != canonical {
		t.Fatal("search ranking ignored")
	}
	if got := selectSpotifyRecording(song, []*catalog.Track{duplicate, canonical}); got != duplicate {
		t.Fatal("search ranking ignored")
	}
}

func TestSpotifyRecordingMatchDiagnostics(t *testing.T) {
	song := db.Song{Title: "Song", Artist: "Artist", Duration: 180}
	_, empty := selectSpotifyRecordingWithDetails(song, nil)
	if empty.reason != "no_search_candidates" {
		t.Fatal(empty)
	}
	wrongTitle := matchCandidate(cachedSpotifyID, "Song (Live)", "Artist", "Album", 180000)
	_, details := selectSpotifyRecordingWithDetails(song, []*catalog.Track{
		wrongTitle, wrongTitle, nil,
		matchCandidate("0123456789012345678901", "Song", "Other", "Album", 180000),
		matchCandidate("1123456789012345678901", "Song", "Artist", "Album", 190000),
	})
	if details.reason != "no_qualifying_recording" || details.title != 1 || details.artist != 1 || details.duration != 1 || details.invalid != 1 || details.duplicate != 1 || details.eligible != 0 {
		t.Fatalf("diagnostics: %+v", details)
	}
}
