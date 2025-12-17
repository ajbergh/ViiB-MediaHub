// Package main is the Wails entry point for ViiB MediaHub.
//
// ViiB MediaHub is a local media player application with a native desktop UI
// powered by Wails v2 and WebView2. This executable:
//   - Embeds the React frontend (built by Vite) into the binary
//   - Starts an HTTP server for API endpoints
//   - Creates a native WebView2 window to display the frontend
//   - Provides system tray integration (optional)
//   - Handles graceful shutdown
//
// Command-line flags:
//
//	-data <path>   Custom data directory (default: %APPDATA%/ViiB-MediaHub)
//	-port <n>      Port for API server (0 = auto-select available port)
//	-debug         Enable debug mode (dev tools, verbose logging)
//
// Data storage:
//   - library.db: SQLite database for songs, playlists, settings
//   - covers/: Cached album artwork
//   - spotify_downloads/: Downloaded Spotify tracks
//   - viib.log: Application log file
//
// This is the Wails-based build for native Windows desktop experience.
// For the web-embedded build, see cmd/viib/main.go.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/ajbergh/viib-mediahub/internal/api"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/server"
)

// frontendFS embeds the built React frontend assets.
// These are copied from the root dist/ folder during the build process.
//
//go:embed all:frontend/dist
var frontendFS embed.FS

// Version information (can be set at build time with ldflags)
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// App struct holds the application state and provides methods
// that can be called from the frontend via Wails bindings.
type App struct {
	ctx       context.Context
	serverURL string
	dataDir   string
	database  *db.DB
}

// NewApp creates a new App instance with the given configuration.
func NewApp(serverURL, dataDir string, database *db.DB) *App {
	return &App{
		serverURL: serverURL,
		dataDir:   dataDir,
		database:  database,
	}
}

// startup is called when the Wails app starts.
// It receives the runtime context for Wails operations.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	logger.Main("Wails application started")
	logger.Main("API server available at %s", a.serverURL)
}

// shutdown is called when the Wails app is closing.
// Use this for cleanup operations.
func (a *App) shutdown(ctx context.Context) {
	logger.Main("Wails application shutting down")
}

// GetServerURL returns the localhost URL for the API server.
// This can be called from the frontend if needed for direct API access.
func (a *App) GetServerURL() string {
	return a.serverURL
}

// GetVersion returns the application version string.
func (a *App) GetVersion() string {
	return Version
}

// GetDataDir returns the application data directory path.
func (a *App) GetDataDir() string {
	return a.dataDir
}

// createAPIProxyHandler creates an http.Handler that proxies /api requests
// to the internal HTTP server. This allows the Wails AssetServer to handle
// frontend assets while routing API calls to the Go backend.
//
// Supports SSE (Server-Sent Events) by setting FlushInterval to stream
// events immediately to the client.
func createAPIProxyHandler(serverURL string) http.Handler {
	targetURL, _ := url.Parse(serverURL)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Enable immediate flushing for SSE support
	// This ensures Server-Sent Events are streamed in real-time
	proxy.FlushInterval = -1 // Flush immediately for every write

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only handle /api requests
		if strings.HasPrefix(r.URL.Path, "/api") {
			logger.Main("Proxying request: %s %s", r.Method, r.URL.Path)
			proxy.ServeHTTP(w, r)
			return
		}
		// Return 404 for non-API requests (assets will be handled by Assets fs)
		http.NotFound(w, r)
	})
}

func main() {
	// Parse command-line flags
	dataDir := flag.String("data", "", "Data directory (default: user config dir)")
	port := flag.Int("port", 0, "Port for API server (0 = auto-select)")
	debug := flag.Bool("debug", false, "Enable debug mode")
	flag.Parse()

	// Setup data directory (same as existing main.go for consistency)
	if *dataDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get user config directory: %v\n", err)
			os.Exit(1)
		}
		*dataDir = filepath.Join(configDir, "ViiB-MediaHub")
	}

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create data directory: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(*dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		// Continue anyway, logging will go to stdout
	}
	defer logger.Close()

	logger.Main("=== ViiB MediaHub (Wails) Starting ===")
	logger.Main("Version: %s", Version)
	logger.Main("Build time: %s", BuildTime)
	logger.Main("Data directory: %s", *dataDir)
	logger.Main("Debug mode: %v", *debug)

	// Initialize database
	dbPath := filepath.Join(*dataDir, "library.db")
	database, err := db.New(dbPath)
	if err != nil {
		logger.Main("Failed to initialize database: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()
	logger.Main("Database initialized: %s", dbPath)

	// Create API handler
	apiHandler := api.New(database, *dataDir)
	logger.Main("API handler created")

	// Get embedded frontend filesystem
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		logger.Main("Failed to access embedded frontend: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to access embedded frontend: %v\n", err)
		os.Exit(1)
	}

	// Start HTTP server on specified or random available port
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		logger.Main("Failed to bind to port: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to bind to port: %v\n", err)
		os.Exit(1)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", actualPort)
	logger.Main("API server starting on %s", serverURL)

	// Create HTTP server with timeouts
	httpServer := &http.Server{
		Handler:      server.New(apiHandler, distFS),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server in background goroutine
	go func() {
		logger.Main("HTTP server goroutine starting")
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Main("HTTP server error: %v", err)
		}
	}()

	// Create app instance for Wails bindings
	app := NewApp(serverURL, *dataDir, database)

	// Create API proxy handler for Wails AssetServer
	// This routes /api/* requests to the HTTP server while letting
	// the embedded distFS handle all other requests (frontend assets)
	apiProxyHandler := createAPIProxyHandler(serverURL)

	// Configure and run Wails application
	err = wails.Run(&options.App{
		Title:             "ViiB MediaHub",
		Width:             1280,
		Height:            820,
		MinWidth:          900,
		MinHeight:         600,
		DisableResize:     false,
		Fullscreen:        false,
		Frameless:         false,
		StartHidden:       false,
		HideWindowOnClose: false,
		BackgroundColour:  &options.RGBA{R: 18, G: 18, B: 18, A: 255}, // Match app background (#121212)
		AssetServer: &assetserver.Options{
			Assets:  distFS,
			Handler: apiProxyHandler,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
			WebviewUserDataPath:               *dataDir,
			Theme:                             windows.Dark,
			// Enable GPU acceleration for better rendering
			WebviewGpuIsDisabled: false,
		},
		Debug: options.Debug{
			OpenInspectorOnStartup: *debug,
		},
	})

	if err != nil {
		logger.Main("Wails application error: %v", err)
		fmt.Fprintf(os.Stderr, "Wails application error: %v\n", err)
		os.Exit(1)
	}

	// Graceful shutdown of HTTP server
	logger.Main("Shutting down HTTP server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Main("HTTP server shutdown error: %v", err)
	} else {
		logger.Main("HTTP server shutdown complete")
	}

	logger.Main("=== ViiB MediaHub (Wails) Stopped ===")
}
