// Package spotify provides Spotify integration for ViiB MediaHub.
// This file implements track downloading using librespot-go.
package spotify

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
	"github.com/art-media-platform/librespot-go/Spotify"
	"github.com/art-media-platform/librespot-go/librespot/asset"
	"github.com/art-media-platform/librespot-go/librespot/respot"
	"go.senan.xyz/taglib"
)

// dLog is a helper for downloader logging
func dLog(format string, v ...interface{}) {
	logger.SpotifyDownloader(format, v...)
}

// DownloadMetadata contains additional metadata for organizing downloaded files.
// This mirrors the DownloadMetadata struct in the api package but is defined here
// to avoid import cycles.
type DownloadMetadata struct {
	TrackNumber   int    `json:"trackNumber,omitempty"`   // Track number within album (1-based)
	DiscNumber    int    `json:"discNumber,omitempty"`    // Disc number for multi-disc albums
	AlbumArtist   string `json:"albumArtist,omitempty"`   // Primary album artist
	ReleaseDate   string `json:"releaseDate,omitempty"`   // Album release date (YYYY-MM-DD)
	Genre         string `json:"genre,omitempty"`         // Genre classification
	PlaylistName  string `json:"playlistName,omitempty"`  // Playlist name (for playlist downloads)
	PlaylistOrder int    `json:"playlistOrder,omitempty"` // Position in playlist (1-based)
	ImageURL      string `json:"imageUrl,omitempty"`      // Album/playlist artwork URL
}

// Downloader handles downloading tracks from Spotify using librespot.
// It streams audio directly from Spotify servers and saves as OGG Vorbis files.
//
// Features:
//   - Direct streaming from Spotify (no transcoding)
//   - Progress callbacks for real-time updates
//   - Automatic directory creation (organized by artist/album or playlist)
//   - Filename sanitization (removes invalid characters)
//   - Duplicate detection (skips if file already exists)
//   - Atomic writes (temp file renamed on completion)
//
// Downloader handles downloading tracks from Spotify using librespot.
// It streams audio from Spotify servers and writes OGG files to disk.
type Downloader struct {
	sessionManager *SessionManager // Session for Spotify authentication
	downloadDir    string          // Root directory for downloads
}

// NewDownloader creates a new Spotify downloader.
//
// Parameters:
//   - sessionManager: Initialized SessionManager for Spotify authentication
//   - downloadDir: Root directory where files will be saved
//
// Returns:
//   - Ready-to-use Downloader instance
//
// NewDownloader constructs a Downloader which uses the provided
// SessionManager for Spotify authentication and writes files to
// the specified download directory.
func NewDownloader(sessionManager *SessionManager, downloadDir string) *Downloader {
	return &Downloader{
		sessionManager: sessionManager,
		downloadDir:    downloadDir,
	}
}

// MinValidFileSize is the minimum size for a valid downloaded track file.
// Files smaller than this are considered incomplete/corrupt and will be re-downloaded.
const MinValidFileSize = 4 * 1024 // Ogg headers plus a small amount of audio

type destinationLock struct {
	semaphore chan struct{}
	refs      int
}

var destinationLocks = struct {
	sync.Mutex
	items map[string]*destinationLock
}{items: make(map[string]*destinationLock)}

func lockDestination(ctx context.Context, path string) (func(), error) {
	destinationLocks.Lock()
	lock := destinationLocks.items[path]
	if lock == nil {
		lock = &destinationLock{semaphore: make(chan struct{}, 1)}
		destinationLocks.items[path] = lock
	}
	lock.refs++
	destinationLocks.Unlock()
	releaseReference := func() {
		destinationLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(destinationLocks.items, path)
		}
		destinationLocks.Unlock()
	}
	select {
	case lock.semaphore <- struct{}{}:
		return func() {
			<-lock.semaphore
			releaseReference()
		}, nil
	case <-ctx.Done():
		releaseReference()
		return nil, ctx.Err()
	}
}

