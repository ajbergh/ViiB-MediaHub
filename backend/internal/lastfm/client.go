// Package lastfm provides Last.FM API integration for ViiB MediaHub.
//
// client.go - Last.FM API client wrapper with rate limiting and caching.
//
// This file wraps the gobble-fm library to provide:
//   - Rate limiting (5 requests/second to comply with Last.FM guidelines)
//   - LRU caching with configurable TTL (default 24h)
//   - Tag mapping to mood/energy/tempo values
//   - Mobile authentication for scrobbling support
//
// Usage:
//
//	client := lastfm.NewClient(apiKey, sharedSecret)
//	info, err := client.GetTrackInfo(ctx, "Artist", "Track")
//
// All API methods respect rate limits and check cache before making requests.
//
// Created: 2025-12-31
// Last Modified: 2025-12-31
package lastfm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/twoscott/gobble-fm/api"
	"github.com/twoscott/gobble-fm/lastfm"
	"github.com/twoscott/gobble-fm/session"
	"golang.org/x/time/rate"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// Client wraps the gobble-fm API client with rate limiting and caching.
type Client struct {
	apiKey       string
	sharedSecret string

	// Read-only client (no authentication required)
	readClient *api.Client

	// Authenticated session client (for scrobbling)
	sessionClient *session.Client
	sessionKey    string

	// Rate limiter: Last.FM allows ~5 requests/second
	limiter *rate.Limiter

	// Cache for API responses
	cache   *Cache
	cacheMu sync.RWMutex

	// Mapper for tags to mood/energy/tempo
	tagMapper *TagMapper
}

// boolPtr is a helper to create a *bool pointer.
func boolPtr(b bool) *bool {
	return &b
}

// NewClient creates a new Last.FM API client.
// apiKey is required for all operations.
// sharedSecret is required for authentication (scrobbling).
func NewClient(apiKey, sharedSecret string) *Client {
	c := &Client{
		apiKey:       apiKey,
		sharedSecret: sharedSecret,
		limiter:      rate.NewLimiter(rate.Limit(5), 1), // 5 req/sec, burst of 1
		cache:        NewCache(10000, 24*time.Hour),     // 10k entries, 24h TTL
		tagMapper:    NewTagMapper(),
	}

	if apiKey != "" {
		if sharedSecret != "" {
			c.readClient = api.NewClient(apiKey, sharedSecret)
		} else {
			c.readClient = api.NewClientKeyOnly(apiKey)
		}
	}

	return c
}

// SetSessionKey sets the authenticated session key for scrobbling.
// Call after successful authentication.
func (c *Client) SetSessionKey(sessionKey string) {
	c.sessionKey = sessionKey
	if c.sharedSecret != "" && sessionKey != "" {
		c.sessionClient = session.NewClient(c.apiKey, c.sharedSecret)
		c.sessionClient.SetSessionKey(sessionKey)
	}
}

// IsConfigured returns true if the client has an API key set.
func (c *Client) IsConfigured() bool {
	return c.apiKey != ""
}

// CanScrobble returns true if the client has authenticated session.
func (c *Client) CanScrobble() bool {
	return c.sessionClient != nil && c.sessionKey != ""
}

// wait blocks until rate limit allows next request.
func (c *Client) wait(ctx context.Context) error {
	return c.limiter.Wait(ctx)
}

// GetTrackInfo fetches track metadata from Last.FM with caching.
func (c *Client) GetTrackInfo(ctx context.Context, artist, track string) (*TrackInfo, error) {
	if c.readClient == nil {
		return nil, fmt.Errorf("last.fm client not configured")
	}

	// Check cache
	cacheKey := fmt.Sprintf("track:%s:%s", normalizeKey(artist), normalizeKey(track))
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.(*TrackInfo), nil
	}

	// Rate limit
	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	// Fetch from API
	params := lastfm.TrackInfoParams{
		Artist:      artist,
		Track:       track,
		AutoCorrect: boolPtr(true),
	}

	res, err := c.readClient.Track.Info(params)
	if err != nil {
		return nil, handleAPIError(err, "track.getInfo")
	}

	// Map to our type
	info := &TrackInfo{
		Name:      res.Title,
		Artist:    res.Artist.Name,
		Duration:  int(res.Duration),
		Listeners: res.Listeners,
		Playcount: res.Playcount,
		MBID:      res.MBID,
		URL:       res.URL,
		FetchedAt: time.Now(),
	}

	// Include album if present (check if title is not empty)
	if res.Album.Title != "" {
		info.Album = res.Album.Title
	}

	// Map top tags from TrackInfo (no counts available here)
	for _, tag := range res.TopTags {
		info.TopTags = append(info.TopTags, TagWithCount{
			Name: tag.Name,
			URL:  tag.URL,
		})
	}

	// Include wiki summary if available
	if res.Wiki.Summary != "" {
		info.Wiki = res.Wiki.Summary
	}

	// Check if name was corrected
	info.Corrected = res.Title != track || res.Artist.Name != artist

	// Cache result
	c.cache.Set(cacheKey, info)

	logger.Debug("LastFM", "Fetched track info: %s - %s", artist, track)
	return info, nil
}

