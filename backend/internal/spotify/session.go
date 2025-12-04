// Package spotify provides Spotify integration for ViiB MediaHub.
// This file implements session management for librespot-go authentication.
package spotify

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
	_ "github.com/art-media-platform/librespot-go/librespot/core" // Blank import to initialize StartNewSession
	"github.com/art-media-platform/librespot-go/librespot/respot"
)

// sLog is a helper for spotify session logging
func sLog(format string, v ...interface{}) {
	logger.SpotifySession(format, v...)
}

// SessionManager manages Spotify librespot sessions for downloading.
// It handles OAuth authentication using access tokens from the Spotify Web API.
//
// The SessionManager provides:
//   - OAuth token-based authentication (no username/password needed)
//   - Session lifecycle management (initialization, login, cleanup)
//   - Integration with amp.SDK task context for proper resource management
//   - Session state tracking to prevent duplicate initializations
//   - Session refresh when token changes
//
// Usage:
//
//	sm := NewSessionManager(accessToken, cacheDir)
//	err := sm.Initialize()
//	if err != nil { ... }
//	session, err := sm.GetSession()
//	defer sm.Close()
type SessionManager struct {
	session       respot.Session // Active librespot session
	accessToken   string         // OAuth access token from Spotify Web API
	lastTokenUsed string         // Track which token was used to init session
	cacheDir      string         // Directory for librespot cache files
	taskCtx       task.Context   // amp.SDK task context for resource management
	initialized   bool           // Whether session has been initialized
	initTime      int64          // Unix timestamp when session was initialized
	mu            sync.RWMutex   // Protects session state
}

// NewSessionManager creates a new Spotify session manager.
// The manager is created in an uninitialized state. Call Initialize() to
// authenticate with Spotify.
//
// Parameters:
//   - accessToken: OAuth access token from Spotify Web API (can be empty, set later with UpdateAccessToken)
//   - cacheDir: Directory for storing librespot cache files
//
// Returns:
//   - Uninitialized SessionManager ready for Initialize() call
func NewSessionManager(accessToken, cacheDir string) *SessionManager {
	return &SessionManager{
		accessToken: accessToken,
		cacheDir:    cacheDir,
	}
}

// UpdateAccessToken updates the access token for the session.
// This should be called if the OAuth token is refreshed or changed.
// Note: If the token has changed since last initialization, the session
// will be re-initialized with the new token.
//
// Parameters:
//   - accessToken: New OAuth access token from Spotify Web API
func (sm *SessionManager) UpdateAccessToken(accessToken string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.accessToken = accessToken
}