// isValidDownloadedFile checks if an existing file is a valid complete download.
// Returns true if the file:
// 1. Exists
// 2. Is larger than MinValidFileSize (1 MB)
// 3. Has valid OGG headers
// 4. Has metadata tags written (artist, title)
//
// If any check fails, the file should be re-downloaded.
func isValidDownloadedFile(filePath string) bool {
	// Check if file exists and get its size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return false // File doesn't exist
	}

	// Check minimum file size
	if fileInfo.Size() < MinValidFileSize {
		dLog("File too small (%d bytes), needs re-download: %s", fileInfo.Size(), filePath)
		return false
	}
	if !isOggFile(filePath) {
		dLog("File is not an Ogg container: %s", filePath)
		return false
	}

	// Check if it's a valid audio file with metadata
	tags, err := taglib.ReadTags(filePath)
	if err != nil {
		dLog("Failed to read tags (may be corrupt): %s - %v", filePath, err)
		return false
	}

	// Helper to get first tag value
	getTag := func(key string) string {
		if vals, ok := tags[key]; ok && len(vals) > 0 {
			return vals[0]
		}
		return ""
	}

	artist := getTag(taglib.Artist)
	title := getTag(taglib.Title)

	// Check for essential metadata - artist and title should be present if properly saved
	if artist == "" || title == "" {
		dLog("Missing metadata (artist=%s, title=%s), needs re-download: %s",
			artist, title, filePath)
		return false
	}

	dLog("Valid existing download found: %s (size=%d, artist=%s, title=%s)",
		filePath, fileInfo.Size(), artist, title)
	return true
}

// ProgressCallback is called during download to report progress.
// This allows the caller to track download progress in real-time.
//
// Parameters:
//   - bytesRead: Number of bytes downloaded so far
//   - totalBytes: Total file size (may be -1 if unknown)
//
// ProgressCallback is invoked with the current bytes read and total bytes
// during a track download so callers may report progress.
type ProgressCallback func(bytesRead int64, totalBytes int64)

// DownloadPhase identifies work that may legitimately have very different
// inactivity windows. In particular, session and asset setup can serialize
// across concurrent workers before the first audio byte is available.
type DownloadPhase string

const (
	DownloadPhaseSession    DownloadPhase = "session setup"
	DownloadPhasePreparing  DownloadPhase = "stream preparation"
	DownloadPhaseStreaming  DownloadPhase = "audio transfer"
	DownloadPhaseFinalizing DownloadPhase = "file finalization"
	DownloadPhaseRetryWait  DownloadPhase = "retry wait"
)

// ActivityCallback reports phase changes and successful setup/finalization
// steps that do not produce byte-progress callbacks.
type ActivityCallback func(phase DownloadPhase)

