// Package logger provides a shared logging facility for ViiB MediaHub.
// It writes to both the viib.log file and stderr for immediate visibility.
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	logFile     *os.File
	logMutex    sync.Mutex
	initialized bool
	currentDir  string
)

// Init initializes the logger with the given data directory.
// It creates the viib.log file and sets up logging.
// If already initialized with the same directory, this is a no-op.
// If initialized with a different directory, the old file is closed.
func Init(dataDir string) error {
	logMutex.Lock()
	defer logMutex.Unlock()

	// If already initialized with the same directory, nothing to do
	if initialized && currentDir == dataDir {
		return nil
	}

	// Close existing file if reinitializing with different directory
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}

	logPath := filepath.Join(dataDir, "viib.log")
	// Use O_TRUNC to start fresh each run
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	logFile = f
	initialized = true
	currentDir = dataDir

	// Write initialization header
	timestamp := time.Now().Format("2006/01/02 15:04:05.000000")
	logFile.WriteString(fmt.Sprintf("%s [INIT] === ViiB MediaHub Logger Initialized ===\n", timestamp))
	logFile.Sync()

	return nil
}

// Close closes the log file.
func Close() {
	logMutex.Lock()
	defer logMutex.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
		initialized = false
	}
}

// Log writes a message to both the log file and stderr.
// Format: timestamp [prefix] message
func Log(prefix, format string, v ...interface{}) {
	timestamp := time.Now().Format("2006/01/02 15:04:05.000000")
	msg := fmt.Sprintf(format, v...)
	fullMsg := fmt.Sprintf("%s [%s] %s", timestamp, prefix, msg)

	// Write to stderr for console visibility
	fmt.Fprintln(os.Stderr, fullMsg)

	// Write to log file
	logMutex.Lock()
	if logFile != nil {
		logFile.WriteString(fullMsg + "\n")
		logFile.Sync()
	}
	logMutex.Unlock()
}

// API logs a message with [API] prefix
func API(format string, v ...interface{}) {
	Log("API", format, v...)
}

// DownloadManager logs a message with [DownloadManager] prefix
func DownloadManager(format string, v ...interface{}) {
	Log("DownloadManager", format, v...)
}

// SpotifySession logs a message with [SpotifySession] prefix
func SpotifySession(format string, v ...interface{}) {
	Log("SpotifySession", format, v...)
}

// SpotifyDownloader logs a message with [SpotifyDownloader] prefix
func SpotifyDownloader(format string, v ...interface{}) {
	Log("SpotifyDownloader", format, v...)
}

// Main logs a message with [Main] prefix
func Main(format string, v ...interface{}) {
	Log("Main", format, v...)
}
