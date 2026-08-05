package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// V2Error is stable, machine-readable, and correlation-friendly.
type V2Error struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Retryable  bool           `json:"retryable"`
	RequestID  string         `json:"requestId,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

func requestID(r *http.Request) string {
	if id := middleware.GetReqID(r.Context()); id != "" { return id }
	return r.Header.Get("X-Request-ID")
}

func respondV2JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondV2Error(w http.ResponseWriter, r *http.Request, status int, code, message string, retryable bool, details map[string]any) {
	respondV2JSON(w, status, map[string]any{
		"error": V2Error{
			Code: code, Message: message, Retryable: retryable,
			RequestID: requestID(r), Details: details,
		},
	})
}
