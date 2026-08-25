package plex

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizeServerURL(t *testing.T) {
	tests := map[string]string{
		"192.168.1.10":                 "http://192.168.1.10:32400",
		"plex.local:32401":             "http://plex.local:32401",
		"https://plex.example.com":     "https://plex.example.com",
		"https://plex.example.com/pms": "https://plex.example.com/pms",
	}
	for input, want := range tests {
		got, err := NormalizeServerURL(input)
		if err != nil {
			t.Fatalf("NormalizeServerURL(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeServerURL(%q)=%q want %q", input, got, want)
		}
	}
	for _, invalid := range []string{"", "ftp://plex.local", "http://user:pass@plex.local", "http://plex.local?token=secret", "http://plex.local:99999"} {
		if _, err := NormalizeServerURL(invalid); err == nil {
			t.Fatalf("expected invalid URL error for %q", invalid)
		}
	}
}

func TestValidateServerAndAuthenticationHeader(t *testing.T) {
	const token = "server-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/identity":
			if r.Header.Get("X-Plex-Token") != "" {
				t.Errorf("identity unexpectedly received token")
			}
			_, _ = w.Write([]byte(`{"MediaContainer":{"claimed":true,"machineIdentifier":"machine-abc","version":"1.43.2"}}`))
		case "/":
			if r.Header.Get("X-Plex-Token") != token {
				t.Errorf("root token=%q want %q", r.Header.Get("X-Plex-Token"), token)
			}
			if r.URL.Query().Get("X-Plex-Token") != "" {
				t.Error("token leaked to query")
			}
			_, _ = w.Write([]byte(`{"MediaContainer":{"friendlyName":"Studio","machineIdentifier":"machine-abc","version":"1.43.2","claimed":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClientWithHTTP(server.URL, token, "viib-test", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ValidateServer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "Studio" || result.MachineIdentifier != "machine-abc" || result.AuthRequired {
		t.Fatalf("unexpected validation result: %#v", result)
	}
}

func TestValidateServerRejectsNonPlexEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	}))
	defer server.Close()
	client, err := NewClientWithHTTP(server.URL, "", "viib-test", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ValidateServer(context.Background()); !errors.Is(err, ErrNotPlexServer) {
		t.Fatalf("expected ErrNotPlexServer, got %v", err)
	}
}

func TestValidateServerPreservesDNSFailure(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &net.DNSError{Err: "no such host", Name: "missing.plex"}
	})}
	client, err := NewClientWithHTTP("http://missing.plex:32400", "", "viib-test", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ValidateServer(context.Background()); !errors.Is(err, ErrDNSFailure) {
		t.Fatalf("expected DNS failure, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)
func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestListMusicLibrariesFiltersVideoAndUsesTrackPivot(t *testing.T) {
	const token = "secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != token {
			t.Fatalf("missing Plex token for %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/media/providers":
			payload := map[string]any{"MediaContainer": map[string]any{"MediaProvider": []any{
				map[string]any{"identifier": "com.plexapp.plugins.library", "title": "Library", "Feature": []any{
					map[string]any{"type": "content", "key": "/library/sections", "Directory": []any{
						map[string]any{"id": 1, "key": "/library/sections/1", "title": "Movies", "type": "movie"},
						map[string]any{"id": 2, "key": "/library/sections/2", "title": "Music", "type": "artist"},
					}},
				}},
			}}}
			_ = json.NewEncoder(w).Encode(payload)
		case "/library/sections/2":
			if r.URL.Query().Get("includeDetails") != "1" {
				t.Errorf("includeDetails=%q", r.URL.Query().Get("includeDetails"))
			}
			_, _ = w.Write([]byte(`{"MediaContainer":{"Type":[{"key":"all?type=8","type":"artist","title":"Artists"},{"key":"all?type=9","type":"album","title":"Albums"},{"key":"all?type=10","type":"track","title":"Tracks"},{"key":"musicVideos","type":"clip","title":"Music Videos"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClientWithHTTP(server.URL, token, "viib-test", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := client.ListMusicLibraries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(libraries) != 1 || libraries[0].Title != "Music" || libraries[0].ID != "2" {
		t.Fatalf("unexpected libraries: %#v", libraries)
	}
	if !strings.Contains(libraries[0].TrackKey, "/library/sections/2/all") || !strings.Contains(libraries[0].TrackKey, "type=10") {
		t.Fatalf("track key did not use documented track pivot: %s", libraries[0].TrackKey)
	}
}

func TestFetchTracksMapsMusicMetadataAndPaginationHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Container-Start") != "0" || r.Header.Get("X-Plex-Container-Size") != "500" {
			t.Errorf("pagination headers missing: start=%q size=%q", r.Header.Get("X-Plex-Container-Start"), r.Header.Get("X-Plex-Container-Size"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"size":1,"totalSize":1,"Metadata":[{"ratingKey":"123","key":"/library/metadata/123","type":"track","title":"Track","parentTitle":"Album","grandparentTitle":"Artist","index":4,"parentIndex":2,"year":2025,"duration":245000,"thumb":"/library/metadata/123/thumb/1","parentThumb":"/library/metadata/100/thumb/9","addedAt":100,"updatedAt":200,"Genre":[{"tag":"Rock"}],"Media":[{"container":"flac","audioCodec":"flac","Part":[{"key":"/library/parts/55/file.flac","container":"flac"}]}]}]}}`))
	}))
	defer server.Close()
	client, err := NewClientWithHTTP(server.URL, "secret", "viib-test", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.FetchTracks(context.Background(), Library{ID: "2", TrackKey: server.URL + "/tracks"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tracks) != 1 {
		t.Fatalf("got %d tracks", len(result.Tracks))
	}
	track := result.Tracks[0]
	if track.RatingKey != "123" || track.Artist != "Artist" || track.Album != "Album" || track.AlbumArtist != "Artist" || track.TrackNumber != 4 || track.DiscNumber != 2 || track.DurationSeconds != 245 || track.MediaKey != "/library/parts/55/file.flac" || track.AudioCodec != "flac" || len(track.Genres) != 1 || track.Genres[0] != "Rock" {
		t.Fatalf("unexpected mapped track: %#v", track)
	}
	if track.ArtworkKey != "/library/metadata/100/thumb/9" {
		t.Fatalf("artwork key=%q want parent album thumb", track.ArtworkKey)
	}
	if StableTrackID("machine", "123") == StableTrackID("machine", "124") || StableTrackID("machine", "123") == "123" {
		t.Fatal("stable Plex IDs are not namespaced/stable")
	}
}

func TestMapPlexTrackUsesFirstUsableMediaPart(t *testing.T) {
	track, ok := mapPlexTrack(plexTrack{
		RatingKey:     "55",
		Title:         "Alternate",
		ParentTitle:   "Album",
		ParentThumb:   "/album/thumb",
		Media: []plexMedia{
			{Container: "flac", AudioCodec: "flac", Part: []plexPart{{Key: ""}}},
			{Container: "mp3", AudioCodec: "mp3", Part: []plexPart{{Key: "/library/parts/55/file.mp3", Container: "mp3"}}},
		},
	})
	if !ok {
		t.Fatal("expected track with a later valid media part to map")
	}
	if track.MediaKey != "/library/parts/55/file.mp3" || track.Container != "mp3" || track.AudioCodec != "mp3" {
		t.Fatalf("unexpected selected media: %#v", track)
	}
}

func TestMapPlexTrackRetainsPresenceWithoutPlayableMediaPart(t *testing.T) {
	track, ok := mapPlexTrack(plexTrack{
		RatingKey:        "77",
		Key:              "/library/metadata/77",
		Type:             "track",
		Title:            "Temporarily unavailable",
		ParentTitle:      "Album",
		GrandparentTitle: "Artist",
		ParentThumb:      "/library/metadata/70/thumb/5",
		UpdatedAt:        123,
		Media:            []plexMedia{{Container: "flac", AudioCodec: "flac", Part: []plexPart{{Key: ""}}}},
	})
	if !ok {
		t.Fatal("expected Plex metadata identity to remain in the authoritative snapshot")
	}
	if track.RatingKey != "77" || track.MediaKey != "" || track.ArtworkKey != "/library/metadata/70/thumb/5" {
		t.Fatalf("unexpected unplayable track mapping: %#v", track)
	}
}

func TestFetchTracksContinuesWhenServerReturnsShortPageWithTotalRemaining(t *testing.T) {
	starts := make([]int, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.Header.Get("X-Plex-Container-Start"))
		starts = append(starts, start)
		w.Header().Set("Content-Type", "application/json")
		makeTrack := func(id string) map[string]any {
			return map[string]any{
				"ratingKey": id,
				"key":       "/library/metadata/" + id,
				"type":      "track",
				"title":     "Track " + id,
				"parentTitle": "Album",
				"parentThumb": "/library/metadata/album/thumb/1",
				"Media": []any{map[string]any{"container": "mp3", "audioCodec": "mp3", "Part": []any{map[string]any{"key": "/library/parts/" + id + "/file.mp3"}}}},
			}
		}
		var metadata []any
		switch start {
		case 0:
			metadata = []any{makeTrack("1"), makeTrack("2")}
		case 2:
			metadata = []any{makeTrack("3")}
		default:
			t.Fatalf("unexpected paging start %d", start)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{
			"offset": start, "size": len(metadata), "totalSize": 3, "Metadata": metadata,
		}})
	}))
	defer server.Close()

	client, err := NewClientWithHTTP(server.URL, "secret", "viib-test", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.FetchTracks(context.Background(), Library{ID: "2", TrackKey: server.URL + "/tracks"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tracks) != 3 {
		t.Fatalf("got %d tracks, want 3", len(result.Tracks))
	}
	if len(starts) != 2 || starts[0] != 0 || starts[1] != 2 {
		t.Fatalf("unexpected page starts: %v", starts)
	}
}

func TestMediaRequestNeverLeaksTokenCrossOrigin(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:32400", "secret-token", "viib-test")
	if err != nil {
		t.Fatal(err)
	}
	req, err := client.MediaRequest(context.Background(), "/library/parts/1/file.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-Plex-Token") != "secret-token" || strings.Contains(req.URL.String(), "secret-token") {
		t.Fatalf("bad same-origin token handling: url=%s header=%q", req.URL, req.Header.Get("X-Plex-Token"))
	}
	external, err := client.MediaRequest(context.Background(), "https://metadata-static.plex.tv/art.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if external.Header.Get("X-Plex-Token") != "" {
		t.Fatal("Plex token leaked cross-origin")
	}
}