// DownloadTrack downloads a single Spotify track and saves as OGG Vorbis.
//
// File Organization (per DOWNLOAD_RULES.md):
//
// For album tracks (when metadata.PlaylistName is empty):
//   - Directory: {downloadDir}/{AlbumArtist}/{Album}/
//   - Filename: {TrackNumber}-{AlbumArtist}-{Title}.ogg
//   - Example: /Pink Floyd/The Dark Side of the Moon/01-Pink Floyd-Speak to Me.ogg
//
// For playlist tracks (when metadata.PlaylistName is set):
//   - Directory: {downloadDir}/{PlaylistName}/
//   - Filename: {PlaylistOrder}-{Artist}-{Title}.ogg
//   - Example: /My Favorites/001-The Beatles-Come Together.ogg
//
// Fallback (when no metadata is provided):
//   - Directory: {downloadDir}/{Artist}/
//   - Filename: {Artist} - {Title}.ogg
//
// Download Process:
//  1. Get authenticated session
//  2. Create target directory based on metadata
//  3. Pin track on Spotify (prepare for streaming)
//  4. Create asset reader for streaming
//  5. Stream to temporary file with progress callbacks
//  6. Rename temp file to final name (atomic operation)
//
// Parameters:
//   - ctx: Context for cancellation
//   - spotifyID: Spotify track ID (e.g., "3n3Ppam7vgaVa1iaRUc9Lp")
//   - artist: Artist name (used for playlist tracks or fallback)
//   - title: Track title for filename
//   - album: Album name (used for album directory structure)
//   - metadata: Optional metadata for file organization (can be nil)
//   - progressCallback: Optional callback for progress updates (can be nil)
//
// Returns:
//   - string: Full path to downloaded file
//   - error: If any step fails
//
// DownloadTrack downloads a single Spotify track to an OGG file using the
// provided metadata to organize file location. The function returns the
// full path to the saved file or an error if the download failed.
func (d *Downloader) DownloadTrack(ctx context.Context, spotifyID string, artist string, title string, album string, metadata *DownloadMetadata, progressCallback ProgressCallback, activityCallback ActivityCallback) (string, error) {
	reportActivity := func(phase DownloadPhase) {
		if activityCallback != nil {
			activityCallback(phase)
		}
	}
	reportActivity(DownloadPhasePreparing)
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Determine directory and filename based on metadata
	var targetDir, fileName string

	if metadata != nil && metadata.PlaylistName != "" {
		// Playlist download: {PlaylistName}/{PlaylistOrder}-{Artist}-{Title}.ogg
		targetDir = filepath.Join(d.downloadDir, sanitizeFilename(metadata.PlaylistName))
		// Zero-pad playlist order to 3 digits (e.g., 001, 002, 003)
		fileName = fmt.Sprintf("%03d-%s-%s.ogg",
			metadata.PlaylistOrder,
			sanitizeFilename(artist),
			sanitizeFilename(title))
		dLog("Playlist download: %s -> %s/%s", title, targetDir, fileName)
	} else if metadata != nil && metadata.AlbumArtist != "" && album != "" {
		// Album download: {AlbumArtist}/{Album}/{TrackNumber}-{AlbumArtist}-{Title}.ogg
		targetDir = filepath.Join(d.downloadDir, sanitizeFilename(metadata.AlbumArtist), sanitizeFilename(album))
		// Zero-pad track number to 2 digits (e.g., 01, 02, 10)
		trackPrefix := fmt.Sprintf("%02d", metadata.TrackNumber)
		if metadata.DiscNumber > 1 {
			trackPrefix = fmt.Sprintf("D%02d-%02d", metadata.DiscNumber, metadata.TrackNumber)
		}
		fileName = fmt.Sprintf("%s-%s-%s.ogg", trackPrefix,
			sanitizeFilename(metadata.AlbumArtist), sanitizeFilename(title))
		dLog("Album download: %s -> %s/%s", title, targetDir, fileName)
	} else if album != "" {
		// Single-track downloads still include the album directory so distinct
		// releases with the same artist/title do not alias one another.
		targetDir = filepath.Join(d.downloadDir, sanitizeFilename(artist), sanitizeFilename(album))
		fileName = fmt.Sprintf("%s - %s.ogg", sanitizeFilename(artist), sanitizeFilename(title))
	} else {
		// Fallback: {Artist}/{Artist - Title}.ogg
		targetDir = filepath.Join(d.downloadDir, sanitizeFilename(artist))
		fileName = fmt.Sprintf("%s - %s.ogg", sanitizeFilename(artist), sanitizeFilename(title))
		dLog("Fallback download: %s -> %s/%s", title, targetDir, fileName)
	}

	// Create download directory if it doesn't exist
	dLog("Creating target directory: %s", targetDir)

	// Security: Verify constructed paths are within the download directory
	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve target directory: %w", err)
	}
	absDownloadDir, err := filepath.Abs(d.downloadDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve download directory: %w", err)
	}
	if !strings.HasPrefix(absTargetDir, absDownloadDir+string(filepath.Separator)) && absTargetDir != absDownloadDir {
		return "", fmt.Errorf("target directory %q escapes download directory %q", absTargetDir, absDownloadDir)
	}

	if err := os.MkdirAll(targetDir, 0700); err != nil {
		dLog("Failed to create directory: %v", err)
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	fullPath := filepath.Join(targetDir, fileName)
	dLog("Target file path: %s", fullPath)
	unlock, err := lockDestination(ctx, fullPath)
	if err != nil {
		return "", err
	}
	defer unlock()

	// Avoid all Spotify and artwork network work when a complete local file is
	// already present. The destination lock also serializes duplicate requests.
	if isValidDownloadedFile(fullPath) {
		dLog("Valid existing file found, skipping download: %s", fullPath)
		return fullPath, nil
	}
	if _, err := os.Stat(fullPath); err == nil {
		if err := os.Remove(fullPath); err != nil {
			return "", fmt.Errorf("remove invalid existing file: %w", err)
		}
	}

	dLog("Downloading track: %s - %s (ID: %s)", artist, title, spotifyID)

	// Download album/playlist artwork if available and not already downloaded
	if metadata != nil && metadata.ImageURL != "" {
		coverPath := filepath.Join(targetDir, "cover.jpg")
		if err := downloadArtworkIfNeeded(ctx, metadata.ImageURL, coverPath); err != nil {
			dLog("Warning: Failed to download artwork: %v", err)
		} else {
			dLog("Artwork downloaded or already exists: %s", coverPath)
		}
		reportActivity(DownloadPhasePreparing)
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}

	dLog("Getting session for download...")
	reportActivity(DownloadPhasePreparing)
	sess, releaseSession, err := d.sessionManager.AcquireSession()
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}
	defer releaseSession()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Pin the track without attaching it to the long-lived session task. Each
	// asset gets a child task that is closed at the end of this function.
	dLog("Pinning track on Spotify with 320 kbps quality...")
	reportActivity(DownloadPhasePreparing)
	pinOpts := respot.PinOpts{
		StartInternally: false,
		Format: asset.AssetFormat{
			// Request highest quality OGG Vorbis (320 kbps) first, then fall back to lower qualities
			AudioFormats: []Spotify.AudioFile_Format{
				Spotify.AudioFile_OGG_VORBIS_320,
				Spotify.AudioFile_OGG_VORBIS_160,
				Spotify.AudioFile_OGG_VORBIS_96,
			},
		},
	}
	releaseAudioKeyRequest, err := d.sessionManager.acquireAudioKeyRequest(ctx)
	if err != nil {
		return "", fmt.Errorf("wait to request Spotify audio key: %w", err)
	}
	assetMedia, err := sess.PinTrack(spotifyID, pinOpts)
	releaseAudioKeyRequest()
	if err != nil {
		err = normalizeAudioKeyError(err)
		dLog("Failed to pin track: %v", err)
		return "", fmt.Errorf("failed to pin track: %w", err)
	}
	reportActivity(DownloadPhasePreparing)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dLog("Track pinned successfully (requested 320 kbps)")
	assetCtx, err := sess.Context().Context.StartChild(task.Task{Info: task.Info{Label: "spotify-download-" + spotifyID}})
	if err != nil {
		return "", fmt.Errorf("create asset context: %w", err)
	}
	defer assetCtx.Close()
	if err := assetMedia.OnStart(assetCtx); err != nil {
		return "", fmt.Errorf("start asset: %w", err)
	}
	reportActivity(DownloadPhasePreparing)
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Log the asset label for debugging
	origName := assetMedia.Label()
	dLog("Asset label: %s", origName)

	// Create asset reader
	dLog("Creating asset reader...")
	assetReader, err := assetMedia.NewAssetReader()
	if err != nil {
		dLog("Failed to create asset reader: %v", err)
		return "", fmt.Errorf("failed to create asset reader: %w", err)
	}
	defer assetReader.Close()
	reportActivity(DownloadPhasePreparing)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dLog("Asset reader created successfully")

	// A unique temp path prevents unrelated requests and stale temp files from
	// sharing the same inode.
	outFile, err := os.CreateTemp(targetDir, "."+fileName+".*.vctemp")
	if err != nil {
		dLog("Failed to create output file: %v", err)
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	tempPath := outFile.Name()
	committed := false
	defer func() {
		_ = outFile.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	// Use buffered writer for better I/O performance on high-bandwidth connections
	bufWriter := bufio.NewWriterSize(outFile, 256*1024) // 256KB write buffer

	dLog("Starting download stream...")
	reportActivity(DownloadPhaseStreaming)
	var bytesRead int64
	buf := make([]byte, 64*1024) // 64KB read buffer for better throughput
	readerDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			dLog("Context cancelled, stopping asset download")
			_ = assetReader.Close()
			_ = assetCtx.Close()
		case <-readerDone:
		}
	}()
	defer close(readerDone)

	for {
		n, readErr := assetReader.Read(buf)
		if n > 0 {
			if _, writeErr := bufWriter.Write(buf[:n]); writeErr != nil {
				dLog("Failed to write to file: %v", writeErr)
				return "", fmt.Errorf("failed to write to file: %w", writeErr)
			}
			bytesRead += int64(n)

			// Report progress (we don't know total size, so pass -1)
			if progressCallback != nil {
				progressCallback(bytesRead, -1)
			}
		}
		if readErr == io.EOF {
			dLog("Download complete, received EOF. Total bytes: %d", bytesRead)
			reportActivity(DownloadPhaseFinalizing)
			break
		}
		if readErr != nil {
			// Check if this was caused by context cancellation
			if ctx.Err() != nil {
				dLog("Read error due to context cancellation: %v", readErr)
				return "", ctx.Err()
			}
			dLog("Failed to read from asset: %v", readErr)
			return "", fmt.Errorf("failed to read from asset: %w", readErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Flush the buffered writer before closing
	if err := bufWriter.Flush(); err != nil {
		dLog("Failed to flush buffer: %v", err)
		return "", fmt.Errorf("failed to flush buffer: %w", err)
	}
	reportActivity(DownloadPhaseFinalizing)
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Close the file before renaming
	if err := outFile.Close(); err != nil {
		dLog("Failed to close file: %v", err)
		return "", fmt.Errorf("failed to close file: %w", err)
	}

	// Tags and integrity are part of the successful download transaction.
	if err := d.writeOggMetadata(tempPath, artist, title, album, metadata); err != nil {
		return "", fmt.Errorf("write downloaded file metadata: %w", err)
	}
	reportActivity(DownloadPhaseFinalizing)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !isValidDownloadedFile(tempPath) {
		return "", fmt.Errorf("downloaded file failed Ogg integrity validation")
	}

	// Rename temporary file to final name
	dLog("Renaming temp file to final: %s", fullPath)
	if err := os.Rename(tempPath, fullPath); err != nil {
		dLog("Failed to rename file: %v", err)
		return "", fmt.Errorf("failed to rename file: %w", err)
	}
	committed = true

	dLog("Successfully downloaded: %s (%d bytes)", fileName, bytesRead)
	return fullPath, nil
}

// isOggFile checks if a file is an Ogg container by reading the magic bytes
func isOggFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		return false
	}

	return string(magic) == "OggS"
}

