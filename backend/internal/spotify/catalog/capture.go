package catalog

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/spotify/metadata"
)

type CapturedRelation struct {
	Kind        string
	Position    int
	ChildType   string
	ChildID     string
	Unavailable bool
	Metadata    []byte
}
type CapturedEntity struct {
	EntityType string
	ID         string
	Resource   string
	Payload    []byte
	Relations  []CapturedRelation
}

// CaptureDomain extracts domain objects only; credentials, profile data and
// GraphQL errors/envelopes never reach the persistence callback.
func CaptureDomain(stage string, request any, raw []byte) ([]CapturedEntity, error) {
	if stage == "client_token" || stage == "profile" {
		return nil, nil
	}
	var envelope struct {
		Errors []json.RawMessage          `json:"errors"`
		Data   map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Errors) > 0 {
		return nil, ErrSchema
	}
	encoded, _ := json.Marshal(request)
	var query struct {
		Operation string `json:"operationName"`
		Variables struct {
			URI     string   `json:"uri"`
			Offset  int      `json:"offset"`
			Limit   int      `json:"limit"`
			Filters []string `json:"filters"`
		} `json:"variables"`
	}
	if json.Unmarshal(encoded, &query) != nil {
		return nil, ErrSchema
	}
	resource := query.Operation + ":page:" + strconv.Itoa(query.Variables.Offset) + ":" + strconv.Itoa(query.Variables.Limit)
	var root json.RawMessage
	switch stage {
	case "track":
		root = envelope.Data["trackUnion"]
	case "album":
		root = envelope.Data["albumUnion"]
	case "artist", "artist_albums":
		root = envelope.Data["artistUnion"]
	case "playlist":
		root = envelope.Data["playlistV2"]
	case "search":
		root = envelope.Data["searchV2"]
	case "library":
		var me map[string]json.RawMessage
		_ = json.Unmarshal(envelope.Data["me"], &me)
		root = me["libraryV3"]
	default:
		return nil, nil
	}
	if len(root) == 0 {
		return nil, ErrSchema
	}
	var objects []CapturedEntity
	if stage == "library" {
		safe, err := metadata.Sanitize(root, metadata.CatalogLimit)
		if err != nil {
			return nil, err
		}
		// A library scope is an account-local resource name, never a Spotify entity ID.
		scope := "libraryV3"
		if len(query.Variables.Filters) > 0 {
			scope += "_" + strings.Join(query.Variables.Filters, "_")
		}
		relations, err := captureRelations(root, query.Variables.Offset)
		if err != nil {
			return nil, err
		}
		objects = append(objects, CapturedEntity{EntityType: "library", ID: scope, Resource: resource, Payload: safe, Relations: relations})
	}
	var walk func(json.RawMessage, string, string) error
	walk = func(value json.RawMessage, path, fallbackURI string) error {
		var object map[string]json.RawMessage
		if json.Unmarshal(value, &object) != nil || object == nil {
			var array []json.RawMessage
			if json.Unmarshal(value, &array) == nil {
				for i, child := range array {
					if err := walk(child, fmt.Sprintf("%s.%d", path, i), ""); err != nil {
						return err
					}
				}
			}
			return nil
		}
		kind, id, _ := captureIdentity(object, fallbackURI)
		name := captureString(object["__typename"])
		if kind != "" && id != "" && !unavailableType(name) && name != "LocalTrack" && name != "Episode" {
			if path == "root" && query.Variables.URI != "" && query.Variables.URI != "spotify:"+kind+":"+id {
				return ErrSchema
			}
			safe, err := metadata.Sanitize(value, metadata.CatalogLimit)
			if err != nil {
				return err
			}
			// Nested projections have their own resource key and cannot replace the full entity.
			key := resource
			if path != "root" {
				key = query.Operation + ":related"
			}
			relations, err := captureRelations(value, query.Variables.Offset)
			if err != nil {
				return err
			}
			objects = append(objects, CapturedEntity{EntityType: kind, ID: id, Resource: key, Payload: safe, Relations: relations})
		}
		for name, child := range object {
			wrapperURI := ""
			if name == "data" {
				wrapperURI = captureString(object["uri"])
				if wrapperURI == "" {
					wrapperURI = captureString(object["_uri"])
				}
			}
			if err := walk(child, path+"."+name, wrapperURI); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, "root", query.Variables.URI); err != nil {
		return nil, err
	}
	dedup := map[string]int{}
	unique := []CapturedEntity{}
	for _, entity := range objects {
		key := entity.EntityType + ":" + entity.ID + ":" + entity.Resource
		if index, ok := dedup[key]; ok {
			if len(entity.Payload) > len(unique[index].Payload) {
				unique[index] = entity
			}
		} else {
			dedup[key] = len(unique)
			unique = append(unique, entity)
		}
	}
	return unique, nil
}
func captureString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}
func captureIdentity(object map[string]json.RawMessage, fallback string) (string, string, bool) {
	name := captureString(object["__typename"])
	unavailable := unavailableType(name) || name == "LocalTrack" || name == "Episode"
	uri := captureString(object["uri"])
	if uri == "" {
		uri = captureString(object["_uri"])
	}
	if uri == "" {
		uri = fallback
	}
	if uri == "" && objectID.MatchString(captureString(object["id"])) {
		switch strings.ToLower(name) {
		case "track", "album", "artist", "playlist":
			uri = "spotify:" + strings.ToLower(name) + ":" + captureString(object["id"])
		}
	}
	parts := strings.Split(uri, ":")
	if len(parts) != 3 || parts[0] != "spotify" || !objectID.MatchString(parts[2]) {
		return "", "", unavailable
	}
	switch parts[1] {
	case "track", "album", "artist", "playlist":
	default:
		return "", "", unavailable
	}
	if name != "" && !unavailable && strings.ToLower(name) != parts[1] {
		return "", "", true
	}
	id := captureString(object["id"])
	if id != "" && id != parts[2] {
		return "", "", true
	}
	var playable struct {
		Playable *bool `json:"playable"`
	}
	_ = json.Unmarshal(object["playability"], &playable)
	if playable.Playable != nil && !*playable.Playable {
		unavailable = true
	}
	return parts[1], parts[2], unavailable
}

