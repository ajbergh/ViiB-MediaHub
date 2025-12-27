// Package api provides the REST API handlers for ViiB MediaHub.
// This file implements the Spotify download management system.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/ajbergh/viib-mediahub/internal/spotify"
	"github.com/google/uuid"
)

// Default and limits for concurrent downloads
const (
	DefaultConcurrentDownloads = 3
	MinConcurrentDownloads     = 1
	MaxConcurrentDownloads     = 10
)

// dmLog is a helper for download manager logging
func dmLog(format string, v ...interface{}) {
	logger.DownloadManager(format, v...)
}

// dmLogDebug is a helper for verbose download manager logging (debug mode only)
func dmLogDebug(format string, v ...interface{}) {
	logger.DownloadManagerDebug(format, v...)
}

// StallTimeout is the duration after which a download is considered stalled
// if no progress has been made.
const StallTimeout = 60 * time.Second

// DownloadManager manages the Spotify download queue and processing.
// It maintains a persistent queue in SQLite and processes downloads concurrently.
// The manager uses librespot-go to authenticate with Spotify using OAuth tokens
// and streams tracks directly from Spotify servers.
//
// Architecture:
//   - Downloads are queued in the spotify_downloads table
//   - A worker pool processes downloads concurrently (configurable 1-10 workers)
//   - Real-time progress updates are sent via SSE (Server-Sent Events)
//   - OAuth access tokens are retrieved from database before each download
//   - Downloaded files are saved as OGG Vorbis in {downloadDir}/{artist}/{track}.ogg
//
// Reliability Features:
//   - Rate limiting (1s delay between downloads) to prevent Spotify throttling
//   - Retry logic with exponential backoff (3 attempts) for transient errors
//   - Session reset on persistent "invalid key size" errors
//   - Stall detection: downloads without progress for 60s are auto-restarted
//   - Auth failure detection: notifies frontend when re-authentication is needed
type DownloadManager struct {
	db               *db.DB                        // Database for persistent queue
	downloadDir      string                        // Root directory for downloaded files
	sessionManager   *spotify.SessionManager       // Manages librespot session lifecycle
	downloader       *spotify.Downloader           // Handles actual track downloads
	scanner          *scanner.Scanner              // Scanner for auto-rescan on download complete
	isRunning        bool                          // Whether the background processor is active
	activeDownloads  map[string]context.CancelFunc // Map of download ID -> cancel function
	downloadProgress map[string]downloadTracker    // Tracks last progress update time for stall detection
	activeCount      int32                         // Atomic counter for active downloads
	maxConcurrent    int32                         // Maximum concurrent downloads (configurable)
	authRequired     bool                          // Whether Spotify re-authentication is needed
	authRequiredMu   sync.RWMutex                  // Protects authRequired
	mu               sync.RWMutex                  // Protects activeDownloads and isRunning
	ctx              context.Context               // Context for graceful shutdown
	cancel           context.CancelFunc            // Cancel function for context
	progressChan     chan DownloadProgress         // Channel for SSE progress updates
	workChan         chan *db.SpotifyDownload      // Channel for dispatching work to workers
	workerWg         sync.WaitGroup                // Wait group for worker goroutines
	lastDownloadTime time.Time                     // For rate limiting between downloads
	rateLimitMu      sync.Mutex                    // Protects lastDownloadTime
}

// downloadTracker tracks the progress of an active download for stall detection
type downloadTracker struct {
	lastProgress   int       // Last reported progress percentage
	lastUpdateTime time.Time // When progress was last updated
	title          string    // Track title for logging
}

// DownloadProgress represents download progress for real-time updates via SSE.
// This struct is sent through the progressChan and serialized to JSON for
// Server-Sent Events streaming to the frontend.
//
// Status values:
//   - "queued": Download is waiting in queue
//   - "downloading": Download is in progress
//   - "completed": Download finished successfully
//   - "failed": Download failed (see Error field)
//   - "auth_required": Spotify authentication required (token expired/revoked)
type DownloadProgress struct {
	DownloadID string `json:"downloadId"`      // Unique download identifier
	Status     string `json:"status"`          // "queued", "downloading", "completed", "failed", "auth_required"
	Progress   int    `json:"progress"`        // 0-100 percentage
	Error      string `json:"error,omitempty"` // Error message if status is "failed"
}

// DownloadMetadata contains additional metadata for organizing downloaded files.
// This struct is serialized to JSON and stored in the SpotifyDownload.Metadata field.
//
// For album downloads:
//   - Files are saved to: {AlbumArtist}/{Album}/{TrackNumber}-{AlbumArtist}-{Title}.ogg
//
// For playlist downloads:
//   - Files are saved to: {PlaylistName}/{PlaylistOrder}-{Artist}-{Title}.ogg
type DownloadMetadata struct {
	TrackNumber   int    `json:"trackNumber,omitempty"`   // Track number within album (1-based)
	DiscNumber    int    `json:"discNumber,omitempty"`    // Disc number for multi-disc albums
	AlbumArtist   string `json:"albumArtist,omitempty"`   // Primary album artist
	ReleaseDate   string `json:"releaseDate,omitempty"`   // Album release date (YYYY-MM-DD)
	Genre         string `json:"genre,omitempty"`         // Genre classification
	PlaylistName  string `json:"playlistName,omitempty"`  // Playlist name (for playlist downloads)
	PlaylistOrder int    `json:"playlistOrder,omitempty"` // Position in playlist (1-based)
	ImageURL      string `json:"imageUrl,omitempty"`      // Album/playlist artwork URL for cover.jpg
}

