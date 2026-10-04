//go:build spotify_research

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

var webAPIObjectID = regexp.MustCompile("^[A-Za-z0-9]{22}$")
var webAPIMarket = regexp.MustCompile("^[A-Z]{2}$")

type webAPIResult struct {
	Route             string   `json:"route"`
	HTTPStatus        int      `json:"httpStatus,omitempty"`
	Code              string   `json:"code"`
	ShapeCompatible   *bool    `json:"shapeCompatible"`
	SampleAvailable   bool     `json:"sampleAvailable"`
	MissingFields     []string `json:"missingFields,omitempty"`
	RetryAfterSeconds float64  `json:"retryAfterSeconds,omitempty"`
}
type webAPIReport struct {
	Surface                string         `json:"surface"`
	RequestCount           int            `json:"requestCount"`
	ReconnectVerified      bool           `json:"reconnectVerified"`
	AuthenticationVerified bool           `json:"authenticationVerified"`
	RenewalVerified        bool           `json:"renewalVerified"`
	CoverageState          string         `json:"coverageState"`
	StoppedEarly           bool           `json:"stoppedEarly"`
	Results                []webAPIResult `json:"results"`
}
type webAPILeads struct{ album, artist, playlist, market string }
type webAPIShape struct {
	missing           []string
	sample            bool
	leads             webAPILeads
	marketUnavailable bool
}

func validateWebAPIOptions(enabled bool, audioBytes int, features, compare bool) error {
	if enabled && (audioBytes != 0 || features || compare) {
		return errWebAPIModes
	}
	return nil
}

var errWebAPIModes = &probeFailure{stage: "configuration", cause: auth.ErrDisabled}

// probeWebAPI uses only known GET paths, small pages and validated internal IDs.
// Upstream data is held in memory only; diagnostics never include field values.
func probeWebAPI(ctx context.Context, token auth.Token, id string, renewed bool, client *http.Client, out io.Writer) error {
	return probeWebAPISurface(ctx, token, id, renewed, "all", false, client, out)
}

func validWebAPISurface(surface string) bool {
	switch surface {
	case "all", "profile", "track", "search", "library", "catalog", "recent":
		return true
	}
	return false
}

func probeWebAPISurface(ctx context.Context, token auth.Token, id string, renewed bool, surface string, reconnected bool, client *http.Client, out io.Writer) error {
	if !validWebAPISurface(surface) {
		return errWebAPIModes
	}
	if token.Kind != auth.WebPlayer || token.Bearer() == "" || !webAPIObjectID.MatchString(id) {
		return auth.ErrAuthenticationRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	safeClient := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		safeClient = *client
		if safeClient.Timeout == 0 || safeClient.Timeout > 10*time.Second {
			safeClient.Timeout = 10 * time.Second
		}
	}
	safeClient.Jar = nil
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	report := webAPIReport{Surface: surface, ReconnectVerified: reconnected, AuthenticationVerified: true, RenewalVerified: renewed, Results: make([]webAPIResult, 0, 11)}
	leads := webAPILeads{}
	request := func(name, path string) bool {
		report.RequestCount++
		result, found := webAPIRequest(ctx, &safeClient, token, name, path, id)
		if found.album != "" {
			leads.album = found.album
		}
		if found.artist != "" {
			leads.artist = found.artist
		}
		if found.playlist != "" {
			leads.playlist = found.playlist
		}
		if found.market != "" {
			leads.market = found.market
		}
		report.Results = append(report.Results, result)
		if ctx.Err() != nil || result.HTTPStatus == 429 || result.HTTPStatus == 401 {
			report.StoppedEarly = true
			return false
		}
		return true
	}
	for _, route := range []struct{ name, path string }{
		{"profile", "me"},
		{"track", "tracks/" + id},
		{"search", "search?" + url.Values{"q": {"music"}, "type": {"track,album,artist,playlist"}, "limit": {"1"}, "offset": {"0"}}.Encode()},
		{"saved_albums", "me/albums?limit=1&offset=0"},
		{"saved_playlists", "me/playlists?limit=1&offset=0"},
		{"recently_played", "me/player/recently-played?limit=1"},
	} {
		if !webAPIRouteSelected(surface, route.name) {
			continue
		}
		if !request(route.name, route.path) {
			return writeWebAPIReport(out, report)
		}
	}
	dependent := []struct {
		name, path string
		available  bool
	}{
		{"album", "albums/" + leads.album, leads.album != ""},
		{"artist", "artists/" + leads.artist, leads.artist != ""},
		{"artist_top_tracks", "artists/" + leads.artist + "/top-tracks?market=" + leads.market, leads.artist != "" && leads.market != ""},
		{"playlist", "playlists/" + leads.playlist + "?" + url.Values{"fields": {"id,name,description,images,owner(id,display_name),external_urls,tracks(total)"}}.Encode(), leads.playlist != ""},
		{"playlist_tracks", "playlists/" + leads.playlist + "/tracks?limit=1&offset=0", leads.playlist != ""},
	}
	for _, route := range dependent {
		if !webAPIRouteSelected(surface, route.name) {
			continue
		}
		if !route.available {
			report.Results = append(report.Results, webAPIResult{Route: route.name, Code: "skipped_dependency_unavailable"})
			continue
		}
		if !request(route.name, route.path) {
			break
		}
	}
	return writeWebAPIReport(out, report)
}