func captureRelations(raw json.RawMessage, offset int) ([]CapturedRelation, error) {
	var captureErr error
	var root map[string]json.RawMessage
	_ = json.Unmarshal(raw, &root)
	var relations []CapturedRelation
	add := func(kind string, rows json.RawMessage, positionOffset int) {
		var items []json.RawMessage
		if json.Unmarshal(rows, &items) != nil {
			return
		}
		for i, row := range items {
			var object map[string]json.RawMessage
			_ = json.Unmarshal(row, &object)
			candidate := row
			fallback := ""
			for _, name := range []string{"track", "itemV2", "item", "data"} {
				if child, ok := object[name]; ok {
					candidate = child
					var wrapper map[string]json.RawMessage
					_ = json.Unmarshal(candidate, &wrapper)
					fallback = captureString(wrapper["uri"])
					if fallback == "" {
						fallback = captureString(wrapper["_uri"])
					}
					if data, ok := wrapper["data"]; ok {
						candidate = data
					}
					break
				}
			}
			var child map[string]json.RawMessage
			_ = json.Unmarshal(candidate, &child)
			typ, id, unavailable := captureIdentity(child, fallback)
			if typ == "" {
				typ = captureString(child["__typename"])
			}
			safe, err := metadata.Sanitize(row, metadata.ScalarLimit)
			if err != nil {
				captureErr = err
				return
			}
			relations = append(relations, CapturedRelation{Kind: kind, Position: positionOffset + i, ChildType: typ, ChildID: id, Unavailable: unavailable || id == "", Metadata: safe})
		}
	}
	page := func(name, kind string, positionOffset int) {
		var object map[string]json.RawMessage
		_ = json.Unmarshal(root[name], &object)
		add(kind, object["items"], positionOffset)
	}
	page("artists", "artists", 0)
	page("firstArtist", "first_artists", 0)
	page("otherArtists", "other_artists", 0)
	page("tracksV2", "album_tracks", offset)
	page("content", "playlist_items", offset)
	add("library_items", root["items"], offset)
	var discography map[string]json.RawMessage
	_ = json.Unmarshal(root["discography"], &discography)
	var top map[string]json.RawMessage
	_ = json.Unmarshal(discography["topTracks"], &top)
	add("artist_top_tracks", top["items"], 0)
	var all map[string]json.RawMessage
	_ = json.Unmarshal(discography["all"], &all)
	var groups []json.RawMessage
	_ = json.Unmarshal(all["items"], &groups)
	for i, group := range groups {
		var object map[string]json.RawMessage
		_ = json.Unmarshal(group, &object)
		var releases map[string]json.RawMessage
		_ = json.Unmarshal(object["releases"], &releases)
		add(fmt.Sprintf("artist_releases_group_%d", i), releases["items"], 0)
	}
	if album := root["albumOfTrack"]; len(album) > 0 {
		var object map[string]json.RawMessage
		_ = json.Unmarshal(album, &object)
		kind, id, unavailable := captureIdentity(object, "")
		if id != "" {
			safe, err := metadata.Sanitize(album, metadata.ScalarLimit)
			if err != nil {
				return nil, err
			}
			relations = append(relations, CapturedRelation{Kind: "album", ChildType: kind, ChildID: id, Unavailable: unavailable, Metadata: safe})
		}
	}
	return relations, captureErr
}
