package api

import (
	"context"
	"errors"
	"time"
)

type waveformFlightKey struct {
	lifetime context.Context
	id       string
}
type waveformFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	result  map[string]any
	retry   time.Duration
	err     error
}

func (s *spotifyAuthRuntime) shareWaveform(parent context.Context, id string, fetch func(context.Context) (map[string]any, time.Duration, error)) (map[string]any, time.Duration, error) {
	waiter, cancelWaiter := s.requestContext(parent)
	defer cancelWaiter()
	if err := waiter.Err(); err != nil {
		return nil, 0, err
	}
	key := waveformFlightKey{waiter.Value(spotifyAccountContextKey{}).(context.Context), id}
	s.waveformMu.Lock()
	if s.waveformFlights == nil {
		s.waveformFlights = make(map[waveformFlightKey]*waveformFlight)
	}
	f := s.waveformFlights[key]
	if f == nil {
		if len(s.waveformFlights) >= 32 {
			s.waveformMu.Unlock()
			return nil, 0, errors.New("waveform request queue full")
		}
		// Keep account and admission values but detach individual waiter cancellation.
		root, stopAccount := s.requestContext(context.WithoutCancel(waiter))
		work, stopTimeout := context.WithTimeout(root, 25*time.Second)
		f = &waveformFlight{done: make(chan struct{}), cancel: func() { stopTimeout(); stopAccount() }}
		s.waveformFlights[key] = f
		go func(f *waveformFlight) {
			result, retry, err := fetch(work)
			if work.Err() != nil {
				result, retry, err = nil, 0, work.Err()
			}
			f.cancel()
			s.waveformMu.Lock()
			f.result, f.retry, f.err = result, retry, err
			delete(s.waveformFlights, key)
			close(f.done)
			s.waveformMu.Unlock()
		}(f)
	}
	f.waiters++
	s.waveformMu.Unlock()
	select {
	case <-f.done:
		if err := waiter.Err(); err != nil {
			return nil, 0, err
		}
		// Each caller owns its response map.
		result := make(map[string]any, len(f.result))
		for k, v := range f.result {
			result[k] = v
		}
		return result, f.retry, f.err
	case <-waiter.Done():
		s.waveformMu.Lock()
		if s.waveformFlights[key] == f {
			f.waiters--
			if f.waiters == 0 {
				f.cancel()
			}
		}
		s.waveformMu.Unlock()
		return nil, 0, waiter.Err()
	}
}
