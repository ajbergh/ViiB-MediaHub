// Tests the loopback HTTP server trust boundary, including Host header
// validation (DNS rebinding defense), Origin/Sec-Fetch-Site validation,
// mutating request rejection, and centralized CORS/SSE headers.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ajbergh/viib-mediahub/internal/api"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func newTestServerHandler(t *testing.T) http.Handler {
	t.Helper()
	dataDir := t.TempDir()
	database, err := db.New(filepath.Join(dataDir, "test.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	apiHandler := api.New(database, dataDir)
	t.Cleanup(apiHandler.Close)

	frontendFS := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><html><body>ViiB</body></html>")},
	}
	return New(apiHandler, frontendFS)
}

func TestIsTrustedHost(t *testing.T) {
	tests := []struct {
		host    string
		trusted bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.1:34115", true},
		{"127.0.0.1:8080", true},
		{"localhost", true},
		{"localhost:3000", true},
		{"localhost:5173", true},
		{"LOCALHOST:34115", true},
		{"[::1]", true},
		{"[::1]:34115", true},
		{"wails.localhost", true},
		{"wails.localhost:34115", true},
		{"wails", true},
		{"wails:8080", false},
		{"", false},
		{"evil.test", false},
		{"evil.test:34115", false},
		{"127.0.0.1.evil.test", false},
		{"localhost.evil.test:3000", false},
		{"wails.localhost.evil.test", false},
		{"192.168.1.50:34115", false},
		{"0.0.0.0:34115", false},
		{"127.0.0.1:0", false},
		{"127.0.0.1:70000", false},
		{"127.0.0.1:notaport", false},
		{"[::1", false},
		{"user@127.0.0.1:34115", false},
		{"127.0.0.1/path", false},
	}

	for _, tc := range tests {
		if got := IsTrustedHost(tc.host); got != tc.trusted {
			t.Errorf("IsTrustedHost(%q) = %v, want %v", tc.host, got, tc.trusted)
		}
	}
}

func TestIsTrustedOrigin(t *testing.T) {
	tests := []struct {
		origin  string
		host    string
		trusted bool
	}{
		{"wails://wails", "127.0.0.1:34115", true},
		{"http://wails.localhost", "127.0.0.1:34115", true},
		{"http://wails.localhost:34115", "127.0.0.1:34115", true},
		{"https://wails.localhost", "127.0.0.1:34115", true},
		{"https://wails.localhost:41234", "127.0.0.1:34115", true},
		{"http://localhost:3000", "127.0.0.1:8080", true},
		{"http://localhost:5173", "127.0.0.1:34115", true},
		{"http://127.0.0.1:3000", "127.0.0.1:8080", true},
		{"http://127.0.0.1:5173", "127.0.0.1:34115", true},
		{"http://[::1]:3000", "127.0.0.1:8080", true},
		{"http://127.0.0.1:34115", "127.0.0.1:34115", true},
		{"http://localhost:34115", "127.0.0.1:34115", true},
		{"http://127.0.0.1:49152", "127.0.0.1:49152", true},
		{"http://127.0.0.1:9999", "127.0.0.1:34115", false},
		{"https://localhost:3000", "127.0.0.1:34115", false},
		{"", "127.0.0.1:34115", false},
		{"null", "127.0.0.1:34115", false},
		{"https://evil.test", "127.0.0.1:34115", false},
		{"http://evil.test:3000", "127.0.0.1:34115", false},
		{"wails://evil.test", "127.0.0.1:34115", false},
		{"http://wails.localhost.evil.test", "127.0.0.1:34115", false},
		{"http://localhost:3000/extra", "127.0.0.1:34115", false},
		{"http://user@localhost:3000", "127.0.0.1:34115", false},
		{"http://127.0.0.1:34115", "evil.test:34115", false},
	}

	for _, tc := range tests {
		if got := IsTrustedOrigin(tc.origin, tc.host); got != tc.trusted {
			t.Errorf("IsTrustedOrigin(%q, %q) = %v, want %v", tc.origin, tc.host, got, tc.trusted)
		}
	}
}

