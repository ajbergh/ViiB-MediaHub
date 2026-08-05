package scanner

import (
	"fmt"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

type ContinuousWatchStatus struct {
	Running      bool  `json:"running"`
	IntervalMS   int64 `json:"intervalMs"`
	LastCheckAt  int64 `json:"lastCheckAt,omitempty"`
	LastChanges  int   `json:"lastChanges"`
	LastError    string `json:"lastError,omitempty"`
	Checks       int64 `json:"checks"`
}

type continuousWatcher struct {
	mu       sync.RWMutex
	status   ContinuousWatchStatus
	stop     chan struct{}
	done     chan struct{}
}

var continuousWatchers sync.Map // map[*Scanner]*continuousWatcher

func (s *Scanner) StartContinuousWatcher(interval time.Duration) (ContinuousWatchStatus, error) {
	if interval < 2*time.Second { interval = 2 * time.Second }
	if interval > time.Hour { interval = time.Hour }
	if existing, ok := continuousWatchers.Load(s); ok {
		watcher := existing.(*continuousWatcher)
		watcher.mu.RLock(); status := watcher.status; watcher.mu.RUnlock()
		return status, nil
	}
	watcher := &continuousWatcher{
		status: ContinuousWatchStatus{Running: true, IntervalMS: interval.Milliseconds()},
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	actual, loaded := continuousWatchers.LoadOrStore(s, watcher)
	if loaded {
		existing := actual.(*continuousWatcher)
		existing.mu.RLock(); status := existing.status; existing.mu.RUnlock()
		return status, nil
	}
	go s.runContinuousWatcher(watcher, interval)
	return watcher.status, nil
}

func (s *Scanner) runContinuousWatcher(watcher *continuousWatcher, interval time.Duration) {
	defer close(watcher.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-watcher.stop:
			watcher.mu.Lock(); watcher.status.Running = false; watcher.mu.Unlock()
			return
		case <-s.ctx.Done():
			watcher.mu.Lock(); watcher.status.Running = false; watcher.mu.Unlock()
			return
		case <-ticker.C:
			if s.IsScanning() { continue }
			quick, err := s.QuickStartup()
			changes := 0
			lastError := ""
			if err != nil {
				lastError = err.Error()
				logger.Scanner("Continuous watcher check failed: %v", err)
			} else {
				deleted, deleteErr := s.DetectDeletedFiles()
				if deleteErr != nil { lastError = deleteErr.Error() } else { quick.ChangedFiles = append(quick.ChangedFiles, deleted...) }
				quick.ChangedFiles = coalesceFileChanges(quick.ChangedFiles)
				changes = len(quick.ChangedFiles)
				if changes > 0 { s.QueueChangesForBackground(quick.ChangedFiles) }
			}
			watcher.mu.Lock()
			watcher.status.LastCheckAt = time.Now().UnixMilli()
			watcher.status.LastChanges = changes
			watcher.status.LastError = lastError
			watcher.status.Checks++
			watcher.mu.Unlock()
		}
	}
}

func (s *Scanner) StopContinuousWatcher() ContinuousWatchStatus {
	value, ok := continuousWatchers.LoadAndDelete(s)
	if !ok { return ContinuousWatchStatus{} }
	watcher := value.(*continuousWatcher)
	close(watcher.stop)
	<-watcher.done
	watcher.mu.RLock(); status := watcher.status; watcher.mu.RUnlock()
	return status
}

func (s *Scanner) ContinuousWatcherStatus() ContinuousWatchStatus {
	value, ok := continuousWatchers.Load(s)
	if !ok { return ContinuousWatchStatus{} }
	watcher := value.(*continuousWatcher)
	watcher.mu.RLock(); defer watcher.mu.RUnlock()
	return watcher.status
}

func ParseWatchInterval(milliseconds int64) (time.Duration, error) {
	if milliseconds < 2000 || milliseconds > int64(time.Hour/time.Millisecond) {
		return 0, fmt.Errorf("intervalMs must be between 2000 and %d", int64(time.Hour/time.Millisecond))
	}
	return time.Duration(milliseconds) * time.Millisecond, nil
}
