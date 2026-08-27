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
	if !strings.Contains(libraries[0].AlbumKey, "/library/sections/2/all") || !strings.Contains(libraries[0].AlbumKey, "type=9") {
		t.Fatalf("album key did not use documented album pivot: %s", libraries[0].AlbumKey)
	}
}

func TestFetchTracksMapsMusicMetadataAndPaginationHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Container-Start") != "0" || r.Header.Get("X-Plex-Container-Size") != "500" {
			t.Errorf("pagination headers missing: start=%q size=%q", r.Header.Get("X-Plex-Container-Start"), r.Header.Get("X-Plex-Container-Size"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"size":1,"totalSize":1,"Metadata":[{"ratingKey":"123","key":"/library/metadata/123","type":"track","title":"Track","parentTitle":"Album","grandparentTitle":"Artist","index":4,"parentIndex":2,"year":2025,"duration":245000,"thumb":"/library/metadata/123/thumb/1","parentThumb":"/library/metadata/100/thumb/9","grandparentThumb":"/library/metadata/artist-8/thumb/42","addedAt":100,"updatedAt":200,"Genre":[{"tag":"Rock"}],"Media":[{"container":"flac","audioCodec":"flac","Part":[{"key":"/library/parts/55/file.flac","container":"flac"}]}]}]}}`))
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
	if track.ArtistArtworkKey != "/library/metadata/artist-8/thumb/42" {
		t.Fatalf("artist artwork key=%q", track.ArtistArtworkKey)
	}
	if StableTrackID("machine", "123") == StableTrackID("machine", "124") || StableTrackID("machine", "123") == "123" {
		t.Fatal("stable Plex IDs are not namespaced/stable")
	}
}

func TestFetchTracksFallsBackToAlbumMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Container-Start") != "0" || r.Header.Get("X-Plex-Container-Size") != "500" {
			t.Errorf("pagination headers missing: start=%q size=%q", r.Header.Get("X-Plex-Container-Start"), r.Header.Get("X-Plex-Container-Size"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tracks":
			_, _ = w.Write([]byte(`{"MediaContainer":{"size":1,"totalSize":1,"Metadata":[{"ratingKey":"123","parentRatingKey":"album-9","key":"/library/metadata/123","type":"track","title":"Track","parentTitle":"Album","grandparentTitle":"Artist","Media":[{"container":"flac","audioCodec":"flac","Part":[{"key":"/library/parts/55/file.flac","container":"flac"}]}]}]}}`))
		case "/albums":
			_, _ = w.Write([]byte(`{"MediaContainer":{"size":1,"totalSize":1,"Metadata":[{"ratingKey":"album-9","type":"album","title":"Album","year":1994,"Genre":[{"tag":"Jazz"},{"tag":"Fusion"}]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClientWithHTTP(server.URL, "secret", "viib-test", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.FetchTracks(context.Background(), Library{ID: "2", TrackKey: server.URL + "/tracks", AlbumKey: server.URL + "/albums"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tracks) != 1 || result.Tracks[0].ParentRatingKey != "album-9" || result.Tracks[0].Year != 1994 || len(result.Tracks[0].Genres) != 2 || result.Tracks[0].Genres[0] != "Jazz" || result.Tracks[0].Genres[1] != "Fusion" {
		t.Fatalf("track did not inherit album metadata: %#v", result.Tracks)
	}
}

func TestMapPlexTrackUsesFirstUsableMediaPart(t *testing.T) {
	track, ok := mapPlexTrack(plexTrack{
		RatingKey:   "55",
		Title:       "Alternate",
		ParentTitle: "Album",
		ParentThumb: "/album/thumb",
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

func TestTrackMetadataWritebackUsesLockedPMSFields(t *testing.T) {
	const token = "metadata-token"
	genres := []string{"Rock"}
	year := 1999
	var sawPut bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != token {
			t.Fatalf("Plex token missing for %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/library/metadata/99":
			plexGenres := make([]any, 0, len(genres))
			for _, genre := range genres {
				plexGenres = append(plexGenres, map[string]any{"tag": genre})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": []any{map[string]any{
				"ratingKey": "99", "type": "track", "title": "Dream Song", "year": year, "Genre": plexGenres,
			}}}})
		case r.Method == http.MethodGet && r.URL.Path == "/media/providers":
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"MediaProvider": []any{map[string]any{
				"identifier": "com.plexapp.plugins.library", "Feature": []any{map[string]any{"type": "manage"}},
			}}}})
		case r.Method == http.MethodPut && r.URL.Path == "/library/metadata/99":
			query := r.URL.Query()
			if query.Get("type") != "10" || query.Get("genre.locked") != "1" || query.Get("genre[0].tag.tag") != "Dream Pop" || query.Get("genre[1].tag.tag") != "Indie Rock" || query.Get("year.value") != "1988" || query.Get("year.locked") != "1" {
				t.Fatalf("unexpected locked metadata edit: %q", r.URL.RawQuery)
			}
			genres, year, sawPut = []string{"Dream Pop", "Indie Rock"}, 1988, true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClientWithHTTP(server.URL, token, "viib-test", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	before, err := client.GetTrackMetadata(context.Background(), "99")
	if err != nil || before.Year != 1999 || !equalStringSlices(before.Genres, []string{"Rock"}) {
		t.Fatalf("unexpected metadata before write: %#v err=%v", before, err)
	}
	allowed, err := client.CanManageMetadata(context.Background())
	if err != nil || !allowed {
		t.Fatalf("manage capability=%v err=%v", allowed, err)
	}
	if err := client.UpdateTrackMetadata(context.Background(), "99", TrackMetadataEdit{Genres: []string{"Dream Pop", "Indie Rock"}, Year: 1988}); err != nil {
		t.Fatal(err)
	}
	after, err := client.GetTrackMetadata(context.Background(), "99")
	if err != nil || !sawPut || after.Year != 1988 || !equalStringSlices(after.Genres, []string{"Dream Pop", "Indie Rock"}) {
		t.Fatalf("writeback was not verifiable: %#v err=%v sawPut=%v", after, err, sawPut)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestFetchTracksContinuesWhenServerReturnsShortPageWithTotalRemaining(t *testing.T) {
	starts := make([]int, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.Header.Get("X-Plex-Container-Start"))
		starts = append(starts, start)
		w.Header().Set("Content-Type", "application/json")
		makeTrack := func(id string) map[string]any {
			return map[string]any{
				"ratingKey":   id,
				"key":         "/library/metadata/" + id,
				"type":        "track",
				"title":       "Track " + id,
				"parentTitle": "Album",
				"parentThumb": "/library/metadata/album/thumb/1",
				"Media":       []any{map[string]any{"container": "mp3", "audioCodec": "mp3", "Part": []any{map[string]any{"key": "/library/parts/" + id + "/file.mp3"}}}},
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
