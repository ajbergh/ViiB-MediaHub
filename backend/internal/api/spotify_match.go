package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

// Retain version words (live, remix, edit, etc.) in the comparison. Removing
// them can attach analysis from an entirely different performance.
func spotifyMatchText(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, s)), " ")
}

func spotifyMatchArtist(song db.Song) string {
	if value := strings.TrimSpace(song.Artist); value != "" {
		return value
	}
	return strings.TrimSpace(song.AlbumArtist)
}

type spotifyMatchDetails struct {
	reason                                                string
	eligible, invalid, duplicate, title, artist, duration int
	durationDelta                                         float64
}

// Different album releases may contain the same recording. Keep title/version,
// artist and duration checks, then rank eligible releases by album, duration,
// and Spotify search order rather than rejecting duplicate releases.
func selectSpotifyRecording(song db.Song, candidates []*catalog.Track) *catalog.Track {
	match, _ := selectSpotifyRecordingWithDetails(song, candidates)
	return match
}

func selectSpotifyRecordingWithDetails(song db.Song, candidates []*catalog.Track) (*catalog.Track, spotifyMatchDetails) {
	details := spotifyMatchDetails{reason: "no_qualifying_recording"}
	title, artist := spotifyMatchText(song.Title), spotifyMatchText(spotifyMatchArtist(song))
	if title == "" || artist == "" || song.Duration <= 0 || math.IsNaN(song.Duration) || math.IsInf(song.Duration, 0) {
		details.reason = "missing_track_metadata"
		return nil, details
	}
	if len(candidates) == 0 {
		details.reason = "no_search_candidates"
	}
	tolerance := math.Min(3, math.Max(2, song.Duration*.005))
	var best *catalog.Track
	bestAlbum := false
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == nil || !db.ValidSpotifyRecordingID(candidate.ID) {
			details.invalid++
			continue
		}
		if seen[candidate.ID] {
			details.duplicate++
			continue
		}
		seen[candidate.ID] = true
		if spotifyMatchText(candidate.Name) != title {
			details.title++
			continue
		}
		delta := math.Abs(float64(candidate.DurationMS)/1000 - song.Duration)
		if candidate.DurationMS <= 0 || delta > tolerance {
			details.duration++
			continue
		}
		names := make([]string, 0, len(candidate.Artists))
		for _, name := range candidate.Artists {
			names = append(names, name.Name)
		}
		artistMatch := len(names) > 0 && (spotifyMatchText(names[0]) == artist || spotifyMatchText(strings.Join(names, " ")) == artist)
		if !artistMatch {
			details.artist++
			continue
		}
		details.eligible++
		albumMatch := spotifyMatchText(song.Album) != "" && spotifyMatchText(song.Album) == spotifyMatchText(candidate.Album.Name)
		if best == nil || (albumMatch && !bestAlbum) || (albumMatch == bestAlbum && delta < details.durationDelta) {
			best, bestAlbum, details.durationDelta = candidate, albumMatch, delta
		}
	}
	if best != nil {
		details.reason = "matching_release_ranked"
		if details.eligible == 1 {
			details.reason = "single_matching_recording"
		} else if bestAlbum {
			details.reason = "album_match_ranked"
		}
	}
	return best, details
}

func (a *API) searchSpotifyRecordings(ctx context.Context, query string) ([]*catalog.Track, error) {
	if a.spotifyTrackSearch != nil {
		return a.spotifyTrackSearch(ctx, query)
	}
	values := url.Values{"q": {query}, "type": {"track"}, "limit": {"20"}}
	response, err := a.doSpotifyRequest(ctx, http.MethodGet, "https://api.spotify.com/v1/search?"+values.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &spotifyauth.WebPlayerHTTPError{Status: response.StatusCode, RetryAfter: spotifyauth.RetryAfter(response.Header.Get("Retry-After"), time.Now())}
	}
	var result catalog.SearchResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result); err != nil {
		return nil, err
	}
	if result.Tracks == nil {
		return nil, catalog.ErrSchema
	}
	return result.Tracks.Items, nil
}

func (a *API) spotifyFeaturesEnabled() bool {
	a.spotifyAnalysisMu.RLock()
	defer a.spotifyAnalysisMu.RUnlock()
	return a.spotifyAnalysis != nil && !a.spotifyAnalysisClosed
}

func (a *API) matchSpotifyRecording(ctx context.Context, source analysis.ResolvedSource) *db.ExternalTrackIdentity {
	if !a.spotifyFeaturesEnabled() {
		logger.Scan("spotify_match song_id=%q reason=spotify_disabled", source.SongID)
		return nil
	}
	allowed, err := a.db.SpotifySearchAllowed(source.SongID, source.Fingerprint)
	if err != nil || !allowed {
		logger.Scan("spotify_match song_id=%q reason=search_not_allowed_or_identity_error", source.SongID)
		return nil
	}
	song, err := a.db.GetSongByID(source.SongID)
	if err != nil || song == nil || strings.TrimSpace(song.Title) == "" || spotifyMatchArtist(*song) == "" || song.Duration <= 0 {
		logger.Scan("spotify_match song_id=%q reason=missing_track_metadata", source.SongID)
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	candidates, err := a.searchSpotifyRecordings(ctx, song.Title+" "+spotifyMatchArtist(*song))
	if err != nil || ctx.Err() != nil {
		reason, status, retryAfter := "search_failed", 0, time.Duration(0)
		var upstream *spotifyauth.WebPlayerHTTPError
		if errors.As(err, &upstream) {
			status, retryAfter = upstream.Status, upstream.RetryAfter
			if status == http.StatusTooManyRequests {
				reason = "search_rate_limited"
			}
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "search_timed_out"
		}
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			reason = "search_canceled"
		}
		logger.Scan("spotify_match song_id=%q reason=%s http_status=%d retry_after_seconds=%.0f", source.SongID, reason, status, retryAfter.Seconds())
		return nil
	}
	match, details := selectSpotifyRecordingWithDetails(*song, candidates)
	selectedID := ""
	if match != nil {
		selectedID = match.ID
	}
	logger.Scan("spotify_match_selection song_id=%q title=%q artist=%q reason=%s candidates=%d eligible=%d rejected_title=%d rejected_artist=%d rejected_duration=%d invalid=%d duplicates=%d selected_spotify_id=%q duration_delta_seconds=%.3f", source.SongID, song.Title, spotifyMatchArtist(*song), details.reason, len(candidates), details.eligible, details.title, details.artist, details.duration, details.invalid, details.duplicate, selectedID, details.durationDelta)
	if match == nil {
		return nil
	}
	// Resolve again after the network request; never link changed source bytes.
	current, err := a.resolveAnalysisSource(ctx, source.SongID)
	if err != nil || current.Fingerprint != source.Fingerprint {
		logger.Scan("spotify_match song_id=%q reason=source_changed_or_unavailable", source.SongID)
		return nil
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision(source.SongID, source.Fingerprint); err != nil {
		logger.Scan("spotify_match song_id=%q reason=source_revision_storage_error", source.SongID)
		return nil
	}
	if changed, err := a.db.SaveSpotifySearchMatch(source.SongID, match.ID, source.Fingerprint); err != nil || !changed {
		logger.Scan("spotify_match song_id=%q reason=recording_link_not_saved", source.SongID)
		return nil
	}
	link, _ := a.db.GetSpotifyRecording(source.SongID, source.Fingerprint)
	if link != nil {
		logger.Scan("spotify_match song_id=%q spotify_id=%q reason=automatic_match", source.SongID, link.ExternalID)
	}
	return link
}
