package scanner

// Close stops scanner background services.
func (s *Scanner) Close() {
	if s.backgroundScanner != nil {
		s.backgroundScanner.Stop()
	}
}