// writeOggMetadata validates the target file is an Ogg container and writes Vorbis tags.
// It implements retry logic to handle Windows file locks.
//
// Tags written per DOWNLOAD_RULES.md:
//   - ARTIST: Track artist
//   - TITLE: Track title
//   - ALBUM: Album name
//   - ALBUMARTIST: Album artist (from metadata)
//   - TRACKNUMBER: Track number (from metadata)
//   - DISCNUMBER: Disc number (from metadata)
//   - DATE: Release date (from metadata)
//   - GENRE: Genre (from metadata, if available)
func (d *Downloader) writeOggMetadata(filePath, artist, title, album string, metadata *DownloadMetadata) error {
	if !isOggFile(filePath) {
		info, _ := os.Stat(filePath)
		size := int64(-1)
		if info != nil {
			size = info.Size()
		}
		return fmt.Errorf("not an Ogg container (magic mismatch) size=%d", size)
	}

	// Sanitize metadata to valid UTF-8
	artist = strings.ToValidUTF8(artist, "")
	title = strings.ToValidUTF8(title, "")
	album = strings.ToValidUTF8(album, "")

	// Build tag map
	tags := map[string][]string{
		"ARTIST": {artist},
		"TITLE":  {title},
		"ALBUM":  {album},
	}

	// Add metadata fields if available
	if metadata != nil {
		if metadata.AlbumArtist != "" {
			tags["ALBUMARTIST"] = []string{strings.ToValidUTF8(metadata.AlbumArtist, "")}
		}
		if metadata.TrackNumber > 0 {
			tags["TRACKNUMBER"] = []string{strconv.Itoa(metadata.TrackNumber)}
		}
		if metadata.DiscNumber > 0 {
			tags["DISCNUMBER"] = []string{strconv.Itoa(metadata.DiscNumber)}
		}
		if metadata.ReleaseDate != "" {
			tags["DATE"] = []string{strings.ToValidUTF8(metadata.ReleaseDate, "")}
		}
		if metadata.Genre != "" {
			tags["GENRE"] = []string{strings.ToValidUTF8(metadata.Genre, "")}
		}
		// For playlist downloads, use playlist name as album
		if metadata.PlaylistName != "" {
			tags["ALBUM"] = []string{strings.ToValidUTF8(metadata.PlaylistName, "")}
			tags["ALBUMARTIST"] = []string{strings.ToValidUTF8(metadata.PlaylistName, "")}
			if metadata.PlaylistOrder > 0 {
				tags["TRACKNUMBER"] = []string{strconv.Itoa(metadata.PlaylistOrder)}
			}
			tags["DISCNUMBER"] = []string{"1"} // Always 1 for playlists
		}
	}

	dLog("Writing metadata: Artist='%s', Title='%s', Album='%s'", artist, title, album)

	// Write tags with retry for Windows file lock handling
	writeFunc := func() error {
		return taglib.WriteTags(filePath, tags, 0)
	}

	if err := writeFunc(); err != nil {
		// Windows file lock latency - wait and retry once
		dLog("First metadata write attempt failed, retrying: %v", err)
		time.Sleep(150 * time.Millisecond)
		if err2 := writeFunc(); err2 != nil {
			return fmt.Errorf("failed to write tags after retry: %w", err2)
		}
	}

	return nil
}

