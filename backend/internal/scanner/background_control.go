package scanner

import "github.com/ajbergh/viib-mediahub/internal/logger"

func (s *Scanner) PauseBackgroundWork() {
	if s.backgroundScanner != nil { s.backgroundScanner.Pause() }
}

func (s *Scanner) ResumeBackgroundWork() {
	if s.backgroundScanner != nil { s.backgroundScanner.Resume() }
}

func (s *Scanner) IsBackgroundPaused() bool {
	if s.backgroundScanner != nil { return s.backgroundScanner.IsPaused() }
	return true
}

func (s *Scanner) GetBackgroundStats() (processed, errors int64, queueSize int, activeWorkers int32) {
	if s.backgroundScanner != nil { return s.backgroundScanner.Stats() }
	return 0, 0, 0, 0
}

func (s *Scanner) QueueChangesForBackground(changes []FileChange) {
	s.queueCoalescedBackgroundChanges(changes)
}

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

func (s *Scanner) StopBackgroundWork() {
	s.stopChangeCoordinator()
	s.stopAggregateRefresh()
	if s.backgroundScanner != nil { s.backgroundScanner.Stop() }
}
