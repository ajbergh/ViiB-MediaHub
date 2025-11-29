// Package audio provides audio file metadata extraction functionality.
//
// This package handles reading ID3 tags, Vorbis comments, and other metadata
// formats from audio files. It uses two libraries for maximum compatibility:
//   - taglib-go (go.senan.xyz/taglib): Primary extractor with duration support
//   - dhowden/tag: Fallback for files taglib can't handle
//
// Supported formats:
//   - MP3 (ID3v1, ID3v2)
//   - FLAC (Vorbis comments)
//   - M4A/AAC (iTunes metadata)
//   - OGG/Opus (Vorbis comments)
//   - WAV, WMA
//
// Album artwork is extracted and saved separately to the covers directory.
package audio

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"
	taglib "go.senan.xyz/taglib"
)

type SongMetadata struct {
	ID          string
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	TrackNumber int
	DiscNumber  int
	Genre       []string
	Year        int
	Duration    float64
	FilePath    string
	CoverPath   string
	CoverData   []byte
}

func ExtractMetadata(filePath string) (*SongMetadata, error) {
	// Get file info for hash
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Generate ID from file path and size
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", filePath, info.Size())))
	id := fmt.Sprintf("%x", hash[:8])

	// Try taglib first for better metadata extraction (including duration)
	song, err := extractWithTaglib(filePath, id)
	if err == nil {
		return song, nil
	}

	// Fallback to dhowden/tag if taglib fails
	return extractWithDhowdenTag(filePath, id)
}

// extractWithTaglib uses go.senan.xyz/taglib for metadata extraction
// This provides accurate duration and better format support
func extractWithTaglib(filePath, id string) (*SongMetadata, error) {
	// Read tags
	tags, err := taglib.ReadTags(filePath)
	if err != nil {
		return nil, err
	}

	// Read properties for duration
	props, err := taglib.ReadProperties(filePath)
	if err != nil {
		return nil, err
	}

	// Helper to get first tag value
	getTag := func(key string) string {
		if vals, ok := tags[key]; ok && len(vals) > 0 {
			return vals[0]
		}
		return ""
	}

	// Build metadata struct
	song := &SongMetadata{
		ID:       id,
		Title:    getTag(taglib.Title),
		Artist:   getTag(taglib.Artist),
		Album:    getTag(taglib.Album),
		Duration: float64(props.Length) / float64(time.Second),
		FilePath: filePath,
	}

	// Year
	if yearStr := getTag(taglib.Date); yearStr != "" {
		// Parse year from date (might be YYYY or YYYY-MM-DD)
		if len(yearStr) >= 4 {
			if y, err := strconv.Atoi(yearStr[:4]); err == nil {
				song.Year = y
			}
		}
	}

	// Fallback title to filename
	if song.Title == "" {
		baseName := filepath.Base(filePath)
		song.Title = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	}

	// Fallback artist/album
	if song.Artist == "" {
		song.Artist = "Unknown Artist"
	}
	if song.Album == "" {
		song.Album = "Unknown Album"
	}

	// Album artist
	if aa := getTag(taglib.AlbumArtist); aa != "" {
		song.AlbumArtist = aa
	}

	// Track number
	if trackStr := getTag(taglib.TrackNumber); trackStr != "" {
		// Handle "1/12" format
		if idx := strings.Index(trackStr, "/"); idx > 0 {
			trackStr = trackStr[:idx]
		}
		if t, err := strconv.Atoi(trackStr); err == nil {
			song.TrackNumber = t
		}
	}

	// Disc number
	if discStr := getTag(taglib.DiscNumber); discStr != "" {
		// Handle "1/2" format
		if idx := strings.Index(discStr, "/"); idx > 0 {
			discStr = discStr[:idx]
		}
		if d, err := strconv.Atoi(discStr); err == nil {
			song.DiscNumber = d
		}
	}

	// Genre
	if genre := getTag(taglib.Genre); genre != "" {
		song.Genre = []string{genre}
	}

	// Cover art (front cover)
	if coverData, err := taglib.ReadImage(filePath); err == nil && len(coverData) > 0 {
		song.CoverData = coverData
	}

	return song, nil
}

// extractWithDhowdenTag uses dhowden/tag as a fallback
func extractWithDhowdenTag(filePath, id string) (*SongMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Extract metadata using tag library
	m, err := tag.ReadFrom(file)
	if err != nil {
		// If we can't read tags, create minimal metadata from filename
		baseName := filepath.Base(filePath)
		title := strings.TrimSuffix(baseName, filepath.Ext(baseName))

		return &SongMetadata{
			ID:       id,
			Title:    title,
			Artist:   "Unknown Artist",
			Album:    "Unknown Album",
			FilePath: filePath,
		}, nil
	}

	// Build metadata struct
	song := &SongMetadata{
		ID:       id,
		Title:    m.Title(),
		Artist:   m.Artist(),
		Album:    m.Album(),
		Year:     m.Year(),
		FilePath: filePath,
	}

	// Fallback title to filename
	if song.Title == "" {
		baseName := filepath.Base(filePath)
		song.Title = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	}

	// Fallback artist/album
	if song.Artist == "" {
		song.Artist = "Unknown Artist"
	}
	if song.Album == "" {
		song.Album = "Unknown Album"
	}

	// Album artist
	if aa := m.AlbumArtist(); aa != "" {
		song.AlbumArtist = aa
	}

	// Track number
	trackNum, _ := m.Track()
	song.TrackNumber = trackNum

	// Disc number
	discNum, _ := m.Disc()
	song.DiscNumber = discNum

	// Genre
	if g := m.Genre(); g != "" {
		song.Genre = []string{g}
	}

	// Cover art
	if pic := m.Picture(); pic != nil {
		song.CoverData = pic.Data
	}

	// Duration - dhowden/tag doesn't provide this
	song.Duration = 0

	return song, nil
}

// GetAudioDuration returns the duration of an audio file in seconds
func GetAudioDuration(filePath string) (float64, error) {
	props, err := taglib.ReadProperties(filePath)
	if err != nil {
		return 0, err
	}
	return float64(props.Length) / float64(time.Second), nil
}
