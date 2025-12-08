// Package spotify provides Spotify integration for ViiB MediaHub.
// This file implements web scraping fallback for first-party Spotify playlists
// that are not accessible via the standard Web API.
package spotify

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// psLog is a helper for playlist scraper logging
func psLog(format string, v ...interface{}) {
	logger.SpotifyDownloader("[PlaylistScraper] "+format, v...)
}

// ScrapedPlaylist represents playlist data extracted from the Spotify embed page.
// This is used as a fallback when the Spotify Web API returns 404 for first-party playlists.
type ScrapedPlaylist struct {
	Name    string   `json:"name"`    // Playlist display name
	Artwork string   `json:"artwork"` // Playlist artwork URL (can be empty)
	Tracks  []string `json:"tracks"`  // Array of Spotify track IDs
}

// ScrapedTrackDetail represents a track with full info for display
type ScrapedTrackDetail struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Duration int             `json:"duration_ms"`
	Artists  []ScrapedArtist `json:"artists"`
	Album    *ScrapedAlbum   `json:"album,omitempty"`
}

// ScrapedArtist represents an artist with minimal info
type ScrapedArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ScrapedAlbum represents an album with minimal info
type ScrapedAlbum struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Images []ScrapedImage `json:"images,omitempty"`
}

// ScrapedImage represents an image
type ScrapedImage struct {
	URL string `json:"url"`
}

// ScrapedPlaylistDetail represents full playlist data with track details
type ScrapedPlaylistDetail struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Images      []ScrapedImage       `json:"images"`
	Owner       ScrapedArtist        `json:"owner"`
	Tracks      []ScrapedTrackDetail `json:"tracks"`
}

// ScrapedTrack represents a track with minimal info for download queuing
type ScrapedTrack struct {
	ID string // Spotify track ID (not full URL, just the ID)
}

// ScrapePlaylist fetches playlist data from the Spotify embed page.
// This is a fallback method for first-party playlists that aren't accessible via Web API.
//
// Implementation notes:
//   - Parses the `__NEXT_DATA__` JSON payload from the embed page
//   - Falls back to regex extraction if JSON parsing fails
//   - Returns deduplicated track IDs in order of appearance
//
// Limitations:
//   - The embed page may not contain all tracks for very long playlists
//   - Spotify may change the embed page structure, breaking this scraper
//   - No authentication is used - only publicly accessible data
//
// Parameters:
//   - playlistID: The Spotify playlist ID (e.g., "37i9dQZF1DXcBWIGoYBM5M")
//
// Returns:
//   - *ScrapedPlaylist: Playlist data including name, artwork, and track IDs
//   - error: If fetching or parsing fails
func ScrapePlaylist(playlistID string) (*ScrapedPlaylist, error) {
	psLog("Scraping playlist embed page for ID: %s", playlistID)

	// Build the embed URL for the playlist
	embedURL := "https://open.spotify.com/embed/playlist/" + playlistID

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", embedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent to appear as a regular browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch embed page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed page returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read embed page body: %w", err)
	}

	bodyStr := string(body)

	// Extract track IDs from the embed page
	trackIDs := extractTrackIDs(bodyStr)
	if len(trackIDs) == 0 {
		return nil, fmt.Errorf("no tracks found on playlist embed page")
	}

	psLog("Found %d tracks from embed page", len(trackIDs))

	// Extract playlist name and artwork
	playlistName := extractPlaylistName(bodyStr)
	artwork := extractPlaylistArtwork(bodyStr)

	psLog("Playlist name: %s, Artwork: %s", playlistName, artwork)

	return &ScrapedPlaylist{
		Name:    playlistName,
		Artwork: artwork,
		Tracks:  trackIDs,
	}, nil
}

// ScrapePlaylistDetail fetches full playlist data including track details from the embed page.
// This provides detailed track information for display in the UI.
func ScrapePlaylistDetail(playlistID string) (*ScrapedPlaylistDetail, error) {
	psLog("Scraping playlist detail for ID: %s", playlistID)

	// Build the embed URL for the playlist
	embedURL := "https://open.spotify.com/embed/playlist/" + playlistID

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", embedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch embed page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed page returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read embed page body: %w", err)
	}

	bodyStr := string(body)
	return extractPlaylistDetail(playlistID, bodyStr)
}

