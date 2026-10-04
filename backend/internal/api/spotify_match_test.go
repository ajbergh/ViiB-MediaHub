package api

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
)

func matchCandidate(id, title, artist, album string, duration int) *catalog.Track {
	return &catalog.Track{ID: id, Name: title, Artists: []catalog.SimpleArtist{{Name: artist}}, Album: catalog.AlbumSummary{Name: album}, DurationMS: duration}
}

func TestSpotifyRecordingMatcherRejectsWrongAndAmbiguousVersions(t *testing.T) {
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
		{"ambiguous same album", []*catalog.Track{good, matchCandidate(duplicate.ID, "Song", "Artist", "Album", 180000)}, ""},
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
	for _, scenario := range []string{"match", "changed source", "changed during features", "search failure", "ambiguous", "feature failure"} {
		t.Run(scenario, func(t *testing.T) {
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
				if scenario == "search failure" {
					return nil, errors.New("unavailable")
				}
				if scenario == "changed source" {
					if err := os.WriteFile(path, []byte("different source audio"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				candidates := []*catalog.Track{matchCandidate(cachedSpotifyID, "Song", "Artist", "Album", 180000)}
				if scenario == "ambiguous" {
					candidates = append(candidates, matchCandidate("0123456789012345678901", "Song", "Artist", "Album", 180000))
				}
				return candidates, nil
			}
			o := a.spotifyFeaturesForSource(t.Context(), source)
			if scenario != "match" {
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