// NewDownloadManager creates a new download manager.
// The manager is created in a stopped state and must be started with Start().
// It initializes a SessionManager and Downloader but does not authenticate
// with Spotify until the first download is processed.
//
// Parameters:
//   - database: Database connection for persistent queue and settings
//   - downloadDir: Root directory where downloaded files will be saved
//
// Returns:
//   - Initialized DownloadManager ready to be started
func NewDownloadManager(database *db.DB, downloadDir string) *DownloadManager {
	ctx, cancel := context.WithCancel(context.Background())

	// Create session manager (will be initialized with access token when needed)
	cacheDir := downloadDir + "/.cache"
	sessionManager := spotify.NewSessionManager("", cacheDir)

	// Create downloader
	downloader := spotify.NewDownloader(sessionManager, downloadDir)

	// Load concurrent downloads setting from database, default to 3
	maxConcurrent := int32(DefaultConcurrentDownloads)
	if val, err := database.GetSetting("concurrent_downloads"); err == nil && val != "" {
		if n, err := strconv.Atoi(val); err == nil && n >= MinConcurrentDownloads && n <= MaxConcurrentDownloads {
			maxConcurrent = int32(n)
		}
	}
	dmLog("Max concurrent downloads: %d", maxConcurrent)

	dm := &DownloadManager{
		db:               database,
		downloadDir:      downloadDir,
		sessionManager:   sessionManager,
		downloader:       downloader,
		isRunning:        false,
		activeDownloads:  make(map[string]context.CancelFunc),
		downloadProgress: make(map[string]downloadTracker),
		activeCount:      0,
		maxConcurrent:    maxConcurrent,
		ctx:              ctx,
		cancel:           cancel,
		progressChan:     make(chan DownloadProgress, 100),
		workChan:         make(chan *db.SpotifyDownload, 50), // Buffer for queued work
	}

	return dm
}

// SetScanner sets the scanner reference for auto-rescan on download complete
func (dm *DownloadManager) SetScanner(sc *scanner.Scanner) {
	dm.scanner = sc
}

// Start begins processing the download queue with a pool of worker goroutines.
// This method is idempotent - calling it multiple times has no effect.
// The background processor runs with a 1-second ticker, dispatching queued
// downloads to available workers up to the configured concurrency limit.
//
// The goroutine will continue running until Stop() is called or the
// application context is cancelled.
func (dm *DownloadManager) Start() {
	dm.mu.Lock()
	if dm.isRunning {
		dm.mu.Unlock()
		dmLog("already running")
		return
	}
	dm.isRunning = true
	dm.mu.Unlock()

	// Reset any downloads that were stuck in 'downloading' status from a previous crash
	if count, err := dm.db.ResetStuckDownloads(); err != nil {
		dmLog("Warning: failed to reset stuck downloads: %v", err)
	} else if count > 0 {
		dmLog("Reset %d stuck downloads to queued status", count)
	}

	// Start worker goroutines
	workerCount := int(atomic.LoadInt32(&dm.maxConcurrent))
	for i := 0; i < workerCount; i++ {
		dm.workerWg.Add(1)
		go dm.downloadWorker(i)
	}
	dmLog("Started %d download workers", workerCount)

	go dm.processQueue()
	dmLog("started successfully")
}

// SetMaxConcurrent updates the maximum concurrent downloads.
// This takes effect on the next dispatch cycle. Existing downloads continue.
func (dm *DownloadManager) SetMaxConcurrent(n int) error {
	if n < MinConcurrentDownloads || n > MaxConcurrentDownloads {
		return fmt.Errorf("concurrent downloads must be between %d and %d", MinConcurrentDownloads, MaxConcurrentDownloads)
	}

	oldCount := atomic.LoadInt32(&dm.maxConcurrent)
	atomic.StoreInt32(&dm.maxConcurrent, int32(n))
	dmLog("Updated max concurrent downloads: %d -> %d", oldCount, n)

	// Persist to database
	if err := dm.db.SetSetting("concurrent_downloads", strconv.Itoa(n)); err != nil {
		dmLog("Warning: failed to persist concurrent_downloads setting: %v", err)
	}

	// If we need more workers, spawn them
	if int32(n) > oldCount {
		dm.mu.Lock()
		if dm.isRunning {
			for i := int(oldCount); i < n; i++ {
				dm.workerWg.Add(1)
				go dm.downloadWorker(i)
			}
			dmLog("Spawned %d additional workers", n-int(oldCount))
		}
		dm.mu.Unlock()
	}

	return nil
}

// GetMaxConcurrent returns the current max concurrent downloads setting.
func (dm *DownloadManager) GetMaxConcurrent() int {
	return int(atomic.LoadInt32(&dm.maxConcurrent))
}

