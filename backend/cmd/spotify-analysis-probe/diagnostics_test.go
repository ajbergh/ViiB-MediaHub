//go:build spotify_research

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func TestFailureReportsAuthMilestonesAndHTTPStatus(t *testing.T) {
	for _, tc := range []struct {
		code   analysis.Code
		status int
	}{
		{analysis.NotFound, 404},
		{analysis.AnalysisUnavailable, 200},
	} {
		var output bytes.Buffer
		err := writeFailure(&output, &probeFailure{stage: "audio_analysis", authenticated: true, renewed: true,
			cause: &analysis.Error{Code: tc.code, HTTPStatus: tc.status}})
		if err != nil {
			t.Fatal(err)
		}
		var report failureReport
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if report.HTTPStatus != tc.status || report.Code != string(tc.code) || !report.AuthenticationVerified || !report.RenewalVerified || report.Stage != "audio_analysis" {
			t.Fatalf("incorrect diagnostic: %+v", report)
		}
	}
}
func TestFailureNeverPrintsUnrecognizedErrorDetails(t *testing.T) {
	var output bytes.Buffer
	if err := writeFailure(&output, errors.New("Cookie: session-secret Authorization: bearer-secret")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "session-secret") || strings.Contains(output.String(), "bearer-secret") {
		t.Fatal("secret disclosure")
	}
}
