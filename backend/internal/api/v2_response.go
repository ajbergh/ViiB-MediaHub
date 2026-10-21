// v2_response.go defines the structured error envelope used by jobs and
// Library Operations handlers. The library sync handlers retain legacy JSON
// responses until their compatibility migration is complete.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// DiagnosticTokenHeader carries a server-generated reference for a structured failure.
const DiagnosticTokenHeader = "X-ViiB-Diagnostic-Token"

// V2Error is stable, machine-readable, and correlation-friendly.
type V2Error struct {
	DiagnosticToken string         `json:"diagnosticToken,omitempty"`
	Code            string         `json:"code"`
	Message         string         `json:"message"`
	Retryable       bool           `json:"retryable"`
	RequestID       string         `json:"requestId,omitempty"`
	Details         map[string]any `json:"details,omitempty"`
}

func requestID(r *http.Request) string {
	if id := middleware.GetReqID(r.Context()); id != "" {
		return id
	}
	return r.Header.Get("X-Request-ID")
}

func respondV2JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondV2Error(w http.ResponseWriter, r *http.Request, status int, code, message string, retryable bool, details map[string]any) {
	// Independent of caller headers and the hostname-bearing legacy request ID.
	token := uuid.NewString()
	w.Header().Set(DiagnosticTokenHeader, token)
	logger.API("support reference issued token=%q status=%d", token, status)
	respondV2JSON(w, status, map[string]any{
		"error": V2Error{
			Code: code, Message: message, Retryable: retryable,
			RequestID: requestID(r), DiagnosticToken: token, Details: details,
		},
	})
}
