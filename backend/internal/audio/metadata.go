package audio

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
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
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info for hash
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// Generate ID from file path and size
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", filePath, info.Size())))
	id := fmt.Sprintf("%x", hash[:8])

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

	// Duration - tag library doesn't provide this directly
	// We'd need another library for accurate duration, but we can estimate
	// For now, leave it at 0 and let the frontend calculate from audio element
	song.Duration = 0

	return song, nil
}

// GetAudioDuration returns the duration of an audio file in seconds
// This is a placeholder - for accurate duration we'd need a specialized library
func GetAudioDuration(filePath string) (float64, error) {
	// For accurate duration, you'd want to use something like:
	// - ffprobe (command line)
	// - go-mp3 for MP3 files
	// - go-flac for FLAC files
	// etc.

	// For now, return 0 and let the browser calculate it
	return 0, nil
}
