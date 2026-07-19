// Package api provides DJ waveform generation and analysis endpoints.
//
// This file implements:
//   - Waveform peak generation from audio files
//   - DJ analysis caching (waveform data, hot cues)
//   - Waveform data serving via REST API
//
// Waveforms are generated lazily on first request and cached in SQLite.
package api

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/go-chi/chi/v5"
	"github.com/hajimehoshi/go-mp3"
)

// WaveformResponse contains the waveform peak data for a track.
type WaveformResponse struct {
	TrackID    string    `json:"trackId"`
	Duration   float64   `json:"duration"`   // Total duration in seconds
	SampleRate int       `json:"sampleRate"` // Source sample rate
	Resolution int       `json:"resolution"` // Samples per peak (e.g., 256)
	Peaks      []float64 `json:"peaks"`      // Normalized peak values (0-1)
}

// HotCue represents a saved position in a track.
type HotCue struct {
	Slot     int     `json:"slot"`     // 1-8
	Position float64 `json:"position"` // Position in seconds
	Label    string  `json:"label,omitempty"`
	Color    string  `json:"color"` // Hex color code
}

// HotCuesResponse contains hot cue data for a track.
type HotCuesResponse struct {
	TrackID string   `json:"trackId"`
	HotCues []HotCue `json:"hotCues"`
}

// getDJWaveform generates or returns cached waveform data for a track.
// GET /api/dj/waveform/{id}
func (a *API) getDJWaveform(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "id")
	if trackID == "" {
		respondError(w, http.StatusBadRequest, "Track ID required")
		return
	}

	// Check cache first
	cached, err := a.db.GetDJWaveform(trackID)
	if err == nil && cached != nil {
		// Return cached waveform
		respondJSON(w, WaveformResponse{
			TrackID:    trackID,
			Duration:   cached.Duration,
			SampleRate: cached.SampleRate,
			Resolution: cached.Resolution,
			Peaks:      cached.Peaks,
		})
		return
	}

	// Get song from database to find file path
	song, err := a.db.GetSongByID(trackID)
	if err != nil {
		logger.API("DJ waveform: song not found: %s - %v", trackID, err)
		respondError(w, http.StatusNotFound, "Track not found")
		return
	}

	// Generate waveform from audio file
	waveform, err := generateWaveform(song.FilePath)
	if err != nil {
		logger.API("DJ waveform: generation failed for %s: %v", trackID, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Waveform generation failed: %v", err))
		return
	}

	// Cache the waveform
	if err := a.db.SaveDJWaveform(trackID, waveform); err != nil {
		logger.API("DJ waveform: failed to cache for %s: %v", trackID, err)
		// Continue anyway, just don't cache
	}

	respondJSON(w, WaveformResponse{
		TrackID:    trackID,
		Duration:   waveform.Duration,
		SampleRate: waveform.SampleRate,
		Resolution: waveform.Resolution,
		Peaks:      waveform.Peaks,
	})
}

// getDJHotCues returns hot cue points for a track.
// GET /api/dj/hotcues/{id}
func (a *API) getDJHotCues(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "id")
	if trackID == "" {
		respondError(w, http.StatusBadRequest, "Track ID required")
		return
	}

	hotCues, err := a.db.GetDJHotCues(trackID)
	if err != nil {
		logger.API("DJ hot cues: failed to get for %s: %v", trackID, err)
		respondError(w, http.StatusInternalServerError, "Failed to get hot cues")
		return
	}

	response := HotCuesResponse{
		TrackID: trackID,
		HotCues: make([]HotCue, 0, len(hotCues)),
	}

	for _, hc := range hotCues {
		response.HotCues = append(response.HotCues, HotCue{
			Slot:     hc.Slot,
			Position: hc.Position,
			Label:    hc.Label,
			Color:    hc.Color,
		})
	}

	respondJSON(w, response)
}

// saveDJHotCues saves hot cue points for a track.
// PUT /api/dj/hotcues/{id}
func (a *API) saveDJHotCues(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "id")
	if trackID == "" {
		respondError(w, http.StatusBadRequest, "Track ID required")
		return
	}

	var req HotCuesResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert to database format
	dbHotCues := make([]db.DJHotCue, 0, len(req.HotCues))
	for _, hc := range req.HotCues {
		dbHotCues = append(dbHotCues, db.DJHotCue{
			Slot:     hc.Slot,
			Position: hc.Position,
			Label:    hc.Label,
			Color:    hc.Color,
		})
	}

	if err := a.db.SaveDJHotCues(trackID, dbHotCues); err != nil {
		logger.API("DJ hot cues: failed to save for %s: %v", trackID, err)
		respondError(w, http.StatusInternalServerError, "Failed to save hot cues")
		return
	}

	respondJSON(w, map[string]bool{"success": true})
}

// generateWaveform generates peak data from an audio file.
// Returns normalized peak values (0-1) at specified resolution.
func generateWaveform(filePath string) (*db.DJWaveform, error) {
	ext := filepath.Ext(filePath)

	switch ext {
	case ".mp3":
		return generateMP3Waveform(filePath)
	case ".ogg", ".oga", ".opus":
		// OGG Vorbis/Opus not yet supported server-side
		// Client will use Web Audio API for waveform generation
		return nil, fmt.Errorf("ogg/vorbis format: use client-side generation")
	case ".flac":
		// FLAC not yet supported server-side
		return nil, fmt.Errorf("flac format: use client-side generation")
	case ".wav", ".wave":
		// WAV not yet supported server-side
		return nil, fmt.Errorf("wav format: use client-side generation")
	case ".m4a", ".aac":
		// AAC not yet supported server-side
		return nil, fmt.Errorf("aac format: use client-side generation")
	default:
		// For now, return a placeholder for unsupported formats
		return nil, fmt.Errorf("unsupported audio format: %s", ext)
	}
}

// generateMP3Waveform generates waveform peaks from an MP3 file.
func generateMP3Waveform(filePath string) (*db.DJWaveform, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Decode MP3
	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create MP3 decoder: %w", err)
	}

	sampleRate := decoder.SampleRate()

	// Calculate total samples from file length
	// MP3 is 2 channels * 2 bytes per sample
	totalBytes := decoder.Length()
	totalSamples := totalBytes / 4 // stereo, 16-bit
	duration := float64(totalSamples) / float64(sampleRate)

	// Resolution: samples per peak (256 = ~8ms at 44.1kHz = ~125 peaks/second)
	resolution := 256
	peakCount := int(totalSamples) / resolution
	if peakCount < 1 {
		peakCount = 1
	}

	peaks := make([]float64, 0, peakCount)

	// Read and process audio in chunks
	buf := make([]byte, resolution*4) // stereo 16-bit

	for {
		n, err := decoder.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read audio data: %w", err)
		}
		if n == 0 {
			break
		}

		// Find peak in this chunk
		var maxAbs float64
		for i := 0; i < n-1; i += 2 {
			// Read 16-bit sample (little-endian)
			sample := int16(binary.LittleEndian.Uint16(buf[i : i+2]))
			abs := math.Abs(float64(sample)) / 32768.0
			if abs > maxAbs {
				maxAbs = abs
			}
		}
		peaks = append(peaks, maxAbs)
	}

	return &db.DJWaveform{
		Duration:   duration,
		SampleRate: sampleRate,
		Resolution: resolution,
		Peaks:      peaks,
	}, nil
}