// SetDownloadDir updates the download directory for new downloads.
// Existing active downloads will continue to their original destination.
// The scanner is also updated if one is configured.
func (dm *DownloadManager) SetDownloadDir(dir string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	oldDir := dm.downloadDir
	dm.downloadDir = dir

	// Update the downloader with the new directory
	dm.downloader = spotify.NewDownloader(dm.sessionManager, dir)

	// Update the scanner if configured
	if dm.scanner != nil {
		dm.scanner.SetSpotifyDownloadDir(dir)
	}

	dmLog("Updated download directory: %s -> %s", oldDir, dir)
	return nil
}

// GetDownloadDir returns the current download directory
func (dm *DownloadManager) GetDownloadDir() string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.downloadDir
}

// ensureSession ensures the Spotify session is initialized with current access token.
// This method retrieves OAuth credentials from the database and initializes the
// librespot session if needed. It's called before each download to ensure fresh
// authentication.
//
// The method performs the following steps:
//  1. Retrieves spotify_credentials from database settings
//  2. Parses the JSON to extract the access token
//  3. Checks for token expiry and refreshes if needed
//  4. Updates the SessionManager with the current token
//  5. Initializes the librespot session (if not already initialized)
//
// Returns:
//   - error if credentials are missing, invalid, or session initialization fails
func (dm *DownloadManager) ensureSession() error {
	dmLog("Ensuring Spotify session is ready...")

	// Get Spotify credentials from database
	val, err := dm.db.GetSetting("spotify_credentials")
	if err != nil {
		dmLog("Error getting spotify_credentials from database: %v", err)
		return fmt.Errorf("spotify credentials not configured: %w", err)
	}
	if val == "" {
		dmLog("spotify_credentials is empty in database")
		return fmt.Errorf("spotify credentials not configured")
	}
	dmLog("Got credentials from database (length: %d)", len(val))

	var creds struct {
		ClientId     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		Expiry       int64  `json:"expiry"`
	}
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		dmLog("Failed to parse credentials JSON: %v", err)
		return fmt.Errorf("failed to parse credentials: %w", err)
	}

	if creds.AccessToken == "" {
		dmLog("Access token is empty in parsed credentials")
		dm.setAuthRequired(true, "Access token missing - please reconnect to Spotify")
		return fmt.Errorf("access token not found in credentials")
	}

	// Check if token is expired or about to expire (within 5 minutes)
	if creds.Expiry > 0 {
		expiryTime := time.Unix(creds.Expiry/1000, 0) // Expiry is in milliseconds
		expiryBuffer := 5 * time.Minute
		dmLog("Token expiry: %s (now: %s)", expiryTime.Format(time.RFC3339), time.Now().Format(time.RFC3339))

		if time.Now().Add(expiryBuffer).After(expiryTime) {
			dmLog("Access token expired or expiring soon, attempting refresh...")

			// Attempt token refresh if we have refresh token and client credentials
			if creds.RefreshToken != "" && creds.ClientId != "" && creds.ClientSecret != "" {
				newToken, newExpiry, err := dm.refreshAccessToken(creds.ClientId, creds.ClientSecret, creds.RefreshToken)
				if err != nil {
					dmLog("Failed to refresh token: %v", err)
					// Check if refresh token was revoked - this requires re-authentication
					if strings.Contains(err.Error(), "invalid_grant") || strings.Contains(err.Error(), "revoked") {
						dm.setAuthRequired(true, "Spotify session expired - please reconnect to Spotify")
						return fmt.Errorf("spotify re-authentication required: %w", err)
					}
					// Other refresh failures - try with old token but it will likely fail
				} else {
					dmLog("Token refreshed successfully, new expiry: %s", time.Unix(newExpiry/1000, 0).Format(time.RFC3339))
					creds.AccessToken = newToken
					creds.Expiry = newExpiry
					// Clear authRequired since we have fresh credentials
					dm.setAuthRequired(false, "")

					// Save the refreshed credentials back to database
					updatedCreds, _ := json.Marshal(creds)
					if err := dm.db.SetSetting("spotify_credentials", string(updatedCreds)); err != nil {
						dmLog("Warning: Failed to save refreshed credentials: %v", err)
					}
				}
			} else {
				dmLog("Cannot refresh: missing refresh token or client credentials")
				dm.setAuthRequired(true, "Cannot refresh Spotify token - please reconnect to Spotify")
				return fmt.Errorf("spotify re-authentication required: missing credentials for refresh")
			}
		}
	}

	dmLog("Access token present (length: %d), updating session manager...", len(creds.AccessToken))

	// Update session manager with current access token
	dm.sessionManager.UpdateAccessToken(creds.AccessToken)

	// Initialize session if not already done
	dmLog("Initializing session...")
	if err := dm.sessionManager.Initialize(); err != nil {
		dmLog("Failed to initialize session: %v", err)
		return fmt.Errorf("failed to initialize session: %w", err)
	}

	dmLog("Session ready!")
	return nil
}

