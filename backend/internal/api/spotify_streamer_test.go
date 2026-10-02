package api

import (
	"github.com/ajbergh/viib-mediahub/internal/spotify"
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
