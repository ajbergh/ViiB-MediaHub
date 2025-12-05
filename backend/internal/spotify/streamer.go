// Package spotify provides Spotify integration for ViiB MediaHub.
// This file implements direct audio streaming using librespot-go.
package spotify

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyProto "github.com/art-media-platform/librespot-go/Spotify"
	"github.com/art-media-platform/librespot-go/librespot/asset"
	"github.com/art-media-platform/librespot-go/librespot/respot"
)

// stLog is a helper for streamer logging
func stLog(format string, v ...interface{}) {
	logger.SpotifyStreamer(format, v...)
}

// StreamInfo contains metadata about an active stream.
type StreamInfo struct {
	SpotifyID   string // Spotify track ID
	Format      string // Audio format (e.g., "OGG_VORBIS_320")
	ContentType string // MIME type for HTTP response
}

// ActiveStream represents an active audio stream from Spotify.
// It wraps the librespot asset reader and provides additional metadata.
// ActiveStream wraps a librespot asset reader and implements Read/Seek/Close
// semantics for streaming audio to an HTTP response. It also exposes
// metadata (Info) about the stream and uses a cancel function for cleanup.
type ActiveStream struct {
	reader    io.ReadSeekCloser  // Underlying audio data reader
	info      StreamInfo         // Stream metadata
	mu        sync.RWMutex       // Protects closed state
	closed    bool               // Whether stream has been closed
	cancelCtx context.CancelFunc // Cancel function for cleanup
}

// Read implements io.Reader for streaming audio data.
// Read reads audio bytes from the underlying asset reader. It returns io.EOF
// if the stream is closed.
func (s *ActiveStream) Read(p []byte) (n int, err error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return 0, io.EOF
	}
	s.mu.RUnlock()
	return s.reader.Read(p)
}

// Seek implements io.Seeker for seeking within the audio stream.
// This enables HTTP Range request support for seeking during playback.
// Seek implements io.Seeker for ActiveStream and allows seeking within
// the open audio stream supporting HTTP Range requests.
func (s *ActiveStream) Seek(offset int64, whence int) (int64, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return 0, fmt.Errorf("stream closed")
	}
	s.mu.RUnlock()
	return s.reader.Seek(offset, whence)
}

// Close releases resources associated with the stream.
// Close releases resources associated with the active stream and
// cancels any internal context used for cleanup.
func (s *ActiveStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.cancelCtx != nil {
		s.cancelCtx()
	}

	if s.reader != nil {
		return s.reader.Close()
	}
	return nil
}

// Info returns metadata about the stream.
// Info returns stream-level metadata such as SpotifyID and format
// which can be used to set HTTP headers for streaming responses.
func (s *ActiveStream) Info() StreamInfo {
	return s.info
}

// Streamer handles streaming audio from Spotify using librespot.
// Unlike Downloader which saves to disk, Streamer provides an io.ReadSeekCloser
// for direct HTTP streaming to the client.
//
// Features:
//   - Direct streaming from Spotify (no disk I/O)
//   - Quality selection with fallback (320kbps → 160kbps → 96kbps)
//   - Seek support for HTTP Range requests
//   - Concurrent stream management
//   - Automatic cleanup on context cancellation
//
// Streamer manages active Spotify audio streams and coordinates session
// usage and cleanup. It exposes StreamTrack methods to open streams
// and CloseAllStreams for cleanup.
type Streamer struct {
	sessionManager *SessionManager          // Session for Spotify authentication
	activeStreams  map[string]*ActiveStream // Track active streams by request ID
	mu             sync.RWMutex             // Protects activeStreams
}

// NewStreamer creates a new Spotify streamer.
//
// Parameters:
//   - sessionManager: Initialized SessionManager for Spotify authentication
//
// Returns:
//   - Ready-to-use Streamer instance
//
// NewStreamer creates a new Streamer instance which uses the provided
// SessionManager for Spotify session authentication and asset pinning.
func NewStreamer(sessionManager *SessionManager) *Streamer {
	return &Streamer{
		sessionManager: sessionManager,
		activeStreams:  make(map[string]*ActiveStream),
	}
}

// StreamTrack opens an audio stream for a Spotify track.
// The returned ActiveStream implements io.ReadSeekCloser and can be used
// to stream audio data directly to an HTTP response.
//
// Quality Selection:
//   - Requests 320kbps OGG Vorbis first (Premium quality)
//   - Falls back to 160kbps, then 96kbps if higher quality unavailable
//
// Parameters:
//   - ctx: Context for cancellation (stream closes when context is cancelled)
//   - spotifyID: Spotify track ID (e.g., "3n3Ppam7vgaVa1iaRUc9Lp")
//   - requestID: Unique identifier for this stream request (for cleanup tracking)
//
// Returns:
//   - *ActiveStream: Audio stream with Read/Seek/Close methods
//   - error: If session or track pinning fails
//
// StreamTrack opens an audio stream for the given Spotify ID using the
// default quality preference and returns an ActiveStream for consumption
// by an HTTP handler.
func (s *Streamer) StreamTrack(ctx context.Context, spotifyID string, requestID string) (*ActiveStream, error) {
	return s.StreamTrackWithQuality(ctx, spotifyID, requestID, "high")
}