// refreshAccessToken refreshes an expired OAuth access token using the refresh token.
// This implements the Spotify OAuth token refresh flow.
//
// Parameters:
//   - clientId: Spotify OAuth client ID
//   - clientSecret: Spotify OAuth client secret
//   - refreshToken: Refresh token from previous authorization
//
// Returns:
//   - newAccessToken: Fresh access token
//   - newExpiry: New expiry timestamp in milliseconds
//   - error: If refresh fails
func (dm *DownloadManager) refreshAccessToken(clientId, clientSecret, refreshToken string) (string, int64, error) {
	dmLog("Refreshing Spotify access token...")

	// Make token refresh request to Spotify
	client := &http.Client{Timeout: 10 * time.Second}

	data := fmt.Sprintf("grant_type=refresh_token&refresh_token=%s&client_id=%s&client_secret=%s",
		refreshToken, clientId, clientSecret)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token",
		strings.NewReader(data))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", 0, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Calculate new expiry timestamp (in milliseconds, with small buffer)
	newExpiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second).UnixMilli()

	dmLog("Token refresh successful, expires in %d seconds", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, newExpiry, nil
}

// setAuthRequired updates the authRequired flag and sends an SSE notification.
// This is called when the Spotify token is expired/revoked and re-authentication is needed.
func (dm *DownloadManager) setAuthRequired(required bool, message string) {
	dm.authRequiredMu.Lock()
	wasRequired := dm.authRequired
	dm.authRequired = required
	dm.authRequiredMu.Unlock()

	// Only send notification if state changed to required
	if required && !wasRequired {
		dmLog("AUTH REQUIRED: %s", message)
		// Send auth_required event via SSE
		dm.progressChan <- DownloadProgress{
			DownloadID: "auth",
			Status:     "auth_required",
			Progress:   0,
			Error:      message,
		}
	} else if !required && wasRequired {
		dmLog("Auth requirement cleared")
	}
}

// IsAuthRequired returns whether Spotify re-authentication is needed.
// The frontend can poll this or listen for auth_required SSE events.
func (dm *DownloadManager) IsAuthRequired() bool {
	dm.authRequiredMu.RLock()
	defer dm.authRequiredMu.RUnlock()
	return dm.authRequired
}

// ClearAuthRequired clears the auth required flag.
// Called after successful re-authentication from the frontend.
func (dm *DownloadManager) ClearAuthRequired() {
	dm.setAuthRequired(false, "")
	// Also reset the session manager to use new credentials
	dm.sessionManager.ResetSession()
}

// EnsureSession is an exported wrapper for ensureSession.
// Used by the streaming API to initialize the session before streaming.
func (dm *DownloadManager) EnsureSession() error {
	return dm.ensureSession()
}

// GetSessionManager returns the session manager for use by the streaming API.
// The session manager must be initialized via EnsureSession before use.
func (dm *DownloadManager) GetSessionManager() *spotify.SessionManager {
	return dm.sessionManager
}

// Stop halts download processing and waits for workers to finish
func (dm *DownloadManager) Stop() {
	dm.mu.Lock()
	if !dm.isRunning {
		dm.mu.Unlock()
		return
	}

	// Cancel all active downloads
	for id, cancelFunc := range dm.activeDownloads {
		dmLog("Cancelling download: %s", id)
		cancelFunc()
	}
	dm.activeDownloads = make(map[string]context.CancelFunc)
	dm.mu.Unlock()

	// Cancel the context to stop the queue processor and workers
	dm.cancel()

	// Wait for all workers to finish (with timeout)
	done := make(chan struct{})
	go func() {
		dm.workerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		dmLog("All workers stopped gracefully")
	case <-time.After(10 * time.Second):
		dmLog("Warning: workers did not stop within timeout")
	}

	dm.mu.Lock()
	dm.isRunning = false
	dm.mu.Unlock()

	dmLog("stopped")
}

// QueueDownload adds a new download to the persistent queue.
// The download is added with status "queued" and will be processed by the
// background worker. Downloads are processed in FIFO order based on added_at timestamp.
//
// Parameters:
//   - spotifyID: Spotify track ID (e.g., "3n3Ppam7vgaVa1iaRUc9Lp")
//   - spotifyURI: Spotify URI (e.g., "spotify:track:3n3Ppam7vgaVa1iaRUc9Lp")
//   - downloadType: "track", "album", or "playlist"
//   - title: Track title
//   - artist: Primary artist name
//   - album: Album name
//   - metadata: Optional additional metadata for file organization (can be nil)
//
// Returns:
//   - Download ID (UUID) for tracking
//   - error if database insertion fails
func (dm *DownloadManager) QueueDownload(spotifyID, spotifyURI, downloadType, title, artist, album string, metadata *DownloadMetadata) (string, error) {
	var metadataJSON string
	if metadata != nil {
		jsonBytes, err := json.Marshal(metadata)
		if err != nil {
			dmLog("Warning: failed to marshal metadata: %v", err)
		} else {
			metadataJSON = string(jsonBytes)
		}
	}

	download := &db.SpotifyDownload{
		ID:         uuid.New().String(),
		SpotifyID:  spotifyID,
		SpotifyURI: spotifyURI,
		Type:       downloadType,
		Title:      title,
		Artist:     artist,
		Album:      album,
		Status:     "queued",
		Progress:   0,
		AddedAt:    time.Now().Unix(),
		Metadata:   metadataJSON,
	}

	if err := dm.db.AddDownload(download); err != nil {
		return "", fmt.Errorf("failed to queue download: %w", err)
	}

	dmLog("Queued %s download: %s (ID: %s)", downloadType, title, download.ID)
	return download.ID, nil
}

