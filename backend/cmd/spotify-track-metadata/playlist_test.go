//go:build spotify_research

package main

import (
	"encoding/json"
	"testing"
)

func TestPlaylistNativeMetadata(t *testing.T) {
	const id = "5RIh3Tsq9U7CAvivB2iB7q"
	raw := []byte(`{"data":{"playlistV2":{"content":{"totalCount":2,"items":[{"itemV2":{"data":{"uri":"spotify:track:other","name":"Same title","playlistAudioAttributes":{"bpm":137.98}}}},{"itemV2":{"data":{"uri":"spotify:track:5RIh3Tsq9U7CAvivB2iB7q","name":"The First Snowfall","playlistAudioAttributes":{"bpm":70.4827,"key":{"key":"D#","mode":"MODE_MAJOR","camelotKey":{"value":"5B"}}}}}}]}}}}`)
	track, total, err := findPlaylistTrack(raw, id)
	if err != nil || track == nil || total != 2 {
		t.Fatalf("track=%v total=%d err=%v", track, total, err)
	}
	var attributes struct {
		BPM float64 `json:"bpm"`
		Key struct {
			Camelot struct {
				Value string `json:"value"`
			} `json:"camelotKey"`
		} `json:"key"`
	}
	if err := json.Unmarshal(track.Attributes, &attributes); err != nil {
		t.Fatal(err)
	}
	if attributes.BPM != 70.4827 || attributes.Key.Camelot.Value != "5B" {
		t.Fatalf("lost native values: %s", track.Attributes)
	}
	missing, _, err := findPlaylistTrack(raw, "1OwGYy9S2R0lzhoGn7vv6S")
	if err != nil || missing != nil {
		t.Fatalf("wrong identity matched: %v %v", missing, err)
	}
	for _, invalid := range []string{`{}`, `{"errors":[{"message":"failure"}]}`, `not-json`} {
		if _, _, err := findPlaylistTrack([]byte(invalid), id); err == nil {
			t.Fatal("accepted invalid response")
		}
	}
}
