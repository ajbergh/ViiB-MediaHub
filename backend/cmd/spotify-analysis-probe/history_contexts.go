//go:build spotify_research

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

// Read-only protocol evidence: openclaw/spogo 243315d1e7c518e9abac35d4322ffcf3461d775d,
// connect_user.go. This is context history, not verified individual play events.
// No arbitrary URL/filter, dynamic hash discovery, retry or production fallback.
type historyContextPageReport struct {
	Offset           int `json:"offset"`
	Records          int `json:"records"`
	TrackContexts    int `json:"trackContexts"`
	OtherContexts    int `json:"otherContexts"`
	TrackReferences  int `json:"trackReferences"`
	Timestamps       int `json:"timestamps"`
	RepeatedContexts int `json:"repeatedContexts"`
	RepeatedTracks   int `json:"repeatedTracks"`
}
type historyContextWire struct {
	Contexts []struct {
		URI            string `json:"uri"`
		LastPlayedTime int64  `json:"lastPlayedTime"`
		TrackURI       string `json:"lastPlayedTrackUri"`
	} `json:"playContexts"`
}

func runHistoryContextResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer) error {
	return runHistoryResearch(ctx, cookie, contract, renew, opts, httpClient, out, false)
}

func runHistoryResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer, events bool) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	p, err := auth.NewWebPlayerProvider(cookie, contract, opts)
	cookie = ""
	if err != nil {
		return err
	}
	defer p.Disconnect()
	token, err := p.Token(ctx)
	if err != nil {
		return &probeFailure{stage: "authentication", cause: err}
	}
	if renew {
		token, err = p.Refresh(ctx, token)
		if err != nil {
			return &probeFailure{stage: "token_renewal", authenticated: true, cause: err}
		}
	}
	var profile catalog.Profile
	if !events {
		client := catalog.New(catalog.Options{Client: httpClient})
		defer client.Close()
		profile, err = client.Profile(ctx, token)
		if err != nil {
			return &probeFailure{stage: "history_profile", authenticated: true, renewed: renew, cause: err}
		}
	}
	safe := http.Client{Timeout: 10 * time.Second}
	if httpClient != nil {
		safe = *httpClient
		if safe.Timeout == 0 || safe.Timeout > 10*time.Second {
			safe.Timeout = 10 * time.Second
		}
	}
	safe.Jar = nil
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// This independent mint supplies the internal GET's client header, not a developer token.
	object, mint := pathfinderRequest(ctx, &safe, "client_token", "https://clienttoken.spotify.com/v1/clienttoken", map[string]any{"client_data": map[string]any{"client_version": contract.AppVersion, "client_id": token.WebPlayerClientID(), "js_sdk_data": map[string]string{"device_brand": "", "device_id": "", "device_model": "", "device_type": "", "os": "", "os_version": ""}}}, "", "", contract.AppVersion)
	clientToken := stringAt(objectAt(object, "granted_token"), "token")
	defer func() { clientToken = "" }()
	if mint.HTTPStatus != 200 || clientToken == "" {
		return &probeFailure{stage: "history_client_token", authenticated: true, renewed: renew, cause: auth.ErrTemporarilyUnavailable}
	}
	if events {
		report, err := probeHistoryEvents(ctx, &safe, token, clientToken, contract.AppVersion)
		if err != nil {
			return &probeFailure{stage: "history_events", authenticated: true, renewed: renew, cause: err}
		}
		return json.NewEncoder(out).Encode(map[string]any{
			"authenticationVerified": true, "renewalVerified": renew,
			"trackEventParityVerified": false, "diagnostic": report,
		})
	}
	pages, err := probeHistoryContexts(ctx, &safe, token, clientToken, contract.AppVersion, profile.ID)
	if err != nil {
		return &probeFailure{stage: "history_contexts", authenticated: true, renewed: renew, cause: err}
	}
	return json.NewEncoder(out).Encode(struct {
		Authenticated    bool                       `json:"authenticationVerified"`
		Renewed          bool                       `json:"renewalVerified"`
		TrackEventParity bool                       `json:"trackEventParityVerified"`
		Pages            []historyContextPageReport `json:"pages"`
	}{true, renew, false, pages})
}
func probeHistoryContexts(ctx context.Context, client *http.Client, token auth.Token, clientToken, version, username string) ([]historyContextPageReport, error) {
	if username == "" || len(username) > 256 || username == "." || username == ".." || strings.ContainsAny(username, "\r\n") {
		return nil, catalog.ErrSchema
	}
	pages := []historyContextPageReport{}
	seenContexts, seenTracks := map[string]bool{}, map[string]bool{}
	for _, offset := range []int{0, 50} {
		query := url.Values{"format": {"json"}, "limit": {"50"}, "offset": {strconv.Itoa(offset)}, "filter": {"default,track"}}
		request, err := http.NewRequestWithContext(ctx, "GET", "https://spclient.wg.spotify.com/recently-played/v3/user/"+url.PathEscape(username)+"/recently-played?"+query.Encode(), nil)
		if err != nil {
			return nil, catalog.ErrSchema
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Bearer "+token.Bearer())
		request.Header.Set("Client-Token", clientToken)
		request.Header.Set("App-Platform", "WebPlayer")
		request.Header.Set("Spotify-App-Version", version)
		request.Header.Set("Origin", "https://open.spotify.com")
		request.Header.Set("Referer", "https://open.spotify.com/")
		response, err := client.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, auth.ErrTemporarilyUnavailable
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			return nil, &catalog.HTTPError{Stage: "history_contexts", Status: response.StatusCode, RetryAfter: auth.RetryAfter(response.Header.Get("Retry-After"), time.Now())}
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		response.Body.Close()
		var wire historyContextWire
		invalid := err != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &wire) != nil || wire.Contexts == nil || len(wire.Contexts) > 50
		clear(raw)
		if invalid {
			return nil, catalog.ErrSchema
		}
		report := historyContextPageReport{Offset: offset, Records: len(wire.Contexts)}
		for _, item := range wire.Contexts {
			if strings.HasPrefix(item.URI, "spotify:track:") {
				report.TrackContexts++
			} else {
				report.OtherContexts++
			}
			if strings.HasPrefix(item.TrackURI, "spotify:track:") && webAPIObjectID.MatchString(strings.TrimPrefix(item.TrackURI, "spotify:track:")) {
				report.TrackReferences++
			}
			if item.LastPlayedTime > 0 {
				report.Timestamps++
			}
			if seenContexts[item.URI] {
				report.RepeatedContexts++
			}
			seenContexts[item.URI] = true
			if seenTracks[item.TrackURI] {
				report.RepeatedTracks++
			}
			seenTracks[item.TrackURI] = true
		}
		pages = append(pages, report)
		if len(wire.Contexts) < 50 {
			break
		}
	}
	return pages, ctx.Err()
}