// GetDownload retrieves a download by ID
func (dm *DownloadManager) GetDownload(id string) (*db.SpotifyDownload, error) {
	return dm.db.GetDownload(id)
}

// GetAllDownloads retrieves all downloads
func (dm *DownloadManager) GetAllDownloads() ([]db.SpotifyDownload, error) {
	return dm.db.GetAllDownloads()
}

// DeleteDownload removes a download from the queue
func (dm *DownloadManager) DeleteDownload(id string) error {
	// Check if it's an active download
	dm.mu.Lock()
	if cancelFunc, exists := dm.activeDownloads[id]; exists {
		// Cancel the download
		cancelFunc()
		delete(dm.activeDownloads, id)
		delete(dm.downloadProgress, id)
		dmLog("Cancelled active download: %s", id)
	}
	dm.mu.Unlock()

	return dm.db.DeleteDownload(id)
}

// RetryDownload resets a failed download back to queued status for retry
func (dm *DownloadManager) RetryDownload(id string) error {
	if err := dm.db.ResetDownloadForRetry(id); err != nil {
		return fmt.Errorf("failed to reset download for retry: %w", err)
	}
	dmLog("Reset download for retry: %s", id)

	// Send progress update to trigger UI refresh
	dm.progressChan <- DownloadProgress{
		DownloadID: id,
		Status:     "queued",
		Progress:   0,
	}

	return nil
}

// ForceRestartDownload forcefully cancels an active download (if running) and requeues it.
// This is useful for stuck downloads that are not making progress.
func (dm *DownloadManager) ForceRestartDownload(id string) error {
	// First, check if this download exists
	download, err := dm.db.GetDownload(id)
	if err != nil {
		return fmt.Errorf("download not found: %w", err)
	}

	// Cancel the download if it's active
	dm.mu.Lock()
	if cancelFunc, exists := dm.activeDownloads[id]; exists {
		dmLog("Force cancelling active download: %s - %s", download.Title, id)
		cancelFunc()
		delete(dm.activeDownloads, id)
		delete(dm.downloadProgress, id)
		atomic.AddInt32(&dm.activeCount, -1)
	}
	dm.mu.Unlock()

	// Reset the download to queued status
	if err := dm.db.ResetDownloadForRetry(id); err != nil {
		return fmt.Errorf("failed to reset download for retry: %w", err)
	}

	dmLog("Force restarted download: %s - %s", download.Title, id)

	// Send progress update to trigger UI refresh
	dm.progressChan <- DownloadProgress{
		DownloadID: id,
		Status:     "queued",
		Progress:   0,
	}

	return nil
}

// ClearCompletedDownloads removes all completed downloads from the queue
func (dm *DownloadManager) ClearCompletedDownloads() (int64, error) {
	count, err := dm.db.DeleteCompletedDownloads()
	if err != nil {
		return 0, fmt.Errorf("failed to clear completed downloads: %w", err)
	}
	dmLog("Cleared %d completed downloads", count)
	return count, nil
}

// GetProgressChan returns the progress channel for real-time updates
func (dm *DownloadManager) GetProgressChan() <-chan DownloadProgress {
	return dm.progressChan
}

// processQueue continuously dispatches queued downloads to worker goroutines.
// This is the main dispatcher loop that checks for new downloads every 1 second.
// The loop continues until the context is cancelled (via Stop() or application shutdown).
//
// Design notes:
//   - 5-second startup delay to allow application (systray, etc.) to fully initialize
//   - 1-second interval for responsive queue processing
//   - Dispatches up to maxConcurrent downloads in parallel
//   - Stall detection: checks for downloads without progress for 60 seconds
//   - Graceful shutdown via context cancellation
func (dm *DownloadManager) processQueue() {
	dmLog("Queue processor starting...")

	// Wait for application to fully initialize before processing downloads.
	// This prevents crashes caused by librespot session initialization
	// racing with systray initialization on the main goroutine.
	select {
	case <-dm.ctx.Done():
		dmLog("Queue processor stopped during startup delay")
		return
	case <-time.After(5 * time.Second):
		dmLog("Startup delay complete, beginning queue processing")
	}

	ticker := time.NewTicker(1 * time.Second)            // Faster polling for better responsiveness
	stallCheckTicker := time.NewTicker(10 * time.Second) // Check for stalled downloads every 10 seconds
	statusTicker := time.NewTicker(30 * time.Second)     // Log status every 30 seconds for diagnostics
	defer ticker.Stop()
	defer stallCheckTicker.Stop()
	defer statusTicker.Stop()

	for {
		select {
		case <-dm.ctx.Done():
			dmLog("Queue processor stopped")
			close(dm.workChan) // Signal workers to stop
			return
		case <-ticker.C:
			dm.dispatchDownloads()
		case <-stallCheckTicker.C:
			dm.checkForStalledDownloads()
		case <-statusTicker.C:
			// Periodic status logging for diagnostics
			activeCount := atomic.LoadInt32(&dm.activeCount)
			maxConcurrent := atomic.LoadInt32(&dm.maxConcurrent)
			dm.mu.RLock()
			activeMapLen := len(dm.activeDownloads)
			progressMapLen := len(dm.downloadProgress)
			dm.mu.RUnlock()
			dmLog("Queue status: activeCount=%d/%d, activeDownloads=%d, progressTrackers=%d",
				activeCount, maxConcurrent, activeMapLen, progressMapLen)
		}
	}
}

