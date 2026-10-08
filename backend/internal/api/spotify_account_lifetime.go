package api

import (
	"context"
	"sync"
)

type spotifyLifetimeRegistryKey struct{}
type spotifyLifetimeRegistry struct {
	mu       sync.Mutex
	closed   bool
	requests map[*int]context.CancelCauseFunc
}

// Requests remain direct children of their ordinary parent, preserving parent
// cancellation and values. The registry makes account retirement synchronous too.
func newSpotifyAccountLifetime() (context.Context, context.CancelFunc) {
	base, cancel := context.WithCancelCause(context.Background())
	registry := &spotifyLifetimeRegistry{requests: make(map[*int]context.CancelCauseFunc)}
	lifetime := context.WithValue(base, spotifyLifetimeRegistryKey{}, registry)
	return lifetime, func() {
		registry.mu.Lock()
		registry.closed = true
		requests := registry.requests
		registry.requests = make(map[*int]context.CancelCauseFunc)
		for _, request := range requests {
			request(errSpotifyAccountChanged)
		}
		cancel(errSpotifyAccountChanged)
		registry.mu.Unlock()
	}
}

func registerSpotifyAccountRequest(lifetime context.Context, cancel context.CancelCauseFunc) func() {
	registry, ok := lifetime.Value(spotifyLifetimeRegistryKey{}).(*spotifyLifetimeRegistry)
	if !ok {
		return func() {}
	}
	// A non-zero-size token guarantees distinct keys for simultaneous requests.
	token := new(int)
	registry.mu.Lock()
	if registry.closed {
		registry.mu.Unlock()
		cancel(errSpotifyAccountChanged)
		return func() {}
	}
	registry.requests[token] = cancel
	registry.mu.Unlock()
	return func() { registry.mu.Lock(); delete(registry.requests, token); registry.mu.Unlock() }
}
