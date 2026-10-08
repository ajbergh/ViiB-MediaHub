package api

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAccountLifetimeCancelsEveryRequestSynchronously(t *testing.T) {
	for i := 0; i < 1000; i++ {
		lifetime, end := newSpotifyAccountLifetime()
		runtime := &spotifyAuthRuntime{lifetime: lifetime, endLifetime: end}
		first, releaseFirst := runtime.requestContext(t.Context())
		second, releaseSecond := runtime.requestContext(first)
		independent, releaseIndependent := runtime.requestContext(t.Context())
		end()
		for _, ctx := range []context.Context{first, second, independent} {
			if ctx.Err() == nil || !errors.Is(context.Cause(ctx), errSpotifyAccountChanged) {
				t.Fatalf("account retirement returned before cancellation: %v %v", ctx.Err(), context.Cause(ctx))
			}
		}
		releaseFirst()
		releaseSecond()
		releaseIndependent()
	}
}

func TestAccountRequestPreservesParentValuesDeadlineAndCancellation(t *testing.T) {
	lifetime, end := newSpotifyAccountLifetime()
	defer end()
	runtime := &spotifyAuthRuntime{lifetime: lifetime, endLifetime: end}
	type key struct{}
	parent, cancel := context.WithTimeout(context.WithValue(t.Context(), key{}, "capture"), time.Hour)
	ctx, release := runtime.requestContext(parent)
	defer release()
	if ctx.Value(key{}) != "capture" || ctx.Value(spotifyAccountContextKey{}) != lifetime {
		t.Fatal("request values lost")
	}
	expected, _ := parent.Deadline()
	actual, ok := ctx.Deadline()
	if !ok || !actual.Equal(expected) {
		t.Fatal("deadline lost")
	}
	cancel()
	if ctx.Err() != context.Canceled || context.Cause(ctx) != context.Canceled {
		t.Fatal("parent cancellation delayed or rewritten")
	}
}
