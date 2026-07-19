package scanner

// Close stops scanner background services and waits for enrichment work to exit.
func (s *Scanner) Close() {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.backgroundScanner != nil {
			s.backgroundScanner.Stop()
		}
		s.enrichmentWg.Wait()
	})
}
