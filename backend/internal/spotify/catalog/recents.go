// Implements the Recents list observed in Spotify's live Web App on 2026-11-20.
package catalog

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

const recentsHash = "94be830be40f0f3759ef83a4ea6e4dd1bef1f6108f7502c11ee1fa7fb1bb1658"
const recentsURI = "spotify:list:recents:page"

type RecentsQuery struct{ Limit, Offset int }

func RecentsPath(path string) bool { return path == "/v1/me/player/recently-played" }
func ParseRecentsQuery(v url.Values) (RecentsQuery, error) {
	q := RecentsQuery{Limit: 100}
	for key, values := range v {
		if (key != "limit" && key != "offset") || len(values) != 1 {
			return q, ErrInvalidQuery
		}
		n, err := strconv.Atoi(values[0])
		if err != nil {
			return q, ErrInvalidQuery
		}
		if key == "limit" {
			q.Limit = n
		} else {
			q.Offset = n
		}
	}
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 100000 {
		return q, ErrInvalidQuery
	}
	return q, nil
}

type RecentEntry struct {
	URI             string  `json:"uri"`
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	Date            string  `json:"date"`
	Subtitle        string  `json:"subtitle"`
	Images          []Image `json:"images"`
	GroupID         string  `json:"group_id"`
	ChildrenGroupID string  `json:"children_group_id"`
	GroupCount      int     `json:"group_count"`
	Track           *Track  `json:"track,omitempty"`
}
type RecentsResult struct {
	Source     string        `json:"source"`
	Items      []RecentEntry `json:"items"`
	Limit      int           `json:"limit"`
	Offset     int           `json:"offset"`
	NextOffset *int          `json:"next_offset"`
	Total      int           `json:"total"`
}
type recentIdentity struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Parent *struct {
		URI      string `json:"uri"`
		Identity struct {
			Name string `json:"name"`
		} `json:"identityTrait"`
	} `json:"contentHierarchyParent"`
	Contributors struct {
		Items []struct {
			Name string `json:"name"`
			URI  string `json:"uri"`
		} `json:"items"`
	} `json:"contributors"`
}
type recentWireItem struct {
	AddedAt    struct{ Day, Month, Year int } `json:"addedAt"`
	Attributes []struct{ Key, Value string }  `json:"formatListAttributes"`
	Entity     *struct {
		URI  string `json:"_uri"`
		Data struct {
			TypeName    string         `json:"__typename"`
			URI         string         `json:"uri"`
			Identity    recentIdentity `json:"identityTrait"`
			Consumption struct {
				Duration *struct {
					Seconds     int64 `json:"seconds"`
					NanoSeconds int64 `json:"nanoSeconds"`
				} `json:"duration"`
				Ratings []string `json:"contentRatings"`
			} `json:"consumptionExperienceTrait"`
			Visual struct {
				Cover struct {
					Image struct {
						Data struct {
							Sources []struct {
								URL    string `json:"url"`
								Height int    `json:"maxHeight"`
								Width  int    `json:"maxWidth"`
							} `json:"sources"`
						} `json:"data"`
					} `json:"image"`
				} `json:"squareCoverImage"`
			} `json:"visualIdentityTrait"`
		} `json:"data"`
	} `json:"entity"`
}

func (c *Client) Recents(ctx context.Context, token auth.Token, q RecentsQuery) (RecentsResult, error) {
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 100000 {
		return RecentsResult{}, ErrInvalidQuery
	}
	ctx, cancel := c.context(ctx)
	defer cancel()
	ct, err := c.clientContext(ctx, token)
	if err != nil {
		return RecentsResult{}, err
	}
	payload := map[string]any{"operationName": "recents", "variables": map[string]any{"uris": []string{recentsURI}, "offset": q.Offset, "limit": q.Limit}, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": recentsHash}}}
	var reply struct {
		Errors []any `json:"errors"`
		Data   struct {
			Lists []struct {
				URI   string `json:"uri"`
				Items *struct {
					Items  []recentWireItem `json:"items"`
					Paging *struct {
						Limit      int  `json:"limit"`
						Offset     int  `json:"offset"`
						NextOffset *int `json:"nextOffset"`
					} `json:"pagingInfo"`
					Total *int `json:"totalCount"`
				} `json:"items"`
			} `json:"lists"`
		} `json:"data"`
	}
	if err = c.request(ctx, "recents", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), ct, &reply); err != nil {
		return RecentsResult{}, err
	}
	if len(reply.Errors) > 0 || len(reply.Data.Lists) != 1 || reply.Data.Lists[0].URI != recentsURI {
		return RecentsResult{}, ErrSchema
	}
	page := reply.Data.Lists[0].Items
	if page == nil || page.Items == nil || page.Total == nil || *page.Total < 0 || page.Paging == nil || page.Paging.Offset != q.Offset || page.Paging.Limit != q.Limit || len(page.Items) > q.Limit {
		return RecentsResult{}, ErrSchema
	}
	end := q.Offset + len(page.Items)
	if (len(page.Items) > 0 && end > *page.Total) || (page.Paging.NextOffset == nil && end < *page.Total) {
		return RecentsResult{}, ErrSchema
	}
	if next := page.Paging.NextOffset; next != nil && (*next <= q.Offset || *next != end || *next >= *page.Total) {
		return RecentsResult{}, ErrSchema
	}
	result := RecentsResult{Source: "webplayer_recents", Items: []RecentEntry{}, Limit: q.Limit, Offset: q.Offset, NextOffset: page.Paging.NextOffset, Total: *page.Total}
	for _, item := range page.Items {
		entry, err := normalizeRecent(item)
		if err != nil {
			return RecentsResult{}, err
		}
		result.Items = append(result.Items, entry)
	}
	return result, ctx.Err()
}