func webAPIRouteSelected(surface, route string) bool {
	switch surface {
	case "all":
		return true
	case "profile", "track", "search":
		return route == surface
	case "recent":
		return route == "recently_played"
	case "library":
		return route == "saved_albums" || route == "saved_playlists" || route == "recently_played" || route == "playlist" || route == "playlist_tracks"
	case "catalog":
		return route == "track" || route == "album" || route == "artist"
	}
	return false
}

// Reconnect proves provider lifecycle and token acquisition; HTTP compatibility is reported separately.
func runWebAPIResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, id, surface string, renew, reconnect bool, opts auth.WebPlayerOptions, client *http.Client, out io.Writer) error {
	if !validWebAPISurface(surface) {
		return errWebAPIModes
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	provider, err := auth.NewWebPlayerProvider(cookie, contract, opts)
	if err != nil {
		return err
	}
	defer func() {
		if provider != nil {
			provider.Disconnect()
		}
	}()
	token, err := provider.Token(ctx)
	if err != nil {
		return &probeFailure{stage: "authentication", cause: err}
	}
	if reconnect {
		provider.Disconnect()
		provider, err = auth.NewWebPlayerProvider(cookie, contract, opts)
		if err != nil {
			return err
		}
		token, err = provider.Token(ctx)
		if err != nil {
			return &probeFailure{stage: "session_reconnect", authenticated: true, cause: err}
		}
	}
	cookie = ""
	if renew {
		token, err = provider.Refresh(ctx, token)
		if err != nil {
			return &probeFailure{stage: "token_renewal", authenticated: true, cause: err}
		}
	}
	return probeWebAPISurface(ctx, token, id, renew, surface, reconnect, client, out)
}

func webAPIRequest(ctx context.Context, client *http.Client, token auth.Token, name, path, id string) (webAPIResult, webAPILeads) {
	result := webAPIResult{Route: name}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.spotify.com/v1/"+path, nil)
	if err != nil {
		result.Code = "request_failed"
		return result, webAPILeads{}
	}
	request.Header.Set("Authorization", "Bearer "+token.Bearer())
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		result.Code = "temporarily_unavailable"
		return result, webAPILeads{}
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case 401:
			result.Code = "token_rejected"
		case 403:
			result.Code = "access_or_scope_denied"
		case 404:
			result.Code = "route_or_object_unavailable"
		case 429:
			result.Code = "rate_limited"
			result.RetryAfterSeconds = auth.RetryAfter(response.Header.Get("Retry-After"), time.Now()).Seconds()
		default:
			if response.StatusCode >= 500 {
				result.Code = "temporarily_unavailable"
			} else {
				result.Code = "unexpected_http_status"
			}
		}
		return result, webAPILeads{}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	defer clear(data)
	if err != nil {
		result.Code = "read_failed"
		return result, webAPILeads{}
	}
	if len(data) > 1<<20 {
		result.Code = "payload_too_large"
		return result, webAPILeads{}
	}
	var object map[string]any
	if json.Unmarshal(data, &object) != nil || object == nil {
		result.Code = "schema_incompatible"
		result.MissingFields = []string{"object"}
		return result, webAPILeads{}
	}
	expectedID := id
	if name == "album" || name == "artist" || name == "playlist" {
		parts := strings.Split(strings.SplitN(path, "?", 2)[0], "/")
		if len(parts) == 2 {
			expectedID = parts[1]
		}
	}
	shape := checkWebAPIShape(name, object, expectedID)
	result.MissingFields = shape.missing
	result.SampleAvailable = shape.sample
	compatible := len(shape.missing) == 0
	result.ShapeCompatible = &compatible
	result.Code = "compatible"
	if !compatible {
		result.Code = "schema_incompatible"
		return result, webAPILeads{}
	}
	if !shape.sample {
		result.Code = "compatible_empty"
	}
	if shape.marketUnavailable {
		result.Code = "market_unavailable"
	}
	return result, shape.leads
}
func objectAt(m map[string]any, key string) map[string]any {
	value, _ := m[key].(map[string]any)
	return value
}
func stringAt(m map[string]any, key string) string { value, _ := m[key].(string); return value }
func (s *webAPIShape) require(ok bool, name string) {
	if !ok {
		s.missing = append(s.missing, name)
	}
}
func validID(m map[string]any) bool { return webAPIObjectID.MatchString(stringAt(m, "id")) }
func basicNamedObject(m map[string]any) bool {
	return m != nil && validID(m) && stringAt(m, "name") != ""
}
func trackShape(m map[string]any, album bool) bool {
	if !basicNamedObject(m) {
		return false
	}
	duration, ok := m["duration_ms"].(float64)
	if !ok || duration <= 0 {
		return false
	}
	artists, ok := m["artists"].([]any)
	if !ok || len(artists) == 0 {
		return false
	}
	artist, _ := artists[0].(map[string]any)
	if stringAt(artist, "name") == "" {
		return false
	}
	return !album || stringAt(objectAt(m, "album"), "name") != ""
}
func pageShape(s *webAPIShape, m map[string]any, itemKind string) {
	items, ok := m["items"].([]any)
	s.require(ok, "items")
	if !ok {
		return
	}
	_, totalOK := m["total"].(float64)
	// recently-played uses cursors instead of total.
	if itemKind != "recently_played" {
		s.require(totalOK, "total")
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		object, ok := item.(map[string]any)
		if !ok {
			s.require(false, "item")
			continue
		}
		valid := false
		switch itemKind {
		case "saved_albums":
			valid = basicNamedObject(objectAt(object, "album"))
		case "saved_playlists":
			valid = basicNamedObject(object)
			if valid && s.leads.playlist == "" {
				s.leads.playlist = stringAt(object, "id")
			}
		case "playlist_tracks", "recently_played":
			track := objectAt(object, "track")
			if track == nil {
				continue
			}
			if local, _ := track["is_local"].(bool); local {
				continue
			}
			valid = trackShape(track, true)
		case "album_tracks":
			valid = trackShape(object, false)
		case "search_tracks":
			valid = trackShape(object, true)
		default:
			valid = basicNamedObject(object)
		}
		s.require(valid, "item."+itemKind)
		if valid {
			s.sample = true
		}
	}
}
func checkWebAPIShape(name string, m map[string]any, id string) webAPIShape {
	s := webAPIShape{}
	switch name {
	case "profile":
		s.require(stringAt(m, "id") != "", "id")
		_, displayOK := m["display_name"].(string)
		if value, present := m["display_name"]; present && value == nil {
			displayOK = true
		}
		s.require(displayOK, "display_name")
		s.require(stringAt(m, "product") != "", "product")
		_, emailOK := m["email"].(string)
		s.require(emailOK, "email")
		s.require(webAPIMarket.MatchString(stringAt(m, "country")), "country")
		_, followersOK := objectAt(m, "followers")["total"].(float64)
		s.require(followersOK, "followers.total")
		s.require(stringAt(objectAt(m, "external_urls"), "spotify") != "", "external_urls.spotify")
		_, imagesOK := m["images"].([]any)
		s.require(imagesOK, "images")
		s.sample = true
		country := stringAt(m, "country")
		if webAPIMarket.MatchString(country) {
			s.leads.market = country
		}
	case "track":
		s.require(trackShape(m, true), "track_fields")
		s.require(stringAt(m, "id") == id, "recording_identity")
		s.sample = true
		album := objectAt(m, "album")
		if validID(album) {
			s.leads.album = stringAt(album, "id")
		}
		artists, _ := m["artists"].([]any)
		if len(artists) > 0 {
			artist, _ := artists[0].(map[string]any)
			if validID(artist) {
				s.leads.artist = stringAt(artist, "id")
			}
		}
		if playable, ok := m["is_playable"].(bool); ok && !playable {
			s.marketUnavailable = true
		}
	case "search":
		for _, bucket := range []string{"tracks", "albums", "artists", "playlists"} {
			group := objectAt(m, bucket)
			s.require(group != nil, bucket)
			if group == nil {
				continue
			}
			sub := webAPIShape{}
			pageShape(&sub, group, "search_"+bucket)
			for _, field := range sub.missing {
				s.require(false, bucket+"."+field)
			}
			s.sample = s.sample || sub.sample
		}
	case "saved_albums", "saved_playlists", "recently_played", "playlist_tracks":
		pageShape(&s, m, name)
	case "album":
		s.require(basicNamedObject(m), "album_fields")
		s.require(stringAt(m, "id") == id, "object_identity")
		_, imagesOK := m["images"].([]any)
		s.require(imagesOK, "images")
		s.require(stringAt(m, "release_date") != "", "release_date")
		tracks := objectAt(m, "tracks")
		s.require(tracks != nil, "tracks")
		if tracks != nil {
			pageShape(&s, tracks, "album_tracks")
		}
	case "artist":
		s.require(basicNamedObject(m), "artist_fields")
		s.require(stringAt(m, "id") == id, "object_identity")
		_, imagesOK := m["images"].([]any)
		s.require(imagesOK, "images")
		s.sample = true
	case "artist_top_tracks":
		tracks, ok := m["tracks"].([]any)
		s.require(ok, "tracks")
		for _, item := range tracks {
			track, _ := item.(map[string]any)
			valid := trackShape(track, true)
			s.require(valid, "track_fields")
			s.sample = s.sample || valid
		}
	case "playlist":
		s.require(basicNamedObject(m), "playlist_fields")
		s.require(stringAt(m, "id") == id, "object_identity")
		s.require(stringAt(objectAt(m, "owner"), "id") != "", "owner.id")
		_, imagesOK := m["images"].([]any)
		s.require(imagesOK, "images")
		_, totalOK := objectAt(m, "tracks")["total"].(float64)
		s.require(totalOK, "tracks.total")
		s.sample = true
	default:
		s.require(false, "unsupported_shape")
	}
	return s
}

func writeWebAPIReport(out io.Writer, report webAPIReport) error {
	report.CoverageState = "compatible_sampled_routes"
	for _, result := range report.Results {
		switch result.Code {
		case "rate_limited":
			report.CoverageState = "inconclusive_rate_limited"
		case "token_rejected":
			report.CoverageState = "token_rejected"
		case "compatible", "compatible_empty":
		case "skipped_dependency_unavailable":
			if report.CoverageState == "compatible_sampled_routes" {
				report.CoverageState = "partial_coverage"
			}
		default:
			if report.CoverageState != "inconclusive_rate_limited" && report.CoverageState != "token_rejected" {
				report.CoverageState = "incomplete_or_incompatible"
			}
		}
	}
	if report.StoppedEarly && report.CoverageState == "compatible_sampled_routes" {
		report.CoverageState = "inconclusive_timeout"
	}
	return json.NewEncoder(out).Encode(report)
}