// extractPlaylistDetail parses the __NEXT_DATA__ JSON to extract full track details
func extractPlaylistDetail(playlistID, body string) (*ScrapedPlaylistDetail, error) {
	result := &ScrapedPlaylistDetail{
		ID: playlistID,
	}

	// Find the __NEXT_DATA__ JSON payload
	startTag := `<script id="__NEXT_DATA__" type="application/json">`
	start := strings.Index(body, startTag)
	if start < 0 {
		return nil, fmt.Errorf("__NEXT_DATA__ script not found")
	}
	start += len(startTag)
	end := strings.Index(body[start:], "</script>")
	if end < 0 {
		return nil, fmt.Errorf("__NEXT_DATA__ script end not found")
	}
	jsonStr := body[start : start+end]

	var root map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &root); err != nil {
		return nil, fmt.Errorf("failed to parse __NEXT_DATA__: %w", err)
	}

	// Navigate to props.pageProps.state.data
	props, ok := root["props"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("props not found")
	}
	pageProps, ok := props["pageProps"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("pageProps not found")
	}
	state, ok := pageProps["state"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("state not found")
	}
	data, ok := state["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("data not found")
	}

	// Extract playlist info from entity
	if entity, ok := data["entity"].(map[string]interface{}); ok {
		if name, ok := entity["name"].(string); ok {
			result.Name = name
		}
		if title, ok := entity["title"].(string); ok && result.Name == "" {
			result.Name = title
		}

		// Get owner info
		if owner, ok := entity["owner"].(map[string]interface{}); ok {
			if name, ok := owner["name"].(string); ok {
				result.Owner.Name = name
			}
			if displayName, ok := owner["displayName"].(string); ok && result.Owner.Name == "" {
				result.Owner.Name = displayName
			}
			if uri, ok := owner["uri"].(string); ok {
				// Extract ID from spotify:user:xxx
				result.Owner.ID = extractIDFromURI(uri)
			}
		}

		// Get coverArt for images
		if coverArt, ok := entity["coverArt"].(map[string]interface{}); ok {
			if sources, ok := coverArt["sources"].([]interface{}); ok {
				for _, src := range sources {
					if srcMap, ok := src.(map[string]interface{}); ok {
						if urlStr, ok := srcMap["url"].(string); ok && urlStr != "" {
							result.Images = append(result.Images, ScrapedImage{URL: urlStr})
						}
					}
				}
			}
		}
	}

	// Try to get artwork from visualIdentity as backup
	if len(result.Images) == 0 {
		if vi, ok := data["visualIdentity"].(map[string]interface{}); ok {
			if imgs, ok := vi["image"].([]interface{}); ok {
				for _, img := range imgs {
					if imgMap, ok := img.(map[string]interface{}); ok {
						if urlStr, ok := imgMap["url"].(string); ok && urlStr != "" {
							result.Images = append(result.Images, ScrapedImage{URL: urlStr})
						}
					}
				}
			}
		}
	}

	// Extract tracks from trackList or trackListData
	extractTracksFromData(data, &result.Tracks)

	if len(result.Tracks) == 0 {
		// Fallback to regex extraction and minimal track info
		trackIDs := extractTrackIDs(body)
		for _, id := range trackIDs {
			result.Tracks = append(result.Tracks, ScrapedTrackDetail{
				ID:   id,
				Name: "Track " + id, // Placeholder name
			})
		}
	}

	psLog("Scraped playlist detail: %s with %d tracks", result.Name, len(result.Tracks))

	return result, nil
}

