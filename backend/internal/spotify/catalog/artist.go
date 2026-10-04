package catalog

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"net/url"
	"strings"
)

// Fixed protocol facts from Spotui f9d05b6450e730469d15f30f9dd4bc790db794db.
const artistHash = "5b9e64f43843fa3a9b6a98543600299b0a2cbbbccfdcdcef2402eb9c1017ca4c"

type Artist struct {
	ArtistSummary
	Genres     []string `json:"genres"`
	Popularity *int     `json:"popularity"`
}
type ArtistTopTracks struct {
	Tracks []*Track `json:"tracks"`
}

func ArtistPath(path string) (id string, top, ok bool) {
	p := strings.Split(strings.TrimPrefix(path, "/v1/"), "/")
	if len(p) < 2 || p[0] != "artists" {
		return
	}
	if len(p) == 2 {
		return p[1], false, true
	}
	if len(p) == 3 && p[2] == "top-tracks" {
		return p[1], true, true
	}
	return
}
func ValidateArtistQuery(id string, values url.Values) error {
	if !objectID.MatchString(id) {
		return ErrInvalidQuery
	}
	market := values.Get("market")
	if market != "" && market != "from_token" && (len(market) != 2 || market[0] < 'A' || market[0] > 'Z' || market[1] < 'A' || market[1] > 'Z') {
		return ErrInvalidQuery
	}
	return nil
}
func (c *Client) ArtistOverview(ctx context.Context, token auth.Token, id string, includeTracks bool) (Artist, ArtistTopTracks, error) {
	if !objectID.MatchString(id) {
		return Artist{}, ArtistTopTracks{}, ErrInvalidQuery
	}
	ctx, cancel := c.context(ctx)
	defer cancel()
	ct, err := c.clientContext(ctx, token)
	if err != nil {
		return Artist{}, ArtistTopTracks{}, err
	}
	var reply struct {
		Errors []any `json:"errors"`
		Data   struct {
			Artist json.RawMessage `json:"artistUnion"`
		} `json:"data"`
	}
	payload := map[string]any{"operationName": "queryArtistOverview", "variables": map[string]any{"uri": "spotify:artist:" + id, "locale": ""}, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": artistHash}}}
	if err := c.request(ctx, "artist", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), ct, &reply); err != nil {
		return Artist{}, ArtistTopTracks{}, err
	}
	if len(reply.Errors) > 0 || len(reply.Data.Artist) == 0 {
		return Artist{}, ArtistTopTracks{}, ErrSchema
	}
	var identity struct {
		TypeName string `json:"__typename"`
		ID       string `json:"id"`
		URI      string `json:"uri"`
	}
	if json.Unmarshal(reply.Data.Artist, &identity) != nil {
		return Artist{}, ArtistTopTracks{}, ErrSchema
	}
	if unavailableType(identity.TypeName) {
		return Artist{}, ArtistTopTracks{}, &HTTPError{Stage: "artist", Status: 404}
	}
	if identity.TypeName != "Artist" {
		return Artist{}, ArtistTopTracks{}, ErrSchema
	}
	if identity.URI == "" {
		if identity.ID != id {
			return Artist{}, ArtistTopTracks{}, ErrSchema
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(reply.Data.Artist, &object) != nil {
			return Artist{}, ArtistTopTracks{}, ErrSchema
		}
		object["uri"], _ = json.Marshal("spotify:artist:" + identity.ID)
		reply.Data.Artist, err = json.Marshal(object)
		if err != nil {
			return Artist{}, ArtistTopTracks{}, ErrSchema
		}
	} else if identity.ID != "" && identity.ID != id {
		return Artist{}, ArtistTopTracks{}, ErrSchema
	}
	wrapped, err := json.Marshal(map[string]any{"data": reply.Data.Artist})
	if err != nil {
		return Artist{}, ArtistTopTracks{}, ErrSchema
	}
	summary, err := normalizeArtist(wrapped)
	if err != nil {
		return Artist{}, ArtistTopTracks{}, err
	}
	if summary == nil {
		return Artist{}, ArtistTopTracks{}, &HTTPError{Stage: "artist", Status: 404}
	}
	if summary.ID != id {
		return Artist{}, ArtistTopTracks{}, ErrSchema
	}
	artist := Artist{ArtistSummary: *summary, Genres: []string{}}
	tracks := ArtistTopTracks{Tracks: []*Track{}}
	if !includeTracks {
		return artist, tracks, nil
	}
	var wire struct {
		Discography *struct {
			Top *struct {
				Items []struct {
					Track struct {
						trackWire
						Playability *struct {
							Playable bool `json:"playable"`
						} `json:"playability"`
					} `json:"track"`
				} `json:"items"`
			} `json:"topTracks"`
		} `json:"discography"`
	}
	if json.Unmarshal(reply.Data.Artist, &wire) != nil || wire.Discography == nil || wire.Discography.Top == nil || wire.Discography.Top.Items == nil || len(wire.Discography.Top.Items) > 100 {
		return Artist{}, ArtistTopTracks{}, ErrSchema
	}
	albums := map[string]albumWire{}
	for _, item := range wire.Discography.Top.Items {
		if unavailableType(item.Track.TypeName) || (item.Track.Playability != nil && !item.Track.Playability.Playable) {
			continue
		}
		if item.Track.TypeName == "" {
			item.Track.TypeName = "Track"
		}
		if item.Track.Album.Name == "" {
			albumID, err := uriID(item.Track.Album.URI, "album")
			if err != nil {
				return Artist{}, ArtistTopTracks{}, err
			}
			parent, ok := albums[albumID]
			if !ok {
				full, err := c.Album(ctx, token, AlbumQuery{ID: albumID, Limit: 1})
				if err != nil {
					return Artist{}, ArtistTopTracks{}, err
				}
				names := artistList{Items: []artistWire{}}
				for _, a := range full.Artists {
					w := artistWire{URI: a.URI}
					w.Profile.Name = a.Name
					names.Items = append(names.Items, w)
				}
				parent = albumWire{TypeName: "Album", URI: full.URI, Name: full.Name, Artists: names, CoverArt: artWire{Sources: full.Images}, Type: strings.ToUpper(full.AlbumType)}
				albums[albumID] = parent
			}
			item.Track.Album = parent
		}
		raw, err := json.Marshal(map[string]any{"item": map[string]any{"data": item.Track.trackWire}})
		if err != nil {
			return Artist{}, ArtistTopTracks{}, ErrSchema
		}
		track, err := normalizeTrack(raw)
		if err != nil || track == nil {
			return Artist{}, ArtistTopTracks{}, ErrSchema
		}
		tracks.Tracks = append(tracks.Tracks, track)
	}
	return artist, tracks, nil
}
