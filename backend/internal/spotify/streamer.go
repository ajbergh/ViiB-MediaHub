// Package spotify provides Spotify integration for ViiB MediaHub.
// This file implements direct audio streaming using librespot-go.
package spotify

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
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
	reader     io.ReadSeekCloser  // Underlying audio data reader
	info       StreamInfo         // Stream metadata
	mu         sync.RWMutex       // Protects closed state
	closed     bool               // Whether stream has been closed
	cancelCtx  context.CancelFunc // Cancel function for cleanup
	totalSize  int64
	assetCtx   task.Context // Per-stream asset lifecycle
	invalidate func()
	release    func() // Releases the shared session lease
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
	n, err = s.reader.Read(p)
	if err != nil && err != io.EOF && s.invalidate != nil {
		s.invalidate()
	}
	return n, err
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
	position, err := s.reader.Seek(offset, whence)
	if err != nil && s.invalidate != nil {
		s.invalidate()
	}
	return position, err
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
	if s.assetCtx != nil {
		_ = s.assetCtx.Close()
	}

	var err error
	if s.reader != nil {
		err = s.reader.Close()
	}
	if s.release != nil {
		s.release()
		s.release = nil
	}
	return err
}

// Size returns the asset size discovered once during shared preparation.
func (s *ActiveStream) Size() int64 { return s.totalSize }

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
	closed         bool
	assets         *streamAssetPool
	maxConcurrent  int // Maximum concurrent streams allowed
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
		maxConcurrent:  5,
		assets:         newStreamAssetPool(),
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stLog("Starting stream for track: %s (request: %s, quality: %s)", spotifyID, requestID, quality)

	releaseCapacity, err := s.reserveStream(requestID)
	if err != nil {
		return nil, err
	}
	capacityOwned := true
	defer func() {
		if capacityOwned {
			releaseCapacity()
		}
	}()

	// Get authenticated session
	sess, releaseSession, err := s.sessionManager.AcquireSession()
	if err != nil {
		stLog("Failed to get session: %v", err)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	releaseNeeded := true
	defer func() {
		if releaseNeeded {
			releaseSession()
		}
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
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

	key := streamAssetKey{track: spotifyID, quality: quality, generation: s.sessionManager.Generation()}
	shared, releaseAsset, err := s.assets.acquire(ctx, key, func() (*streamAsset, error) {
		pinOpts := respot.PinOpts{StartInternally: false, Format: asset.AssetFormat{AudioFormats: audioFormats}}
		releaseKey, err := s.sessionManager.acquireAudioKeyRequest(ctx)
		if err != nil {
			return nil, err
		}
		assetMedia, err := sess.PinTrack(spotifyID, pinOpts)
		releaseKey()
		if err != nil {
			return nil, fmt.Errorf("pin stream asset: %w", normalizeAudioKeyError(err))
		}
		assetCtx, err := sess.Context().Context.StartChild(task.Task{Info: task.Info{Label: "spotify-stream-" + requestID}})
		if err != nil {
			return nil, err
		}
		// Abort first-chunk preparation if its initiating HTTP request is cancelled.
		stopCancel := context.AfterFunc(ctx, func() { _ = assetCtx.Close() })
		if err = assetMedia.OnStart(assetCtx); err != nil {
			stopCancel()
			_ = assetCtx.Close()
			return nil, err
		}
		probe, err := assetMedia.NewAssetReader()
		if err != nil {
			stopCancel()
			_ = assetCtx.Close()
			return nil, err
		}
		size, err := probe.Seek(0, io.SeekEnd)
		_ = probe.Close()
		stopped := stopCancel()
		if err != nil || !stopped || ctx.Err() != nil {
			_ = assetCtx.Close()
			if err == nil {
				err = ctx.Err()
				if err == nil {
					err = context.Canceled
				}
			}
			return nil, err
		}
		return &streamAsset{size: size, contentType: assetMedia.ContentType(), close: func() { _ = assetCtx.Close() }, newReader: func() (io.ReadSeekCloser, error) {
			select {
			case <-assetCtx.Closing():
				return nil, fmt.Errorf("stream asset unavailable")
			default:
			}
			return assetMedia.NewAssetReader()
		}}, nil
	})
	if err != nil {
		return nil, err
	}
	reader, err := shared.newReader()
	if err != nil {
		s.assets.discard(key, shared)
		releaseAsset()
		return nil, err
	}

	// Create cancellable context for cleanup
	streamCtx, cancel := context.WithCancel(ctx)

	// Create active stream wrapper
	stream := &ActiveStream{
		reader: reader,
		info: StreamInfo{
			SpotifyID:   spotifyID,
			Format:      "OGG_VORBIS",
			ContentType: shared.contentType,
		},
		cancelCtx:  cancel,
		totalSize:  shared.size,
		invalidate: func() { s.assets.discard(key, shared) },
		release:    func() { releaseAsset(); releaseSession(); releaseCapacity() },
	}
	releaseNeeded = false
	capacityOwned = false

	// Track active stream
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		stream.Close()
		return nil, fmt.Errorf("streamer closed during preparation")
	}
	s.activeStreams[requestID] = stream
	s.mu.Unlock()

	// Cleanup on context cancellation
	go func() {
		<-streamCtx.Done()
		stLog("Stream context done, cleaning up: %s", requestID)
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
	s.closed = true
	streams := make(map[string]*ActiveStream, len(s.activeStreams))
	for id, stream := range s.activeStreams {
		streams[id] = stream
	}
	s.mu.Unlock()

	if s.assets != nil {
		s.assets.close()
	}
	for id, stream := range streams {
		stLog("Closing stream: %s", id)
		if stream != nil {
			stream.Close()
		}
	}
	stLog("All streams closed")
}

// Reserve capacity before session acquisition, including streams still preparing.
func (s *Streamer) reserveStream(requestID string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("streamer closed")
	}
	if _, exists := s.activeStreams[requestID]; exists {
		return nil, fmt.Errorf("duplicate stream request")
	}
	if len(s.activeStreams) >= s.maxConcurrent {
		return nil, fmt.Errorf("too many concurrent streams (max %d)", s.maxConcurrent)
	}
	s.activeStreams[requestID] = nil
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); delete(s.activeStreams, requestID); s.mu.Unlock() }) }, nil
}
