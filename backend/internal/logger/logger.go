// Package logger provides a shared logging facility for ViiB MediaHub.
// It writes to both the viib.log file and stderr for immediate visibility.
//
// Log levels:
//   - DEBUG: Verbose messages for development (request logs, detailed state)
//   - INFO:  Normal operational messages (startup, shutdown, major events)
//   - WARN:  Warning conditions
//   - ERROR: Error conditions
//
// The debug flag controls whether DEBUG level messages are written.
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	logFile     *os.File
	logMutex    sync.Mutex
	initialized bool
	currentDir  string
	debugMode   bool // Controls whether DEBUG level messages are written
)

// Init initializes the logger with the given data directory.
// It creates the viib.log file and sets up logging.
// If already initialized with the same directory, this is a no-op.
// If initialized with a different directory, the old file is closed.
func Init(dataDir string) error {
	return InitWithDebug(dataDir, false)
}

// InitWithDebug initializes the logger with debug mode control.
// When debug is true, DEBUG level messages are written to the log.
func InitWithDebug(dataDir string, debug bool) error {
	logMutex.Lock()
	defer logMutex.Unlock()

	// If already initialized with the same directory, just update debug mode
	if initialized && currentDir == dataDir {
		debugMode = debug
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
	debugMode = debug

	// Write initialization header
	timestamp := time.Now().Format("2006/01/02 15:04:05.000000")
	logFile.WriteString(fmt.Sprintf("%s [INIT] === ViiB MediaHub Logger Initialized ===\n", timestamp))
	if debug {
		logFile.WriteString(fmt.Sprintf("%s [INIT] Debug mode enabled - verbose logging active\n", timestamp))
	}
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

// SetDebug enables or disables debug logging at runtime.
func SetDebug(debug bool) {
	logMutex.Lock()
	debugMode = debug
	logMutex.Unlock()
}

// IsDebug returns whether debug mode is enabled.
func IsDebug() bool {
	logMutex.Lock()
	defer logMutex.Unlock()
	return debugMode
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

// Debug writes a debug-level message (only when debug mode is enabled).
// Use for verbose operational details like request logs.
func Debug(prefix, format string, v ...interface{}) {
	logMutex.Lock()
	isDebug := debugMode
	logMutex.Unlock()

	if !isDebug {
		return
	}
	Log(prefix, format, v...)
}

// API logs a message with [API] prefix
func API(format string, v ...interface{}) {
	Log("API", format, v...)
}

// APIDebug logs a debug message with [API] prefix
func APIDebug(format string, v ...interface{}) {
	Debug("API", format, v...)
}

// DownloadManager logs a message with [DownloadManager] prefix
func DownloadManager(format string, v ...interface{}) {
	Log("DownloadManager", format, v...)
}

// DownloadManagerDebug logs a debug message with [DownloadManager] prefix
func DownloadManagerDebug(format string, v ...interface{}) {
	Debug("DownloadManager", format, v...)
}

// SpotifySession logs a message with [SpotifySession] prefix
func SpotifySession(format string, v ...interface{}) {
	Log("SpotifySession", format, v...)
}

// SpotifyDownloader logs a message with [SpotifyDownloader] prefix
func SpotifyDownloader(format string, v ...interface{}) {
	Log("SpotifyDownloader", format, v...)
}

// SpotifyStreamer logs a message with [SpotifyStreamer] prefix
func SpotifyStreamer(format string, v ...interface{}) {
	Log("SpotifyStreamer", format, v...)
}

// Main logs a message with [Main] prefix
func Main(format string, v ...interface{}) {
	Log("Main", format, v...)
}

// MainDebug logs a debug message with [Main] prefix
func MainDebug(format string, v ...interface{}) {
	Debug("Main", format, v...)
}

// Scanner logs a message with [Scanner] prefix
func Scanner(format string, v ...interface{}) {
	Log("Scanner", format, v...)
}

// ScannerDebug logs a debug message with [Scanner] prefix
func ScannerDebug(format string, v ...interface{}) {
	Debug("Scanner", format, v...)
}

// Gemini logs a message with [Gemini] prefix
func Gemini(format string, v ...interface{}) {
	Log("Gemini", format, v...)
}

// GeminiDebug logs a debug message with [Gemini] prefix
func GeminiDebug(format string, v ...interface{}) {
	Debug("Gemini", format, v...)
}
