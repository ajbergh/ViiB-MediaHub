// Package server configures the HTTP server with routing and middleware.
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

func New(apiHandler *api.API, frontendFS fs.FS) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if requestID := middleware.GetReqID(req.Context()); requestID != "" { w.Header().Set("X-Request-ID", requestID) }
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			next.ServeHTTP(w, req)
		})
	})

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"http://localhost:3000", "http://localhost:5173", "http://127.0.0.1:3000", "http://127.0.0.1:5173", "http://wails.localhost", "http://wails.localhost:*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "Last-Event-ID", "Range", "X-Request-ID"},
		ExposedHeaders: []string{"X-Request-ID", "Content-Range", "Accept-Ranges", "Content-Length"},
		AllowCredentials: true,
		MaxAge: 300,
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
	r.Mount("/api/v2/jobs", apiHandler.V2JobRoutes())
	r.Mount("/api/v2/performance", apiHandler.V2PerformanceRoutes())
	r.Mount("/api/v2", apiHandler.V2Routes())
	r.Mount("/api", apiHandler.Routes())

	fileServer := http.FileServer(http.FS(frontendFS))
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" { path = "index.html" }
		if f, err := frontendFS.Open(path); err == nil { f.Close(); fileServer.ServeHTTP(w, r); return }
		if !strings.Contains(path, ".") { r.URL.Path = "/"; fileServer.ServeHTTP(w, r); return }
		http.NotFound(w, r)
	})
	return r
}
