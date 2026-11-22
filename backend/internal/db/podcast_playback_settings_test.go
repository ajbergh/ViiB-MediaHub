package db

import (
    "context"
    "database/sql"
    "errors"
    "path/filepath"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastShowAndGlobalPlaybackSettings(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "playback-settings.db")
    d, err := New(dbPath)
    if err != nil {
        t.Fatal(err)
    }
    ctx := context.Background()

    globalDefaults, err := d.GetPodcastGlobalPlaybackSettings(ctx)
    if err != nil || globalDefaults != (PodcastGlobalPlaybackSettings{
        DefaultSpeed: 1, SkipBackSeconds: 15, SkipForwardSeconds: 30,
    }) {
        t.Fatalf("unexpected initial global settings: %+v %v", globalDefaults, err)
    }

    if _, err := d.UpdatePodcastGlobalPlaybackSettings(ctx, PodcastGlobalPlaybackSettings{
        DefaultSpeed: 0, SkipBackSeconds: 15, SkipForwardSeconds: 30,
    }); err == nil {
        t.Fatal("accepted 0 speed for global default")
    }
    if _, err := d.UpdatePodcastGlobalPlaybackSettings(ctx, PodcastGlobalPlaybackSettings{
        DefaultSpeed: 1.5, SkipBackSeconds: 2, SkipForwardSeconds: 30,
    }); err == nil {
        t.Fatal("accepted out-of-range skipBackSeconds")
    }
    savedGlobal, err := d.UpdatePodcastGlobalPlaybackSettings(ctx, PodcastGlobalPlaybackSettings{
        DefaultSpeed: 1.25, SkipBackSeconds: 10, SkipForwardSeconds: 45,
    })
    if err != nil || savedGlobal.DefaultSpeed != 1.25 || savedGlobal.SkipBackSeconds != 10 || savedGlobal.SkipForwardSeconds != 45 {
        t.Fatalf("save global failed: %+v %v", savedGlobal, err)
    }

    feedID, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
        URL:   "https://example.org/show.xml",
        Title: "Show One",
        Episodes: []podcast.Episode{{
            GUID:          "ep-1",
            Title:         "Episode 1",
            EnclosureURL:  "https://media.example.org/ep1.mp3",
            EnclosureType: "audio/mpeg",
        }},
    }, true)
    if err != nil {
        t.Fatal(err)
    }

    showDefaults, err := d.GetPodcastShowPlaybackSettings(ctx, feedID)
    if err != nil || showDefaults != (PodcastShowPlaybackSettings{DefaultSpeed: 0, SkipIntroSeconds: 0, SkipOutroSeconds: 0}) {
        t.Fatalf("unexpected initial show settings: %+v %v", showDefaults, err)
    }

    if _, err := d.UpdatePodcastShowPlaybackSettings(ctx, feedID, PodcastShowPlaybackSettings{
        DefaultSpeed: 1.5, SkipIntroSeconds: 601, SkipOutroSeconds: 0,
    }); err == nil {
        t.Fatal("accepted skipIntroSeconds > 600")
    }
    if _, err := d.UpdatePodcastShowPlaybackSettings(ctx, "missing-feed", PodcastShowPlaybackSettings{
        DefaultSpeed: 1.5, SkipIntroSeconds: 15, SkipOutroSeconds: 30,
    }); !errors.Is(err, sql.ErrNoRows) {
        t.Fatalf("accepted unfollowed feed: %v", err)
    }

    if _, err := d.UpdatePodcastShowPlaybackSettings(ctx, feedID, PodcastShowPlaybackSettings{
        DefaultSpeed: 1.75, SkipIntroSeconds: 30, SkipOutroSeconds: 45,
    }); err != nil {
        t.Fatal(err)
    }

    // Seed progress so Continue Listening also carries show playback defaults.
    eps, err := d.ListPodcastEpisodes(ctx, feedID, 10)
    if err != nil || len(eps) != 1 {
        t.Fatalf("list episodes: %+v %v", eps, err)
    }
    if _, err := d.SetPodcastProgress(ctx, eps[0].ID, 5000, 5000, 0, false); err != nil {
        t.Fatal(err)
    }

    if err := d.Close(); err != nil {
        t.Fatal(err)
    }

    reopened, err := New(dbPath)
    if err != nil {
        t.Fatal(err)
    }
    defer reopened.Close()

    reloadedGlobal, err := reopened.GetPodcastGlobalPlaybackSettings(ctx)
    if err != nil || reloadedGlobal != savedGlobal {
        t.Fatalf("reloaded global mismatch: %+v %v", reloadedGlobal, err)
    }
    feedRecord, err := reopened.GetSubscribedPodcast(ctx, feedID)
    if err != nil || feedRecord.DefaultSpeed != 1.75 || feedRecord.SkipIntroSeconds != 30 || feedRecord.SkipOutroSeconds != 45 {
        t.Fatalf("feed record missing playback settings after reopen: %+v %v", feedRecord, err)
    }
    continues, err := reopened.ListContinueListening(ctx, 10)
    if err != nil || len(continues) != 1 || continues[0].DefaultSpeed != 1.75 || continues[0].SkipOutroSeconds != 45 {
        t.Fatalf("continue listening missing playback settings: %+v %v", continues, err)
    }
}
