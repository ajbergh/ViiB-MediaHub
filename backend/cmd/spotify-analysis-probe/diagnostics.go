//go:build spotify_research

package main

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

// probeFailure captures safe milestones rather than secret-bearing exchange data.
type probeFailure struct {
	stage         string
	authenticated bool
	renewed       bool
	cause         error
}

func (e *probeFailure) Error() string { return "spotify probe failed at " + e.stage }
func (e *probeFailure) Unwrap() error { return e.cause }

type failureReport struct {
	Status                 string  `json:"status"`
	Stage                  string  `json:"stage"`
	AuthenticationVerified bool    `json:"authenticationVerified"`
	RenewalVerified        bool    `json:"renewalVerified"`
	Code                   string  `json:"code"`
	HTTPStatus             int     `json:"httpStatus,omitempty"`
	RetryAfterSeconds      float64 `json:"retryAfterSeconds,omitempty"`
}

func writeFailure(w io.Writer, err error) error {
	report := failureReport{Status: "failed", Stage: "configuration", Code: "probe_error"}
	var failure *probeFailure
	if errors.As(err, &failure) {
		report.Stage, report.AuthenticationVerified, report.RenewalVerified = failure.stage, failure.authenticated, failure.renewed
	}
	var analysisErr *analysis.Error
	var authErr *auth.WebPlayerHTTPError
	switch {
	case errors.As(err, &analysisErr):
		report.Code, report.HTTPStatus = string(analysisErr.Code), analysisErr.HTTPStatus
		report.RetryAfterSeconds = analysisErr.RetryAfter.Seconds()
	case errors.As(err, &authErr):
		report.Code, report.HTTPStatus = "token_request_denied", authErr.Status
		report.RetryAfterSeconds = authErr.RetryAfter.Seconds()
	case errors.Is(err, catalog.ErrSchema):
		report.Code = "catalog_schema_incompatible"
	case errors.Is(err, auth.ErrAuthenticationRequired):
		report.Code = "authentication_required"
	case errors.Is(err, auth.ErrProviderChanged):
		report.Code = "provider_changed"
	case errors.Is(err, auth.ErrDisabled):
		report.Code = "disabled"
	case errors.Is(err, auth.ErrTemporarilyUnavailable):
		report.Code = "temporarily_unavailable"
	}
	return json.NewEncoder(w).Encode(report)
}
