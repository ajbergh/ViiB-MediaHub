//go:build spotify_research

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

func fetchFromBackend(ctx context.Context, base, id string, out io.Writer, endpoints ...string) error {
	endpoint := "audio_features"
	if len(endpoints) > 0 {
		endpoint = endpoints[0]
	}
	if endpoint != "audio_features" && endpoint != "audio_attributes_v2" {
		return errors.New("unsupported metadata endpoint")
	}
	u, err := url.Parse(base)
	if err != nil {
		return errors.New("backend URL must be an HTTP loopback URL")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "http" || ip == nil || !ip.IsLoopback() || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("backend URL must be an HTTP loopback URL such as http://127.0.0.1:34115")
	}
	u.Path = "/api/spotify/analysis/" + id + "/refresh"
	u.RawQuery = "endpoint=" + endpoint
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("running backend request failed: %w", err)
	}
	defer response.Body.Close()
	const limit = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(body) > limit {
		return errors.New("cannot read bounded backend response")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("backend returned HTTP %d; check Spotify connection in that ViiB instance", response.StatusCode)
	}
	var result struct {
		FromCache *bool `json:"fromCache"`
		Cache     *struct {
			Observation struct {
				TrackID        string `json:"trackId"`
				SourceEndpoint string `json:"sourceEndpoint"`
			} `json:"observation"`
		} `json:"cache"`
	}
	if json.Unmarshal(body, &result) != nil || result.FromCache == nil || result.Cache == nil || result.Cache.Observation.TrackID != id || result.Cache.Observation.SourceEndpoint != endpoint {
		return errors.New("unexpected backend metadata response")
	}
	// Preserve backend provenance and fromCache rather than claiming a fresh fetch.
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, body, "", "  "); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, formatted.String())
	return err
}
