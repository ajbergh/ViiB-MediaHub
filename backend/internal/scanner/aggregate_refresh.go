package scanner

import (
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

type aggregateRefreshState struct {
	mu      sync.Mutex
	timer   *time.Timer
	pending bool
}

var aggregateRefreshes sync.Map // map[*Scanner]*aggregateRefreshState

func (s *Scanner) scheduleAggregateRefresh() {
	value, _ := aggregateRefreshes.LoadOrStore(s, &aggregateRefreshState{})
	state := value.(*aggregateRefreshState)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.pending = true
	if state.timer != nil {
		state.timer.Reset(750 * time.Millisecond)
		return
	}
	state.timer = time.AfterFunc(750*time.Millisecond, func() {
		state.mu.Lock()
		if !state.pending {
			state.timer = nil
			state.mu.Unlock()
			return
		}
		state.pending = false
		state.timer = nil
		state.mu.Unlock()

		if err := s.db.UpdateGenreStats(); err != nil {
			logger.Scanner("Failed to refresh genre aggregates: %v", err)
		}
		if err := s.db.CheckpointWAL(); err != nil {
			logger.Scanner("WAL checkpoint after aggregate refresh failed: %v", err)
		}
	})
}

func (s *Scanner) stopAggregateRefresh() {
	value, ok := aggregateRefreshes.LoadAndDelete(s)
	if !ok { return }
	state := value.(*aggregateRefreshState)
	state.mu.Lock()
	if state.timer != nil { state.timer.Stop() }
	state.pending = false
	state.timer = nil
	state.mu.Unlock()
}