// GetTrackTopTags fetches top tags for a track with counts.
func (c *Client) GetTrackTopTags(ctx context.Context, artist, track string) ([]TagWithCount, error) {
	if c.readClient == nil {
		return nil, fmt.Errorf("last.fm client not configured")
	}

	// Check cache
	cacheKey := fmt.Sprintf("tracktags:%s:%s", normalizeKey(artist), normalizeKey(track))
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.([]TagWithCount), nil
	}

	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	params := lastfm.TrackTopTagsParams{
		Artist:      artist,
		Track:       track,
		AutoCorrect: boolPtr(true),
	}

	res, err := c.readClient.Track.TopTags(params)
	if err != nil {
		return nil, handleAPIError(err, "track.getTopTags")
	}

	var tags []TagWithCount
	for _, tag := range res.Tags {
		tags = append(tags, TagWithCount{
			Name:  tag.Name,
			Count: tag.Count,
			URL:   tag.URL,
		})
	}

	c.cache.Set(cacheKey, tags)
	return tags, nil
}

// GetSimilarTracks fetches tracks similar to the given track.
func (c *Client) GetSimilarTracks(ctx context.Context, artist, track string, limit int) ([]SimilarTrack, error) {
	if c.readClient == nil {
		return nil, fmt.Errorf("last.fm client not configured")
	}

	if limit <= 0 {
		limit = 20
	}

	// Check cache
	cacheKey := fmt.Sprintf("similar:%s:%s:%d", normalizeKey(artist), normalizeKey(track), limit)
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.([]SimilarTrack), nil
	}

	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	params := lastfm.TrackSimilarParams{
		Artist:      artist,
		Track:       track,
		AutoCorrect: boolPtr(true),
		Limit:       uint(limit),
	}

	res, err := c.readClient.Track.Similar(params)
	if err != nil {
		return nil, handleAPIError(err, "track.getSimilar")
	}

	var similar []SimilarTrack
	for _, t := range res.Tracks {
		similar = append(similar, SimilarTrack{
			Name:   t.Title,
			Artist: t.Artist.Name,
			Match:  t.Match,
			MBID:   t.MBID,
			URL:    t.URL,
		})
	}

	c.cache.Set(cacheKey, similar)
	return similar, nil
}

// GetArtistInfo fetches artist metadata from Last.FM.
func (c *Client) GetArtistInfo(ctx context.Context, artist string) (*ArtistInfo, error) {
	if c.readClient == nil {
		return nil, fmt.Errorf("last.fm client not configured")
	}

	cacheKey := fmt.Sprintf("artist:%s", normalizeKey(artist))
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.(*ArtistInfo), nil
	}

	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	params := lastfm.ArtistInfoParams{
		Artist:      artist,
		AutoCorrect: boolPtr(true),
	}

	res, err := c.readClient.Artist.Info(params)
	if err != nil {
		return nil, handleAPIError(err, "artist.getInfo")
	}

	info := &ArtistInfo{
		Name:      res.Name,
		Listeners: res.Listeners,
		Playcount: res.Playcount,
		MBID:      res.MBID,
		URL:       res.URL,
		FetchedAt: time.Now(),
	}

	// Map tags
	for _, tag := range res.Tags {
		info.TopTags = append(info.TopTags, TagWithCount{
			Name: tag.Name,
			URL:  tag.URL,
		})
	}

	// Include bio if available
	if res.Bio.Summary != "" {
		info.Bio = res.Bio.Summary
	}

	// Include similar artists
	for _, similar := range res.SimilarArtists {
		info.Similar = append(info.Similar, SimilarArtist{
			Name: similar.Name,
			URL:  similar.URL,
		})
	}

	c.cache.Set(cacheKey, info)
	logger.Debug("LastFM", "Fetched artist info: %s", artist)
	return info, nil
}

