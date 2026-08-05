package scanner

import (
	"path/filepath"
	"sync"
	"time"
)

type changeCoordinator struct {
	mu      sync.Mutex
	pending map[string]FileChange
	order   []string
	timer   *time.Timer
}

var changeCoordinators sync.Map // map[*Scanner]*changeCoordinator

func (s *Scanner) queueCoalescedBackgroundChanges(changes []FileChange) {
	if s.backgroundScanner == nil || len(changes) == 0 { return }
	value, _ := changeCoordinators.LoadOrStore(s, &changeCoordinator{pending: make(map[string]FileChange)})
	coordinator := value.(*changeCoordinator)
	coordinator.mu.Lock()
	for _, change := range coalesceFileChanges(changes) {
		path := filepath.Clean(change.Path)
		change.Path = path
		if previous, exists := coordinator.pending[path]; exists {
			merged := coalesceFileChanges([]FileChange{previous, change})
			if len(merged) == 1 { coordinator.pending[path] = merged[0] }
			continue
		}
		coordinator.pending[path] = change
		coordinator.order = append(coordinator.order, path)
	}
	if coordinator.timer != nil {
		coordinator.timer.Reset(250 * time.Millisecond)
		coordinator.mu.Unlock()
		return
	}
	coordinator.timer = time.AfterFunc(250*time.Millisecond, func() {
		coordinator.mu.Lock()
		batch := make([]FileChange, 0, len(coordinator.pending))
		for _, path := range coordinator.order {
			if change, ok := coordinator.pending[path]; ok { batch = append(batch, change) }
		}
		coordinator.pending = make(map[string]FileChange)
		coordinator.order = coordinator.order[:0]
		coordinator.timer = nil
		coordinator.mu.Unlock()

		if len(batch) == 0 || s.backgroundScanner == nil { return }
		priority := PriorityHigh
		if len(batch) > 100 { priority = PriorityNormal }
		s.backgroundScanner.QueueFileTasks(batch, priority)
	})
	coordinator.mu.Unlock()
}

func (s *Scanner) stopChangeCoordinator() {
	value, ok := changeCoordinators.LoadAndDelete(s)
	if !ok { return }
	coordinator := value.(*changeCoordinator)
	coordinator.mu.Lock()
	if coordinator.timer != nil { coordinator.timer.Stop() }
	coordinator.pending = nil
	coordinator.order = nil
	coordinator.timer = nil
	coordinator.mu.Unlock()
}
