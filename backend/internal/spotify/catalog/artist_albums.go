// Maps a bounded artist discography response into navigable album and single summaries.
package catalog

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

// Protocol facts: Spicetify cli v2.38.4, CustomApps/new-releases/index.js.
// Only the persisted query contract is used; no upstream implementation is included.
const artistAlbumsHash = "9380995a9d4663cbcb5113fef3c6aabf70ae6d407ba61793fd01e2a1dd6929b0"
const artistAlbumsLimit = 100

type ArtistAlbums struct {
	Items     []*AlbumSummary `json:"items"`
	Limit     int             `json:"limit"`
	Truncated bool            `json:"truncated"`
}

func ArtistAlbumsPath(path string) (string, bool) {
	p := strings.Split(strings.TrimPrefix(path, "/v1/"), "/")
	if len(p) == 3 && p[0] == "artists" && p[2] == "albums" {
		return p[1], true
	}
	return "", false
}

// The reviewed consumer reads the first 100 release groups. Do not fabricate
// total counts or next-page cursors until the provider paging shape is verified.
func ValidateArtistAlbumsQuery(id string, values url.Values) error {
	if err := ValidateArtistQuery(id, values); err != nil {
		return err
	}
	if value := values.Get("offset"); value != "" && value != "0" {
		return ErrInvalidQuery
	}
	if value := values.Get("limit"); value != "" && value != strconv.Itoa(artistAlbumsLimit) {
		return ErrInvalidQuery
	}
	if values.Get("include_groups") != "" {
		return ErrInvalidQuery
	}
	return nil
}

func (c *Client) ArtistAlbums(ctx context.Context, token auth.Token, id string) (ArtistAlbums, error) {
	if !objectID.MatchString(id) {
		return ArtistAlbums{}, ErrInvalidQuery
	}
	ctx, cancel := c.context(ctx)
	defer cancel()
	ct, err := c.clientContext(ctx, token)
	if err != nil {
		return ArtistAlbums{}, err
	}
	var reply struct {
		Errors []any `json:"errors"`
		Data   struct {
			Artist json.RawMessage `json:"artistUnion"`
		} `json:"data"`
	}
	payload := map[string]any{"operationName": "queryArtistDiscographyAll", "variables": map[string]any{"uri": "spotify:artist:" + id, "offset": 0, "limit": artistAlbumsLimit}, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": artistAlbumsHash}}}
	if err := c.request(ctx, "artist", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), ct, &reply); err != nil {
		return ArtistAlbums{}, err
	}
	if len(reply.Errors) > 0 {
		return ArtistAlbums{}, ErrSchema
	}
	return normalizeArtistAlbums(reply.Data.Artist, id)
}

func normalizeArtistAlbums(raw json.RawMessage, id string) (ArtistAlbums, error) {
	var wire struct {
		ID          string `json:"id"`
		URI         string `json:"uri"`
		TypeName    string `json:"__typename"`
		Discography *struct {
			All *struct {
				Items []struct {
					Releases *struct {
						Items []struct {
							albumWire
							ReleaseDate *struct {
								ISO string `json:"isoString"`
							} `json:"date"`
							Tracks *struct {
								Total *int `json:"totalCount"`
							} `json:"tracks"`
						} `json:"items"`
					} `json:"releases"`
				} `json:"items"`
			} `json:"all"`
		} `json:"discography"`
	}
	if json.Unmarshal(raw, &wire) != nil {
		return ArtistAlbums{}, ErrSchema
	}
	if unavailableType(wire.TypeName) {
		return ArtistAlbums{}, &HTTPError{Stage: "artist", Status: 404}
	}
	// These identity fields are optional in the reviewed operation. Reject conflicts
	// when supplied; request identity comes from the fixed artist URI variable.
	if (wire.ID != "" && wire.ID != id) || (wire.URI != "" && wire.URI != "spotify:artist:"+id) || (wire.TypeName != "" && wire.TypeName != "Artist") {
		return ArtistAlbums{}, ErrSchema
	}
	if wire.Discography == nil || wire.Discography.All == nil || wire.Discography.All.Items == nil || len(wire.Discography.All.Items) > artistAlbumsLimit {
		return ArtistAlbums{}, ErrSchema
	}
	result := ArtistAlbums{Items: []*AlbumSummary{}, Limit: artistAlbumsLimit, Truncated: len(wire.Discography.All.Items) == artistAlbumsLimit}
	seen := map[string]bool{}
	for _, group := range wire.Discography.All.Items {
		if group.Releases == nil || group.Releases.Items == nil || len(group.Releases.Items) > 100 {
			return ArtistAlbums{}, ErrSchema
		}
		for _, release := range group.Releases.Items {
			if unavailableType(release.TypeName) {
				continue
			}
			if release.Type != "ALBUM" && release.Type != "SINGLE" && release.Type != "EP" && release.Type != "COMPILATION" {
				return ArtistAlbums{}, ErrSchema
			}
			if release.Tracks == nil || release.Tracks.Total == nil || *release.Tracks.Total < 0 {
				return ArtistAlbums{}, ErrSchema
			}
			album, err := albumSummary(release.albumWire)
			if err != nil {
				return ArtistAlbums{}, err
			}
			if release.ReleaseDate != nil && release.ReleaseDate.ISO != "" {
				date := strings.Split(release.ReleaseDate.ISO, "T")[0]
				if _, err := time.Parse("2006-01-02", date); err != nil {
					return ArtistAlbums{}, ErrSchema
				}
				album.ReleaseDate = &date
			}
			if !seen[album.ID] {
				result.Items = append(result.Items, &album)
				seen[album.ID] = true
			}
			if len(result.Items) > 1000 {
				return ArtistAlbums{}, ErrSchema
			}
		}
	}
	return result, nil
}