// GetSimilarArtists fetches artists similar to the given artist.
func (c *Client) GetSimilarArtists(ctx context.Context, artist string, limit int) ([]SimilarArtist, error) {
	if c.readClient == nil {
		return nil, fmt.Errorf("last.fm client not configured")
	}

	if limit <= 0 {
		limit = 20
	}

	cacheKey := fmt.Sprintf("similarartist:%s:%d", normalizeKey(artist), limit)
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.([]SimilarArtist), nil
	}

	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	params := lastfm.ArtistSimilarParams{
		Artist:      artist,
		AutoCorrect: boolPtr(true),
		Limit:       uint(limit),
	}

	res, err := c.readClient.Artist.Similar(params)
	if err != nil {
		return nil, handleAPIError(err, "artist.getSimilar")
	}

	var similar []SimilarArtist
	for _, a := range res.Artists {
		similar = append(similar, SimilarArtist{
			Name:  a.Name,
			Match: a.Match,
			URL:   a.URL,
		})
	}

	c.cache.Set(cacheKey, similar)
	return similar, nil
}

// GetAlbumInfo fetches album metadata from Last.FM.
func (c *Client) GetAlbumInfo(ctx context.Context, artist, album string) (*AlbumInfo, error) {
	if c.readClient == nil {
		return nil, fmt.Errorf("last.fm client not configured")
	}

	cacheKey := fmt.Sprintf("album:%s:%s", normalizeKey(artist), normalizeKey(album))
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.(*AlbumInfo), nil
	}

	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	params := lastfm.AlbumInfoParams{
		Artist:      artist,
		Album:       album,
		AutoCorrect: boolPtr(true),
	}

	res, err := c.readClient.Album.Info(params)
	if err != nil {
		return nil, handleAPIError(err, "album.getInfo")
	}

	info := &AlbumInfo{
		Name:      res.Title,
		Artist:    res.Artist,
		MBID:      res.MBID,
		URL:       res.URL,
		Listeners: res.Listeners,
		Playcount: res.Playcount,
		FetchedAt: time.Now(),
	}

	// Map tags
	for _, tag := range res.Tags {
		info.TopTags = append(info.TopTags, TagWithCount{
			Name: tag.Name,
			URL:  tag.URL,
		})
	}

	// Include wiki if available
	if res.Wiki.Summary != "" {
		info.Wiki = res.Wiki.Summary
	}

	c.cache.Set(cacheKey, info)
	logger.Debug("LastFM", "Fetched album info: %s - %s", artist, album)
	return info, nil
}

// MapTagsToEnrichment uses the tag mapper to extract mood, energy, tempo from tags.
func (c *Client) MapTagsToEnrichment(tags []TagWithCount, minTagCount int) *TagEnrichment {
	return c.tagMapper.MapTags(tags, minTagCount)
}

// TestConnection tests the API connection by fetching a known artist.
func (c *Client) TestConnection(ctx context.Context) error {
	if c.readClient == nil {
		return fmt.Errorf("last.fm client not configured: missing API key")
	}

	if err := c.wait(ctx); err != nil {
		return err
	}

	// Try to fetch a well-known artist as a connection test
	_, err := c.readClient.Artist.Info(lastfm.ArtistInfoParams{
		Artist: "The Beatles",
	})
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	return nil
}

// Authenticate performs mobile authentication and returns session key.
func (c *Client) Authenticate(ctx context.Context, username, password string) (string, error) {
	if c.sharedSecret == "" {
		return "", fmt.Errorf("shared secret required for authentication")
	}

	if err := c.wait(ctx); err != nil {
		return "", err
	}

	client := session.NewClient(c.apiKey, c.sharedSecret)
	if err := client.Login(username, password); err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	sessionKey := client.SessionKey
	c.SetSessionKey(sessionKey)

	logger.Log("LastFM", "Authentication successful for user: %s", username)
	return sessionKey, nil
}

// handleAPIError wraps Last.FM API errors with context.
func handleAPIError(err error, method string) error {
	if apiErr, ok := err.(*api.LastFMError); ok {
		switch apiErr.Code {
		case api.ErrInvalidAPIKey:
			return fmt.Errorf("invalid Last.FM API key")
		case api.ErrRateLimitExceeded:
			return fmt.Errorf("Last.FM rate limit exceeded")
		case api.ErrInvalidParameters:
			return fmt.Errorf("track/artist not found in Last.FM database")
		default:
			return fmt.Errorf("Last.FM %s error (%d): %s", method, apiErr.Code, apiErr.Message)
		}
	}
	return fmt.Errorf("Last.FM %s: %w", method, err)
}