// Initialize creates and authenticates a Spotify session using OAuth.
// If the session is already initialized with a different token, it will
// be re-initialized with the new token.
//
// The initialization process:
//  1. Creates cache directory if it doesn't exist
//  2. Starts an amp.SDK task context for resource management
//  3. Validates the access token is present
//  4. Creates a respot.SessionContext with OAuth token
//  5. Starts a new librespot session
//  6. Performs login authentication
//
// Returns:
//   - error if any step fails (cache creation, token validation, session start, login)
func (sm *SessionManager) Initialize() (err error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if already initialized with the same token
	if sm.initialized && sm.lastTokenUsed == sm.accessToken {
		sLog("Session already initialized with current token, skipping")
		return nil
	}

	// If initialized with a different token, we need to re-initialize
	if sm.initialized && sm.lastTokenUsed != sm.accessToken {
		sLog("Token changed, re-initializing session...")
		// Close existing session
		if sm.session != nil {
			sm.session.Close()
			sm.session = nil
		}
		if sm.taskCtx != nil {
			sm.taskCtx.Close()
			sm.taskCtx = nil
		}
		sm.initialized = false
	}

	sLog("Starting session initialization...")

	// Recover from panics during session initialization
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			err = fmt.Errorf("panic during session initialization: %v", r)
			sLog("PANIC in Initialize: %v\nStack trace:\n%s", r, string(stack))
			// Clean up any partial state
			if sm.taskCtx != nil {
				sm.taskCtx.Close()
				sm.taskCtx = nil
			}
			sm.session = nil
			sm.initialized = false
			sm.lastTokenUsed = ""
		}
	}()

	// Ensure cache directory exists
	sLog("Creating cache directory: %s", sm.cacheDir)
	if err := os.MkdirAll(sm.cacheDir, 0755); err != nil {
		sLog("Failed to create cache directory: %v", err)
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create task context for session management
	sLog("Starting amp.SDK task context...")
	taskCtx, err := task.Start(task.Task{
		Info: task.Info{
			Label: "viib-spotify",
		},
	})
	if err != nil {
		sLog("Failed to start task context: %v", err)
		return fmt.Errorf("failed to start task context: %w", err)
	}
	sm.taskCtx = taskCtx
	sLog("Task context started successfully")

	// Validate access token
	if sm.accessToken == "" {
		sLog("Access token is empty!")
		return fmt.Errorf("access token is required")
	}
	sLog("Access token present (length: %d)", len(sm.accessToken))

	// Create session context
	sLog("Creating respot session context...")
	ctx := respot.DefaultSessionContext("ViiB MediaHub")
	sLog("DefaultSessionContext returned: ctx=%v, ctx.Context=%v", ctx != nil, ctx.Context != nil)
	ctx.Context = taskCtx
	ctx.Login.AuthToken = sm.accessToken
	sLog("SessionContext configured: DeviceName=%s, DeviceUID=%s, AuthToken length=%d, Context=%v",
		ctx.DeviceName, ctx.DeviceUID, len(ctx.Login.AuthToken), ctx.Context != nil)

	// Create new session
	sLog("Starting new librespot session...")
	sess, err := respot.StartNewSession(ctx)
	if err != nil {
		sLog("Failed to create Spotify session: %v", err)
		return fmt.Errorf("failed to create Spotify session: %w", err)
	}
	sLog("Librespot session created, attempting login...")

	// Perform login with OAuth token
	if err := sess.Login(); err != nil {
		sLog("Failed to login to Spotify: %v", err)
		return fmt.Errorf("failed to login to Spotify: %w", err)
	}

	sm.session = sess
	sm.initialized = true
	sm.lastTokenUsed = sm.accessToken
	sm.initTime = time.Now().Unix()
	sLog("Session initialized and logged in successfully!")
	return nil
}

// GetSession returns the active Spotify session.
// The session must be initialized before calling this method.
//
// Returns:
//   - respot.Session: Active session for making Spotify API calls
//   - error: If session is not initialized
//
// Usage:
//
//	session, err := sm.GetSession()
//	if err != nil { ... }
//	asset, err := session.PinTrack(spotifyID, opts)
func (sm *SessionManager) GetSession() (respot.Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if !sm.initialized {
		return nil, fmt.Errorf("session not initialized")
	}
	return sm.session, nil
}

// IsInitialized returns whether the session is currently initialized.
func (sm *SessionManager) IsInitialized() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.initialized
}

// GetInitTime returns the Unix timestamp when the session was initialized.
// Returns 0 if not initialized.
func (sm *SessionManager) GetInitTime() int64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.initTime
}

// Close closes the Spotify session and releases resources.
// This should be called when the session is no longer needed to prevent
// resource leaks. It's safe to call multiple times.
//
// Cleanup operations:
//   - Closes the librespot session
//   - Closes the amp.SDK task context
//   - Sets initialized flag to false
//
// Returns:
//   - error: Only if session close fails (logged as warning, not returned)
func (sm *SessionManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.session != nil {
		if err := sm.session.Close(); err != nil {
			sLog("Warning: Failed to close session: %v", err)
		}
		sm.session = nil
		sm.initialized = false
		sm.lastTokenUsed = ""
		sm.initTime = 0
	}
	if sm.taskCtx != nil {
		sm.taskCtx.Close()
		sm.taskCtx = nil
	}
	return nil
}