// checkForStalledDownloads detects downloads that haven't made progress
// within the StallTimeout and automatically restarts them.
func (dm *DownloadManager) checkForStalledDownloads() {
	dm.mu.RLock()
	activeCount := atomic.LoadInt32(&dm.activeCount)
	activeMapLen := len(dm.activeDownloads)
	progressMapLen := len(dm.downloadProgress)
	dm.mu.RUnlock()

	dmLogDebug("Stall check: activeCount=%d, activeDownloads=%d, downloadProgress=%d",
		activeCount, activeMapLen, progressMapLen)

	// Safety check: if activeCount is higher than actual active downloads, correct it
	// This can happen if a worker crashed or exited abnormally
	if int32(activeMapLen) < activeCount {
		diff := activeCount - int32(activeMapLen)
		dmLog("WARNING: activeCount (%d) is higher than activeDownloads map (%d), correcting by %d",
			activeCount, activeMapLen, diff)
		atomic.AddInt32(&dm.activeCount, -diff)
	}

	dm.mu.RLock()
	defer dm.mu.RUnlock()

	now := time.Now()
	for id, tracker := range dm.downloadProgress {
		if now.Sub(tracker.lastUpdateTime) > StallTimeout {
			dmLog("Download stalled (no progress for %v): %s - %s",
				now.Sub(tracker.lastUpdateTime).Round(time.Second), tracker.title, id)

			// Cancel the stalled download
			if cancelFunc, exists := dm.activeDownloads[id]; exists {
				cancelFunc()
			}

			// Mark as failed with stall message so it can be retried
			go func(downloadID string) {
				dm.db.MarkDownloadFailed(downloadID, "Download stalled - no progress for 60 seconds")
				dm.progressChan <- DownloadProgress{
					DownloadID: downloadID,
					Status:     "failed",
					Progress:   0,
					Error:      "Download stalled - no progress for 60 seconds. Click retry to restart.",
				}
			}(id)
		}
	}
}

// dispatchDownloads fetches queued downloads and dispatches them to workers
// up to the configured concurrency limit.
func (dm *DownloadManager) dispatchDownloads() {
	// Check how many slots are available
	currentActive := atomic.LoadInt32(&dm.activeCount)
	maxConcurrent := atomic.LoadInt32(&dm.maxConcurrent)
	available := maxConcurrent - currentActive

	if available <= 0 {
		dmLogDebug("dispatchDownloads: no slots available (active=%d, max=%d)", currentActive, maxConcurrent)
		return // All workers are busy
	}

	// Get queued downloads
	queued, err := dm.db.GetQueuedDownloads()
	if err != nil {
		dmLog("Error fetching queued downloads: %v", err)
		return
	}

	if len(queued) == 0 {
		return // No downloads in queue
	}

	dmLogDebug("dispatchDownloads: %d queued, %d slots available", len(queued), available)

	// Filter out already-active downloads
	dm.mu.RLock()
	var toDispatch []db.SpotifyDownload
	for _, d := range queued {
		if _, active := dm.activeDownloads[d.ID]; !active {
			toDispatch = append(toDispatch, d)
		}
	}
	dm.mu.RUnlock()

	// Dispatch up to 'available' downloads
	dispatched := 0
	for _, download := range toDispatch {
		if int32(dispatched) >= available {
			break
		}

		// Mark as active before sending to worker
		dm.mu.Lock()
		if _, exists := dm.activeDownloads[download.ID]; exists {
			dm.mu.Unlock()
			continue // Already being processed
		}
		_, cancel := context.WithCancel(dm.ctx)
		dm.activeDownloads[download.ID] = cancel
		dm.mu.Unlock()

		// Increment active count
		atomic.AddInt32(&dm.activeCount, 1)

		// Send to worker (non-blocking with buffer)
		select {
		case dm.workChan <- &db.SpotifyDownload{
			ID:         download.ID,
			SpotifyID:  download.SpotifyID,
			SpotifyURI: download.SpotifyURI,
			Type:       download.Type,
			Title:      download.Title,
			Artist:     download.Artist,
			Album:      download.Album,
			Status:     download.Status,
			Progress:   download.Progress,
			AddedAt:    download.AddedAt,
			Metadata:   download.Metadata,
		}:
			dmLog("Dispatched download to worker: %s - %s", download.Title, download.ID)
			dispatched++
			// Store the context for this download
			dm.mu.Lock()
			dm.activeDownloads[download.ID] = cancel
			dm.mu.Unlock()
		default:
			// Work channel full, try again next tick
			dm.mu.Lock()
			delete(dm.activeDownloads, download.ID)
			dm.mu.Unlock()
			atomic.AddInt32(&dm.activeCount, -1)
			cancel() // Cancel context to avoid leak
		}
	}
}

