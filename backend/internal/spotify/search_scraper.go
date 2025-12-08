// Package spotify provides Spotify integration for ViiB MediaHub.
// This file implements web scraping for Spotify playlist search using chromedp.
// The Spotify Web API does not return "First Party" playlists (Made For You,
// Discover Weekly, etc.) in search results. This scraper uses a headless browser
// to render the JavaScript-driven Spotify search page and extract playlist data.
package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/chromedp/chromedp"
)

// ssLog is a helper for search scraper logging
func ssLog(format string, v ...interface{}) {
	logger.SpotifyDownloader("[SearchScraper] "+format, v...)
}

// SearchedPlaylist represents a playlist found via search scraping
type SearchedPlaylist struct {
	ID     string `json:"id"`
	URL    string `json:"url,omitempty"`
	Name   string `json:"name"`
	Author string `json:"owner"` // Using "owner" to match Spotify API format
	Images []struct {
		URL string `json:"url"`
	} `json:"images,omitempty"`
}

// SearchPlaylistsResult is the result of a playlist search
type SearchPlaylistsResult struct {
	Playlists []SearchedPlaylist `json:"playlists"`
	Total     int                `json:"total"`
}

// SearchPlaylists searches for Spotify playlists by scraping the search page
// using a headless Chrome browser via chromedp.
// This is a fallback for when the Spotify Web API doesn't return First Party playlists.
//
// Parameters:
//   - query: Search query string
//
// Returns:
//   - *SearchPlaylistsResult: Array of found playlists
//   - error: If fetching or parsing fails
func SearchPlaylists(query string) (*SearchPlaylistsResult, error) {
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}

	ssLog("Searching playlists for query: %s", query)

	// Build the Spotify search URL
	encoded := url.PathEscape(query)
	searchURL := fmt.Sprintf("https://open.spotify.com/search/%s/playlists", encoded)

	// Silence chromedp internal logs
	log.SetOutput(io.Discard)

	// Use chromedp to render the JavaScript-driven page
	playlists, err := renderPlaylistsWithChromedp(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to render search page: %w", err)
	}

	ssLog("Found %d playlists from search page", len(playlists))

	return &SearchPlaylistsResult{
		Playlists: playlists,
		Total:     len(playlists),
	}, nil
}

