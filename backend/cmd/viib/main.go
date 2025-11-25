package main

import (
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
	"syscall"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/api"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/server"
	"github.com/pkg/browser"
)

//go:embed dist/*
var frontendFS embed.FS

func main() {
	// Command line flags
	port := flag.Int("port", 0, "Port to run server on (0 = auto-select)")
	noBrowser := flag.Bool("no-browser", false, "Don't open browser automatically")
	dataDir := flag.String("data", "", "Data directory (default: user config dir)")
	flag.Parse()

	// Setup data directory
	if *dataDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			log.Fatal("Failed to get user config directory:", err)
		}
		*dataDir = filepath.Join(configDir, "ViiB-MediaHub")
	}

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatal("Failed to create data directory:", err)
	}

	log.Printf("Using data directory: %s", *dataDir)

	// Initialize database
	database, err := db.New(filepath.Join(*dataDir, "library.db"))
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer database.Close()

	// Create API handler
	apiHandler := api.New(database, *dataDir)

	// Get embedded frontend files
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		log.Fatal("Failed to access embedded frontend:", err)
	}

	// Create server
	srv := server.New(apiHandler, distFS)

	// Find available port
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal("Failed to bind to port:", err)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", actualPort)

	log.Printf("ViiB MediaHub starting on %s", serverURL)

	// Start HTTP server in goroutine
	httpServer := &http.Server{
		Handler:      srv,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server error:", err)
		}
	}()

	// Open browser
	if !*noBrowser {
		time.Sleep(500 * time.Millisecond) // Give server time to start
		if err := browser.OpenURL(serverURL); err != nil {
			log.Printf("Failed to open browser: %v", err)
			log.Printf("Please open %s manually", serverURL)
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

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
}
