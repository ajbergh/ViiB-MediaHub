//go:build spotify_research

// Compares independently probed Spotify endpoint capabilities.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

// compareAvailability performs fixed-route research requests. It never returns
// upstream bodies, messages, headers, cookies, or bearer tokens.
func compareAvailability(ctx context.Context, token auth.Token, id string, contract auth.WebPlayerContract, renewed bool, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	type result struct {
		Route        string `json:"route"`
		HTTPStatus   int    `json:"httpStatus,omitempty"`
		BodyBytes    int    `json:"bodyBytes"`
		JSON         bool   `json:"json"`
		TrackPresent bool   `json:"analysisTrackPresent"`
		TempoPresent bool   `json:"tempoPresent"`
		KeyPresent   bool   `json:"keyPresent"`
		Code         string `json:"code,omitempty"`
	}
	results := make([]result, 0, 3)
	for _, route := range []struct{ name, path string }{
		{"audio_analysis", "audio-analysis/" + id},
		{"audio_features_single", "audio-features/" + id + "?format=json"},
		{"audio_features_batch", "audio-features?ids=" + id},
	} {
		diagnostic := result{Route: route.name}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://spclient.wg.spotify.com/audio-attributes/v1/"+route.path, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token.Bearer())
		request.Header.Set("App-Platform", "WebPlayer")
		request.Header.Set("Spotify-App-Version", contract.AppVersion)
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			diagnostic.Code = "request_failed"
			results = append(results, diagnostic)
			continue
		}
		diagnostic.HTTPStatus = response.StatusCode
		body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
		response.Body.Close()
		diagnostic.BodyBytes = len(body)
		if err != nil {
			diagnostic.Code = "read_failed"
			results = append(results, diagnostic)
			continue
		}
		if len(body) > 8<<20 {
			diagnostic.Code = "payload_too_large"
			results = append(results, diagnostic)
			continue
		}
		var data struct {
			Track         json.RawMessage `json:"track"`
			Tempo         *float64        `json:"tempo"`
			Key           *int            `json:"key"`
			AudioFeatures []struct {
				Tempo *float64 `json:"tempo"`
				Key   *int     `json:"key"`
			} `json:"audio_features"`
		}
		diagnostic.JSON = json.Unmarshal(body, &data) == nil
		if diagnostic.JSON {
			diagnostic.TrackPresent = len(data.Track) > 0 && string(data.Track) != "null"
			diagnostic.TempoPresent = data.Tempo != nil
			diagnostic.KeyPresent = data.Key != nil
			var track struct {
				Tempo *float64 `json:"tempo"`
				Key   *int     `json:"key"`
			}
			if diagnostic.TrackPresent && json.Unmarshal(data.Track, &track) == nil {
				diagnostic.TempoPresent = track.Tempo != nil
				diagnostic.KeyPresent = track.Key != nil
			}
			if len(data.AudioFeatures) > 0 {
				diagnostic.TempoPresent = data.AudioFeatures[0].Tempo != nil
				diagnostic.KeyPresent = data.AudioFeatures[0].Key != nil
			}
		}
		results = append(results, diagnostic)
		// Do not increase pressure after the service asks the caller to back off.
		if response.StatusCode == http.StatusTooManyRequests {
			break
		}
	}
	return json.NewEncoder(out).Encode(struct {
		TrackID                string   `json:"trackId"`
		AuthenticationVerified bool     `json:"authenticationVerified"`
		RenewalVerified        bool     `json:"renewalVerified"`
		Results                []result `json:"results"`
	}{id, true, renewed, results})
}
