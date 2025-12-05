// Package spotify provides Spotify integration for ViiB MediaHub.
// This file implements track downloading using librespot-go.
package spotify

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
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
func (d *Downloader) DownloadTrack(ctx context.Context, spotifyID string, artist string, title string, album string, metadata *DownloadMetadata, progressCallback ProgressCallback) (string, error) {
	dLog("Getting session for download...")
	sess, err := d.sessionManager.GetSession()
	if err != nil {
		dLog("Failed to get session: %v", err)
		return "", fmt.Errorf("failed to get session: %w", err)
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
		fileName = fmt.Sprintf("%02d-%s-%s.ogg",
			metadata.TrackNumber,
			sanitizeFilename(metadata.AlbumArtist),
			sanitizeFilename(title))
		dLog("Album download: %s -> %s/%s", title, targetDir, fileName)
	} else {
		// Fallback: {Artist}/{Artist - Title}.ogg
		targetDir = filepath.Join(d.downloadDir, sanitizeFilename(artist))
		fileName = fmt.Sprintf("%s - %s.ogg", sanitizeFilename(artist), sanitizeFilename(title))
		dLog("Fallback download: %s -> %s/%s", title, targetDir, fileName)
	}

	// Create download directory if it doesn't exist
	dLog("Creating target directory: %s", targetDir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		dLog("Failed to create directory: %v", err)
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	fullPath := filepath.Join(targetDir, fileName)
	dLog("Target file path: %s", fullPath)

	dLog("Downloading track: %s - %s (ID: %s)", artist, title, spotifyID)

	// Download album/playlist artwork if available and not already downloaded
	if metadata != nil && metadata.ImageURL != "" {
		coverPath := filepath.Join(targetDir, "cover.jpg")
		if err := downloadArtworkIfNeeded(metadata.ImageURL, coverPath); err != nil {
			dLog("Warning: Failed to download artwork: %v", err)
		} else {
			dLog("Artwork downloaded or already exists: %s", coverPath)
		}
	}

	// Pin the track (prepare for download) with 320 kbps quality preference
	dLog("Pinning track on Spotify with 320 kbps quality...")
	pinOpts := respot.PinOpts{
		StartInternally: true,
		Format: asset.AssetFormat{
			// Request highest quality OGG Vorbis (320 kbps) first, then fall back to lower qualities
			AudioFormats: []Spotify.AudioFile_Format{
				Spotify.AudioFile_OGG_VORBIS_320,
				Spotify.AudioFile_OGG_VORBIS_160,
				Spotify.AudioFile_OGG_VORBIS_96,
			},
		},
	}
	assetMedia, err := sess.PinTrack(spotifyID, pinOpts)
	if err != nil {
		dLog("Failed to pin track: %v", err)
		return "", fmt.Errorf("failed to pin track: %w", err)
	}
	dLog("Track pinned successfully (requested 320 kbps)")

	// Log the asset label for debugging
	origName := assetMedia.Label()
	dLog("Asset label: %s", origName)

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		dLog("File already exists, skipping: %s", fullPath)
		return fullPath, nil
	}

	// Create temporary file
	tempPath := fullPath + ".vctemp"
	dLog("Creating temp file: %s", tempPath)

	// Create asset reader
	dLog("Creating asset reader...")
	assetReader, err := assetMedia.NewAssetReader()
	if err != nil {
		dLog("Failed to create asset reader: %v", err)
		return "", fmt.Errorf("failed to create asset reader: %w", err)
	}
	dLog("Asset reader created successfully")

	// Create output file with buffered writer for better I/O performance
	outFile, err := os.Create(tempPath)
	if err != nil {
		dLog("Failed to create output file: %v", err)
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Use buffered writer for better I/O performance on high-bandwidth connections
	bufWriter := bufio.NewWriterSize(outFile, 256*1024) // 256KB write buffer

	// Download with progress tracking
	dLog("Starting download stream...")
	var bytesRead int64
	buf := make([]byte, 64*1024) // 64KB read buffer for better throughput

	for {
		select {
		case <-ctx.Done():
			dLog("Download cancelled by context")
			outFile.Close()
			os.Remove(tempPath)
			return "", ctx.Err()
		default:
		}

		n, err := assetReader.Read(buf)
		if n > 0 {
			if _, writeErr := bufWriter.Write(buf[:n]); writeErr != nil {
				dLog("Failed to write to file: %v", writeErr)
				os.Remove(tempPath)
				return "", fmt.Errorf("failed to write to file: %w", writeErr)
			}
			bytesRead += int64(n)

			// Report progress (we don't know total size, so pass -1)
			if progressCallback != nil {
				progressCallback(bytesRead, -1)
			}
		}
		if err == io.EOF {
			dLog("Download complete, received EOF. Total bytes: %d", bytesRead)
			break
		}
		if err != nil {
			dLog("Failed to read from asset: %v", err)
			os.Remove(tempPath)
			return "", fmt.Errorf("failed to read from asset: %w", err)
		}
	}

	// Flush the buffered writer before closing
	if err := bufWriter.Flush(); err != nil {
		dLog("Failed to flush buffer: %v", err)
		os.Remove(tempPath)
		return "", fmt.Errorf("failed to flush buffer: %w", err)
	}

	// Close the file before renaming
	if err := outFile.Close(); err != nil {
		dLog("Failed to close file: %v", err)
		os.Remove(tempPath)
		return "", fmt.Errorf("failed to close file: %w", err)
	}

	// Rename temporary file to final name
	dLog("Renaming temp file to final: %s", fullPath)
	if err := os.Rename(tempPath, fullPath); err != nil {
		dLog("Failed to rename file: %v", err)
		os.Remove(tempPath)
		return "", fmt.Errorf("failed to rename file: %w", err)
	}

	// Write Vorbis metadata tags to the OGG file
	dLog("Writing metadata tags to file...")
	if err := d.writeOggMetadata(fullPath, artist, title, album, metadata); err != nil {
		// Log but don't fail - file is still usable without metadata
		dLog("Warning: Failed to write metadata: %v", err)
	} else {
		dLog("Metadata written successfully")
	}

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
	// Replace invalid characters with underscores
	re := regexp.MustCompile(`[<>:"/\\|?*]`)
	sanitized := re.ReplaceAllString(name, "_")

	// Remove leading/trailing spaces and dots
	sanitized = strings.TrimSpace(sanitized)
	sanitized = strings.Trim(sanitized, ".")

	// Limit length to 200 characters
	if len(sanitized) > 200 {
		sanitized = sanitized[:200]
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
func downloadArtworkIfNeeded(imageURL, coverPath string) error {
	if imageURL == "" {
		dLog("No artwork URL provided, skipping cover download")
		return nil
	}

	// Check if cover already exists
	if _, err := os.Stat(coverPath); err == nil {
		dLog("Cover already exists at %s, skipping download", coverPath)
		return nil
	}

	dLog("Downloading artwork to %s", coverPath)

	// Download the image
	resp, err := http.Get(imageURL)
	if err != nil {
		return fmt.Errorf("failed to download artwork: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download artwork: HTTP %d", resp.StatusCode)
	}

	// Create the cover file
	coverFile, err := os.Create(coverPath)
	if err != nil {
		return fmt.Errorf("failed to create cover file: %w", err)
	}
	defer coverFile.Close()

	// Copy the image data
	_, err = io.Copy(coverFile, resp.Body)
	if err != nil {
		os.Remove(coverPath) // Clean up partial file
		return fmt.Errorf("failed to save artwork: %w", err)
	}

	dLog("Artwork downloaded successfully to %s", coverPath)
	return nil
}
