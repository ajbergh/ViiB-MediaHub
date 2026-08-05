// v2_performance.go exposes local-only scanner and SQLite diagnostics.
package api

import (
	"net/http"

	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/go-chi/chi/v5"
)

// V2PerformanceRoutes returns performance metrics and scanner-quarantine routes.
func (a *API) V2PerformanceRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", a.getPerformanceDiagnosticsV2)
	r.Get("/scanner-failures", a.getScannerFailuresV2)
	r.Delete("/scanner-failures", a.clearScannerFailureV2)
	return r
}

func (a *API) getPerformanceDiagnosticsV2(w http.ResponseWriter, r *http.Request) {
	processed, errors, queueSize, activeWorkers := a.scanner.GetBackgroundStats()
	metadataLimit, metadataActive, metadataPeak := scanner.MetadataExecutorStats()
	response := map[string]any{
		"database": a.db.RuntimeStats(),
		"scanner": map[string]any{
			"scanning": a.scanner.IsScanning(),
			"progress": a.scanner.GetProgress(),
			"backgroundPaused": a.scanner.IsBackgroundPaused(),
			"processed": processed,
			"errors": errors,
			"queueSize": queueSize,
			"activeWorkers": activeWorkers,
			"metadataConcurrencyLimit": metadataLimit,
			"metadataActive": metadataActive,
			"metadataPeak": metadataPeak,
		},
	}
	if revision, retained, err := a.db.LibrarySyncStats(); err == nil {
		response["librarySync"] = map[string]int64{"revision": revision, "retainedChanges": retained}
	}
	respondJSON(w, response)
}

func (a *API) getScannerFailuresV2(w http.ResponseWriter, r *http.Request) {
	failures, err := a.db.ListScannerFailures(parseBoundedInt(r.URL.Query().Get("limit"), 250, 1000))
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]any{"failures": failures})
}

func (a *API) clearScannerFailureV2(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		respondError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := a.db.ClearScannerFailure(filePath); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "cleared"})
}
