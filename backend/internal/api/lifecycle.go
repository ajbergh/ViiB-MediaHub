package api

// Close stops background workers owned by the API.
func (a *API) Close() {
	a.semanticMu.Lock()
	a.semanticClosed = true
	semanticService := a.semanticService
	a.semanticService = nil
	a.semanticMu.Unlock()
	if semanticService != nil {
		_ = semanticService.Close()
	}
	if a.downloadManager != nil {
		a.downloadManager.Stop()
	}
	if a.scanner != nil {
		a.scanner.Close()
	}
}
