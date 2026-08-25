package plex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlexTokenDoesNotFollowCrossOriginRedirect(t *testing.T) {
	const token = "top-secret-plex-token"
	var mediaToken, jsonToken string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/media":
			mediaToken = r.Header.Get("X-Plex-Token")
			_, _ = w.Write([]byte("audio"))
		case "/json":
			jsonToken = r.Header.Get("X-Plex-Token")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != token {
			t.Errorf("source request missing Plex token for %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/media":
			http.Redirect(w, r, target.URL+"/media", http.StatusTemporaryRedirect)
		case "/json":
			http.Redirect(w, r, target.URL+"/json", http.StatusTemporaryRedirect)
		default:
			http.NotFound(w, r)
		}
	}))
	defer source.Close()

	client, err := NewClientWithHTTP(source.URL, token, "viib-test", source.Client())
	if err != nil {
		t.Fatal(err)
	}

	mediaReq, err := client.MediaRequest(context.Background(), "/media")
	if err != nil {
		t.Fatal(err)
	}
	mediaResp, err := client.MediaHTTPClient().Do(mediaReq)
	if err != nil {
		t.Fatal(err)
	}
	mediaResp.Body.Close()

	jsonReq, err := client.newRequest(context.Background(), http.MethodGet, "", "/json", true)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := client.doJSON(jsonReq, &payload); err != nil {
		t.Fatal(err)
	}

	if mediaToken != "" || jsonToken != "" {
		t.Fatalf("Plex token leaked across origin: media=%q json=%q", mediaToken, jsonToken)
	}
}