func TestLoopbackTrustBoundaryRejectsUntrustedHostsAndOrigins(t *testing.T) {
	handler := newTestServerHandler(t)

	t.Run("DNS rebinding host rejected on GET and POST", func(t *testing.T) {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			req := httptest.NewRequest(method, "http://127.0.0.1:34115/api/health", nil)
			req.Host = "rebind.attacker.example:34115"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s with untrusted Host status = %d, want 403", method, rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("untrusted Host emitted CORS header %q", got)
			}
		}
	})

	t.Run("Attacker origin rejected on mutating and read routes", func(t *testing.T) {
		cases := []struct {
			method string
			path   string
			origin string
		}{
			{http.MethodPost, "/api/scan/quick", "https://evil.example"},
			{http.MethodDelete, "/api/songs", "http://evil.example"},
			{http.MethodPost, "/api/settings/plex_token", "null"},
			{http.MethodPost, "/api/v2/jobs", "http://127.0.0.1:9999"},
			{http.MethodGet, "/api/settings/plex_token", "https://evil.example"},
			{http.MethodOptions, "/api/songs", "https://evil.example"},
		}
		for _, tc := range cases {
			req := httptest.NewRequest(tc.method, "http://127.0.0.1:34115"+tc.path, strings.NewReader(`{}`))
			req.Host = "127.0.0.1:34115"
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s Origin=%q status = %d, want 403 (body=%s)", tc.method, tc.path, tc.origin, rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("untrusted Origin %q received Access-Control-Allow-Origin=%q", tc.origin, got)
			}
		}
	})

	t.Run("Cross-site Sec-Fetch-Site without trusted Origin is rejected", func(t *testing.T) {
		for _, tc := range []struct {
			method string
			path   string
		}{
			{http.MethodPost, "/api/scan"},
			{http.MethodPost, "/api/v2/operations/repair"},
			{http.MethodGet, "/api/songs"},
			{http.MethodGet, "/api/library/events"},
		} {
			req := httptest.NewRequest(tc.method, "http://127.0.0.1:34115"+tc.path, nil)
			req.Host = "127.0.0.1:34115"
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s with Sec-Fetch-Site=cross-site status = %d, want 403", tc.method, tc.path, rec.Code)
			}
		}
	})

	t.Run("Structured v2 error envelope returned for v2 routes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:34115/api/v2/jobs", strings.NewReader(`{"type":"quick_scan"}`))
		req.Host = "evil.example"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		var payload struct {
			Error api.V2Error `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal v2 error: %v", err)
		}
		if payload.Error.Code != "host_not_allowed" || payload.Error.Retryable {
			t.Fatalf("unexpected v2 error payload: %+v", payload.Error)
		}
	})
}

func TestLoopbackTrustBoundaryAllowsApprovedClientsAndNormalizesSSEHeaders(t *testing.T) {
	handler := newTestServerHandler(t)

	t.Run("Approved origins and native no-origin requests succeed", func(t *testing.T) {
		origins := []string{
			"",
			"wails://wails",
			"http://wails.localhost",
			"https://wails.localhost:34115",
			"http://localhost:3000",
			"http://localhost:5173",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:5173",
			"http://127.0.0.1:34115",
		}
		for _, origin := range origins {
			req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:34115/api/health", nil)
			req.Host = "127.0.0.1:34115"
			if origin != "" {
				req.Header.Set("Origin", origin)
				// Wails WebView on Windows/macOS/Linux may send Sec-Fetch-Site: cross-site
				// when connecting from wails://wails or http://wails.localhost to 127.0.0.1:34115.
				req.Header.Set("Sec-Fetch-Site", "cross-site")
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("Origin=%q status = %d, want 200 (body=%s)", origin, rec.Code, rec.Body.String())
			}
			gotCORS := rec.Header().Get("Access-Control-Allow-Origin")
			if origin == "" {
				if gotCORS != "" {
					t.Fatalf("no-origin request emitted Access-Control-Allow-Origin=%q", gotCORS)
				}
			} else if gotCORS != origin {
				t.Fatalf("Origin=%q Access-Control-Allow-Origin = %q, want %q", origin, gotCORS, origin)
			}
		}
	})

	t.Run("SSE endpoints never emit wildcard Access-Control-Allow-Origin", func(t *testing.T) {
		ssePaths := []string{
			"/api/library/events",
			"/api/library/enrich-genres/stream",
			"/api/library/enrich-all/stream",
			"/api/library/enrich-years/stream",
			"/api/library/enrich-mood/stream",
			"/api/spotify/downloads/events",
			"/api/v2/library/events",
			"/api/v2/jobs/events",
		}
		for _, path := range ssePaths {
			// Verify with an approved Wails origin
			req, cancel := httpTestRequestWithCancel(http.MethodGet, "http://127.0.0.1:34115"+path)
			req.Host = "127.0.0.1:34115"
			req.Header.Set("Origin", "http://wails.localhost")
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			cancel() // Cancel immediately so streaming loops exit cleanly after setting headers
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
				t.Fatalf("SSE %s emitted wildcard Access-Control-Allow-Origin: *", path)
			} else if got != "http://wails.localhost" {
				t.Fatalf("SSE %s Access-Control-Allow-Origin = %q, want http://wails.localhost", path, got)
			}

			// Verify without Origin header (same-origin / proxy)
			reqNoOrigin, cancelNoOrigin := httpTestRequestWithCancel(http.MethodGet, "http://127.0.0.1:34115"+path)
			reqNoOrigin.Host = "127.0.0.1:34115"
			cancelNoOrigin()
			recNoOrigin := httptest.NewRecorder()
			handler.ServeHTTP(recNoOrigin, reqNoOrigin)

			if got := recNoOrigin.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("SSE %s without Origin emitted Access-Control-Allow-Origin = %q, want empty", path, got)
			}
		}
	})

	t.Run("CORS preflight allows If-Match and exposes ETag for approved origins", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:34115/api/v2/analysis/song1/bpm", nil)
		req.Host = "127.0.0.1:34115"
		req.Header.Set("Origin", "wails://wails")
		req.Header.Set("Access-Control-Request-Method", "PUT")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type, If-Match, X-Request-ID")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
			t.Fatalf("preflight status = %d, want 200/204", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "wails://wails" {
			t.Fatalf("preflight Access-Control-Allow-Origin = %q, want wails://wails", got)
		}
		allowHeaders := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
		if !strings.Contains(allowHeaders, "if-match") {
			t.Fatalf("preflight Access-Control-Allow-Headers = %q, want If-Match included", allowHeaders)
		}
	})

	t.Run("IPv6 loopback [::1] Host and Origin matching", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://[::1]:34115/api/health", nil)
		req.Host = "[::1]:34115"
		req.Header.Set("Origin", "http://[::1]:34115")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("IPv6 loopback status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://[::1]:34115" {
			t.Fatalf("IPv6 Access-Control-Allow-Origin = %q, want http://[::1]:34115", got)
		}
	})
}

func httpTestRequestWithCancel(method, target string) (*http.Request, func()) {
	req := httptest.NewRequest(method, target, nil)
	ctx, cancel := context.WithCancel(req.Context())
	return req.WithContext(ctx), cancel
}

func TestMaintenanceDiagnosticReferenceCORSAndLegacyRequestID(t *testing.T) {
	handler := newTestServerHandler(t)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:34115/api/v2/operations/repair", strings.NewReader(`{"broken":`))
	request.Header.Set("Origin", "wails://wails")
	request.Header.Set("X-Request-ID", "private-caller-request")
	request.Header.Set(api.DiagnosticTokenHeader, "private-incoming-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var envelope struct{ Error api.V2Error }
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusBadRequest {
		t.Fatalf("response: %d %v", response.Code, err)
	}
	token := response.Header().Get(api.DiagnosticTokenHeader)
	if len(token) != 36 || token != envelope.Error.DiagnosticToken || token == "private-incoming-token" {
		t.Fatal("missing independent diagnostic reference")
	}
	if response.Header().Get("X-Request-ID") != "private-caller-request" || envelope.Error.RequestID != "private-caller-request" {
		t.Fatal("request ID compatibility changed")
	}
	if !strings.Contains(strings.ToLower(response.Header().Get("Access-Control-Expose-Headers")), strings.ToLower(api.DiagnosticTokenHeader)) {
		t.Fatal("diagnostic response header is not exposed")
	}
}
