// Maps individual and batched Spotify track responses into catalog models.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

// Protocol facts: Islom4ik/SpotdlRip 5120b890ee0a8d8ed4599fe32ed10bca7cd795c3, spotdlrip.py.
// Independently implemented fixed operation; no upstream executable code is included.
const trackHash = "612585ae06ba435ad26369870deaae23b5c8800a256cd8a57e08eddc25a37294"

type TracksResult struct {
	Tracks []*Track `json:"tracks"`
}

func TrackPath(path string) (id string, batch, ok bool) {
	if path == "/v1/tracks" {
		return "", true, true
	}
	parts := strings.Split(path, "/")
	if len(parts) == 4 && parts[0] == "" && parts[1] == "v1" && parts[2] == "tracks" && parts[3] != "" {
		return parts[3], false, true
	}
	return
}
func ParseTrackQuery(id string, batch bool, v url.Values) ([]string, error) {
	if market := v.Get("market"); market != "" && market != "from_token" && (len(market) != 2 || market[0] < 'A' || market[0] > 'Z' || market[1] < 'A' || market[1] > 'Z') {
		return nil, ErrInvalidQuery
	}
	ids := []string{id}
	if batch {
		ids = strings.Split(v.Get("ids"), ",")
	}
	if len(ids) < 1 || len(ids) > 50 {
		return nil, ErrInvalidQuery
	}
	for _, id := range ids {
		if !objectID.MatchString(id) {
			return nil, ErrInvalidQuery
		}
	}
	return ids, nil
}
func (c *Client) Track(ctx context.Context, token auth.Token, id string) (*Track, error) {
	result, err := c.Tracks(ctx, token, []string{id})
	if err != nil {
		return nil, err
	}
	if result.Tracks[0] == nil {
		return nil, &HTTPError{Stage: "track", Status: 404}
	}
	return result.Tracks[0], nil
}
func (c *Client) Tracks(ctx context.Context, token auth.Token, ids []string) (TracksResult, error) {
	if len(ids) < 1 || len(ids) > 50 {
		return TracksResult{}, ErrInvalidQuery
	}
	for _, id := range ids {
		if !objectID.MatchString(id) {
			return TracksResult{}, ErrInvalidQuery
		}
	}
	ctx, cancel := c.context(ctx)
	defer cancel()
	ct, err := c.clientContext(ctx, token)
	if err != nil {
		return TracksResult{}, err
	}
	result := TracksResult{Tracks: make([]*Track, 0, len(ids))}
	cache := map[string]*Track{}
	albums := map[string]AlbumSummary{}
	for _, id := range ids {
		track, ok := cache[id]
		if !ok {
			track, err = c.track(ctx, token, ct, id, albums)
			if err != nil {
				return TracksResult{}, err
			}
			cache[id] = track
		}
		result.Tracks = append(result.Tracks, track)
	}
	return result, ctx.Err()
}
func (c *Client) track(ctx context.Context, token auth.Token, ct, id string, albums map[string]AlbumSummary) (*Track, error) {
	var reply struct {
		Errors []any `json:"errors"`
		Data   struct {
			Track *struct {
				trackWire
				ID           string     `json:"id"`
				FirstArtist  artistList `json:"firstArtist"`
				OtherArtists artistList `json:"otherArtists"`
				Playability  *struct {
					Playable bool `json:"playable"`
				} `json:"playability"`
			} `json:"trackUnion"`
		} `json:"data"`
	}
	payload := map[string]any{"operationName": "getTrack", "variables": map[string]any{"uri": "spotify:track:" + id}, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": trackHash}}}
	if err := c.request(ctx, "track", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), ct, &reply); err != nil {
		var upstream *HTTPError
		if errors.As(err, &upstream) && upstream.Status == 404 {
			return nil, nil
		}
		return nil, err
	}
	w := reply.Data.Track
	if len(reply.Errors) > 0 || w == nil {
		return nil, ErrSchema
	}
	if unavailableType(w.TypeName) || (w.Playability != nil && !w.Playability.Playable) {
		return nil, nil
	}
	if w.TypeName != "Track" || (w.ID != "" && w.ID != id) || (w.URI != "" && w.URI != "spotify:track:"+id) || (w.OtherURI != "" && w.OtherURI != "spotify:track:"+id) {
		return nil, ErrSchema
	}
	if w.URI == "" {
		if w.ID != id {
			return nil, ErrSchema
		}
		w.URI = "spotify:track:" + id
	}
	if len(w.Artists.Items) == 0 {
		w.Artists.Items = append(w.FirstArtist.Items, w.OtherArtists.Items...)
	}
	if w.Album.URI == "" && objectID.MatchString(w.Album.ID) {
		w.Album.URI = "spotify:album:" + w.Album.ID
	}
	albumID, err := uriID(w.Album.URI, "album")
	if err != nil {
		return nil, err
	}
	if w.Album.ID != "" && w.Album.ID != albumID {
		return nil, ErrSchema
	}
	album, ok := albums[albumID]
	if !ok {
		full, err := c.Album(ctx, token, AlbumQuery{ID: albumID, Limit: 1})
		if err != nil {
			return nil, err
		}
		album = full.AlbumSummary
		albums[albumID] = album
	}
	// Normalize the track using the full parent metadata, including exact release date.
	names := artistList{Items: []artistWire{}}
	for _, a := range album.Artists {
		wire := artistWire{URI: a.URI}
		wire.Profile.Name = a.Name
		names.Items = append(names.Items, wire)
	}
	w.Album = albumWire{TypeName: "Album", URI: album.URI, Name: album.Name, Artists: names, CoverArt: artWire{Sources: album.Images}, Type: strings.ToUpper(album.AlbumType)}
	raw, err := json.Marshal(map[string]any{"item": map[string]any{"data": w.trackWire}})
	if err != nil {
		return nil, ErrSchema
	}
	track, err := normalizeTrack(raw)
	if err != nil || track == nil {
		return nil, ErrSchema
	}
	track.Album = album
	return track, nil
}