func normalizeRecent(w recentWireItem) (RecentEntry, error) {
	d := w.AddedAt
	date := time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC)
	if d.Year < 1970 || d.Year > 9999 || date.Year() != d.Year || int(date.Month()) != d.Month || date.Day() != d.Day {
		return RecentEntry{}, ErrSchema
	}
	e := RecentEntry{Date: date.Format("2006-01-02"), Images: []Image{}}
	for _, a := range w.Attributes {
		switch {
		case strings.HasPrefix(a.Key, "group_id_"):
			e.GroupID = strings.TrimPrefix(a.Key, "group_id_")
		case a.Key == "children_group_id":
			e.ChildrenGroupID = a.Value
		case a.Key == "group_metadata":
			// The first protobuf field is the group size. Other metadata is left opaque.
			data, err := base64.StdEncoding.DecodeString(a.Value)
			if err == nil && len(data) > 1 && data[0] == 8 {
				n, size := binary.Uvarint(data[1:])
				if size > 0 && n <= 100000 {
					e.GroupCount = int(n)
				}
			}
		}
	}
	if w.Entity == nil {
		if e.ChildrenGroupID == "" {
			return e, ErrSchema
		}
		e.Type = "group"
		return e, nil
	}
	wire := w.Entity.Data
	if wire.TypeName != "Entity" {
		e.Type = "unavailable"
		return e, nil
	}
	e.URI = wire.URI
	if e.URI == "" || (w.Entity.URI != "" && e.URI != w.Entity.URI) || !strings.HasPrefix(e.URI, "spotify:") {
		return e, ErrSchema
	}
	e.Name = wire.Identity.Name
	e.Type = strings.ToLower(wire.Identity.Type)
	for _, s := range wire.Visual.Cover.Image.Data.Sources {
		u, err := url.Parse(s.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			continue
		}
		h, width := s.Height, s.Width
		e.Images = append(e.Images, Image{URL: s.URL, Height: &h, Width: &width})
	}
	names := []string{}
	artists := []SimpleArtist{}
	for _, a := range wire.Identity.Contributors.Items {
		names = append(names, a.Name)
		if id, err := uriID(a.URI, "artist"); err == nil {
			artists = append(artists, SimpleArtist{ID: id, URI: a.URI, Name: a.Name, ExternalURLs: map[string]string{"spotify": "https://open.spotify.com/artist/" + id}})
		}
	}
	e.Subtitle = strings.Join(names, ", ")
	if wire.Identity.Parent != nil && len(names) == 0 {
		e.Subtitle = wire.Identity.Parent.Identity.Name
	}
	if !strings.HasPrefix(e.URI, "spotify:track:") {
		return e, nil
	}
	id, err := uriID(e.URI, "track")
	if err != nil {
		return e, err
	}
	if wire.Identity.Parent == nil || wire.Consumption.Duration == nil || e.Name == "" || len(artists) == 0 {
		return e, ErrSchema
	}
	albumID, err := uriID(wire.Identity.Parent.URI, "album")
	if err != nil {
		return e, err
	}
	duration := wire.Consumption.Duration
	if duration.Seconds < 0 || duration.Seconds > 86400 || duration.NanoSeconds < 0 || duration.NanoSeconds >= 1000000000 {
		return e, ErrSchema
	}
	explicit := false
	for _, rating := range wire.Consumption.Ratings {
		if rating == "CONTENT_RATING_EXPLICIT" {
			explicit = true
		}
	}
	e.Track = &Track{ID: id, Name: e.Name, URI: e.URI, DurationMS: int(duration.Seconds*1000 + duration.NanoSeconds/1000000), Artists: artists, Explicit: &explicit, ExternalURLs: map[string]string{"spotify": "https://open.spotify.com/track/" + id}, Album: AlbumSummary{ID: albumID, URI: wire.Identity.Parent.URI, Name: wire.Identity.Parent.Identity.Name, Artists: artists, Images: e.Images, ExternalURLs: map[string]string{"spotify": "https://open.spotify.com/album/" + albumID}}}
	return e, nil
}
