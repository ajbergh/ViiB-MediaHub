// Tests recording bounded catalog timing and status diagnostics without response bodies or credentials.
package api

import (
	"context"
	"errors"
	"fmt"
	"testing"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

func TestSpotifyCatalogDiagnosticUsesOnlyFixedSafeFields(t *testing.T) {
	for _, tc := range []struct {
		err         error
		code, stage string
		status      int
	}{
		{fmt.Errorf("secret-bearing cause: %w", catalog.ErrSchema), "schema_incompatible", "catalog", 0},
		{errors.New("cookie-secret bearer-secret URL-secret"), "unknown", "catalog", 0},
		{&catalog.HTTPError{Stage: "search", Status: 502}, "upstream_http", "search", 502},
		{&catalog.HTTPError{Stage: "private-account-or-token", Status: 404}, "upstream_http", "catalog", 404},
		{context.Canceled, "cancelled", "catalog", 0},
		{context.DeadlineExceeded, "deadline", "catalog", 0},
		{spotifyauth.ErrTemporarilyUnavailable, "temporarily_unavailable", "catalog", 0},
	} {
		code, stage, status := spotifyCatalogDiagnostic(tc.err)
		if code != tc.code || stage != tc.stage || status != tc.status {
			t.Fatalf("classification: %q %q %d", code, stage, status)
		}
	}
}
