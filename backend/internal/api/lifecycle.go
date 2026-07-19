package api

// Close stops background workers owned by the API.
func (a *API) Close() {
	if a.downloadManager != nil {
		a.downloadManager.Stop()
	}
	if a.scanner != nil {
		a.scanner.Close()
	}
}