// downloadWorker is a worker goroutine that processes downloads from the work channel.
func (dm *DownloadManager) downloadWorker(workerID int) {
	defer dm.workerWg.Done()
	dmLogDebug("Worker %d started", workerID)

	for {
		select {
		case <-dm.ctx.Done():
			dmLogDebug("Worker %d stopping (context cancelled)", workerID)
			return
		case download, ok := <-dm.workChan:
			if !ok {
				dmLogDebug("Worker %d stopping (channel closed)", workerID)
				return
			}
			dm.processDownload(workerID, download)
		}
	}
}

// processDownload handles a single download in a worker goroutine.
// This method:
//  1. Ensures Spotify session is authenticated with current OAuth token
//  2. Downloads the track using librespot-go
//  3. Updates database with completion status
//  4. Sends progress updates via SSE channel
//  5. Tracks progress for stall detection
//
// Error handling:
//   - Panics: Recovered and logged, download marked as failed
//   - Session errors: Marks download as failed with session error message
//   - Download errors: Marks download as failed with download error message
//   - All errors are logged and sent via SSE for frontend display
func (dm *DownloadManager) processDownload(workerID int, download *db.SpotifyDownload) {
	// Recover from any panics in the download process to prevent worker crash
	defer func() {
		if r := recover(); r != nil {
			dmLog("Worker %d PANIC processing %s: %v", workerID, download.ID, r)
			dm.db.MarkDownloadFailed(download.ID, fmt.Sprintf("Internal error: %v", r))
			dm.progressChan <- DownloadProgress{
				DownloadID: download.ID,
				Status:     "failed",
				Progress:   0,
				Error:      fmt.Sprintf("Internal error: %v", r),
			}
		}

		// Cleanup: remove from active downloads, progress tracker, and decrement counter
		dm.mu.Lock()
		delete(dm.activeDownloads, download.ID)
		delete(dm.downloadProgress, download.ID)
		dm.mu.Unlock()
		newCount := atomic.AddInt32(&dm.activeCount, -1)
		dmLogDebug("Worker %d: finished processing %s, activeCount now %d", workerID, download.ID, newCount)
	}()

	// Get the cancel context for this download
	dm.mu.RLock()
	_, exists := dm.activeDownloads[download.ID]
	dm.mu.RUnlock()
	if !exists {
		dmLog("Worker %d: download %s was cancelled before processing", workerID, download.ID)
		return
	}

	// Initialize progress tracker for stall detection
	dm.mu.Lock()
	dm.downloadProgress[download.ID] = downloadTracker{
		lastProgress:   0,
		lastUpdateTime: time.Now(),
		title:          download.Title,
	}
	dm.mu.Unlock()

	// Mark as started
	if err := dm.db.MarkDownloadStarted(download.ID); err != nil {
		dmLog("Worker %d: error marking download as started: %v", workerID, err)
		return
	}

	// Send progress update
	dm.progressChan <- DownloadProgress{
		DownloadID: download.ID,
		Status:     "downloading",
		Progress:   0,
	}

	// Apply rate limiting to avoid Spotify throttling
	dm.applyRateLimit()

	// Process the download
	dmLog("Worker %d: starting download: %s - %s (SpotifyID: %s)", workerID, download.Title, download.Type, download.SpotifyID)

	// Ensure session is initialized with current access token
	if err := dm.ensureSession(); err != nil {
		dmLog("Worker %d: failed to initialize Spotify session: %v", workerID, err)
		dm.db.MarkDownloadFailed(download.ID, fmt.Sprintf("Session error: %v", err))
		dm.progressChan <- DownloadProgress{
			DownloadID: download.ID,
			Status:     "failed",
			Progress:   0,
			Error:      err.Error(),
		}
		return
	}

	// Create context for this download (allows cancellation)
	ctx, cancel := context.WithCancel(dm.ctx)
	defer cancel()

	// Update the cancel function in case it was replaced
	dm.mu.Lock()
	dm.activeDownloads[download.ID] = cancel
	dm.mu.Unlock()

	// Download with retry logic for transient errors
	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		lastErr = dm.downloadTrack(ctx, download)
		if lastErr == nil {
			// Success - exit retry loop
			break
		}

		// Check if error is retriable (session/key errors)
		errStr := lastErr.Error()
		isRetriable := strings.Contains(errStr, "invalid key size") ||
			strings.Contains(errStr, "session") ||
			strings.Contains(errStr, "crypto") ||
			strings.Contains(errStr, "EOF")

		if !isRetriable || attempt == maxRetries {
			// Non-retriable error or final attempt - give up
			break
		}

		dmLog("Worker %d: download failed for '%s' (attempt %d/%d): %v - retrying...",
			workerID, download.Title, attempt, maxRetries, lastErr)

		// Reset session on persistent failures (after 2nd attempt)
		if attempt >= 2 {
			dmLog("Worker %d: resetting Spotify session after repeated failures", workerID)
			dm.sessionManager.ResetSession()
			// Re-initialize session
			if err := dm.ensureSession(); err != nil {
				dmLog("Worker %d: failed to re-initialize session: %v", workerID, err)
			}
		}

		// Exponential backoff: 2s, 4s, 8s
		backoff := time.Duration(1<<uint(attempt)) * time.Second
		dmLog("Worker %d: waiting %v before retry", workerID, backoff)
		select {
		case <-ctx.Done():
			lastErr = ctx.Err()
			break
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	if lastErr != nil {
		dmLog("Worker %d: download failed for '%s' after %d attempts: %v", workerID, download.Title, maxRetries, lastErr)
		dm.db.MarkDownloadFailed(download.ID, lastErr.Error())
		dm.progressChan <- DownloadProgress{
			DownloadID: download.ID,
			Status:     "failed",
			Progress:   0,
			Error:      lastErr.Error(),
		}
	}
}

// downloadTrack downloads a track using librespot-go and reports progress.
// This method wraps the Downloader.DownloadTrack call with progress tracking
// and database updates.
//
// Progress Tracking:
//   - Since total file size is unknown beforehand, progress is estimated
//   - Assumes average track size of 7MB (typical for ~3-4 minute song)
//   - Progress updates sent every 5% to reduce database writes
//   - Progress capped at 99% until download is fully complete
//   - Updates stall detection tracker on each progress update
//
// Parameters:
//   - download: Database record for the download being processed
//
// Returns:
//   - error if download fails, nil if successful
func (dm *DownloadManager) downloadTrack(ctx context.Context, download *db.SpotifyDownload) error {
	// Progress callback to update database and send SSE updates
	// Captures lastProgress to implement throttling (updates every 5%)
	var lastProgress int
	progressCallback := func(bytesRead int64, totalBytes int64) {
		// Estimate progress based on typical track size (5-10 MB)
		// Since we don't know the total size, we'll estimate progress
		estimatedTotal := int64(7 * 1024 * 1024) // 7MB average
		progress := int((float64(bytesRead) / float64(estimatedTotal)) * 100)
		if progress > 99 {
			progress = 99 // Don't go to 100 until file is complete
		}

		// Update stall detection tracker on ANY progress (even if we don't send SSE update)
		dm.mu.Lock()
		if tracker, exists := dm.downloadProgress[download.ID]; exists {
			tracker.lastProgress = progress
			tracker.lastUpdateTime = time.Now()
			dm.downloadProgress[download.ID] = tracker
		}
		dm.mu.Unlock()

		// Only update UI if progress changed by at least 5%
		if progress >= lastProgress+5 {
			lastProgress = progress
			dm.db.UpdateDownloadProgress(download.ID, progress)
			dm.progressChan <- DownloadProgress{
				DownloadID: download.ID,
				Status:     "downloading",
				Progress:   progress,
			}
		}
	}

	// Parse metadata from database record if present
	var metadata *spotify.DownloadMetadata
	if download.Metadata != "" {
		var m spotify.DownloadMetadata
		if err := json.Unmarshal([]byte(download.Metadata), &m); err != nil {
			dmLog("Warning: failed to parse metadata for download %s: %v", download.ID, err)
		} else {
			metadata = &m
			dmLog("Parsed metadata for download: TrackNumber=%d, AlbumArtist=%s, PlaylistName=%s",
				m.TrackNumber, m.AlbumArtist, m.PlaylistName)
		}
	}

	// Download the track
	filePath, err := dm.downloader.DownloadTrack(
		ctx,
		download.SpotifyID,
		download.Artist,
		download.Title,
		download.Album,
		metadata,
		progressCallback,
	)
	if err != nil {
		return fmt.Errorf("failed to download track: %w", err)
	}

	// Mark as completed
	if err := dm.db.MarkDownloadCompleted(download.ID, filePath); err != nil {
		dmLog("Error marking download as completed: %v", err)
		return err
	}

	dm.progressChan <- DownloadProgress{
		DownloadID: download.ID,
		Status:     "completed",
		Progress:   100,
	}

	dmLog("Download completed: %s -> %s", download.Title, filePath)

	// Notify scanner of completed download for auto-rescan feature
	if dm.scanner != nil {
		dm.scanner.NotifyDownloadComplete()
	}

	return nil
}

// GetActiveDownloadCount returns the number of currently active downloads
func (dm *DownloadManager) GetActiveDownloadCount() int {
	return int(atomic.LoadInt32(&dm.activeCount))
}

// GetActiveDownloads returns the IDs of currently active downloads
func (dm *DownloadManager) GetActiveDownloads() []string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	ids := make([]string, 0, len(dm.activeDownloads))
	for id := range dm.activeDownloads {
		ids = append(ids, id)
	}
	return ids
}

// applyRateLimit ensures minimum delay between downloads to avoid Spotify throttling.
// This helps prevent "invalid key size 0" errors that occur when requests are made too rapidly.
func (dm *DownloadManager) applyRateLimit() {
	const minDelay = 1000 * time.Millisecond // 1 second between downloads

	dm.rateLimitMu.Lock()
	defer dm.rateLimitMu.Unlock()

	if !dm.lastDownloadTime.IsZero() {
		elapsed := time.Since(dm.lastDownloadTime)
		if elapsed < minDelay {
			sleepTime := minDelay - elapsed
			dmLog("Rate limiting: waiting %v before next download", sleepTime)
			time.Sleep(sleepTime)
		}
	}

	dm.lastDownloadTime = time.Now()
}
