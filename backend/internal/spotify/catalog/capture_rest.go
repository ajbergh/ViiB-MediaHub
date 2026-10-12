package catalog

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/spotify/metadata"
)

// CaptureREST retains original catalog objects, never profile or auth envelopes.
// Related/search projections have separate resource keys from entity details.
func CaptureREST(target *url.URL, raw []byte) ([]CapturedEntity, error) {
	if target == nil || target.Scheme != "https" || target.Host != "api.spotify.com" || target.User != nil {
		return nil, nil
	}
	parts := strings.Split(strings.Trim(target.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "v1" {
		return nil, nil
	}
	library := len(parts) == 3 && parts[1] == "me" && (parts[2] == "tracks" || parts[2] == "albums" || parts[2] == "playlists")
	allowed := library || parts[1] == "search" || parts[1] == "tracks" || parts[1] == "albums" || parts[1] == "artists" || parts[1] == "playlists"
	if !allowed {
		return nil, nil
	}
	safe, err := metadata.Sanitize(raw, metadata.CatalogLimit)
	if err != nil {
		return nil, err
	}
	resource := "rest:" + target.Path + ":page:" + target.Query().Get("offset") + ":" + target.Query().Get("limit")
	offset := 0
	if rawOffset := target.Query().Get("offset"); rawOffset != "" {
		var err error
		offset, err = strconv.Atoi(rawOffset)
		if err != nil {
			return nil, ErrSchema
		}
	}
	if offset < 0 {
		return nil, ErrSchema
	}
	var entities []CapturedEntity
	entityPage := len(parts) == 4 && (parts[1] == "albums" || parts[1] == "playlists") && parts[3] == "tracks"
	artistAlbumsPage := len(parts) == 4 && parts[1] == "artists" && parts[3] == "albums"
	if (entityPage || artistAlbumsPage) && !objectID.MatchString(parts[2]) {
		return nil, ErrSchema
	}
	if artistAlbumsPage {
		// These documented filters select different membership projections. Hash
		// only known query fields, never arbitrary URL parameters or credentials.
		pageQuery := url.Values{
			"offset": {target.Query().Get("offset")}, "limit": {target.Query().Get("limit")},
			"include_groups": {target.Query().Get("include_groups")}, "market": {target.Query().Get("market")},
		}
		// Keep the complete digest within the repository's 128-byte resource key
		// limit, even when the request contains long numeric page parameters.
		resource = fmt.Sprintf("rest:%s:page:%x", target.Path, sha256.Sum256([]byte(pageQuery.Encode())))
	}
	if library || entityPage || artistAlbumsPage {
		var page struct {
			Offset int               `json:"offset"`
			Items  []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(safe, &page) != nil || page.Offset < 0 || page.Items == nil {
			return nil, ErrSchema
		}
		if artistAlbumsPage {
			var position struct {
				Offset *int `json:"offset"`
			}
			if json.Unmarshal(safe, &position) != nil || position.Offset == nil || *position.Offset != offset {
				return nil, ErrSchema
			}
		}
		entity := CapturedEntity{EntityType: "library", ID: "rest_saved_" + parts[2], Resource: resource, Payload: safe}
		relationKind := "library_items"
		if entityPage {
			entity.EntityType = strings.TrimSuffix(parts[1], "s")
			entity.ID = parts[2]
			relationKind = "tracks"
		}
		if artistAlbumsPage {
			entity.EntityType, entity.ID, relationKind = "artist", parts[2], "albums"
		}
		for i, row := range page.Items {
			var object map[string]json.RawMessage
			_ = json.Unmarshal(row, &object)
			candidate := object
			if !artistAlbumsPage {
				for _, field := range []string{"track", "album"} {
					if child, ok := object[field]; ok {
						_ = json.Unmarshal(child, &candidate)
						break
					}
				}
			}
			kind, id, available, err := captureRESTIdentity(candidate)
			if err != nil {
				return nil, err
			}
			if artistAlbumsPage && available && kind != "album" {
				return nil, ErrSchema
			}
			unavailable := !available
			if unavailable {
				kind, id = captureRESTUnavailableType(candidate), ""
				if artistAlbumsPage {
					kind = "album"
				}
			}
			relationPayload := row
			if string(row) == "null" {
				relationPayload = json.RawMessage(`{}`)
			}
			payload, err := metadata.Sanitize(relationPayload, metadata.ScalarLimit)
			if err != nil {
				return nil, err
			}
			entity.Relations = append(entity.Relations, CapturedRelation{Kind: relationKind, Position: page.Offset + i, ChildType: kind, ChildID: id, Unavailable: unavailable, Metadata: payload})
		}
		entities = append(entities, entity)
	}
	var walk func(json.RawMessage, bool) error
	walk = func(value json.RawMessage, root bool) error {
		var object map[string]json.RawMessage
		if json.Unmarshal(value, &object) != nil || object == nil {
			var rows []json.RawMessage
			if json.Unmarshal(value, &rows) == nil {
				for _, row := range rows {
					if err := walk(row, false); err != nil {
						return err
					}
				}
			}
			return nil
		}
		kind, id, valid, err := captureRESTIdentity(object)
		if err != nil {
			return err
		}
		if valid {
			key := resource + ":related"
			if root {
				key = resource
			}
			var relations []CapturedRelation
			add := func(name string, rows json.RawMessage, base int) error {
				var items []json.RawMessage
				if json.Unmarshal(rows, &items) != nil {
					return nil
				}
				for i, row := range items {
					var child map[string]json.RawMessage
					_ = json.Unmarshal(row, &child)
					if track, ok := child["track"]; ok {
						_ = json.Unmarshal(track, &child)
					}
					typ, childID, available, err := captureRESTIdentity(child)
					if err != nil {
						return err
					}
					unavailable := !available
					if unavailable {
						typ, childID = captureRESTUnavailableType(child), ""
					}
					relationPayload := row
					if string(row) == "null" {
						relationPayload = json.RawMessage(`{}`)
					}
					payload, err := metadata.Sanitize(relationPayload, metadata.ScalarLimit)
					if err != nil {
						return err
					}
					relations = append(relations, CapturedRelation{Kind: name, Position: base + i, ChildType: typ, ChildID: childID, Unavailable: unavailable, Metadata: payload})
				}
				return nil
			}
			if err := add("artists", object["artists"], 0); err != nil {
				return err
			}
			if kind == "track" {
				if albumRaw, exists := object["album"]; exists {
					var album map[string]json.RawMessage
					_ = json.Unmarshal(albumRaw, &album)
					albumID := captureString(album["id"])
					albumURI := captureString(album["uri"])
					if albumURI != "" && objectID.MatchString(albumID) && albumURI != "spotify:album:"+albumID {
						return ErrSchema
					}
					albumType, albumID, available, err := captureRESTIdentity(album)
					if err != nil {
						return err
					}
					unavailable := !available || albumType != "album"
					relation := CapturedRelation{Kind: "album", Position: 0, ChildType: albumType, ChildID: albumID, Unavailable: unavailable}
					if unavailable {
						relation.ChildType = captureRESTUnavailableType(album)
						relation.ChildID = ""
					}
					relationPayload := albumRaw
					if string(albumRaw) == "null" {
						relationPayload = json.RawMessage("{}")
					}
					relation.Metadata, err = metadata.Sanitize(relationPayload, metadata.ScalarLimit)
					if err != nil {
						return err
					}
					relations = append(relations, relation)
				}
			}
			var page map[string]json.RawMessage
			if json.Unmarshal(object["tracks"], &page) == nil {
				base := offset
				_ = json.Unmarshal(page["offset"], &base)
				if base < 0 {
					return ErrSchema
				}
				if err := add("tracks", page["items"], base); err != nil {
					return err
				}
			}
			entities = append(entities, CapturedEntity{EntityType: kind, ID: id, Resource: key, Payload: append([]byte(nil), value...), Relations: relations})
		}
		for _, child := range object {
			if err := walk(child, false); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(safe, true); err != nil {
		return nil, err
	}

	return DeduplicateCapturedEntities(entities), nil

}

func captureRESTIdentity(object map[string]json.RawMessage) (kind, id string, available bool, err error) {
	kind, id = captureString(object["type"]), captureString(object["id"])
	uri := captureString(object["uri"])
	uriParts := strings.Split(uri, ":")
	uriKind, uriID := "", ""
	if len(uriParts) == 3 && uriParts[0] == "spotify" && objectID.MatchString(uriParts[2]) {
		switch uriParts[1] {
		case "track", "album", "artist", "playlist":
			uriKind, uriID = uriParts[1], uriParts[2]
		}
	}
	if uri != "" && objectID.MatchString(id) && kind != "" && uri != "spotify:"+kind+":"+id {
		return "", "", false, ErrSchema
	}
	if uriKind != "" {
		if kind != "" && kind != uriKind || id != "" && id != uriID {
			return "", "", false, ErrSchema
		}
		if kind == "" {
			kind = uriKind
		}
		if id == "" {
			id = uriID
		}
	}
	available = objectID.MatchString(id) && (kind == "track" || kind == "album" || kind == "artist" || kind == "playlist")
	if !available {
		return "", "", false, nil
	}
	return kind, id, true, nil
}

func captureRESTUnavailableType(object map[string]json.RawMessage) string {
	kind := captureString(object["type"])
	if kind == "" {
		parts := strings.Split(captureString(object["uri"]), ":")
		if len(parts) == 3 && parts[0] == "spotify" && objectID.MatchString(parts[2]) {
			kind = parts[1]
		}
	}
	if kind == "" || len(kind) > 128 {
		return ""
	}
	for _, character := range kind {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '_' && character != '-' {
			return ""
		}
	}
	return kind
}
