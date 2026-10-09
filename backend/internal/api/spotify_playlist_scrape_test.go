package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify"
)

func TestCookieFirstPartyPlaylistScraperCapturesUnboundEvidence(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			playlistID := strings.Repeat("P", 22)
			trackOne := strings.Repeat("T", 22)
			trackTwo := strings.Repeat("B", 22)
			missing := strings.Repeat("C", 22)
			a.spotifyPlaylistScraper = func(ctx context.Context, id string) (*spotify.ScrapedPlaylist, error) {
				if err := ctx.Err(); err != nil || id != playlistID {
					t.Fatalf("scraper context or identity: %v %s", err, id)
				}
				return &spotify.ScrapedPlaylist{Name: "Fixture Mix", Artwork: "https://images.example/cover", Tracks: []string{trackOne, trackTwo, trackOne, missing}}, nil
			}
			var trackCalls int
			trackArtist := strings.Repeat("A", 22)
			trackAlbum := strings.Repeat("D", 22)
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Cookie") != "" {
					t.Fatal("cookie escaped catalog request")
				}
				if r.URL.Host == "clienttoken.spotify.com" {
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"))}, nil
				}
				if r.URL.Host != "api-partner.spotify.com" {
					t.Fatalf("unexpected upstream host %s", r.URL.Host)
				}
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				operation, _ := request["operationName"].(string)
				variables, _ := request["variables"].(map[string]any)
				uri, _ := variables["uri"].(string)
				switch operation {
				case "fetchPlaylist":
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
				case "getTrack":
					id := strings.TrimPrefix(uri, "spotify:track:")
					if id == missing {
						return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
					}
					if id != trackOne && id != trackTwo {
						t.Fatalf("unexpected track %s", id)
					}
					trackCalls++
					artist := map[string]any{"uri": "spotify:artist:" + trackArtist, "profile": map[string]any{"name": "Fixture Artist"}}
					track := map[string]any{"__typename": "Track", "uri": uri, "name": "Fixture Track", "duration": map[string]any{"totalMilliseconds": 180000}, "artists": map[string]any{"items": []any{artist}}, "albumOfTrack": map[string]any{"uri": "spotify:album:" + trackAlbum, "name": "Fixture Album"}}
					raw, _ := json.Marshal(map[string]any{"data": map[string]any{"trackUnion": track}})
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
				case "getAlbum":
					artist := map[string]any{"uri": "spotify:artist:" + trackArtist, "profile": map[string]any{"name": "Fixture Artist"}}
					album := map[string]any{"__typename": "Album", "uri": "spotify:album:" + trackAlbum, "name": "Fixture Album", "type": "ALBUM", "artists": map[string]any{"items": []any{artist}}, "coverArt": map[string]any{"sources": []any{}}, "tracksV2": map[string]any{"totalCount": 0, "items": []any{}}}
					raw, _ := json.Marshal(map[string]any{"data": map[string]any{"albumUnion": album}})
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
				default:
					t.Fatalf("unexpected catalog operation %s", operation)
					return nil, nil
				}
			})}

			tracks, name, artwork, err := a.fetchPlaylistTracks(t.Context(), playlistID, nil)
			if err != nil || name != "Fixture Mix" || artwork != "https://images.example/cover" {
				t.Fatalf("scraped fallback failed: name=%q artwork=%q err=%v", name, artwork, err)
			}
			if len(tracks) != 3 || tracks[0].ID != trackOne || tracks[1].ID != trackTwo || tracks[2].ID != trackOne || trackCalls != 2 {
				t.Fatalf("fallback lost returned order or duplicate: tracks=%+v calls=%d", tracks, trackCalls)
			}
			runtime := a.spotifyTokens()
			root, err := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: playlistID, Resource: scrapedPlaylistUnboundResource, ContextKey: runtime.metadataContext})
			if err != nil || root == nil || root.CaptureRevision != "" || len(root.Relations) != 4 {
				t.Fatalf("unbound scraped playlist was not persisted: %+v err=%v", root, err)
			}
			var provenance map[string]any
			if json.Unmarshal(root.Payload, &provenance) != nil || provenance["provenance"] != "embed_scrape_unbound" || provenance["revisionKnown"] != false {
				t.Fatalf("scrape provenance was not explicit: %s", root.Payload)
			}
			for position, id := range []string{trackOne, trackTwo, trackOne, missing} {
				relation := root.Relations[position]
				if relation.Kind != "playlist_items" || relation.Position != position || relation.ChildType != "track" || relation.ChildID != id || relation.Unavailable != (id == missing) {
					t.Fatalf("scraped relation %d lost identity/order/availability: %+v", position, relation)
				}
			}
			for _, id := range []string{trackOne, trackTwo} {
				snapshot, readErr := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "track", SpotifyID: id, Resource: "getTrack:page:0:0", ContextKey: runtime.metadataContext})
				if readErr != nil || snapshot == nil {
					t.Fatalf("captured track %s missing: %+v %v", id, snapshot, readErr)
				}
			}
			missingSnapshot, err := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "track", SpotifyID: missing, Resource: "getTrack:page:0:0", ContextKey: runtime.metadataContext})
			if err != nil || missingSnapshot != nil {
				t.Fatalf("missing batch row fabricated a track entity: %+v %v", missingSnapshot, err)
			}
		})
	}
}

