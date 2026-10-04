// Maps fixed Web Player album responses and paged tracks into catalog models.
package catalog

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"net/url"
	"strconv"
	"strings"
)

// Protocol facts: Spotui f9d05b6450e730469d15f30f9dd4bc790db794db.
const albumHash = "b9bfabef66ed756e5e13f68a942deb60bd4125ec1f1be8cc42769dc0259b4b10"

type AlbumTrack struct {
	Track
	TrackNumber int `json:"track_number"`
	DiscNumber  int `json:"disc_number"`
}
type Copyright struct {
	Text string `json:"text"`
	Type string `json:"type"`
}
type Album struct {
	AlbumSummary
	TotalTracks int              `json:"total_tracks"`
	Tracks      Page[AlbumTrack] `json:"tracks"`
	Label       *string          `json:"label"`
	Copyrights  []Copyright      `json:"copyrights"`
	Genres      []string         `json:"genres"`
	Popularity  *int             `json:"popularity"`
}
type AlbumQuery struct {
	ID            string
	Limit, Offset int
}

func ParseAlbumQuery(id string, v url.Values) (AlbumQuery, error) {
	q := AlbumQuery{ID: id, Limit: 50}
	var err error
	if s := v.Get("limit"); s != "" {
		q.Limit, err = strconv.Atoi(s)
		if err != nil {
			return q, ErrInvalidQuery
		}
	}
	if s := v.Get("offset"); s != "" {
		q.Offset, err = strconv.Atoi(s)
		if err != nil {
			return q, ErrInvalidQuery
		}
	}
	return q, q.validate()
}
func (q AlbumQuery) validate() error {
	if !objectID.MatchString(q.ID) || q.Limit < 1 || q.Limit > 50 || q.Offset < 0 || q.Offset > 100000 {
		return ErrInvalidQuery
	}
	return nil
}
func AlbumPath(path string) (id string, tracks, ok bool) {
	p := strings.Split(strings.TrimPrefix(path, "/v1/"), "/")
	if len(p) < 2 || p[0] != "albums" {
		return
	}
	if len(p) == 2 {
		return p[1], false, true
	}
	if len(p) == 3 && p[2] == "tracks" {
		return p[1], true, true
	}
	return
}
func albumTrackLink(q AlbumQuery, offset int) string {
	return "https://api.spotify.com/v1/albums/" + q.ID + "/tracks?" + url.Values{"limit": {strconv.Itoa(q.Limit)}, "offset": {strconv.Itoa(offset)}}.Encode()
}
func (c *Client) Album(ctx context.Context, token auth.Token, q AlbumQuery) (Album, error) {
	if err := q.validate(); err != nil {
		return Album{}, err
	}
	ctx, cancel := c.context(ctx)
	defer cancel()
	ct, err := c.clientContext(ctx, token)
	if err != nil {
		return Album{}, err
	}
	var reply struct {
		Errors []any `json:"errors"`
		Data   struct {
			Album *struct {
				albumWire
				Label      *string `json:"label"`
				Copyrights struct {
					Items []Copyright `json:"items"`
				} `json:"copyright"`
				Date *struct {
					ISO string `json:"isoString"`
				} `json:"date"`
				Tracks *wirePage `json:"tracksV2"`
			} `json:"albumUnion"`
		} `json:"data"`
	}
	payload := map[string]any{"operationName": "getAlbum", "variables": map[string]any{"uri": "spotify:album:" + q.ID, "locale": "", "offset": q.Offset, "limit": q.Limit}, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": albumHash}}}
	if err := c.request(ctx, "album", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), ct, &reply); err != nil {
		return Album{}, err
	}
	w := reply.Data.Album
	if len(reply.Errors) > 0 || w == nil {
		return Album{}, ErrSchema
	}
	if unavailableType(w.TypeName) {
		return Album{}, &HTTPError{Stage: "album", Status: 404}
	}
	if w.TypeName != "Album" || w.URI != "spotify:album:"+q.ID {
		return Album{}, ErrSchema
	}
	summary, err := albumSummary(w.albumWire)
	if err != nil || len(summary.Artists) == 0 {
		return Album{}, ErrSchema
	}
	if w.Date != nil && w.Date.ISO != "" {
		date := strings.Split(w.Date.ISO, "T")[0]
		summary.ReleaseDate = &date
	}
	p := w.Tracks
	if p == nil || p.Total == nil || *p.Total < 0 || p.Items == nil || len(p.Items) > q.Limit {
		return Album{}, ErrSchema
	}
	r := Album{AlbumSummary: summary, TotalTracks: *p.Total, Label: w.Label, Copyrights: w.Copyrights.Items, Genres: []string{}, Tracks: Page[AlbumTrack]{Href: albumTrackLink(q, q.Offset), Items: []*AlbumTrack{}, Limit: q.Limit, Offset: q.Offset, Total: *p.Total}}
	if r.Copyrights == nil {
		r.Copyrights = []Copyright{}
	}
	for _, raw := range p.Items {
		var item struct {
			Track struct {
				trackWire
				Playability *struct {
					Playable bool `json:"playable"`
				} `json:"playability"`
				TrackNumber int `json:"trackNumber"`
				DiscNumber  int `json:"discNumber"`
			} `json:"track"`
		}
		if json.Unmarshal(raw, &item) != nil {
			return Album{}, ErrSchema
		}
		if unavailableType(item.Track.TypeName) || (item.Track.Playability != nil && !item.Track.Playability.Playable) {
			r.Tracks.Items = append(r.Tracks.Items, nil)
			continue
		}
		if item.Track.TrackNumber < 1 || item.Track.DiscNumber < 1 {
			return Album{}, ErrSchema
		}
		if item.Track.TypeName == "" {
			item.Track.TypeName = "Track"
		}
		item.Track.Album = w.albumWire
		data, err := json.Marshal(map[string]any{"item": map[string]any{"data": item.Track.trackWire}})
		if err != nil {
			return Album{}, ErrSchema
		}
		track, err := normalizeTrack(data)
		if err != nil {
			return Album{}, err
		}
		track.Album = summary
		r.Tracks.Items = append(r.Tracks.Items, &AlbumTrack{Track: *track, TrackNumber: item.Track.TrackNumber, DiscNumber: item.Track.DiscNumber})
	}
	if len(p.Items) > 0 && q.Offset+q.Limit < r.TotalTracks {
		next := albumTrackLink(q, q.Offset+q.Limit)
		r.Tracks.Next = &next
	}
	if q.Offset > 0 {
		offset := q.Offset - q.Limit
		if offset < 0 {
			offset = 0
		}
		prev := albumTrackLink(q, offset)
		r.Tracks.Previous = &prev
	}
	return r, nil
}
