// Package main is the entry point for ViiB MediaHub.
//
// ViiB MediaHub is a local media player application with a web-based UI.
// This executable:
//   - Embeds the React frontend (built by Vite) into the binary
//   - Starts an HTTP server to serve both API and frontend
//   - Opens the default browser to the application
//   - Provides system tray integration for Windows
//   - Handles graceful shutdown on SIGINT/SIGTERM
//
// Command-line flags:
//
//	-port <n>      Port to run server on (0 = auto-select available port)
//	-no-browser    Don't open browser automatically
//	-no-tray       Disable system tray icon (useful for debugging)
//	-data <path>   Custom data directory (default: %APPDATA%/ViiB-MediaHub)
//	-debug         Enable verbose debug logging
//
// Data storage:
//   - library.db: SQLite database for songs, playlists, settings
//   - covers/: Cached album artwork
//   - spotify_downloads/: Downloaded Spotify tracks
//   - viib.log: Application log file
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/api"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/server"
	"github.com/getlantern/systray"
	"github.com/pkg/browser"
)

//go:embed dist/*
var frontendFS embed.FS

//go:embed icon.ico
var iconData []byte

var crashLogPath string

type shutdownSignal struct {
	reason string
	err    error
}

func init() {
	// Earliest possible logging - before anything else runs
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	dataDir := filepath.Join(configDir, "ViiB-MediaHub")
	crashLogPath = filepath.Join(dataDir, "crash.log")
	os.MkdirAll(dataDir, 0755)

	// Write immediately to crash log
	f, err := os.OpenFile(crashLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		f.WriteString(fmt.Sprintf("\n=== INIT at %s ===\n", time.Now().Format(time.RFC3339)))
		f.Sync()
		f.Close()
	}

	// Initialize the shared logger early from init()
	// This ensures all log messages go to the same file
	if err := logger.Init(dataDir); err != nil {
		// Can't use logger, write to crash log
		if f, err := os.OpenFile(crashLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
			f.WriteString(fmt.Sprintf("ERROR: Failed to init logger: %v\n", err))
			f.Close()
		}
	}
}

func logCrash(msg string) {
	// Write to crash log (always works, even if logger not initialized)
	if f, err := os.OpenFile(crashLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
		f.WriteString(msg + "\n")
		f.Sync()
		f.Close()
	}
	// Also write through shared logger
	logger.Main("%s", msg)
}

