package catalog

import (
	"encoding/json"
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
	if entityPage && !objectID.MatchString(parts[2]) {
		return nil, ErrSchema
	}
	if library || entityPage {
		var page struct {
			Offset int               `json:"offset"`
			Items  []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(safe, &page) != nil || page.Offset < 0 || page.Items == nil {
			return nil, ErrSchema
		}
		entity := CapturedEntity{EntityType: "library", ID: "rest_saved_" + parts[2], Resource: resource, Payload: safe}
		relationKind := "library_items"
		if entityPage {
			entity.EntityType = strings.TrimSuffix(parts[1], "s")
			entity.ID = parts[2]
			relationKind = "tracks"
		}
		for i, row := range page.Items {
			var object map[string]json.RawMessage
			_ = json.Unmarshal(row, &object)
			candidate := object
			for _, field := range []string{"track", "album"} {
				if child, ok := object[field]; ok {
					_ = json.Unmarshal(child, &candidate)
					break
				}
			}
			kind, id := captureString(candidate["type"]), captureString(candidate["id"])
			unavailable := !objectID.MatchString(id) || (kind != "track" && kind != "album" && kind != "playlist")
			if unavailable {
				kind, id = "", ""
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
		kind := captureString(object["type"])
		id := captureString(object["id"])
		valid := (kind == "track" || kind == "album" || kind == "artist" || kind == "playlist") && objectID.MatchString(id)
		uri := captureString(object["uri"])
		if valid && uri != "" && uri != "spotify:"+kind+":"+id {
			return ErrSchema
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
					typ, childID := captureString(child["type"]), captureString(child["id"])
					unavailable := !objectID.MatchString(childID) || (typ != "track" && typ != "album" && typ != "artist" && typ != "playlist")
					if unavailable {
						typ, childID = "", ""
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
					albumType := captureString(album["type"])
					albumURI := captureString(album["uri"])
					if albumURI != "" && objectID.MatchString(albumID) && albumURI != "spotify:album:"+albumID {
						return ErrSchema
					}
					unavailable := albumType != "album" || !objectID.MatchString(albumID)
					relation := CapturedRelation{Kind: "album", Position: 0, ChildType: "album", ChildID: albumID, Unavailable: unavailable}
					if unavailable {
						relation.ChildType = ""
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
	// Duplicate nested projections cannot let a sparse object erase a richer one.
	unique := []CapturedEntity{}
	seen := map[string]int{}
	for _, entity := range entities {
		key := entity.EntityType + ":" + entity.ID + ":" + entity.Resource
		if index, ok := seen[key]; ok {
			if len(entity.Payload) > len(unique[index].Payload) {
				unique[index] = entity
			}
		} else {
			seen[key] = len(unique)
			unique = append(unique, entity)
		}
	}
	return unique, nil
}
