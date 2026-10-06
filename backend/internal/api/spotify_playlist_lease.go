package api

import (
	"context"
	"errors"
	"sync"
)

type playlistTraversalContextKey struct{}

type playlistLeaseKey struct {
	lifetime context.Context
	id       string
}

type playlistLease struct {
	gate  chan struct{}
	users int
}

// Serialize a playlist's root lookup, checkpoints and final publication within
// one account lifetime. Independent playlists and replacement accounts proceed
// independently; canceled waiters never retain an idle map entry.
func (s *spotifyAuthRuntime) acquirePlaylistTraversal(ctx context.Context, id string) (func(), error) {
	lifetime, ok := ctx.Value(spotifyAccountContextKey{}).(context.Context)
	if !ok {
		return nil, errors.New("playlist traversal lacks account context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := playlistLeaseKey{lifetime, id}
	s.playlistMu.Lock()
	if s.playlistLeases == nil {
		s.playlistLeases = make(map[playlistLeaseKey]*playlistLease)
	}
	lease := s.playlistLeases[key]
	if lease == nil {
		if len(s.playlistLeases) >= 128 {
			s.playlistMu.Unlock()
			return nil, errors.New("playlist traversal queue full")
		}
		lease = &playlistLease{gate: make(chan struct{}, 1)}
		s.playlistLeases[key] = lease
	}
	lease.users++
	s.playlistMu.Unlock()
	drop := func() {
		s.playlistMu.Lock()
		defer s.playlistMu.Unlock()
		lease.users--
		if lease.users == 0 {
			delete(s.playlistLeases, key)
		}
	}
	select {
	case lease.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-lease.gate
			drop()
			return nil, err
		}
		var once sync.Once
		return func() { once.Do(func() { <-lease.gate; drop() }) }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}