// StreamTrackWithQuality opens an audio stream with specified quality preference.
// Quality options:
//   - "high": 320kbps (with 160/96 fallback)
//   - "medium": 160kbps (with 96 fallback)
//   - "low": 96kbps only
//
// Parameters:
//   - ctx: Context for cancellation
//   - spotifyID: Spotify track ID
//   - requestID: Unique identifier for this stream
//   - quality: Quality preference ("high", "medium", "low")
//
// Returns:
//   - *ActiveStream: Audio stream
//   - error: If streaming fails
//
// StreamTrackWithQuality opens a Spotify track stream using the specified
// quality preference ("high"/"medium"/"low") and returns an ActiveStream
// which supports seeking.
func (s *Streamer) StreamTrackWithQuality(ctx context.Context, spotifyID string, requestID string, quality string) (*ActiveStream, error) {
	stLog("Starting stream for track: %s (request: %s, quality: %s)", spotifyID, requestID, quality)

	// Get authenticated session
	sess, err := s.sessionManager.GetSession()
	if err != nil {
		stLog("Failed to get session: %v", err)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Build audio format list based on quality preference
	var audioFormats []spotifyProto.AudioFile_Format
	switch quality {
	case "low":
		audioFormats = []spotifyProto.AudioFile_Format{
			spotifyProto.AudioFile_OGG_VORBIS_96,
		}
	case "medium":
		audioFormats = []spotifyProto.AudioFile_Format{
			spotifyProto.AudioFile_OGG_VORBIS_160,
			spotifyProto.AudioFile_OGG_VORBIS_96,
		}
	default: // "high" or any other value
		audioFormats = []spotifyProto.AudioFile_Format{
			spotifyProto.AudioFile_OGG_VORBIS_320,
			spotifyProto.AudioFile_OGG_VORBIS_160,
			spotifyProto.AudioFile_OGG_VORBIS_96,
		}
	}

	// Pin the track with quality preferences
	stLog("Pinning track with quality preferences: %v", audioFormats)
	pinOpts := respot.PinOpts{
		StartInternally: true,
		Format: asset.AssetFormat{
			AudioFormats: audioFormats,
		},
	}

	assetMedia, err := sess.PinTrack(spotifyID, pinOpts)
	if err != nil {
		stLog("Failed to pin track: %v", err)
		return nil, fmt.Errorf("failed to pin track: %w", err)
	}
	stLog("Track pinned successfully: %s", assetMedia.Label())

	// Create asset reader
	reader, err := assetMedia.NewAssetReader()
	if err != nil {
		stLog("Failed to create asset reader: %v", err)
		return nil, fmt.Errorf("failed to create asset reader: %w", err)
	}
	stLog("Asset reader created, stream ready")

	// Create cancellable context for cleanup
	streamCtx, cancel := context.WithCancel(ctx)

	// Create active stream wrapper
	stream := &ActiveStream{
		reader: reader,
		info: StreamInfo{
			SpotifyID:   spotifyID,
			Format:      "OGG_VORBIS",
			ContentType: "audio/ogg",
		},
		cancelCtx: cancel,
	}

	// Track active stream
	s.mu.Lock()
	s.activeStreams[requestID] = stream
	s.mu.Unlock()

	// Cleanup on context cancellation
	go func() {
		<-streamCtx.Done()
		stLog("Stream context done, cleaning up: %s", requestID)
		s.mu.Lock()
		delete(s.activeStreams, requestID)
		s.mu.Unlock()
		stream.Close()
	}()

	stLog("Stream started successfully for track: %s", spotifyID)
	return stream, nil
}

// GetActiveStreamCount returns the number of currently active streams.
func (s *Streamer) GetActiveStreamCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.activeStreams)
}

// CloseAllStreams closes all active streams.
// This should be called during application shutdown.
// CloseAllStreams terminates all active streams managed by the Streamer
// and releases associated resources.
func (s *Streamer) CloseAllStreams() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, stream := range s.activeStreams {
		stLog("Closing stream: %s", id)
		stream.Close()
	}
	s.activeStreams = make(map[string]*ActiveStream)
	stLog("All streams closed")
}
