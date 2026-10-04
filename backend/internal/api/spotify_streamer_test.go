package api

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/spotify"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentHTTPStreamDependenciesShareCapacityOwner(t *testing.T) {
	api := &API{}
	manager := &spotify.SessionManager{}
	var wg sync.WaitGroup
	owners := make(chan *spotify.Streamer, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); owners <- api.streamerForSession(manager) }()
	}
	wg.Wait()
	close(owners)
	var first *spotify.Streamer
	for owner := range owners {
		if first == nil {
			first = owner
		}
		if owner != first {
			t.Fatal("HTTP requests have separate capacity owners")
		}
	}
	if api.streamerForSession(&spotify.SessionManager{}) == first {
		t.Fatal("changed session reused old streamer")
	}
}

func TestRetiredHTTPStreamerIsReplacedForSameSessionManager(t *testing.T) {
	api := &API{}
	manager := &spotify.SessionManager{}
	retired := api.streamerForSession(manager)
	api.cancelSpotifyMedia()
	replacement := api.streamerForSession(manager)
	if replacement == retired {
		t.Fatal("reconnected session reused retired streamer")
	}
	if api.streamerForSession(manager) != replacement {
		t.Fatal("reconnected requests do not share a capacity owner")
	}
	_, err := retired.StreamTrack(context.Background(), "5r9W9MJLvHk83fcZSPQ8SE", "retired")
	if err == nil || !strings.Contains(err.Error(), "streamer closed") {
		t.Fatalf("retired streamer accepted preparation: %v", err)
	}
	_, err = replacement.StreamTrack(context.Background(), "5r9W9MJLvHk83fcZSPQ8SE", "replacement")
	if err == nil || strings.Contains(err.Error(), "streamer closed") {
		t.Fatalf("replacement should require session initialization, not reject retirement: %v", err)
	}
}
