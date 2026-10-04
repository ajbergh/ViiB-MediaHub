//go:build spotify_research

package main

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var bareTrackID = regexp.MustCompile("^[A-Za-z0-9]{22}$")

func parseTrackInput(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "spotify:track:") {
		value = strings.TrimPrefix(value, "spotify:track:")
	}
	if bareTrackID.MatchString(value) {
		return value, nil
	}
	link, err := url.Parse(value)
	if err == nil && link.Scheme == "https" && link.Host == "open.spotify.com" && link.User == nil {
		parts := strings.Split(strings.Trim(link.Path, "/"), "/")
		if len(parts) == 3 && strings.HasPrefix(parts[0], "intl-") {
			parts = parts[1:]
		}
		if len(parts) == 2 && parts[0] == "track" && bareTrackID.MatchString(parts[1]) {
			return parts[1], nil
		}
	}
	return "", errors.New("provide a Spotify song link, track URI, or 22-character track ID")
}
