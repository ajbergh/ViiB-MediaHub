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
	a.spotifyStreamerMu.Lock()
	if a.spotifyStreamer != nil {
		a.spotifyStreamer.CloseAllStreams()
	}
	a.spotifyStreamerMu.Unlock()
	if a.downloadManager != nil {
		a.downloadManager.Stop()
	}
	if a.scanner != nil {
		a.scanner.Close()
	}
}