// extractTracksFromData recursively searches for track data in the JSON
func extractTracksFromData(data interface{}, tracks *[]ScrapedTrackDetail) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this is a track object
		if uri, ok := v["uri"].(string); ok && strings.HasPrefix(uri, "spotify:track:") {
			track := ScrapedTrackDetail{
				ID: extractIDFromURI(uri),
			}

			if name, ok := v["name"].(string); ok {
				track.Name = name
			}
			if title, ok := v["title"].(string); ok && track.Name == "" {
				track.Name = title
			}
			if duration, ok := v["duration"].(float64); ok {
				track.Duration = int(duration)
			}
			if duration, ok := v["duration_ms"].(float64); ok {
				track.Duration = int(duration)
			}
			if durationMs, ok := v["durationMs"].(float64); ok {
				track.Duration = int(durationMs)
			}

			// Extract artists
			if artists, ok := v["artists"].([]interface{}); ok {
				for _, a := range artists {
					if aMap, ok := a.(map[string]interface{}); ok {
						artist := ScrapedArtist{}
						if name, ok := aMap["name"].(string); ok {
							artist.Name = name
						}
						if uri, ok := aMap["uri"].(string); ok {
							artist.ID = extractIDFromURI(uri)
						}
						if artist.Name != "" {
							track.Artists = append(track.Artists, artist)
						}
					}
				}
			}

			// Extract album
			if album, ok := v["album"].(map[string]interface{}); ok {
				track.Album = &ScrapedAlbum{}
				if name, ok := album["name"].(string); ok {
					track.Album.Name = name
				}
				if uri, ok := album["uri"].(string); ok {
					track.Album.ID = extractIDFromURI(uri)
				}
				if coverArt, ok := album["coverArt"].(map[string]interface{}); ok {
					if sources, ok := coverArt["sources"].([]interface{}); ok {
						for _, src := range sources {
							if srcMap, ok := src.(map[string]interface{}); ok {
								if urlStr, ok := srcMap["url"].(string); ok && urlStr != "" {
									track.Album.Images = append(track.Album.Images, ScrapedImage{URL: urlStr})
								}
							}
						}
					}
				}
			}

			// Only add if we got meaningful data
			if track.Name != "" {
				*tracks = append(*tracks, track)
			}
			return
		}

		// Check for trackList specifically
		if trackList, ok := v["trackList"].([]interface{}); ok {
			for _, t := range trackList {
				extractTracksFromData(t, tracks)
			}
			return
		}

		// Recurse into other keys
		for _, val := range v {
			extractTracksFromData(val, tracks)
		}
	case []interface{}:
		for _, item := range v {
			extractTracksFromData(item, tracks)
		}
	}
}

// extractIDFromURI extracts the ID from a Spotify URI (e.g., "spotify:track:xxx" -> "xxx")
func extractIDFromURI(uri string) string {
	parts := strings.Split(uri, ":")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return ""
}

