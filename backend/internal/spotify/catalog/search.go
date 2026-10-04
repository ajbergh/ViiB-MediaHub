package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

// Protocol facts checked in Spotui f9d05b6450e730469d15f30f9dd4bc790db794db,
// Spotify.kt and SpotifyHashProvider.kt. No upstream implementation is included.
const searchHash = "4801118d4a100f756e833d33984436a3899cff359c532f8fd3aaf174b60b3b49"

var ErrInvalidQuery = errors.New("invalid Spotify catalog query")
var objectID = regexp.MustCompile("^[A-Za-z0-9]{22}$")

type SimpleArtist struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	URI          string            `json:"uri"`
	ExternalURLs map[string]string `json:"external_urls"`
}
type AlbumSummary struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	URI          string            `json:"uri"`
	Artists      []SimpleArtist    `json:"artists"`
	Images       []Image           `json:"images"`
	ReleaseDate  *string           `json:"release_date"`
	AlbumType    string            `json:"album_type"`
	ExternalURLs map[string]string `json:"external_urls"`
}
type Track struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	URI          string            `json:"uri"`
	DurationMS   int               `json:"duration_ms"`
	Artists      []SimpleArtist    `json:"artists"`
	Album        AlbumSummary      `json:"album"`
	Explicit     *bool             `json:"explicit"`
	PreviewURL   *string           `json:"preview_url"`
	ExternalURLs map[string]string `json:"external_urls"`
}
type ArtistSummary struct {
	SimpleArtist
	Images    []Image    `json:"images"`
	Followers *Followers `json:"followers"`
}
type PlaylistOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	URI         string `json:"uri"`
}
type PlaylistSummary struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	URI          string            `json:"uri"`
	Description  string            `json:"description"`
	Images       []Image           `json:"images"`
	Owner        PlaylistOwner     `json:"owner"`
	ExternalURLs map[string]string `json:"external_urls"`
}
type Page[T any] struct {
	Href       string  `json:"href"`
	Items      []*T    `json:"items"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
	Total      int     `json:"total"`
	Next       *string `json:"next"`
	Previous   *string `json:"previous"`
	SnapshotID *string `json:"snapshot_id,omitempty"`
}
type SearchResult struct {
	Tracks    *Page[Track]           `json:"tracks,omitempty"`
	Albums    *Page[AlbumSummary]    `json:"albums,omitempty"`
	Artists   *Page[ArtistSummary]   `json:"artists,omitempty"`
	Playlists *Page[PlaylistSummary] `json:"playlists,omitempty"`
}
type SearchQuery struct {
	Term          string
	Types         []string
	Limit, Offset int
}

func ParseSearchQuery(values url.Values) (SearchQuery, error) {
	q := SearchQuery{Term: strings.TrimSpace(values.Get("q")), Types: []string{"track", "album", "artist", "playlist"}, Limit: 20}
	if value := values.Get("type"); value != "" {
		q.Types = strings.Split(value, ",")
	}
	var err error
	if value := values.Get("limit"); value != "" {
		q.Limit, err = strconv.Atoi(value)
		if err != nil {
			return q, ErrInvalidQuery
		}
	}
	if value := values.Get("offset"); value != "" {
		q.Offset, err = strconv.Atoi(value)
		if err != nil {
			return q, ErrInvalidQuery
		}
	}
	return q, q.validate()
}
func (q SearchQuery) validate() error {
	if strings.TrimSpace(q.Term) == "" || len(q.Term) > 1024 || q.Limit < 1 || q.Limit > 50 || q.Offset < 0 || q.Offset > 100000 || len(q.Types) == 0 || len(q.Types) > 4 {
		return ErrInvalidQuery
	}
	seen := map[string]bool{}
	for _, kind := range q.Types {
		if seen[kind] {
			return ErrInvalidQuery
		}
		seen[kind] = true
		switch kind {
		case "track", "album", "artist", "playlist":
		default:
			return ErrInvalidQuery
		}
	}
	return nil
}

type wirePage struct {
	Total *int              `json:"totalCount"`
	Items []json.RawMessage `json:"items"`
}
type searchEnvelope struct {
	Errors []any `json:"errors"`
	Data   struct {
		Search *struct {
			Tracks    *wirePage `json:"tracksV2"`
			Albums    *wirePage `json:"albumsV2"`
			Artists   *wirePage `json:"artists"`
			Playlists *wirePage `json:"playlists"`
		} `json:"searchV2"`
	} `json:"data"`
}
type artistWire struct {
	URI     string `json:"uri"`
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
}
type artistList struct {
	Items []artistWire `json:"items"`
}
type artWire struct {
	Sources []Image `json:"sources"`
}
type albumWire struct {
	ID       string     `json:"id"`
	TypeName string     `json:"__typename"`
	URI      string     `json:"uri"`
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Artists  artistList `json:"artists"`
	CoverArt artWire    `json:"coverArt"`
	Date     *struct {
		Year int `json:"year"`
	} `json:"date"`
}
type trackWire struct {
	TypeName string     `json:"__typename"`
	URI      string     `json:"uri"`
	OtherURI string     `json:"_uri"`
	Name     string     `json:"name"`
	Artists  artistList `json:"artists"`
	Album    albumWire  `json:"albumOfTrack"`
	Duration *struct {
		MS int `json:"totalMilliseconds"`
	} `json:"duration"`
	ContentRating *struct {
		Label string `json:"label"`
	} `json:"contentRating"`
}

func (c *Client) Search(ctx context.Context, token auth.Token, q SearchQuery) (SearchResult, error) {
	if err := q.validate(); err != nil {
		return SearchResult{}, err
	}
	ctx, cancel := c.context(ctx)
	defer cancel()
	clientToken, err := c.clientContext(ctx, token)
	if err != nil {
		return SearchResult{}, err
	}
	variables := map[string]any{"searchTerm": q.Term, "offset": q.Offset, "limit": q.Limit, "numberOfTopResults": 5, "includeAudiobooks": false, "includeArtistHasConcertsField": false, "includePreReleases": false, "includeLocalConcertsField": false, "includeAuthors": false}
	payload := map[string]any{"operationName": "searchDesktop", "variables": variables, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": searchHash}}}
	var reply searchEnvelope
	if err := c.request(ctx, "search", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), clientToken, &reply); err != nil {
		return SearchResult{}, err
	}
	if len(reply.Errors) > 0 || reply.Data.Search == nil {
		return SearchResult{}, ErrSchema
	}
	var result SearchResult
	for _, kind := range q.Types {
		switch kind {
		case "track":
			result.Tracks, err = normalizePage(reply.Data.Search.Tracks, q, "track", normalizeTrack)
		case "album":
			result.Albums, err = normalizePage(reply.Data.Search.Albums, q, "album", normalizeAlbum)
		case "artist":
			result.Artists, err = normalizePage(reply.Data.Search.Artists, q, "artist", normalizeArtist)
		case "playlist":
			result.Playlists, err = normalizePage(reply.Data.Search.Playlists, q, "playlist", normalizePlaylist)
		}
		if err != nil {
			return SearchResult{}, err
		}
	}
	return result, nil
}
func searchLink(q SearchQuery, offset int) string {
	values := url.Values{"q": {q.Term}, "type": {strings.Join(q.Types, ",")}, "limit": {strconv.Itoa(q.Limit)}, "offset": {strconv.Itoa(offset)}}
	return "https://api.spotify.com/v1/search?" + values.Encode()
}
func normalizePage[T any](wire *wirePage, q SearchQuery, kind string, convert func(json.RawMessage) (*T, error)) (*Page[T], error) {
	if wire == nil || wire.Total == nil || *wire.Total < 0 || wire.Items == nil || len(wire.Items) > q.Limit {
		return nil, ErrSchema
	}
	page := &Page[T]{Href: searchLink(q, q.Offset), Items: make([]*T, 0, len(wire.Items)), Limit: q.Limit, Offset: q.Offset, Total: *wire.Total}
	for _, raw := range wire.Items {
		item, err := convert(raw)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, item)
	}
	if len(wire.Items) > 0 && q.Offset+q.Limit < page.Total {
		next := searchLink(q, q.Offset+q.Limit)
		page.Next = &next
	}
	if q.Offset > 0 {
		offset := q.Offset - q.Limit
		if offset < 0 {
			offset = 0
		}
		prev := searchLink(q, offset)
		page.Previous = &prev
	}
	return page, nil
}
func uriID(uri, kind string) (string, error) {
	prefix := "spotify:" + kind + ":"
	if !strings.HasPrefix(uri, prefix) || !objectID.MatchString(strings.TrimPrefix(uri, prefix)) {
		return "", ErrSchema
	}
	return strings.TrimPrefix(uri, prefix), nil
}
func externalURL(kind, id string) map[string]string {
	return map[string]string{"spotify": "https://open.spotify.com/" + kind + "/" + url.PathEscape(id)}
}
func images(sources []Image) []Image {
	result := []Image{}
	for _, image := range sources {
		u, err := url.Parse(image.URL)
		if err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil {
			result = append(result, image)
		}
	}
	return result
}
func artists(wire artistList) ([]SimpleArtist, error) {
	result := make([]SimpleArtist, 0, len(wire.Items))
	for _, item := range wire.Items {
		id, err := uriID(item.URI, "artist")
		if err != nil || item.Profile.Name == "" {
			return nil, ErrSchema
		}
		result = append(result, SimpleArtist{ID: id, Name: item.Profile.Name, URI: item.URI, ExternalURLs: externalURL("artist", id)})
	}
	return result, nil
}
func albumSummary(wire albumWire) (AlbumSummary, error) {
	id, err := uriID(wire.URI, "album")
	if err != nil || wire.Name == "" {
		return AlbumSummary{}, ErrSchema
	}
	names, err := artists(wire.Artists)
	if err != nil {
		return AlbumSummary{}, err
	}
	result := AlbumSummary{ID: id, Name: wire.Name, URI: wire.URI, Artists: names, Images: images(wire.CoverArt.Sources), AlbumType: strings.ToLower(wire.Type), ExternalURLs: externalURL("album", id)}
	if wire.Date != nil && wire.Date.Year > 0 {
		year := fmt.Sprintf("%04d", wire.Date.Year)
		result.ReleaseDate = &year
	}
	return result, nil
}
func normalizeAlbum(raw json.RawMessage) (*AlbumSummary, error) {
	var wrapper struct {
		Data albumWire `json:"data"`
	}
	if json.Unmarshal(raw, &wrapper) != nil {
		return nil, ErrSchema
	}
	if wrapper.Data.TypeName != "Album" {
		if unavailableType(wrapper.Data.TypeName) {
			return nil, nil
		}
		return nil, ErrSchema
	}
	value, err := albumSummary(wrapper.Data)
	return &value, err
}
func normalizeTrack(raw json.RawMessage) (*Track, error) {
	var outer struct {
		Item struct {
			URI      string    `json:"uri"`
			OtherURI string    `json:"_uri"`
			Data     trackWire `json:"data"`
		} `json:"item"`
	}
	if json.Unmarshal(raw, &outer) != nil {
		return nil, ErrSchema
	}
	wire := outer.Item.Data
	if wire.TypeName != "Track" {
		if unavailableType(wire.TypeName) {
			return nil, nil
		}
		return nil, ErrSchema
	}
	uri := wire.URI
	if uri == "" {
		uri = wire.OtherURI
	}
	if uri == "" {
		uri = outer.Item.OtherURI
	}
	if uri == "" {
		uri = outer.Item.URI
	}
	id, err := uriID(uri, "track")
	if err != nil || wire.Name == "" || wire.Duration == nil || wire.Duration.MS <= 0 {
		return nil, ErrSchema
	}
	names, err := artists(wire.Artists)
	if err != nil || len(names) == 0 {
		return nil, ErrSchema
	}
	album, err := albumSummary(wire.Album)
	if err != nil {
		return nil, err
	}
	value := Track{ID: id, Name: wire.Name, URI: uri, DurationMS: wire.Duration.MS, Artists: names, Album: album, ExternalURLs: externalURL("track", id)}
	if wire.ContentRating != nil {
		label := wire.ContentRating.Label
		explicit := label == "EXPLICIT"
		if label == "EXPLICIT" || label == "NONE" {
			value.Explicit = &explicit
		}
	}
	return &value, nil
}
func normalizeArtist(raw json.RawMessage) (*ArtistSummary, error) {
	var wrapper struct {
		Data struct {
			TypeName string `json:"__typename"`
			artistWire
			Visuals struct {
				Avatar artWire `json:"avatarImage"`
			} `json:"visuals"`
			Stats *struct {
				Followers *int `json:"followers"`
			} `json:"stats"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &wrapper) != nil {
		return nil, ErrSchema
	}
	wire := wrapper.Data
	if wire.TypeName != "Artist" {
		if unavailableType(wrapper.Data.TypeName) {
			return nil, nil
		}
		return nil, ErrSchema
	}
	id, err := uriID(wire.URI, "artist")
	if err != nil || wire.Profile.Name == "" {
		return nil, ErrSchema
	}
	value := ArtistSummary{SimpleArtist: SimpleArtist{ID: id, Name: wire.Profile.Name, URI: wire.URI, ExternalURLs: externalURL("artist", id)}, Images: images(wire.Visuals.Avatar.Sources)}
	if wire.Stats != nil && wire.Stats.Followers != nil {
		value.Followers = &Followers{Total: *wire.Stats.Followers}
	}
	return &value, nil
}
func normalizePlaylist(raw json.RawMessage) (*PlaylistSummary, error) {
	var wrapper struct {
		Data struct {
			TypeName    string  `json:"__typename"`
			URI         string  `json:"uri"`
			Name        *string `json:"name"`
			Description string  `json:"description"`
			Images      struct {
				Items []artWire `json:"items"`
			} `json:"images"`
			Owner struct {
				Data struct {
					URI  string `json:"uri"`
					Name string `json:"name"`
				} `json:"data"`
			} `json:"ownerV2"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &wrapper) != nil {
		return nil, ErrSchema
	}
	wire := wrapper.Data
	if wire.TypeName != "Playlist" {
		if unavailableType(wrapper.Data.TypeName) {
			return nil, nil
		}
		return nil, ErrSchema
	}
	id, err := uriID(wire.URI, "playlist")
	if err != nil || wire.Name == nil {
		return nil, ErrSchema
	}
	value := PlaylistSummary{ID: id, Name: *wire.Name, URI: wire.URI, Description: wire.Description, Images: []Image{}, ExternalURLs: externalURL("playlist", id)}
	owner := wire.Owner.Data
	if owner.URI != "" && !strings.HasPrefix(owner.URI, "spotify:user:") {
		return nil, ErrSchema
	}
	value.Owner = PlaylistOwner{ID: strings.TrimPrefix(owner.URI, "spotify:user:"), DisplayName: owner.Name, URI: owner.URI}
	for _, group := range wire.Images.Items {
		value.Images = append(value.Images, images(group.Sources)...)
	}
	return &value, nil
}

func unavailableType(kind string) bool { return kind == "NotFound" || kind == "RestrictedContent" }
