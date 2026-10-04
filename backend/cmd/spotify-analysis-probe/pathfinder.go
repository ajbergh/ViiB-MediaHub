//go:build spotify_research

// Probes fixed Web Player operations and client-token behavior for protocol research.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

// Fixed protocol facts from the same pinned Apache-2.0 source as token auth.
// This mode is never an automatic fallback from a throttled public API request.
const pathfinderURL = "https://api-partner.spotify.com/pathfinder/v2/query"

type pathfinderResult struct {
	Schema            []string `json:"schema,omitempty"`
	Stage             string   `json:"stage"`
	HTTPStatus        int      `json:"httpStatus,omitempty"`
	Code              string   `json:"code"`
	ShapeCompatible   bool     `json:"shapeCompatible"`
	RetryAfterSeconds float64  `json:"retryAfterSeconds,omitempty"`
}
type pathfinderReport struct {
	AuthenticationVerified bool               `json:"authenticationVerified"`
	RenewalVerified        bool               `json:"renewalVerified"`
	ClientTokenVerified    bool               `json:"clientTokenVerified"`
	Results                []pathfinderResult `json:"results"`
}

func runPathfinderResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, client *http.Client, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	provider, err := auth.NewWebPlayerProvider(cookie, contract, opts)
	cookie = ""
	if err != nil {
		return err
	}
	defer provider.Disconnect()
	token, err := provider.Token(ctx)
	if err != nil {
		return &probeFailure{stage: "authentication", cause: err}
	}
	if renew {
		token, err = provider.Refresh(ctx, token)
		if err != nil {
			return &probeFailure{stage: "token_renewal", authenticated: true, cause: err}
		}
	}
	return probePathfinder(ctx, token, contract.AppVersion, renew, client, out)
}
func probePathfinder(ctx context.Context, token auth.Token, version string, renew bool, client *http.Client, out io.Writer) error {
	report := pathfinderReport{AuthenticationVerified: true, RenewalVerified: renew, Results: []pathfinderResult{}}
	write := func() error { return json.NewEncoder(out).Encode(report) }
	if token.Kind != auth.WebPlayer || token.Bearer() == "" {
		return auth.ErrAuthenticationRequired
	}
	if token.WebPlayerClientID() == "" {
		report.Results = append(report.Results, pathfinderResult{Stage: "client_context", Code: "provider_client_id_missing"})
		return write()
	}
	safe := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		safe = *client
		if safe.Timeout == 0 || safe.Timeout > 10*time.Second {
			safe.Timeout = 10 * time.Second
		}
	}
	safe.Jar = nil
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	body := map[string]any{"client_data": map[string]any{"client_version": version, "client_id": token.WebPlayerClientID(), "js_sdk_data": map[string]string{"device_brand": "", "device_id": "", "device_model": "", "device_type": "", "os": "", "os_version": ""}}}
	object, result := pathfinderRequest(ctx, &safe, "client_token", "https://clienttoken.spotify.com/v1/clienttoken", body, "", "", version)
	clientToken := ""
	if granted, ok := object["granted_token"].(map[string]any); ok {
		clientToken, _ = granted["token"].(string)
	}
	if result.HTTPStatus == 200 {
		result.ShapeCompatible = clientToken != ""
		if clientToken != "" {
			result.Code = "compatible"
		} else {
			result.Code = "client_token_not_granted"
		}
	}
	report.Results = append(report.Results, result)
	if clientToken == "" {
		return write()
	}
	defer func() { clientToken = "" }()
	report.ClientTokenVerified = true
	for _, op := range []struct {
		stage, name, hash string
		variables         map[string]any
	}{
		{"profile", "profileAttributes", "53bcb064f6cd18c23f752bc324a791194d20df612d8e1239c735144ab0399ced", map[string]any{}},
		{"top_results", "findTopResults", "755858df4daab8d212980b02a81dcf8c9a58447de318b59d07c4651a1d0450b9", map[string]any{"query": "music", "numberOfTopResults": 1}},
	} {
		body = map[string]any{"operationName": op.name, "variables": op.variables, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": op.hash}}}
		object, result = pathfinderRequest(ctx, &safe, op.stage, pathfinderURL, body, token.Bearer(), clientToken, version)
		if result.HTTPStatus == 200 {
			result.ShapeCompatible = pathfinderShape(op.stage, object)
			if op.stage == "profile" {
				result.Schema = pathfinderSchema(object["data"])
			}
			result.Code = "schema_incompatible"
			if result.ShapeCompatible {
				result.Code = "compatible"
			}
		}
		report.Results = append(report.Results, result)
		if result.HTTPStatus != 200 || !result.ShapeCompatible {
			break
		}
	}
	return write()
}
func pathfinderShape(stage string, object map[string]any) bool {
	if errs, ok := object["errors"].([]any); ok && len(errs) > 0 {
		return false
	}
	data, _ := object["data"].(map[string]any)
	if stage == "profile" {
		me, _ := data["me"].(map[string]any)
		profile, _ := me["profile"].(map[string]any)
		username, _ := profile["username"].(string)
		return username != ""
	}
	search, _ := data["searchV2"].(map[string]any)
	top, _ := search["topResultsV2"].(map[string]any)
	_, ok := top["itemsV2"].([]any)
	return ok
}
func pathfinderRequest(ctx context.Context, client *http.Client, stage, target string, body map[string]any, bearer, clientToken, version string) (map[string]any, pathfinderResult) {
	result := pathfinderResult{Stage: stage, Code: "temporarily_unavailable"}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, result
	}
	defer clear(encoded)
	request, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(encoded))
	if err != nil {
		return nil, result
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("App-Platform", "WebPlayer")
	request.Header.Set("Spotify-App-Version", version)
	request.Header.Set("Origin", "https://open.spotify.com")
	request.Header.Set("Referer", "https://open.spotify.com/")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if clientToken != "" {
		request.Header.Set("Client-Token", clientToken)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, result
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	if response.StatusCode != 200 {
		switch response.StatusCode {
		case 401:
			result.Code = "token_rejected"
		case 403:
			result.Code = "access_denied"
		case 429:
			result.Code = "rate_limited"
			result.RetryAfterSeconds = auth.RetryAfter(response.Header.Get("Retry-After"), time.Now()).Seconds()
		default:
			result.Code = "unexpected_http_status"
		}
		return nil, result
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	defer clear(raw)
	if err != nil || len(raw) > 1<<20 {
		result.Code = "payload_unavailable"
		return nil, result
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil || object == nil {
		result.Code = "schema_incompatible"
		return nil, result
	}
	return object, result
}

var schemaKey = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]{0,63}$")

// GraphQL field names/types only: never scalar values, usernames or credentials.
func pathfinderSchema(value any) []string {
	result := []string{}
	var walk func(any, string, int)
	walk = func(value any, path string, depth int) {
		if depth > 6 || len(result) >= 100 {
			return
		}
		switch v := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(v))
			for key := range v {
				if schemaKey.MatchString(key) {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				next := key
				if path != "" {
					next = path + "." + key
				}
				walk(v[key], next, depth+1)
			}
		case []any:
			result = append(result, path+":array")
			if len(v) > 0 {
				walk(v[0], path+"[]", depth+1)
			}
		case string:
			result = append(result, path+":string")
		case float64:
			result = append(result, path+":number")
		case bool:
			result = append(result, path+":boolean")
		case nil:
			result = append(result, path+":null")
		}
	}
	walk(value, "", 0)
	return result
}
