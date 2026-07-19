// Package server configures the HTTP server with routing and middleware.
//
// Uses chi router with the following middleware stack:
//   - Logger: Request logging for debugging
//   - Recoverer: Panic recovery to prevent crashes
//   - Compress: Response compression (level 5)
//   - CORS: Cross-origin support for development mode
//
// Routes are organized as:
//   - /api/* : REST API handlers (delegated to api.API)
//   - /* : Frontend static file serving with SPA fallback
//
// The frontend is served from an embedded filesystem, with SPA routing
// fallback that serves index.html for non-file requests.
package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/api"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// New constructs an HTTP handler configured with routing and middleware
// for the application. The returned handler mounts the API routes at /api
// and serves the embedded frontend assets for all other routes.
func New(apiHandler *api.API, frontendFS fs.FS) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			next.ServeHTTP(w, req)
		})
	})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			next.ServeHTTP(w, req)
		})
	})

	// CORS for development and Wails builds (wails.localhost)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:5173", "http://127.0.0.1:3000", "http://127.0.0.1:5173", "http://wails.localhost", "http://wails.localhost:*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Global rate limiting: allow up to 100 concurrent requests
	r.Use(middleware.Throttle(100))

	// API routes
	r.Mount("/api", apiHandler.Routes())

	// Serve frontend static files
	fileServer := http.FileServer(http.FS(frontendFS))

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the exact file first
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Check if file exists
		if f, err := frontendFS.Open(path); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// For SPA routing, serve index.html for non-file requests
		if !strings.Contains(path, ".") {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}

		// 404 for missing files
		http.NotFound(w, r)
	})

	return r
}
