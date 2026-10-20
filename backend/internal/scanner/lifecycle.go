// Defines lifecycle functionality for package scanner.

package scanner

// Close stops scanner background services and waits for automatic scans and enrichment to exit.
func (s *Scanner) Close() {
	s.closeOnce.Do(func() {
		// Serialize shutdown with worker admission so Add cannot race Wait.
		s.rescanMutex.Lock()
		s.autoScanClosed = true
		if s.cancel != nil {
			s.cancel()
		}
		s.rescanMutex.Unlock()
		s.autoScanWg.Wait()
		if s.backgroundScanner != nil {
			s.backgroundScanner.Stop()
		}
		s.enrichmentWg.Wait()
	})
}
