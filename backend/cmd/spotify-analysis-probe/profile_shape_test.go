//go:build spotify_research

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type profileShapeFixtureTransport func(*http.Request) (*http.Response, error)

func (f profileShapeFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestProfileWireShapeDoesNotExposeValuesOrUnknownKeys(t *testing.T) {
	raw := `{"data":{"me":{"secret_unknown":"private-token","country":null,"profile":{"username":"private-user","name":"private-name","email":"private-email","product":"private-plan","followers":{"total":12345},"country":null,"private_extra_key":"private-secret"}}}}`
	var shape map[string]any
	transport := profileShapeTransport{report: &shape, base: profileShapeFixtureTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})}
	request, _ := http.NewRequest("POST", "https://api-partner.spotify.com/pathfinder/v2/query", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	restored, _ := io.ReadAll(response.Body)
	if string(restored) != raw {
		t.Fatal("wire observer changed decoder input")
	}
	encoded, _ := json.Marshal(shape)
	for _, secret := range []string{"private-", "private_extra_key", "secret_unknown", "12345"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("wire shape leaked %s", secret)
		}
	}
	profile := shape["profile"].(map[string]string)
	if profile["email"] != "string" || profile["country"] != "null" || profile["followersTotal"] != "number" {
		t.Fatalf("wrong types: %+v", profile)
	}
	me := shape["me"].(map[string]string)
	if me["email"] != "missing" || me["country"] != "null" {
		t.Fatalf("wrong absence: %+v", me)
	}
}
func TestProfileWireObserverIgnoresOtherOperations(t *testing.T) {
	var shape map[string]any
	transport := profileShapeTransport{report: &shape, base: profileShapeFixtureTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"me":{"libraryV3":{}}}}`))}, nil
	})}
	request, _ := http.NewRequest("POST", "https://api-partner.spotify.com/pathfinder/v2/query", nil)
	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatal(err)
	}
	if shape != nil {
		t.Fatal("unrelated operation recorded")
	}
}