func main() {
	// Write to crash log that main started
	logCrash(fmt.Sprintf("MAIN STARTED at %s", time.Now().Format(time.RFC3339)))

	// Recover from panics and log them
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("PANIC RECOVERED: %v", r)
			logger.Main("%s", msg)
			logCrash(msg)
		}
	}()

	logCrash("CHECKPOINT: Before flag parsing")

	// Command line flags
	port := flag.Int("port", 0, "Port to run server on (0 = auto-select)")
	noBrowser := flag.Bool("no-browser", false, "Don't open browser automatically")
	noTray := flag.Bool("no-tray", false, "Disable system tray icon (useful for debugging)")
	dataDir := flag.String("data", "", "Data directory (default: user config dir)")
	debug := flag.Bool("debug", false, "Enable verbose debug logging")
	flag.Parse()

	logCrash("CHECKPOINT: After flag parsing")

	// Setup data directory
	if *dataDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			logCrash(fmt.Sprintf("ERROR: Failed to get user config directory: %v", err))
			log.Fatal("Failed to get user config directory:", err)
		}
		*dataDir = filepath.Join(configDir, "ViiB-MediaHub")
	}

	logCrash(fmt.Sprintf("CHECKPOINT: Data dir = %s", *dataDir))

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		logCrash(fmt.Sprintf("ERROR: Failed to create data directory: %v", err))
		log.Fatal("Failed to create data directory:", err)
	}

	// Initialize the shared logger
	if err := logger.InitWithDebug(*dataDir, *debug); err != nil {
		logCrash(fmt.Sprintf("ERROR: Failed to initialize logger: %v", err))
	}
	defer logger.Close()

	logCrash("CHECKPOINT: Logger initialized")

	logger.Main("=== ViiB MediaHub Starting ===")
	logger.Main("Using data directory: %s", *dataDir)

	logCrash("CHECKPOINT: Before database init")

	// Initialize database
	database, err := db.New(filepath.Join(*dataDir, "library.db"))
	if err != nil {
		logCrash(fmt.Sprintf("ERROR: Failed to initialize database: %v", err))
		log.Fatal("Failed to initialize database:", err)
	}
	defer database.Close()

	logCrash("CHECKPOINT: Database initialized")

	// Create API handler
	logCrash("CHECKPOINT: Before API handler creation")
	logger.Main("Creating API handler...")
	apiHandler := api.New(database, *dataDir)
	logCrash("CHECKPOINT: API handler created")
	logger.Main("API handler created")

	// Get embedded frontend files
	logCrash("CHECKPOINT: Before frontend FS")
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		logCrash(fmt.Sprintf("ERROR: Failed to access embedded frontend: %v", err))
		log.Fatal("Failed to access embedded frontend:", err)
	}
	logCrash("CHECKPOINT: Frontend FS ready")

	// Create server
	logCrash("CHECKPOINT: Before server creation")
	logger.Main("Creating server...")
	srv := server.New(apiHandler, distFS)
	logCrash("CHECKPOINT: Server created")
	logger.Main("Server created")

	// Find available port
	logCrash("CHECKPOINT: Before port binding")
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		logCrash(fmt.Sprintf("ERROR: Failed to bind to port: %v", err))
		log.Fatal("Failed to bind to port:", err)
	}
	logCrash("CHECKPOINT: Port bound")

	actualPort := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", actualPort)

	logger.Main("ViiB MediaHub starting on %s", serverURL)
	logCrash(fmt.Sprintf("CHECKPOINT: Server URL = %s", serverURL))

	// Start HTTP server in goroutine
	// Note: WriteTimeout is disabled (0) to support long-running SSE connections
	// for operations like AI enrichment that can take several minutes per batch.
	httpServer := &http.Server{
		Handler:      srv,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Disabled for SSE streams
		IdleTimeout:  120 * time.Second,
	}

	logCrash("CHECKPOINT: Before HTTP server start")
	serverErrCh := make(chan error, 1)
	go func() {
		logCrash("CHECKPOINT: HTTP server goroutine starting")
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	// Open browser
	if !*noBrowser {
		time.Sleep(500 * time.Millisecond) // Give server time to start
		if err := browser.OpenURL(serverURL); err != nil {
			logger.Main("Failed to open browser: %v", err)
			logger.Main("Please open %s manually", serverURL)
		}
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║           🎵 ViiB MediaHub is running! 🎵            ║")
	fmt.Printf("║           Server: %-35s║\n", serverURL)
	fmt.Println("║                                                      ║")
	fmt.Println("║           Press Ctrl+C to stop                       ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		shutdownOnce sync.Once
		shutdownInfo = shutdownSignal{reason: "unspecified"}
	)

	requestShutdown := func(reason string, err error) {
		shutdownOnce.Do(func() {
			shutdownInfo = shutdownSignal{reason: reason, err: err}
			if err != nil {
				logCrash(fmt.Sprintf("CHECKPOINT: Shutdown requested (%s): %v", reason, err))
			} else {
				logCrash(fmt.Sprintf("CHECKPOINT: Shutdown requested (%s)", reason))
			}
			cancel()
		})
	}

	go func() {
		if sig := <-quit; sig != nil {
			reason := fmt.Sprintf("signal %s", sig)
			logger.Main("Received %s", sig)
			requestShutdown(reason, nil)
		}
	}()

	go func() {
		if err := <-serverErrCh; err != nil {
			requestShutdown("http server error", err)
		} else {
			logCrash("CHECKPOINT: HTTP server goroutine stopped")
		}
	}()

	useTray := !*noTray
	if useTray {
		trayReady := make(chan struct{}, 1)
		go func() {
			<-trayReady
			<-ctx.Done()
			logCrash("CHECKPOINT: Context canceled; invoking systray.Quit()")
			systray.Quit()
		}()

		logCrash("CHECKPOINT: Entering systray.Run on main goroutine")
		func() {
			defer func() {
				if r := recover(); r != nil {
					requestShutdown("systray panic", fmt.Errorf("%v", r))
				}
			}()
			systray.Run(onReady(serverURL, func(reason string) {
				requestShutdown(reason, nil)
			}, trayReady), onExit)
		}()
		logCrash("CHECKPOINT: systray.Run returned")
		requestShutdown("systray exit", nil)
	} else {
		logCrash("CHECKPOINT: System tray disabled; waiting for shutdown signal")
		<-ctx.Done()
	}

	logCrash("CHECKPOINT: Context canceled, beginning HTTP server shutdown")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logCrash(fmt.Sprintf("ERROR: HTTP server shutdown failed: %v", err))
		logger.Main("HTTP server shutdown failed: %v", err)
	} else {
		logCrash("CHECKPOINT: HTTP server shutdown complete")
	}

	logger.Main("Shutting down...")
	logger.Main("Shutdown reason: %s", shutdownInfo.reason)
	if shutdownInfo.err != nil {
		logger.Main("Shutdown error: %v", shutdownInfo.err)
	}
}

func onReady(serverURL string, requestShutdown func(reason string), trayReady chan<- struct{}) func() {
	return func() {
		if trayReady != nil {
			select {
			case trayReady <- struct{}{}:
			default:
			}
		}
		systray.SetIcon(iconData)
		systray.SetTitle("ViiB MediaHub")
		systray.SetTooltip("ViiB MediaHub")

		mOpen := systray.AddMenuItem("Open ViiB", "Open ViiB in browser")
		mQuit := systray.AddMenuItem("Quit", "Quit ViiB MediaHub")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					if err := browser.OpenURL(serverURL); err != nil {
						logger.Main("Failed to open browser from tray: %v", err)
					}
				case <-mQuit.ClickedCh:
					requestShutdown("tray menu quit")
					return
				}
			}
		}()
	}
}

func onExit() {
	logger.Main("Systray exited")
	logCrash("CHECKPOINT: systray onExit invoked")
}
