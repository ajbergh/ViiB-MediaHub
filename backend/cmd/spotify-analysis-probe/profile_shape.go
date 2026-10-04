//go:build spotify_research

// Observes profile field presence and types while preserving the response body.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// Only fixed field types/counts escape this research observer. Unknown keys,
// scalar values, credentials and raw provider bodies are never retained.
type profileShapeTransport struct {
	base   http.RoundTripper
	report *map[string]any
}

func (t profileShapeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil || response == nil || request.URL.Host != "api-partner.spotify.com" || request.URL.Path != "/pathfinder/v2/query" || response.StatusCode != http.StatusOK {
		return response, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(raw))
	if readErr != nil {
		return response, readErr
	}
	if len(raw) > 1<<20 {
		return response, nil
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return response, nil
	}
	data, _ := root["data"].(map[string]any)
	me, _ := data["me"].(map[string]any)
	profile, exists := me["profile"].(map[string]any)
	if !exists {
		return response, nil
	}
	shape := map[string]any{"profileFieldCount": len(profile), "meFieldCount": len(me)}
	for _, container := range []struct {
		name  string
		value map[string]any
	}{{"profile", profile}, {"me", me}} {
		fields := map[string]string{}
		for _, key := range []string{"email", "product", "country", "followers"} {
			value, present := container.value[key]
			kind := "missing"
			if present {
				kind = profileValueType(value)
			}
			fields[key] = kind
			if key == "followers" {
				followers, _ := value.(map[string]any)
				total, totalPresent := followers["total"]
				totalType := "missing"
				if totalPresent {
					totalType = profileValueType(total)
				}
				fields["followersTotal"] = totalType
			}
		}
		shape[container.name] = fields
	}
	*t.report = shape
	return response, nil
}
func profileValueType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return "other"
	}
}
