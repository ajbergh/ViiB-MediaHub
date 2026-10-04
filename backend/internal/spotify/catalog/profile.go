package catalog

import (
	"context"
	"net/url"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

type Image struct {
	URL    string `json:"url"`
	Height *int   `json:"height"`
	Width  *int   `json:"width"`
}
type Followers struct {
	Href  *string `json:"href"`
	Total int     `json:"total"`
}
type Profile struct {
	ID           string            `json:"id"`
	DisplayName  string            `json:"display_name"`
	Email        *string           `json:"email"`
	Images       []Image           `json:"images"`
	Product      *string           `json:"product"`
	Country      *string           `json:"country"`
	Followers    *Followers        `json:"followers"`
	ExternalURLs map[string]string `json:"external_urls"`
}
type profileEnvelope struct {
	Errors []any `json:"errors"`
	Data   struct {
		Me struct {
			Profile *struct {
				Username string `json:"username"`
				Name     string `json:"name"`
				URI      string `json:"uri"`
				Avatar   *struct {
					Sources []Image `json:"sources"`
				} `json:"avatar"`
			} `json:"profile"`
		} `json:"me"`
	} `json:"data"`
}

func (c *Client) Profile(ctx context.Context, token auth.Token) (Profile, error) {
	ctx, cancel := c.context(ctx)
	defer cancel()
	clientToken, err := c.clientContext(ctx, token)
	if err != nil {
		return Profile{}, err
	}
	payload := map[string]any{"operationName": "profileAttributes", "variables": map[string]any{}, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": "53bcb064f6cd18c23f752bc324a791194d20df612d8e1239c735144ab0399ced"}}}
	var reply profileEnvelope
	if err := c.request(ctx, "profile", "https://api-partner.spotify.com/pathfinder/v2/query", payload, token.Bearer(), clientToken, &reply); err != nil {
		return Profile{}, err
	}
	profile := reply.Data.Me.Profile
	if len(reply.Errors) > 0 || profile == nil || profile.Username == "" {
		return Profile{}, ErrSchema
	}
	if profile.URI != "" && profile.URI != "spotify:user:"+profile.Username {
		return Profile{}, ErrSchema
	}
	result := Profile{ID: profile.Username, DisplayName: profile.Name, Images: []Image{}, ExternalURLs: map[string]string{"spotify": "https://open.spotify.com/user/" + url.PathEscape(profile.Username)}}
	if profile.Avatar != nil {
		for _, image := range profile.Avatar.Sources {
			parsed, err := url.Parse(image.URL)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || strings.ContainsAny(image.URL, "\r\n") {
				continue
			}
			result.Images = append(result.Images, image)
		}
	}
	return result, nil
}
