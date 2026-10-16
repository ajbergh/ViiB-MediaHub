//go:build spotify_research

// Tests parsing Spotify track IDs, URLs, and URIs for diagnostic inputs.
package main

import "testing"

func TestTrackInputs(t *testing.T) {
	const id = "11dFghVXANMlKmJXsNCbNl"
	for _, value := range []string{id, " spotify:track:" + id + " ", "https://open.spotify.com/track/" + id + "?si=example", "https://open.spotify.com/intl-en/track/" + id} {
		got, err := parseTrackInput(value)
		if err != nil || got != id {
			t.Fatalf("valid input rejected: %v", err)
		}
	}
	for _, value := range []string{"https://example.com/track/" + id, "https://open.spotify.com/episode/" + id, "spotify:album:" + id, "https://open.spotify.com@evil.example/track/" + id, "https://open.spotify.com/track/" + id + "/extra", "bad"} {
		if _, err := parseTrackInput(value); err == nil {
			t.Fatal("invalid track input accepted")
		}
	}
}
