// Maps playlist metadata and positional track pages, preserving unavailable rows and nullable fields.
package catalog

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"net/url"
	"strconv"
	"strings"
)

// Fixed protocol facts: Spotui f9d05b6450e730469d15f30f9dd4bc790db794db.
const playlistHash = "346811f856fb0b7e4f6c59f8ebea78dd081c6e2fb01b77c954b26259d5fc6763"

type PlaylistItem struct {
	AddedAt *string `json:"added_at"`
	Track   *Track  `json:"track"`
}
type Playlist struct {
	PlaylistSummary
	Followers  *Followers         `json:"followers"`
	Public     *bool              `json:"public"`
	SnapshotID *string            `json:"snapshot_id"`
	Tracks     Page[PlaylistItem] `json:"tracks"`
}
type PlaylistQuery struct {
	ID            string
	Limit, Offset int
}

func ParsePlaylistQuery(id string, v url.Values) (PlaylistQuery, error) {
	q := PlaylistQuery{ID: id, Limit: 100}
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
func (q PlaylistQuery) validate() error {
	if !objectID.MatchString(q.ID) || q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 100000 {
		return ErrInvalidQuery
	}
	return nil
}
func PlaylistPath(path string) (id string, tracks, ok bool) {
	p := strings.Split(strings.TrimPrefix(path, "/v1/"), "/")
	if len(p) < 2 || p[0] != "playlists" {
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
func playlistTrackLink(q PlaylistQuery, offset int) string {
	return "https://api.spotify.com/v1/playlists/" + q.ID + "/tracks?" + url.Values{"limit": {strconv.Itoa(q.Limit)}, "offset": {strconv.Itoa(offset)}}.Encode()
}
func (c *Client) Playlist(ctx context.Context, token auth.Token, q PlaylistQuery) (Playlist, error) {
	if err := q.validate(); err != nil {
		return Playlist{}, err
	}
	ctx, cancel := c.context(ctx)
	defer cancel()
	ct, err := c.clientContext(ctx, token)
	if err != nil {
		return Playlist{}, err
	}
	var reply struct {
		Errors []any `json:"errors"`
		Data   struct {
			Playlist json.RawMessage `json:"playlistV2"`
		} `json:"data"`
	}
	payload := map[string]any{"operationName": "fetchPlaylist", "variables": map[string]any{"uri": "spotify:playlist:" + q.ID, "offset": q.Offset, "limit": q.Limit, "enableWatchFeedEntrypoint": false}, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": playlistHash}}}
	if err := c.request(ctx, "playlist", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), ct, &reply); err != nil {
		return Playlist{}, err
	}
	if len(reply.Errors) > 0 || len(reply.Data.Playlist) == 0 {
		return Playlist{}, ErrSchema
	}
	var wire struct {
		TypeName  string    `json:"__typename"`
		ID        string    `json:"id"`
		URI       string    `json:"uri"`
		Followers *int      `json:"followers"`
		Public    *bool     `json:"public"`
		Revision  *string   `json:"revisionId"`
		Content   *wirePage `json:"content"`
	}
	if json.Unmarshal(reply.Data.Playlist, &wire) != nil {
		return Playlist{}, ErrSchema
	}
	if unavailableType(wire.TypeName) {
		return Playlist{}, &HTTPError{Stage: "playlist", Status: 404}
	}
	if wire.TypeName != "Playlist" || (wire.ID != "" && wire.ID != q.ID) {
		return Playlist{}, ErrSchema
	}
	if wire.URI == "" {
		if wire.ID != q.ID {
			return Playlist{}, ErrSchema
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(reply.Data.Playlist, &obj) != nil {
			return Playlist{}, ErrSchema
		}
		obj["uri"], _ = json.Marshal("spotify:playlist:" + q.ID)
		reply.Data.Playlist, _ = json.Marshal(obj)
	} else if wire.URI != "spotify:playlist:"+q.ID {
		return Playlist{}, ErrSchema
	}
	wrapped, _ := json.Marshal(map[string]any{"data": reply.Data.Playlist})
	summary, err := normalizePlaylist(wrapped)
	if err != nil || summary == nil {
		return Playlist{}, ErrSchema
	}
	p := wire.Content
	if p == nil || p.Total == nil || *p.Total < 0 || p.Items == nil || len(p.Items) > q.Limit || (q.Offset < *p.Total && len(p.Items) == 0) || (len(p.Items) > 0 && q.Offset+len(p.Items) > *p.Total) {
		return Playlist{}, ErrSchema
	}
	r := Playlist{PlaylistSummary: *summary, Public: wire.Public, SnapshotID: wire.Revision, Tracks: Page[PlaylistItem]{Href: playlistTrackLink(q, q.Offset), Items: []*PlaylistItem{}, Limit: q.Limit, Offset: q.Offset, Total: *p.Total, SnapshotID: wire.Revision}}
	if wire.Followers != nil {
		if *wire.Followers < 0 {
			return Playlist{}, ErrSchema
		}
		r.Followers = &Followers{Total: *wire.Followers}
	}
	for _, raw := range p.Items {
		var item struct {
			AddedAt *struct {
				ISO string `json:"isoString"`
			} `json:"addedAt"`
			Item *struct {
				URI      string `json:"uri"`
				OtherURI string `json:"_uri"`
				Data     struct {
					trackWire
					TrackDuration *struct {
						MS int `json:"totalMilliseconds"`
					} `json:"trackDuration"`
					Playability *struct {
						Playable bool `json:"playable"`
					} `json:"playability"`
				} `json:"data"`
			} `json:"itemV2"`
		}
		if json.Unmarshal(raw, &item) != nil || item.Item == nil {
			return Playlist{}, ErrSchema
		}
		row := &PlaylistItem{}
		if item.AddedAt != nil && item.AddedAt.ISO != "" {
			row.AddedAt = &item.AddedAt.ISO
		}
		w := item.Item.Data
		if w.Duration == nil {
			w.Duration = w.TrackDuration
		}
		if unavailableType(w.TypeName) || w.TypeName == "LocalTrack" || w.TypeName == "Episode" || (w.Playability != nil && !w.Playability.Playable) {
			r.Tracks.Items = append(r.Tracks.Items, row)
			continue
		}
		normalized, _ := json.Marshal(map[string]any{"item": map[string]any{"uri": item.Item.URI, "_uri": item.Item.OtherURI, "data": w.trackWire}})
		track, err := normalizeTrack(normalized)
		if err != nil || track == nil {
			return Playlist{}, ErrSchema
		}
		row.Track = track
		r.Tracks.Items = append(r.Tracks.Items, row)
	}
	if len(p.Items) > 0 && q.Offset+q.Limit < r.Tracks.Total {
		next := playlistTrackLink(q, q.Offset+q.Limit)
		r.Tracks.Next = &next
	}
	if q.Offset > 0 {
		offset := q.Offset - q.Limit
		if offset < 0 {
			offset = 0
		}
		previous := playlistTrackLink(q, offset)
		r.Tracks.Previous = &previous
	}
	return r, nil
}