// extractPlaylistName attempts to find the playlist's display name from the embed page.
// It prefers parsing the JSON payload inside the <script id="__NEXT_DATA__"> tag,
// and falls back to regex searches if the JSON parse fails.
func extractPlaylistName(body string) string {
	// 1) Find the `<script id="__NEXT_DATA__"` tag and extract the JSON content
	startTag := `<script id="__NEXT_DATA__" type="application/json">`
	start := strings.Index(body, startTag)
	if start >= 0 {
		start += len(startTag)
		end := strings.Index(body[start:], "</script>")
		if end >= 0 {
			jsonStr := body[start : start+end]
			var root map[string]interface{}
			if json.Unmarshal([]byte(jsonStr), &root) == nil {
				// Traverse: props.pageProps.state.data.entity.name
				if props, ok := root["props"].(map[string]interface{}); ok {
					if pageProps, ok := props["pageProps"].(map[string]interface{}); ok {
						if state, ok := pageProps["state"].(map[string]interface{}); ok {
							if data, ok := state["data"].(map[string]interface{}); ok {
								if entity, ok := data["entity"].(map[string]interface{}); ok {
									if name, ok := entity["name"].(string); ok {
										return name
									}
									// Some pages also include a `title` field
									if title, ok := entity["title"].(string); ok {
										return title
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 2) Fallback: look for an `entity` JSON object with type=playlist
	reEntityName := regexp.MustCompile(`"entity"\s*:\s*\{[^}]*"type"\s*:\s*"playlist"[^}]*"name"\s*:\s*"([^"]+)"`)
	if m := reEntityName.FindStringSubmatch(body); len(m) > 1 {
		return m[1]
	}

	// 3) Last fallback: Open Graph title meta tag
	reOgTitle := regexp.MustCompile(`<meta\s+property="og:title"\s+content="([^"]+)"`)
	if m := reOgTitle.FindStringSubmatch(body); len(m) > 1 {
		return m[1]
	}

	return "Unknown Playlist"
}

// extractPlaylistArtwork attempts to locate the playlist's artwork URL in the embed page.
// It prefers the `visualIdentity.image` array inside the JSON payload,
// then falls back to regex searches for image URLs.
func extractPlaylistArtwork(body string) string {
	// 1) Try JSON payload in __NEXT_DATA__
	startTag := `<script id="__NEXT_DATA__" type="application/json">`
	start := strings.Index(body, startTag)
	if start >= 0 {
		start += len(startTag)
		end := strings.Index(body[start:], "</script>")
		if end >= 0 {
			jsonStr := body[start : start+end]
			var root map[string]interface{}
			if json.Unmarshal([]byte(jsonStr), &root) == nil {
				if props, ok := root["props"].(map[string]interface{}); ok {
					if pageProps, ok := props["pageProps"].(map[string]interface{}); ok {
						if state, ok := pageProps["state"].(map[string]interface{}); ok {
							if data, ok := state["data"].(map[string]interface{}); ok {
								// visualIdentity.image[0].url
								if vi, ok := data["visualIdentity"].(map[string]interface{}); ok {
									if imgs, ok := vi["image"].([]interface{}); ok && len(imgs) > 0 {
										if img0, ok := imgs[0].(map[string]interface{}); ok {
											if urlStr, ok := img0["url"].(string); ok && urlStr != "" {
												return urlStr
											}
										}
									}
								}
								// entity.coverArt.sources[0].url
								if entity, ok := data["entity"].(map[string]interface{}); ok {
									if coverArt, ok := entity["coverArt"].(map[string]interface{}); ok {
										if sources, ok := coverArt["sources"].([]interface{}); ok && len(sources) > 0 {
											if s0, ok := sources[0].(map[string]interface{}); ok {
												if urlStr, ok := s0["url"].(string); ok && urlStr != "" {
													return urlStr
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 2) Fallback: regex for Pickasso or i.scdn.co images
	rePickasso := regexp.MustCompile(`https?://pickasso\.spotifycdn\.com/[^"'\s>]+`)
	if m := rePickasso.FindString(body); m != "" {
		return m
	}
	reIScdn := regexp.MustCompile(`https?://i\.scdn\.co/image/[^"'\s>]+`)
	if m := reIScdn.FindString(body); m != "" {
		return m
	}

	return ""
}

// extractTrackIDs returns the unique Spotify track IDs found in the embed page's HTML.
// The returned slice preserves the order of first occurrence.
func extractTrackIDs(body string) []string {
	re := regexp.MustCompile(`"uri":"spotify:track:([A-Za-z0-9]+)"`)
	matches := re.FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool)
	var ids []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		id := m[1]
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// ExtractPlaylistIDFromURL parses the playlist ID from a given Spotify playlist URL.
// Supports various URL formats:
//   - https://open.spotify.com/playlist/<id>
//   - https://open.spotify.com/embed/playlist/<id>
//   - https://open.spotify.com/user/<user>/playlist/<id>
//   - https://open.spotify.com/intl-xx/playlist/<id>
func ExtractPlaylistIDFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	// Split the path and strip leading/trailing slashes
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(segments); i++ {
		if segments[i] == "playlist" && i+1 < len(segments) {
			return segments[i+1]
		}
	}
	if len(segments) > 0 {
		return segments[len(segments)-1]
	}
	return ""
}

// IsFirstPartyPlaylistError checks if an HTTP status code indicates a first-party
// playlist that is not accessible via the Web API.
// First-party playlists (e.g., "Made For You", "Daily Mix", "Discover Weekly")
// typically return 404 or 403 when accessed via the API.
func IsFirstPartyPlaylistError(statusCode int) bool {
	return statusCode == http.StatusNotFound || statusCode == http.StatusForbidden
}