// renderPlaylistsWithChromedp loads the Spotify search page in a headless browser
// and extracts playlist information using in-page DOM traversal via JavaScript.
// This is required because Spotify's search page renders playlists with JavaScript.
func renderPlaylistsWithChromedp(targetURL string) ([]SearchedPlaylist, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create chromedp context with headless options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	cctx, ccancel := chromedp.NewContext(allocCtx)
	defer ccancel()

	// JavaScript to run in the page context to extract playlist data.
	// It finds anchors whose href contains '/playlist/' and extracts
	// title/author info using aria-label, innerText, or child elements.
	// Images are extracted by traversing up to the parent card container.
	js := `(function() {
		const nodes = Array.from(document.querySelectorAll('a[href*="/playlist/"]'));
		const seen = new Set();
		const out = [];
		nodes.forEach(a => {
			const href = a.getAttribute('href') || '';
			const m = href.match(/\/playlist\/([A-Za-z0-9]+)/);
			if (!m) return;
			const id = m[1];
			if (seen.has(id)) return;
			seen.add(id);
			let name = '';
			let author = '';
			let imageUrl = '';
			
			// Try aria-label first (often contains "name By author" format)
			const al = a.getAttribute('aria-label');
			if (al) {
				const bi = al.toLowerCase().lastIndexOf(' by ');
				if (bi !== -1) {
					name = al.slice(0, bi).trim();
					author = al.slice(bi + 4).trim();
				} else {
					name = al.trim();
				}
			}
			
			// Fall back to inner text for name
			if (!name) {
				const t = a.querySelector('h3, h2, span') || a;
				name = (t && t.innerText) ? t.innerText.trim() : '';
			}
			
			// Look for author in child elements
			if (!author) {
				const userLink = a.querySelector('a[href*="/user/"]');
				if (userLink && userLink.innerText) {
					author = userLink.innerText.trim();
				}
			}
			
			// Try to parse "by author" from innerText
			if (!author) {
				const s = a.innerText || '';
				const match = s.match(/by\s+(.+)$/i);
				if (match) author = match[1].trim();
			}
			
			// Try to find playlist image with multiple strategies
			// Strategy 1: Direct child img
			let img = a.querySelector('img');
			if (img && img.src) {
				imageUrl = img.src;
			}
			
			// Strategy 2: Look in parent container (Spotify often nests anchor in a card)
			if (!imageUrl) {
				let parent = a.parentElement;
				let depth = 0;
				while (parent && depth < 5) {
					img = parent.querySelector('img[src*="spotify"]');
					if (!img) img = parent.querySelector('img[src*="mosaic"]');
					if (!img) img = parent.querySelector('img[src*="image"]');
					if (!img) img = parent.querySelector('img');
					if (img && img.src && img.src.startsWith('http')) {
						imageUrl = img.src;
						break;
					}
					parent = parent.parentElement;
					depth++;
				}
			}
			
			// Strategy 3: Look for background-image in parent elements
			if (!imageUrl) {
				let parent = a.parentElement;
				let depth = 0;
				while (parent && depth < 5) {
					const style = window.getComputedStyle(parent);
					const bg = style.backgroundImage;
					if (bg && bg !== 'none' && bg.includes('url(')) {
						const match = bg.match(/url\(["']?([^"')]+)["']?\)/);
						if (match && match[1]) {
							imageUrl = match[1];
							break;
						}
					}
					parent = parent.parentElement;
					depth++;
				}
			}
			
			// Strategy 4: Look for srcset on img elements
			if (!imageUrl) {
				let parent = a.parentElement;
				let depth = 0;
				while (parent && depth < 5) {
					const imgs = parent.querySelectorAll('img[srcset]');
					for (const i of imgs) {
						const srcset = i.getAttribute('srcset');
						if (srcset) {
							// Parse srcset and get the largest image
							const parts = srcset.split(',').map(s => s.trim().split(' ')[0]);
							if (parts.length > 0) {
								imageUrl = parts[parts.length - 1] || parts[0];
								break;
							}
						}
					}
					if (imageUrl) break;
					parent = parent.parentElement;
					depth++;
				}
			}
			
			let url;
			try { url = new URL(href, window.location.href).href; } catch(e) { url = href; }
			out.push({id: id, url: url, name: name, owner: author, imageUrl: imageUrl});
		});
		return JSON.stringify(out);
	})();`

	var res string
	tasks := chromedp.Tasks{
		chromedp.Navigate(targetURL),
		chromedp.Sleep(3 * time.Second), // Wait for JavaScript to render
		chromedp.Evaluate(js, &res),
	}

	if err := chromedp.Run(cctx, tasks); err != nil {
		return nil, fmt.Errorf("chromedp error: %w", err)
	}

	// Parse the JSON result
	var rawPlaylists []struct {
		ID       string `json:"id"`
		URL      string `json:"url"`
		Name     string `json:"name"`
		Author   string `json:"owner"`
		ImageURL string `json:"imageUrl"`
	}

	if err := json.Unmarshal([]byte(res), &rawPlaylists); err != nil {
		return nil, fmt.Errorf("failed to parse playlist JSON: %w", err)
	}

	// Convert to SearchedPlaylist format and deduplicate
	seen := make(map[string]bool)
	playlists := make([]SearchedPlaylist, 0, len(rawPlaylists))

	for _, p := range rawPlaylists {
		if p.ID == "" || seen[p.ID] {
			continue
		}
		seen[p.ID] = true

		sp := SearchedPlaylist{
			ID:     p.ID,
			URL:    p.URL,
			Name:   p.Name,
			Author: p.Author,
		}

		// Add image if available
		if p.ImageURL != "" {
			sp.Images = []struct {
				URL string `json:"url"`
			}{{URL: p.ImageURL}}
		}

		playlists = append(playlists, sp)
	}

	return playlists, nil
}
