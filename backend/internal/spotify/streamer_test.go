package spotify

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestStreamCapacityReservationIsAtomicAndReleasedOnFailure(t *testing.T) {
	streamer := NewStreamer(&SessionManager{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	releases := []func(){}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			release, err := streamer.reserveStream(fmt.Sprint(i))
			if err == nil {
				mu.Lock()
				releases = append(releases, release)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if len(releases) != 5 || streamer.GetActiveStreamCount() != 5 {
		t.Fatalf("capacity exceeded: %d", len(releases))
	}
	for _, release := range releases {
		release()
		release()
	}
	if streamer.GetActiveStreamCount() != 0 {
		t.Fatal("capacity leaked")
	}
	if _, err := streamer.StreamTrackWithQuality(context.Background(), "track", "failure", "high"); err == nil {
		t.Fatal("uninitialized session accepted")
	}
	if streamer.GetActiveStreamCount() != 0 {
		t.Fatal("failed preparation leaked capacity")
	}
}
func TestStreamCloseReleasesCapacityExactlyOnce(t *testing.T) {
	streamer := NewStreamer(nil)
	release, err := streamer.reserveStream("request")
	if err != nil {
		t.Fatal(err)
	}
	stream := &ActiveStream{release: release}
	streamer.mu.Lock()
	streamer.activeStreams["request"] = stream
	streamer.mu.Unlock()
	streamer.CloseAllStreams()
	stream.Close()
	if streamer.GetActiveStreamCount() != 0 {
		t.Fatal("closed stream retained capacity")
	}
}
