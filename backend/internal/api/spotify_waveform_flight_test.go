package api

import (
	"context"
	"errors"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaveformFlightIndependentWaiterCancellation(t *testing.T) {
	s := newSpotifyAuthRuntime(nil, spotifyauth.WebPlayerOptions{})
	ctx, cancel := context.WithCancel(t.Context())
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	fetch := func(ctx context.Context) (map[string]any, time.Duration, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return map[string]any{"state": "available"}, 0, nil
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
	}
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { _, _, err := s.shareWaveform(ctx, "recording", fetch); first <- err }()
	<-started
	go func() { _, _, err := s.shareWaveform(t.Context(), "recording", fetch); second <- err }()
	deadline := time.Now().Add(time.Second)
	for {
		s.waveformMu.Lock()
		count := 0
		for _, f := range s.waveformFlights {
			count = f.waiters
		}
		s.waveformMu.Unlock()
		if count == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second waiter did not join")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("duplicate provider call")
	}
}

func TestWaveformFlightLastWaiterAndRetirementCancel(t *testing.T) {
	for _, retire := range []bool{false, true} {
		s := newSpotifyAuthRuntime(nil, spotifyauth.WebPlayerOptions{})
		ctx, cancel := context.WithCancel(t.Context())
		started, stopped := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			_, _, err := s.shareWaveform(ctx, "recording", func(ctx context.Context) (map[string]any, time.Duration, error) {
				close(started)
				<-ctx.Done()
				close(stopped)
				return map[string]any{"state": "available"}, 0, nil
			})
			done <- err
		}()
		<-started
		if retire {
			s.beginRetirement()
		} else {
			cancel()
		}
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("shared operation was not canceled")
		}
		if err := <-done; err == nil {
			t.Fatal("canceled result accepted")
		}
		cancel()
	}
}