func TestCookieScrapedPlaylistCaptureRejectsRetiredOwner(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	playlistID := strings.Repeat("P", 22)
	trackID := strings.Repeat("T", 22)
	a.spotifyPlaylistScraper = func(ctx context.Context, id string) (*spotify.ScrapedPlaylist, error) {
		if err := a.db.SetSetting("spotify_metadata_active_context", "replacement-owner"); err != nil {
			t.Fatal(err)
		}
		return &spotify.ScrapedPlaylist{Name: "Old Owner", Tracks: []string{trackID}}, nil
	}
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "clienttoken.spotify.com" {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"))}, nil
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		operation, _ := request["operationName"].(string)
		variables, _ := request["variables"].(map[string]any)
		uri, _ := variables["uri"].(string)
		if operation == "fetchPlaylist" {
			return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
		}
		if operation == "getAlbum" {
			artist := map[string]any{"uri": "spotify:artist:" + strings.Repeat("A", 22), "profile": map[string]any{"name": "Artist"}}
			album := map[string]any{"__typename": "Album", "uri": "spotify:album:" + strings.Repeat("B", 22), "name": "Album", "type": "ALBUM", "artists": map[string]any{"items": []any{artist}}, "coverArt": map[string]any{"sources": []any{}}, "tracksV2": map[string]any{"totalCount": 0, "items": []any{}}}
			raw, _ := json.Marshal(map[string]any{"data": map[string]any{"albumUnion": album}})
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
		}
		artist := map[string]any{"uri": "spotify:artist:" + strings.Repeat("A", 22), "profile": map[string]any{"name": "Artist"}}
		album := map[string]any{"uri": "spotify:album:" + strings.Repeat("B", 22), "name": "Album"}
		track := map[string]any{"__typename": "Track", "uri": uri, "name": "Track", "duration": map[string]any{"totalMilliseconds": 120000}, "artists": map[string]any{"items": []any{artist}}, "albumOfTrack": album}
		raw, _ := json.Marshal(map[string]any{"data": map[string]any{"trackUnion": track}})
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	_, _, _, err := a.fetchPlaylistTracks(t.Context(), playlistID, nil)
	if !errors.Is(err, db.ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatalf("retired scrape evidence was published: %v", err)
	}
	runtime := a.spotifyTokens()
	root, readErr := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: playlistID, Resource: scrapedPlaylistUnboundResource, ContextKey: runtime.metadataContext})
	if readErr != nil || root != nil {
		t.Fatalf("stale scrape observation became visible: %+v %v", root, readErr)
	}
	track, trackErr := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "track", SpotifyID: trackID, Resource: "getTrack:page:0:0", ContextKey: runtime.metadataContext})
	if trackErr != nil || track != nil {
		t.Fatalf("stale captured track became visible: %+v %v", track, trackErr)
	}
}
