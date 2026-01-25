// Package lastfm provides Last.FM API integration for ViiB MediaHub.
//
// enricher.go - Batch enrichment of songs and artists with Last.FM metadata.
//
// This file provides the core enrichment logic that:
//   - Fetches track/artist info from Last.FM API
//   - Maps community tags to structured mood/energy/tempo values
//   - Stores enrichment results in the database
//   - Optionally fetches and stores similar tracks/artists
//   - Handles concurrency with configurable worker count
//   - Respects rate limits via the Client wrapper
//
// Enrichment can be triggered via:
//   - POST /api/lastfm/enrich/songs - Enrich songs lacking Last.FM data
//   - POST /api/lastfm/enrich/artists - Enrich artists lacking Last.FM data
//
// Created: 2025-12-31
// Last Modified: 2025-12-31
package lastfm

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// Enricher handles batch enrichment of songs using Last.FM data.
type Enricher struct {
	client *Client
	db     *db.DB
}

// NewEnricher creates a new Last.FM enricher.
func NewEnricher(client *Client, database *db.DB) *Enricher {
	return &Enricher{
		client: client,
		db:     database,
	}
}

// EnrichSongs enriches a batch of songs with Last.FM metadata.
// It respects rate limits and updates the database with enrichment results.
func (e *Enricher) EnrichSongs(ctx context.Context, songs []db.Song, opts EnrichOptions) (*EnrichResult, error) {
	if opts.MinTagCount <= 0 {
		opts.MinTagCount = 30
	}
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = 3
	}

	result := &EnrichResult{}

	// Process songs with semaphore for concurrency control
	sem := make(chan struct{}, opts.MaxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var processed int32

	for _, song := range songs {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Skip if already enriched and not forcing
		if song.LastFMEnrichedAt > 0 && !opts.Force {
			mu.Lock()
			result.Skipped++
			mu.Unlock()
			atomic.AddInt32(&processed, 1)
			continue
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(s db.Song) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			err := e.enrichSong(ctx, s, opts)
			mu.Lock()
			if err != nil {
				result.Errors++
				if len(result.ErrorMsgs) < 10 { // Limit error messages
					result.ErrorMsgs = append(result.ErrorMsgs, err.Error())
				}
			} else {
				result.Enriched++
			}
			mu.Unlock()
			atomic.AddInt32(&processed, 1)
		}(song)
	}

	wg.Wait()
	result.Processed = int(processed)
	return result, nil
}

// enrichSong enriches a single song with Last.FM data.
func (e *Enricher) enrichSong(ctx context.Context, song db.Song, opts EnrichOptions) error {
	// Fetch track info from Last.FM
	trackInfo, err := e.client.GetTrackInfo(ctx, song.Artist, song.Title)
	if err != nil {
		// Log but don't fail - track might not exist in Last.FM
		logger.Debug("LastFM", "Track not found: %s - %s: %v", song.Artist, song.Title, err)
		return nil // Return nil to not count as error for missing tracks
	}

	// Map tags to mood/energy/tempo
	enrichment := e.client.MapTagsToEnrichment(trackInfo.TopTags, opts.MinTagCount)

	// Extract tag names for storage
	tagNames := make([]string, len(trackInfo.TopTags))
	for i, tag := range trackInfo.TopTags {
		tagNames[i] = tag.Name
	}

	// Update song in database
	err = e.db.UpdateSongLastFM(song.ID, db.LastFMSongUpdate{
		Listeners: trackInfo.Listeners,
		Playcount: trackInfo.Playcount,
		Tags:      tagNames,
		URL:       trackInfo.URL,
		MBID:      trackInfo.MBID,
		// Only update mood/energy/tempo if they were detected
		Genres:       enrichment.Genres,
		Mood:         enrichment.Mood,
		Energy:       enrichment.Energy,
		Tempo:        enrichment.Tempo,
		Instrumental: enrichment.Instrumental,
	})
	if err != nil {
		return err
	}

	// Optionally fetch and store similar tracks
	if opts.FetchSimilar {
		similarTracks, err := e.client.GetSimilarTracks(ctx, song.Artist, song.Title, 20)
		if err == nil && len(similarTracks) > 0 {
			// Convert to db types
			dbTracks := make([]db.LastFMSimilarTrack, len(similarTracks))
			for i, t := range similarTracks {
				dbTracks[i] = db.LastFMSimilarTrack{
					SongID:        song.ID,
					SimilarArtist: t.Artist,
					SimilarTrack:  t.Name,
					MatchScore:    t.Match,
				}
				// Try to find if we have this song in our library
				if found, _ := e.db.FindSongByArtistAndTitle(t.Artist, t.Name); found != nil {
					dbTracks[i].SimilarSongID = found.ID
				}
			}
			e.db.StoreSimilarTracks(song.ID, dbTracks)
		}
	}

	logger.Debug("LastFM", "Enriched song: %s - %s (genres: %v, mood: %s)",
		song.Artist, song.Title, enrichment.Genres, enrichment.Mood)

	return nil
}

// GetSongsNeedingEnrichment returns songs that haven't been enriched with Last.FM data.
func (e *Enricher) GetSongsNeedingEnrichment(limit int) ([]db.Song, error) {
	return e.db.GetSongsWithoutLastFM(limit)
}

// EnrichArtists enriches artist metadata from Last.FM.
func (e *Enricher) EnrichArtists(ctx context.Context, artists []string, fetchSimilar bool) (*EnrichResult, error) {
	result := &EnrichResult{}

	for _, artist := range artists {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		artistInfo, err := e.client.GetArtistInfo(ctx, artist)
		if err != nil {
			result.Errors++
			continue
		}

		// Extract tag names
		tagNames := make([]string, len(artistInfo.TopTags))
		for i, tag := range artistInfo.TopTags {
			tagNames[i] = tag.Name
		}

		err = e.db.UpdateArtistLastFM(artist, db.LastFMArtistUpdate{
			Listeners: artistInfo.Listeners,
			Playcount: artistInfo.Playcount,
			Tags:      tagNames,
			Bio:       artistInfo.Bio,
			URL:       artistInfo.URL,
			MBID:      artistInfo.MBID,
		})
		if err != nil {
			result.Errors++
			continue
		}

		// Fetch and store similar artists
		if fetchSimilar {
			similar, err := e.client.GetSimilarArtists(ctx, artist, 20)
			if err == nil && len(similar) > 0 {
				// Convert to db types
				dbArtists := make([]db.LastFMSimilarArtist, len(similar))
				for i, s := range similar {
					dbArtists[i] = db.LastFMSimilarArtist{
						ArtistName:    artist,
						SimilarArtist: s.Name,
						MatchScore:    s.Match,
					}
				}
				e.db.StoreSimilarArtists(artist, dbArtists)
			}
		}

		result.Enriched++
		result.Processed++
	}

	return result, nil
}
