//go:build spotify_research

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// Only fixed structural counters escape; no account scalar values or arbitrary keys.
type playlistShapeTransport struct {
	base   http.RoundTripper
	report *map[string]int
}

func (t playlistShapeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err != nil || response == nil || r.URL.Host != "api-partner.spotify.com" {
		return response, err
	}
	*t.report = map[string]int{"httpStatus": response.StatusCode}
	if response.StatusCode != 200 {
		return response, nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil {
		return response, err
	}
	var root map[string]any
	if len(raw) <= 1<<20 && json.Unmarshal(raw, &root) == nil {
		data, _ := root["data"].(map[string]any)
		if playlist, ok := data["playlistV2"].(map[string]any); ok {
			*t.report = playlistShape(playlist)
		}
	}
	return response, nil
}
func playlistShape(playlist map[string]any) map[string]int {
	counts := map[string]int{"httpStatus": 200}
	if name, _ := playlist["name"].(string); name == "" {
		counts["missingPlaylistName"]++
	}
	owner, _ := playlist["ownerV2"].(map[string]any)
	ownerData, _ := owner["data"].(map[string]any)
	if uri, _ := ownerData["uri"].(string); uri != "" && !strings.HasPrefix(uri, "spotify:user:") {
		counts["invalidOwnerURI"]++
	}
	content, _ := playlist["content"].(map[string]any)
	rows, _ := content["items"].([]any)
	counts["rows"] = len(rows)
	if total, ok := content["totalCount"].(float64); ok {
		counts["total"] = int(total)
	}
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			counts["nullOrInvalidRows"]++
			continue
		}
		item, ok := row["itemV2"].(map[string]any)
		if !ok {
			counts["missingItem"]++
			continue
		}
		data, ok := item["data"].(map[string]any)
		if !ok {
			counts["missingData"]++
			continue
		}
		kind, _ := data["__typename"].(string)
		switch kind {
		case "Track", "LocalTrack", "Episode", "NotFound", "RestrictedContent":
			counts[kind]++
		default:
			counts["otherType"]++
		}
		if _, ok := data["duration"].(map[string]any); ok {
			counts["duration"]++
		}
		if _, ok := data["trackDuration"].(map[string]any); ok {
			counts["trackDuration"]++
		}
		if kind == "Track" {
			if name, _ := data["name"].(string); name == "" {
				counts["missingTrackName"]++
			}
			artistList, _ := data["artists"].(map[string]any)
			artistRows, _ := artistList["items"].([]any)
			for _, rawArtist := range artistRows {
				artist, _ := rawArtist.(map[string]any)
				profile, _ := artist["profile"].(map[string]any)
				if name, _ := profile["name"].(string); name == "" {
					counts["missingArtistName"]++
				}
				if uri, _ := artist["uri"].(string); !strings.HasPrefix(uri, "spotify:artist:") || len(strings.TrimPrefix(uri, "spotify:artist:")) != 22 {
					counts["invalidArtistURI"]++
				}
			}
		}
		if kind != "Track" {
			continue
		}
		album, ok := data["albumOfTrack"].(map[string]any)
		if !ok {
			counts["missingAlbum"]++
			continue
		}
		if uri, _ := album["uri"].(string); uri == "" {
			counts["missingAlbumURI"]++
		}
		if name, _ := album["name"].(string); name == "" {
			counts["missingAlbumName"]++
		}
		artists, _ := data["artists"].(map[string]any)
		names, _ := artists["items"].([]any)
		if len(names) == 0 {
			counts["missingArtists"]++
		}
	}
	return counts
}
