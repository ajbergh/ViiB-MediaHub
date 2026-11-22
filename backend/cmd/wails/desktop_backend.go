// Starts the desktop loopback backend after Wails checks for an existing instance.
// Owns the WebView proxy, Spotify OAuth redirect, and backend shutdown.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/api"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/server"
)

type desktopBackend struct {
	start              func() (*runningDesktopBackend, error)
	once               sync.Once
	running            *runningDesktopBackend
	err                error
	reportStartFailure sync.Once
}

type runningDesktopBackend struct {
	serverURL string
	api       *api.API
	proxy     http.Handler
	close     func()
}

// get starts the backend once and returns the same startup result to all callers.
func (b *desktopBackend) get() (*runningDesktopBackend, error) {
	b.once.Do(func() { b.running, b.err = b.start() })
	if b.running == nil && b.err == nil {
		return nil, context.Canceled
	}
	return b.running, b.err
}

// ServeHTTP forwards WebView API requests to the loopback backend.
func (b *desktopBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api") {
		http.NotFound(w, r)
		return
	}
	running, err := b.get()
	if err != nil {
		http.Error(w, "ViiB MediaHub API could not start", http.StatusServiceUnavailable)
		return
	}
	logger.MainDebug("Proxying request: %s %s", r.Method, r.URL.Path)
	running.proxy.ServeHTTP(w, r)
}

// close waits for any startup already in progress and prevents future startup.
func (b *desktopBackend) close() error {
	// Wait for an in-progress start, or prevent a late callback from starting
	// services after the desktop window has already closed.
	b.once.Do(func() {})
	if b.running != nil {
		b.running.close()
	}
	return b.err
}

// startDesktopBackend binds the loopback listener, opens the library, and serves
// the desktop API, frontend assets, and Spotify redirect.
func startDesktopBackend(port int, dataDir string, assets fs.FS) (*runningDesktopBackend, error) {
	// Reserve the OAuth callback port before opening the database. An unrelated
	// listener must never cause this launch to supersede active Spotify metadata.
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("open API port %d: %w", port, err)
	}
	database, err := db.New(filepath.Join(dataDir, "library.db"))
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("open media library: %w", err)
	}
	handler := api.New(database, dataDir)
	target := &url.URL{Scheme: "http", Host: listener.Addr().String()}
	serverURL := target.String()
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	httpServer, cancelRequests := newHTTPServer(handler.WithSpotifyOAuthCallback(serverURL, server.New(handler, assets)))
	logger.Main("API server starting on %s", serverURL)
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Main("HTTP server error: %v", err)
		}
	}()
	return &runningDesktopBackend{
		serverURL: serverURL,
		api:       handler,
		proxy:     proxy,
		close: func() {
			defer listener.Close()
			cancelRequests()
			handler.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := httpServer.Shutdown(ctx); err != nil {
				logger.Main("HTTP server shutdown error: %v", err)
			}
			_ = database.Close()
		},
	}, nil
}
