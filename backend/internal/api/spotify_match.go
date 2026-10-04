package api

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
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

// Matching deliberately requires duration and an exact normalized title and
// artist. Album can disambiguate releases, but never override a version mismatch.
func selectSpotifyRecording(song db.Song, candidates []*catalog.Track) *catalog.Track {
	title, artist := spotifyMatchText(song.Title), spotifyMatchText(spotifyMatchArtist(song))
	if title == "" || artist == "" || song.Duration <= 0 || math.IsNaN(song.Duration) || math.IsInf(song.Duration, 0) {
		return nil
	}
	tolerance := math.Min(3, math.Max(2, song.Duration*.005))
	var matches, albumMatches []*catalog.Track
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == nil || seen[candidate.ID] || !db.ValidSpotifyRecordingID(candidate.ID) || spotifyMatchText(candidate.Name) != title || candidate.DurationMS <= 0 || math.Abs(float64(candidate.DurationMS)/1000-song.Duration) > tolerance {
			continue
		}
		names := make([]string, 0, len(candidate.Artists))
		for _, name := range candidate.Artists {
			names = append(names, name.Name)
		}
		artistMatch := len(names) > 0 && (spotifyMatchText(names[0]) == artist || spotifyMatchText(strings.Join(names, " ")) == artist)
		if !artistMatch {
			continue
		}
		seen[candidate.ID] = true
		matches = append(matches, candidate)
		if song.Album != "" && spotifyMatchText(song.Album) == spotifyMatchText(candidate.Album.Name) {
			albumMatches = append(albumMatches, candidate)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	if len(albumMatches) == 1 {
		return albumMatches[0]
	}
	return nil
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
		return nil, catalog.ErrSchema
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
		return nil
	}
	allowed, err := a.db.SpotifySearchAllowed(source.SongID, source.Fingerprint)
	if err != nil || !allowed {
		return nil
	}
	song, err := a.db.GetSongByID(source.SongID)
	if err != nil || song == nil || strings.TrimSpace(song.Title) == "" || spotifyMatchArtist(*song) == "" || song.Duration <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	candidates, err := a.searchSpotifyRecordings(ctx, song.Title+" "+spotifyMatchArtist(*song))
	if err != nil || ctx.Err() != nil {
		return nil
	}
	match := selectSpotifyRecording(*song, candidates)
	if match == nil {
		return nil
	}
	// Resolve again after the network request; never link changed source bytes.
	current, err := a.resolveAnalysisSource(ctx, source.SongID)
	if err != nil || current.Fingerprint != source.Fingerprint {
		return nil
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision(source.SongID, source.Fingerprint); err != nil {
		return nil
	}
	if changed, err := a.db.SaveSpotifySearchMatch(source.SongID, match.ID, source.Fingerprint); err != nil || !changed {
		return nil
	}
	link, _ := a.db.GetSpotifyRecording(source.SongID, source.Fingerprint)
	return link
}
