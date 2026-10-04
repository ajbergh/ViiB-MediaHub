// Maps saved albums and playlists with bounded paging into catalog models.
package catalog

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"net/url"
	"strconv"
)

// Protocol facts: Spotui f9d05b6450e730469d15f30f9dd4bc790db794db.
const libraryHash = "973e511ca44261fda7eebac8b653155e7caee3675abb4fb110cc1b8c78b091c3"

type LibraryQuery struct {
	Kind          string
	Limit, Offset int
}
type SavedAlbum struct {
	AddedAt *string `json:"added_at"`
	Album   Album   `json:"album"`
}

func LibraryPath(path string) (kind string, ok bool) {
	switch path {
	case "/v1/me/albums":
		return "album", true
	case "/v1/me/playlists":
		return "playlist", true
	}
	return
}
func ParseLibraryQuery(kind string, v url.Values) (LibraryQuery, error) {
	q := LibraryQuery{Kind: kind, Limit: 20}
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
func (q LibraryQuery) validate() error {
	if (q.Kind != "album" && q.Kind != "playlist") || q.Limit < 1 || q.Limit > 50 || q.Offset < 0 || q.Offset > 100000 {
		return ErrInvalidQuery
	}
	return nil
}
func libraryLink(q LibraryQuery, offset int) string {
	return "https://api.spotify.com/v1/me/" + q.Kind + "s?" + url.Values{"limit": {strconv.Itoa(q.Limit)}, "offset": {strconv.Itoa(offset)}}.Encode()
}
func libraryPage[T any](q LibraryQuery, wire *wirePage) (Page[T], error) {
	if wire == nil || wire.Total == nil || *wire.Total < 0 || wire.Items == nil || len(wire.Items) != min(q.Limit, max(*wire.Total-q.Offset, 0)) || (q.Offset < *wire.Total && len(wire.Items) == 0) || (len(wire.Items) > 0 && q.Offset+len(wire.Items) > *wire.Total) {
		return Page[T]{}, ErrSchema
	}
	p := Page[T]{Href: libraryLink(q, q.Offset), Items: []*T{}, Limit: q.Limit, Offset: q.Offset, Total: *wire.Total}
	if q.Offset+q.Limit < p.Total {
		next := libraryLink(q, q.Offset+q.Limit)
		p.Next = &next
	}
	if q.Offset > 0 {
		previousOffset := q.Offset - q.Limit
		if previousOffset < 0 {
			previousOffset = 0
		}
		previous := libraryLink(q, previousOffset)
		p.Previous = &previous
	}
	return p, nil
}
func (c *Client) Library(ctx context.Context, token auth.Token, q LibraryQuery) (any, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	ctx, cancel := c.context(ctx)
	defer cancel()
	ct, err := c.clientContext(ctx, token)
	if err != nil {
		return nil, err
	}
	filter := "Albums"
	if q.Kind == "playlist" {
		filter = "Playlists"
	}
	payload := map[string]any{"operationName": "libraryV3", "variables": map[string]any{"filters": []string{filter}, "order": nil, "textFilter": "", "features": []string{}, "limit": q.Limit, "offset": q.Offset, "flatten": true, "expandedFolders": []string{}, "folderUri": nil, "includeFoldersWhenFlattening": false}, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": libraryHash}}}
	var reply struct {
		Errors []any `json:"errors"`
		Data   struct {
			Me *struct {
				Library *wirePage `json:"libraryV3"`
			} `json:"me"`
		} `json:"data"`
	}
	if err := c.request(ctx, "library", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), ct, &reply); err != nil {
		return nil, err
	}
	if len(reply.Errors) > 0 || reply.Data.Me == nil {
		return nil, ErrSchema
	}
	wire := reply.Data.Me.Library
	albumPage, err := libraryPage[SavedAlbum](q, wire)
	if err != nil {
		return nil, err
	}
	playlistPage, err := libraryPage[Playlist](q, wire)
	if err != nil {
		return nil, err
	}
	albums := map[string]Album{}
	playlists := map[string]Playlist{}
	for _, raw := range wire.Items {
		var row struct {
			AddedAt *struct {
				ISO string `json:"isoString"`
			} `json:"addedAt"`
			Item *struct {
				URI      string          `json:"uri"`
				OtherURI string          `json:"_uri"`
				Data     json.RawMessage `json:"data"`
			} `json:"item"`
		}
		if json.Unmarshal(raw, &row) != nil || row.Item == nil {
			return nil, ErrSchema
		}
		var identity struct {
			TypeName string `json:"__typename"`
			URI      string `json:"uri"`
		}
		if json.Unmarshal(row.Item.Data, &identity) != nil {
			return nil, ErrSchema
		}
		if unavailableType(identity.TypeName) {
			if q.Kind == "album" {
				albumPage.Items = append(albumPage.Items, nil)
			} else {
				playlistPage.Items = append(playlistPage.Items, nil)
			}
			continue
		}
		expected := "Album"
		if q.Kind == "playlist" {
			expected = "Playlist"
		}
		if identity.TypeName != expected {
			return nil, ErrSchema
		}
		uri := identity.URI
		if row.Item.OtherURI != "" && row.Item.URI != "" && row.Item.OtherURI != row.Item.URI {
			return nil, ErrSchema
		}
		wrapperURI := row.Item.OtherURI
		if wrapperURI == "" {
			wrapperURI = row.Item.URI
		}
		if uri == "" {
			uri = wrapperURI
		} else if wrapperURI != "" && wrapperURI != uri {
			return nil, ErrSchema
		}
		id, err := uriID(uri, q.Kind)
		if err != nil {
			return nil, err
		}
		if q.Kind == "album" {
			album, ok := albums[id]
			if !ok {
				album, err = c.Album(ctx, token, AlbumQuery{ID: id, Limit: 1})
				if err != nil {
					return nil, err
				}
				albums[id] = album
			}
			saved := &SavedAlbum{Album: album}
			if row.AddedAt != nil && row.AddedAt.ISO != "" {
				saved.AddedAt = &row.AddedAt.ISO
			}
			albumPage.Items = append(albumPage.Items, saved)
		} else {
			playlist, ok := playlists[id]
			if !ok {
				playlist, err = c.Playlist(ctx, token, PlaylistQuery{ID: id, Limit: 1})
				if err != nil {
					return nil, err
				}
				playlists[id] = playlist
			}
			playlistPage.Items = append(playlistPage.Items, &playlist)
		}
	}
	if q.Kind == "album" {
		return albumPage, nil
	}
	return playlistPage, nil
}
