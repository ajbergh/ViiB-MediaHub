// Maps the Web Player's saved-track query to a minimal account-list page.
package catalog

import (
	"context"
	"encoding/json"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

// Reviewed fetchLibraryTracks hash from Spotui's SpotifyHashProvider.
const likedTracksHash = "087278b20b743578a6262c2b0b4bcd20d879c503cc359a2285baf083ef944240"

type likedTrackSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type likedTrackItem struct {
	Track *likedTrackSummary `json:"track"`
}

func (c *Client) likedTracks(ctx context.Context, token auth.Token, ct string, q LibraryQuery) (Page[likedTrackItem], error) {
	payload := map[string]any{
		"operationName": "fetchLibraryTracks",
		"variables":     map[string]any{"offset": q.Offset, "limit": q.Limit},
		"extensions":    map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": likedTracksHash}},
	}
	var reply struct {
		Errors []json.RawMessage `json:"errors"`
		Data   struct {
			Me struct {
				Library struct {
					Tracks *wirePage `json:"tracks"`
				} `json:"library"`
			} `json:"me"`
		} `json:"data"`
	}
	if err := c.request(ctx, "liked_tracks", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), ct, &reply); err != nil {
		return Page[likedTrackItem]{}, err
	}
	if len(reply.Errors) != 0 {
		return Page[likedTrackItem]{}, ErrSchema
	}
	wire := reply.Data.Me.Library.Tracks
	page, err := libraryPage[likedTrackItem](q, wire)
	if err != nil {
		return page, err
	}
	for _, raw := range wire.Items {
		var row struct {
			Track *struct {
				URI      string `json:"uri"`
				OtherURI string `json:"_uri"`
				Data     *struct {
					URI  string `json:"uri"`
					Name string `json:"name"`
				} `json:"data"`
			} `json:"track"`
		}
		if json.Unmarshal(raw, &row) != nil {
			return Page[likedTrackItem]{}, ErrSchema
		}
		item := &likedTrackItem{}
		if track := row.Track; track != nil && track.Data != nil {
			var id string
			for _, uri := range []string{track.URI, track.OtherURI, track.Data.URI} {
				if uri == "" {
					continue
				}
				next, err := uriID(uri, "track")
				if err != nil || (id != "" && id != next) {
					id = ""
					break
				}
				id = next
			}
			if id != "" && track.Data.Name != "" {
				item.Track = &likedTrackSummary{ID: id, Name: track.Data.Name, Type: "track"}
			}
		}
		page.Items = append(page.Items, item)
	}
	return page, ctx.Err()
}
