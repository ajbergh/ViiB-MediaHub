//go:build spotify_research

// Observes bounded library response field types without recording raw field values.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// Retain only fixed structural counters from the last operation. No values,
// account identities, arbitrary provider keys or credential headers escape.
type libraryShapeTransport struct {
	base   http.RoundTripper
	report *map[string]int
}

func (t libraryShapeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err != nil || response == nil || r.URL.Host != "api-partner.spotify.com" || response.StatusCode != 200 {
		return response, err
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil {
		return response, err
	}
	var root map[string]any
	if len(raw) > 1<<20 || json.Unmarshal(raw, &root) != nil {
		return response, nil
	}
	data, _ := root["data"].(map[string]any)
	me, _ := data["me"].(map[string]any)
	library, ok := me["libraryV3"].(map[string]any)
	if !ok {
		return response, nil
	}
	counts := map[string]int{"libraryOperation": 1}
	rows, _ := library["items"].([]any)
	counts["rows"] = len(rows)
	if total, ok := library["totalCount"].(float64); ok {
		counts["total"] = int(total)
	}
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		item, ok := row["item"].(map[string]any)
		if !ok {
			counts["missingItem"]++
			continue
		}
		value, ok := item["data"].(map[string]any)
		if !ok {
			counts["missingData"]++
			continue
		}
		kind, _ := value["__typename"].(string)
		switch kind {
		case "Playlist", "Album", "NotFound", "RestrictedContent", "PseudoPlaylist":
			counts[kind]++
		default:
			counts["otherType"]++
		}
		uri, _ := value["uri"].(string)
		wrapped, _ := item["_uri"].(string)
		other, _ := item["uri"].(string)
		if uri == "" && wrapped == "" && other == "" {
			counts["missingURI"]++
		}
		if wrapped != "" && other != "" && wrapped != other {
			counts["wrapperMismatch"]++
		}
		if uri != "" && ((wrapped != "" && wrapped != uri) || (other != "" && other != uri)) {
			counts["identityMismatch"]++
		}
	}
	*t.report = counts
	return response, nil
}
