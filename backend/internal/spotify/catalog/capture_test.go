package catalog

import (
	"bytes"
	"strings"
	"testing"
)

func TestCaptureDomainPreservesUnknownFieldsAndPositions(t *testing.T) {
	id := strings.Repeat("P", 22)
	track := strings.Repeat("T", 22)
	raw := []byte(`{"data":{"playlistV2":{"__typename":"Playlist","uri":"spotify:playlist:` + id + `","unknown":{"value":false,"access_token":"secret"},"content":{"items":[{"addedAt":{"isoString":"2020-01-01"},"itemV2":{"data":{"__typename":"Track","uri":"spotify:track:` + track + `"}}},{"itemV2":{"data":{"__typename":"NotFound"}}},{"itemV2":{"data":{"__typename":"Track","uri":"spotify:track:` + track + `"}}}]}}}}`)
	entities, err := CaptureDomain("playlist", map[string]any{"operationName": "fetchPlaylist", "variables": map[string]any{"uri": "spotify:playlist:" + id, "offset": 5, "limit": 3}}, raw)
	if err != nil {
		t.Fatal(err)
	}
	var parent *CapturedEntity
	for i := range entities {
		if entities[i].EntityType == "playlist" {
			parent = &entities[i]
		}
	}
	if parent == nil || len(parent.Relations) != 3 || parent.Relations[0].Position != 5 || parent.Relations[2].Position != 7 || !parent.Relations[1].Unavailable || parent.Relations[0].ChildID != parent.Relations[2].ChildID {
		t.Fatalf("positions lost: %+v", parent)
	}
	if bytes.Contains(parent.Payload, []byte("secret")) || !bytes.Contains(parent.Payload, []byte(`"value":false`)) || !bytes.Contains(parent.Relations[0].Metadata, []byte("2020-01-01")) {
		t.Fatal("snapshot retention invalid")
	}
}
func TestCaptureIdentityAndSensitiveOperations(t *testing.T) {
	id := strings.Repeat("T", 22)
	for _, stage := range []string{"client_token", "profile"} {
		got, err := CaptureDomain(stage, nil, []byte(`{"token":"private"}`))
		if err != nil || len(got) != 0 {
			t.Fatal("sensitive operation captured")
		}
	}
	raw := []byte(`{"data":{"trackUnion":{"__typename":"Track","uri":"spotify:track:` + id + `"}}}`)
	if _, err := CaptureDomain("track", map[string]any{"variables": map[string]any{"uri": "spotify:track:" + strings.Repeat("A", 22)}}, raw); err == nil {
		t.Fatal("mismatched root captured")
	}
	if _, err := CaptureDomain("track", nil, []byte(`{"errors":[{}],"data":{}}`)); err == nil {
		t.Fatal("error envelope captured")
	}
}

func TestCaptureRetainsUnplayableFactsAndLibraryRows(t *testing.T) {
	id := strings.Repeat("T", 22)
	raw := []byte(`{"data":{"trackUnion":{"__typename":"Track","uri":"spotify:track:` + id + `","playability":{"playable":false},"unknown":0}}}`)
	entities, err := CaptureDomain("track", map[string]any{"operationName": "getTrack", "variables": map[string]any{"uri": "spotify:track:" + id}}, raw)
	if err != nil || len(entities) != 1 || !bytes.Contains(entities[0].Payload, []byte(`"playable":false`)) {
		t.Fatalf("provider false fact lost: %+v %v", entities, err)
	}
	raw = []byte(`{"data":{"me":{"libraryV3":{"items":[{"addedAt":{"isoString":"2020"},"item":{"_uri":"spotify:album:` + id + `","data":{"__typename":"Album"}}}]}}}}`)
	entities, err = CaptureDomain("library", map[string]any{"operationName": "libraryV3", "variables": map[string]any{"filters": []string{"Albums"}, "offset": 10, "limit": 1}}, raw)
	if err != nil || len(entities) < 1 || entities[0].EntityType != "library" || entities[0].Relations[0].Position != 10 || entities[0].Relations[0].ChildID != id {
		t.Fatalf("saved library relation lost: %+v %v", entities, err)
	}
}

func TestCaptureDomainMergesDuplicateTrackRelationCoverage(t *testing.T) {
	playlistID := strings.Repeat("P", 22)
	trackID := strings.Repeat("T", 22)
	artistID := strings.Repeat("A", 22)
	raw := []byte(`{"data":{"playlistV2":{"__typename":"Playlist","uri":"spotify:playlist:` + playlistID + `","content":{"items":[{"itemV2":{"data":{"__typename":"Track","uri":"spotify:track:` + trackID + `","artists":{"items":[{"__typename":"Artist","uri":"spotify:artist:` + artistID + `"}]}}}},{"itemV2":{"data":{"__typename":"Track","uri":"spotify:track:` + trackID + `","opaque_extra":"` + strings.Repeat("x", 1024) + `"}}}]}}}}`)
	entities, err := CaptureDomain("playlist", map[string]any{"operationName": "getPlaylist", "variables": map[string]any{"uri": "spotify:playlist:" + playlistID}}, raw)
	if err != nil {
		t.Fatal(err)
	}
	var track *CapturedEntity
	for i := range entities {
		if entities[i].EntityType == "track" && entities[i].ID == trackID {
			track = &entities[i]
		}
	}
	if track == nil || len(track.Relations) != 1 || track.Relations[0].Kind != "artists" || track.Relations[0].ChildType != "artist" || track.Relations[0].ChildID != artistID || track.Relations[0].Unavailable {
		t.Fatalf("duplicate GraphQL projection dropped artist edge: %+v", entities)
	}
}
