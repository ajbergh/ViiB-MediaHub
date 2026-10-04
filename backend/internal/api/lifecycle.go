package api

import (
	"context"
	"sync"
)

// Close stops background workers owned by the API.
func (a *API) Close() {
	a.closeOnce.Do(a.close)
}

func (a *API) close() {
	a.closeEnrichment()
	a.spotifyTokens().close()
	a.spotifyAnalysisMu.Lock()
	a.spotifyAnalysisClosed = true
	spotifyAnalysis := a.spotifyAnalysis
	a.spotifyAnalysis = nil
	a.spotifyAnalysisMu.Unlock()
	if spotifyAnalysis != nil {
		spotifyAnalysis.Close()
	}
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
		if a.downloadManager.sessionManager != nil {
			_ = a.downloadManager.sessionManager.Close()
		}
	}
	if a.scanner != nil {
		a.scanner.Close()
	}
}

// beginEnrichment registers a job before it can start. Admission and shutdown
// share a lock, so Wait cannot race a newly admitted Add. A Background parent
// deliberately preserves jobs across client disconnects.
func (a *API) beginEnrichment(parent context.Context) (context.Context, func(), bool) {
	a.enrichmentMu.Lock()
	defer a.enrichmentMu.Unlock()
	if a.enrichmentClosed {
		return nil, nil, false
	}
	if a.enrichmentContext == nil {
		a.enrichmentContext, a.enrichmentCancel = context.WithCancel(context.Background())
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(a.enrichmentContext, cancel)
	a.enrichmentWorkers.Add(1)
	var once sync.Once
	finish := func() { once.Do(func() { stop(); cancel(); a.enrichmentWorkers.Done() }) }
	return ctx, finish, true
}

func (a *API) closeEnrichment() {
	a.enrichmentMu.Lock()
	a.enrichmentClosed = true
	if a.enrichmentCancel != nil {
		a.enrichmentCancel()
	}
	a.enrichmentMu.Unlock()
	a.enrichmentWorkers.Wait()
}