// sanitizeFilename removes or replaces characters that are invalid in filenames.
// This ensures downloaded files have valid names across different filesystems.
//
// Operations:
//   - Replaces invalid characters (<>:"/\|?*) with underscores
//   - Removes leading/trailing spaces and dots
//   - Limits length to 200 characters (prevents filesystem errors)
//
// Parameters:
//   - name: Original filename or directory name
//
// Returns:
//   - Sanitized string safe for filesystem use
func sanitizeFilename(name string) string {
	name = strings.ToValidUTF8(name, "")
	// Replace invalid characters with underscores
	re := regexp.MustCompile(`[<>:"/\\|?*]`)
	sanitized := re.ReplaceAllString(name, "_")

	// Remove leading/trailing spaces and dots
	sanitized = strings.TrimSpace(sanitized)
	sanitized = strings.Trim(sanitized, ".")

	// Block path traversal sequences
	sanitized = strings.ReplaceAll(sanitized, "..", "_")
	if sanitized == "" {
		sanitized = "Unknown"
	}

	base := strings.ToUpper(strings.TrimSuffix(sanitized, filepath.Ext(sanitized)))
	reserved := map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
		"COM6": true, "COM7": true, "COM8": true, "COM9": true,
		"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
		"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	}
	if reserved[base] {
		sanitized = "_" + sanitized
	}

	// Limit by UTF-8 bytes without splitting a rune.
	const maxComponentBytes = 120
	if len(sanitized) > maxComponentBytes {
		length := 0
		var builder strings.Builder
		for _, char := range sanitized {
			encoded := string(char)
			if length+len(encoded) > maxComponentBytes {
				break
			}
			builder.WriteString(encoded)
			length += len(encoded)
		}
		sanitized = builder.String()
	}

	return sanitized
}

