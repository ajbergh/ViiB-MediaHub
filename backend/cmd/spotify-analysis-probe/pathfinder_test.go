//go:build spotify_research

// Tests probing fixed Web Player operations and client-token behavior for protocol research.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func pathfinderFixtureToken(t *testing.T) auth.Token {
	t.Helper()
	provider, err := auth.NewWebPlayerProvider("fixture-cookie", researchContract(), auth.WebPlayerOptions{Client: &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
		body := fmt.Sprintf("{\"serverTime\":%d}", time.Now().Unix())
		if r.URL.Path == "/api/token" {
			body = fmt.Sprintf("{\"accessToken\":\"fixture-bearer\",\"clientId\":\"fixture-public-client\",\"isAnonymous\":false,\"accessTokenExpirationTimestampMs\":%d}", time.Now().Add(time.Hour).UnixMilli())
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Disconnect()
	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return token
}
func TestPathfinderFixedReadsAndRedaction(t *testing.T) {
	token := pathfinderFixtureToken(t)
	if token.WebPlayerClientID() != "fixture-public-client" {
		t.Fatal("missing provider client context")
	}
	calls := 0
	client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.Header.Get("Cookie") != "" {
			t.Fatal("unsafe request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		result := ""
		if calls == 1 {
			if r.URL.String() != "https://clienttoken.spotify.com/v1/clienttoken" || r.Header.Get("Authorization") != "" {
				t.Fatal("bearer escaped partner origin")
			}
			data := body["client_data"].(map[string]any)
			if data["client_id"] != "fixture-public-client" {
				t.Fatal("wrong client context")
			}
			result = "{\"granted_token\":{\"token\":\"fixture-client-token\"}}"
		} else {
			if r.URL.String() != pathfinderURL || r.Header.Get("Authorization") != "Bearer fixture-bearer" || r.Header.Get("Client-Token") != "fixture-client-token" {
				t.Fatal("wrong partner context")
			}
			if calls == 2 {
				if body["operationName"] != "profileAttributes" {
					t.Fatal("wrong operation")
				}
				result = "{\"data\":{\"me\":{\"profile\":{\"username\":\"private-profile\"}}}}"
			} else {
				if body["operationName"] != "findTopResults" {
					t.Fatal("wrong operation")
				}
				result = "{\"data\":{\"searchV2\":{\"topResultsV2\":{\"itemsV2\":[]}}}}"
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(result))}, nil
	})}
	var out bytes.Buffer
	if err := probePathfinder(context.Background(), token, researchContract().AppVersion, false, client, &out); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatal("wrong request count")
	}
	for _, secret := range []string{"fixture-bearer", "fixture-cookie", "fixture-client-token", "private-profile"} {
		if strings.Contains(out.String(), secret) {
			t.Fatal("report disclosed credentials or profile")
		}
	}
	var report pathfinderReport
	if json.Unmarshal(out.Bytes(), &report) != nil || !report.ClientTokenVerified || !report.Results[2].ShapeCompatible {
		t.Fatal("incomplete report")
	}
}
func TestPathfinderStopsAtDeniedOperation(t *testing.T) {
	for _, status := range []int{401, 403, 429} {
		token := pathfinderFixtureToken(t)
		calls := 0
		client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			code := status
			body := "{}"
			if calls == 1 {
				code = 200
				body = "{\"granted_token\":{\"token\":\"fixture-client-token\"}}"
			}
			return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		var out bytes.Buffer
		if err := probePathfinder(context.Background(), token, researchContract().AppVersion, false, client, &out); err != nil {
			t.Fatal(err)
		}
		if calls != 2 {
			t.Fatal("continued after denial")
		}
	}
	if pathfinderShape("profile", map[string]any{"errors": []any{"failure"}}) {
		t.Fatal("GraphQL failure accepted")
	}
}
