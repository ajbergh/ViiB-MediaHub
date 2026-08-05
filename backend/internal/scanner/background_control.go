// background_control.go exposes lifecycle and diagnostics controls for queued scanner work.
package scanner

import "github.com/ajbergh/viib-mediahub/internal/logger"

// PauseBackgroundWork pauses consumption of queued background tasks.
func (s *Scanner) PauseBackgroundWork() {
	if s.backgroundScanner != nil { s.backgroundScanner.Pause() }
}

// ResumeBackgroundWork resumes consumption of queued background tasks.
func (s *Scanner) ResumeBackgroundWork() {
	if s.backgroundScanner != nil { s.backgroundScanner.Resume() }
}

// IsBackgroundPaused reports whether background-task consumption is paused.
func (s *Scanner) IsBackgroundPaused() bool {
	if s.backgroundScanner != nil { return s.backgroundScanner.IsPaused() }
	return true
}

// GetBackgroundStats returns processed/error counts, queue depth, and active workers.
func (s *Scanner) GetBackgroundStats() (processed, errors int64, queueSize int, activeWorkers int32) {
	if s.backgroundScanner != nil { return s.backgroundScanner.Stats() }
	return 0, 0, 0, 0
}

// QueueChangesForBackground debounces and coalesces paths before they enter the worker queue.
func (s *Scanner) QueueChangesForBackground(changes []FileChange) {
	s.queueCoalescedBackgroundChanges(changes)
}

// StartBackgroundValidation schedules a low-priority sample of known files for validation.
func (s *Scanner) StartBackgroundValidation() {
	if s.backgroundScanner == nil { return }
	sample, err := s.db.GetRandomFileSample(100)
	if err != nil {
		logger.Scanner("Cannot get file sample for validation: %v", err)
		return
	}
	logger.Scanner("Starting background validation of %d files", len(sample))
	for _, file := range sample {
		s.backgroundScanner.QueueTask(&BackgroundTask{
			Type: TaskValidateFile, Priority: PriorityLow, FilePath: file.FilePath,
		})
	}
}

// StopBackgroundWork stops coalescing timers before stopping the worker queue.
func (s *Scanner) StopBackgroundWork() {
	s.stopChangeCoordinator()
	s.stopAggregateRefresh()
	if s.backgroundScanner != nil { s.backgroundScanner.Stop() }
}
