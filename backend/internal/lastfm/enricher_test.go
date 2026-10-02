package lastfm

import (
	"context"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"path/filepath"
	"testing"
	"time"
)

func TestTrackInfoUnknownTagCountsEnrichGenre(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	song := db.Song{ID: "song", Title: "Song", Artist: "Artist", FilePath: filepath.Join(t.TempDir(), "song.mp3"), AddedAt: 1}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	client := NewClient("fixture-key", "")
	client.cache.Set("track:artist:song", &TrackInfo{TopTags: []TagWithCount{{Name: "rock"}, {Name: "jazz", Count: 1}}})
	result, err := NewEnricher(client, database).EnrichSongs(context.Background(), []db.Song{song}, EnrichOptions{MinTagCount: 30})
	if err != nil || result.Enriched != 1 {
		t.Fatalf("enrichment: %+v %v", result, err)
	}
	updated, err := database.GetSongByID("song")
	if err != nil || len(updated.Genre) != 1 || updated.Genre[0] != "Rock" {
		t.Fatalf("missing counted-endpoint contract: %+v %v", updated, err)
	}
}

type delayedClient struct {
	*Client
	started, release chan struct{}
}

func (client *delayedClient) GetTrackInfo(ctx context.Context, artist, title string) (*TrackInfo, error) {
	close(client.started)
	<-client.release
	return nil, ctx.Err()
}
func TestCancellationJoinsWorkersAndCountsFailure(t *testing.T) {
	client := &delayedClient{Client: NewClient("", ""), started: make(chan struct{}), release: make(chan struct{})}
	enricher := &Enricher{client: client}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	var result *EnrichResult
	var err error
	go func() {
		result, err = enricher.EnrichSongs(ctx, []db.Song{{ID: "one"}, {ID: "two"}}, EnrichOptions{MaxConcurrency: 1})
		close(done)
	}()
	<-client.started
	cancel()
	select {
	case <-done:
		t.Fatal("returned while worker was writing")
	case <-time.After(20 * time.Millisecond):
	}
	close(client.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not release semaphore waiter")
	}
	if !errors.Is(err, context.Canceled) || result.Errors != 1 || result.Enriched != 0 || result.Processed != 1 {
		t.Fatalf("wrong cancellation result: %+v %v", result, err)
	}
}
