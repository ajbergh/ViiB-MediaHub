// Package server configures the HTTP server with routing and middleware.
package server

import (
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/api"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func New(apiHandler *api.API, frontendFS fs.FS) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if requestID := middleware.GetReqID(req.Context()); requestID != "" {
				w.Header().Set("X-Request-ID", requestID)
			}
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			next.ServeHTTP(w, req)
		})
	})

	r.Use(loopbackTrustBoundary)

	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc:  func(r *http.Request, origin string) bool { return IsTrustedOrigin(origin, r.Host) },
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "If-Match", "Last-Event-ID", "Range", "X-Request-ID"},
		ExposedHeaders:   []string{"ETag", "X-Request-ID", "Content-Range", "Accept-Ranges", "Content-Length"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Use(middleware.Throttle(100))

	// Local song IDs are hexadecimal hashes. Plex song IDs use the reserved
	// `plex_` namespace, so ordinary local playback bypasses Plex source/schema
	// lookups entirely while preserving the existing /api/audio and /api/cover
	// browser contracts for both source types.
	r.Get("/api/audio/*", func(w http.ResponseWriter, req *http.Request) {
		songID := strings.TrimPrefix(req.URL.Path, "/api/audio/")
		if strings.HasPrefix(songID, "plex_") {
			apiHandler.ServeAudioSourceAware(w, req)
			return
		}
		apiHandler.ServeLocalAudio(w, req)
	})
	r.Get("/api/cover/*", func(w http.ResponseWriter, req *http.Request) {
		pathOrID := strings.TrimPrefix(req.URL.Path, "/api/cover/")
		if strings.HasPrefix(pathOrID, "plex_") {
			apiHandler.ServeCoverSourceAware(w, req)
			return
		}
		apiHandler.ServeLocalCover(w, req)
	})
	r.Mount("/api/v2/plex", apiHandler.PlexRoutes())

	r.Mount("/api/v2/operations", apiHandler.V2LibraryOperationRoutes())
	r.Mount("/api/v2/stems", apiHandler.V2StemRoutes())
	r.Mount("/api/v2/jobs", apiHandler.V2JobRoutes())
	r.Mount("/api/v2/performance", apiHandler.V2PerformanceRoutes())
	r.Mount("/api/v2", apiHandler.V2Routes())
	r.Mount("/api", apiHandler.Routes())

	fileServer := http.FileServer(http.FS(frontendFS))
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if f, err := frontendFS.Open(path); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		if !strings.Contains(path, ".") {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
	return r
}

// loopbackTrustBoundary enforces the local HTTP server's Host and Origin/CSRF
// boundary before route dispatch or CORS header emission.
//
// Threat model:
//  1. DNS rebinding: an attacker-controlled domain resolves to 127.0.0.1 and
//     issues same-origin requests with Host: attacker.example. IsTrustedHost
//     rejects any non-loopback/non-Wails Host header with 403 Forbidden.
//  2. Cross-site request forgery / cross-origin reading: a browser tab on an
//     untrusted origin sends requests to http://127.0.0.1:<port>. Requests
//     carrying an untrusted Origin header or Sec-Fetch-Site: cross-site are
//     rejected before mutating handlers or CORS/SSE streams execute.
func loopbackTrustBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsTrustedHost(r.Host) {
			respondTrustBoundaryError(w, r, "host_not_allowed", "Untrusted request host")
			return
		}
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" && !IsTrustedOrigin(origin, r.Host) {
			respondTrustBoundaryError(w, r, "origin_not_allowed", "Untrusted request origin")
			return
		}
		if isCrossSiteFetch(r, origin) {
			respondTrustBoundaryError(w, r, "cross_site_request_blocked", "Cross-site requests are not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isCrossSiteFetch(r *http.Request, origin string) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
		return false
	}
	if origin != "" && IsTrustedOrigin(origin, r.Host) {
		return false
	}
	return isMutatingMethod(r.Method) || strings.HasPrefix(r.URL.Path, "/api")
}

func isMutatingMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// IsTrustedHost reports whether host (an HTTP Host authority) belongs to an
// approved local loopback or Wails desktop endpoint.
func IsTrustedHost(host string) bool {
	hostname, port, ok := splitHostAuthority(host)
	if !ok {
		return false
	}
	if port != "" && !isValidPort(port) {
		return false
	}
	switch strings.ToLower(hostname) {
	case "localhost", "127.0.0.1", "::1", "wails.localhost":
		return true
	case "wails":
		return port == ""
	default:
		return false
	}
}

// IsTrustedOrigin reports whether a browser Origin header is an approved
// Wails desktop origin, local development server origin, or same-host loopback
// origin for the active server.
func IsTrustedOrigin(origin, requestHost string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" || strings.EqualFold(origin, "null") {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname, port, ok := splitHostAuthority(parsed.Host)
	if !ok {
		return false
	}
	if port != "" && !isValidPort(port) {
		return false
	}
	hostname = strings.ToLower(hostname)

	if scheme == "wails" {
		return hostname == "wails" && port == ""
	}
	if scheme != "http" && scheme != "https" {
		return false
	}
	switch hostname {
	case "wails.localhost":
		return true
	case "localhost", "127.0.0.1", "::1":
		if scheme == "http" && (port == "3000" || port == "5173") {
			return true
		}
		if !IsTrustedHost(requestHost) {
			return false
		}
		reqHostname, reqPort, reqOK := splitHostAuthority(requestHost)
		if !reqOK {
			return false
		}
		if !isLoopbackHostname(reqHostname) {
			return false
		}
		return port == reqPort
	default:
		return false
	}
}

func isLoopbackHostname(hostname string) bool {
	switch strings.ToLower(hostname) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func splitHostAuthority(authority string) (hostname, port string, ok bool) {
	authority = strings.TrimSpace(authority)
	if authority == "" || strings.ContainsAny(authority, "/\\?#@ \t\r\n") {
		return "", "", false
	}
	if strings.HasPrefix(authority, "[") {
		if strings.HasSuffix(authority, "]") {
			host := strings.TrimSuffix(strings.TrimPrefix(authority, "["), "]")
			if net.ParseIP(host) == nil {
				return "", "", false
			}
			return host, "", true
		}
		host, p, err := net.SplitHostPort(authority)
		if err != nil || host == "" || p == "" || net.ParseIP(host) == nil {
			return "", "", false
		}
		return host, p, true
	}
	if strings.Count(authority, ":") == 1 {
		host, p, err := net.SplitHostPort(authority)
		if err != nil || host == "" || p == "" {
			return "", "", false
		}
		return host, p, true
	}
	if strings.Contains(authority, ":") {
		return "", "", false
	}
	return authority, "", true
}

func isValidPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func respondTrustBoundaryError(w http.ResponseWriter, r *http.Request, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	reqID := middleware.GetReqID(r.Context())
	if reqID == "" {
		reqID = r.Header.Get("X-Request-ID")
	}
	if strings.HasPrefix(r.URL.Path, "/api/v2/") || r.URL.Path == "/api/v2" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": api.V2Error{
				Code:      code,
				Message:   message,
				Retryable: false,
				RequestID: reqID,
			},
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

