// Package lastfm provides Last.FM API integration for ViiB MediaHub.
//
// This package offers an alternative to AI-based metadata enrichment using
// Last.FM's community-sourced data including:
//   - Track tags (genres, moods, styles)
//   - Similar tracks and artists
//   - Popularity metrics (listeners, play counts)
//   - MusicBrainz cross-references
//
// The package supports both read-only API access (API key only) and
// authenticated access (for scrobbling and personalized tags).
package lastfm

import "time"

// TagWithCount represents a Last.FM tag with its usage count.
// Higher count indicates more users have applied this tag.
type TagWithCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	URL   string `json:"url,omitempty"`
}

// TrackInfo contains metadata fetched from Last.FM for a track.
type TrackInfo struct {
	Name      string         `json:"name"`
	Artist    string         `json:"artist"`
	Album     string         `json:"album,omitempty"`
	Duration  int            `json:"duration"` // Duration in milliseconds
	Listeners int            `json:"listeners"`
	Playcount int            `json:"playcount"`
	MBID      string         `json:"mbid,omitempty"` // MusicBrainz ID
	URL       string         `json:"url"`
	TopTags   []TagWithCount `json:"topTags"`
	Wiki      string         `json:"wiki,omitempty"`
	Corrected bool           `json:"corrected"` // True if name was autocorrected
	FetchedAt time.Time      `json:"fetchedAt"`
}

// ArtistInfo contains metadata fetched from Last.FM for an artist.
type ArtistInfo struct {
	Name      string          `json:"name"`
	Listeners int             `json:"listeners"`
	Playcount int             `json:"playcount"`
	MBID      string          `json:"mbid,omitempty"`
	URL       string          `json:"url"`
	TopTags   []TagWithCount  `json:"topTags"`
	Bio       string          `json:"bio,omitempty"`
	Similar   []SimilarArtist `json:"similar,omitempty"`
	FetchedAt time.Time       `json:"fetchedAt"`
}

// AlbumInfo contains metadata fetched from Last.FM for an album.
type AlbumInfo struct {
	Name        string         `json:"name"`
	Artist      string         `json:"artist"`
	ReleaseDate string         `json:"releaseDate,omitempty"`
	Listeners   int            `json:"listeners"`
	Playcount   int            `json:"playcount"`
	MBID        string         `json:"mbid,omitempty"`
	URL         string         `json:"url"`
	TopTags     []TagWithCount `json:"topTags"`
	ImageURL    string         `json:"imageUrl,omitempty"` // Largest available
	Wiki        string         `json:"wiki,omitempty"`
	FetchedAt   time.Time      `json:"fetchedAt"`
}

// SimilarTrack represents a track similar to a queried track.
type SimilarTrack struct {
	Name   string  `json:"name"`
	Artist string  `json:"artist"`
	Match  float64 `json:"match"` // 0-1, higher = more similar
	MBID   string  `json:"mbid,omitempty"`
	URL    string  `json:"url"`
}

// SimilarArtist represents an artist similar to a queried artist.
type SimilarArtist struct {
	Name  string  `json:"name"`
	Match float64 `json:"match"` // 0-1, higher = more similar
	MBID  string  `json:"mbid,omitempty"`
	URL   string  `json:"url"`
}

// TagEnrichment contains the mapped mood/energy/tempo from Last.FM tags.
type TagEnrichment struct {
	Genres       []string `json:"genres"`
	Mood         string   `json:"mood,omitempty"`
	Energy       string   `json:"energy,omitempty"`
	Tempo        string   `json:"tempo,omitempty"`
	Instrumental bool     `json:"instrumental"`
	RawTags      []string `json:"rawTags,omitempty"` // Top tags for reference
}

// EnrichResult contains the result of a batch enrichment operation.
type EnrichResult struct {
	Processed int      `json:"processed"`
	Enriched  int      `json:"enriched"`
	Skipped   int      `json:"skipped"`
	Errors    int      `json:"errors"`
	ErrorMsgs []string `json:"errorMsgs,omitempty"`
}

// EnrichOptions configures the enrichment behavior.
type EnrichOptions struct {
	Force          bool `json:"force"`          // Re-enrich even if already enriched
	FetchSimilar   bool `json:"fetchSimilar"`   // Also fetch similar tracks
	FetchArtist    bool `json:"fetchArtist"`    // Also fetch artist info
	MinTagCount    int  `json:"minTagCount"`    // Minimum tag count to consider (default: 30)
	MaxConcurrency int  `json:"maxConcurrency"` // Concurrent API calls (default: 3)
	BatchSize      int  `json:"batchSize"`      // Songs per batch for progress updates
}

// DefaultEnrichOptions returns sensible defaults for enrichment.
func DefaultEnrichOptions() EnrichOptions {
	return EnrichOptions{
		Force:          false,
		FetchSimilar:   true,
		FetchArtist:    false,
		MinTagCount:    30,
		MaxConcurrency: 3,
		BatchSize:      50,
	}
}

// ScrobbleParams contains the parameters for scrobbling a track.
type ScrobbleParams struct {
	Artist    string    `json:"artist"`
	Track     string    `json:"track"`
	Album     string    `json:"album,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Settings contains the Last.FM API configuration.
type Settings struct {
	APIKey         string `json:"apiKey"`
	SharedSecret   string `json:"sharedSecret"`
	SessionKey     string `json:"sessionKey,omitempty"` // For authenticated requests
	Username       string `json:"username,omitempty"`
	ScrobblingOn   bool   `json:"scrobblingEnabled"`
	EnrichmentMode string `json:"enrichmentMode"` // "lastfm", "ai", or "hybrid"
}
