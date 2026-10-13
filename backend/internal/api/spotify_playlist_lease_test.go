package api

import (
	"context"
	"testing"
	"time"
)

func TestPlaylistTraversalLeaseCancellationAndCleanup(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	s := a.spotifyTokens()
	ctx, cancel := s.requestContext(t.Context())
	defer cancel()
	release, err := s.acquirePlaylistTraversal(ctx, "playlist")
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.acquirePlaylistTraversal(ctx, "other")
	if err != nil {
		t.Fatal(err)
	}
	other()
	waiter, stop := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		unlock, err := s.acquirePlaylistTraversal(waiter, "playlist")
		if unlock != nil {
			unlock()
		}
		result <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		s.playlistMu.Lock()
		users := s.playlistLeases[playlistLeaseKey{ctx.Value(spotifyAccountContextKey{}).(context.Context), "playlist"}].users
		s.playlistMu.Unlock()
		if users == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waiter did not queue")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-result:
		t.Fatal("same playlist overlapped")
	default:
	}
	stop()
	if err := <-result; err != context.Canceled {
		t.Fatal("waiter did not cancel", err)
	}
	release()
	release()
	s.playlistMu.Lock()
	remaining := len(s.playlistLeases)
	s.playlistMu.Unlock()
	if remaining != 0 {
		t.Fatal("idle leases leaked", remaining)
	}
}

func TestPlaylistTraversalLeaseRetirementCancelsWaiter(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	s := a.spotifyTokens()
	ctx, cancel := s.requestContext(t.Context())
	defer cancel()
	release, err := s.acquirePlaylistTraversal(ctx, "playlist")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result := make(chan error, 1)
	go func() {
		unlock, err := s.acquirePlaylistTraversal(ctx, "playlist")
		if unlock != nil {
			unlock()
		}
		result <- err
	}()
	s.beginRetirement()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("retired waiter acquired")
		}
	case <-time.After(time.Second):
		t.Fatal("retirement did not cancel waiter")
	}
}