// downloadArtworkIfNeeded downloads album/playlist artwork to cover.jpg if it doesn't exist.
// This is called once per album/playlist directory to avoid redundant downloads.
//
// Parameters:
//   - imageURL: URL of the artwork image (from Spotify API)
//   - coverPath: Full path where cover.jpg should be saved
//
// Returns:
//   - nil if successful or if cover already exists
//   - error if download or save fails
func downloadArtworkIfNeeded(ctx context.Context, imageURL, coverPath string) error {
	if imageURL == "" {
		dLog("No artwork URL provided, skipping cover download")
		return nil
	}

	unlock, err := lockDestination(ctx, coverPath)
	if err != nil {
		return err
	}
	defer unlock()

	// Check if a non-empty cover already exists.
	if info, err := os.Stat(coverPath); err == nil && info.Size() > 0 {
		dLog("Cover already exists at %s, skipping download", coverPath)
		return nil
	}

	dLog("Downloading artwork to %s", coverPath)

	// Validate artwork URL domain (only allow Spotify CDN)
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		return fmt.Errorf("invalid artwork URL: %w", err)
	}
	host := strings.ToLower(parsedURL.Hostname())
	if host != "i.scdn.co" && host != "mosaic.scdn.co" && !strings.HasSuffix(host, ".spotifycdn.com") && !strings.HasSuffix(host, ".scdn.co") {
		return fmt.Errorf("artwork URL domain %q not allowed", host)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return fmt.Errorf("create artwork request: %w", err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("failed to download artwork: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download artwork: HTTP %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(contentType, "image/") {
		return fmt.Errorf("artwork response is not an image: %s", contentType)
	}

	coverFile, err := os.CreateTemp(filepath.Dir(coverPath), ".cover.*.vctemp")
	if err != nil {
		return fmt.Errorf("failed to create cover file: %w", err)
	}
	tempPath := coverFile.Name()
	committed := false
	defer func() {
		_ = coverFile.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	const maxArtworkBytes = 20 << 20
	written, err := io.Copy(coverFile, io.LimitReader(resp.Body, maxArtworkBytes+1))
	if err != nil {
		return fmt.Errorf("failed to save artwork: %w", err)
	}
	if written == 0 || written > maxArtworkBytes {
		return fmt.Errorf("artwork size is invalid: %d bytes", written)
	}
	if err := coverFile.Close(); err != nil {
		return fmt.Errorf("close artwork file: %w", err)
	}
	if err := os.Rename(tempPath, coverPath); err != nil {
		return fmt.Errorf("commit artwork: %w", err)
	}
	committed = true

	dLog("Artwork downloaded successfully to %s", coverPath)
	return nil
}
