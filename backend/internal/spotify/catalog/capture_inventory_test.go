package catalog

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

func TestCaptureSharedInventoryFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../../../testdata/spotify/catalog-inventory-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name       string
		Transport  string
		EntityType string
		ID         string
		URL        string
		Stage      string
		Request    json.RawMessage
		Payload    json.RawMessage
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			var entities []CapturedEntity
			var err error
			if fixture.Transport == "rest" {
				target, parseErr := url.Parse(fixture.URL)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				entities, err = CaptureREST(target, fixture.Payload)
			} else {
				var request any
				if err := json.Unmarshal(fixture.Request, &request); err != nil {
					t.Fatal(err)
				}
				entities, err = CaptureDomain(fixture.Stage, request, fixture.Payload)
			}
			if err != nil {
				t.Fatal(err)
			}
			var root *CapturedEntity
			for i := range entities {
				if entities[i].EntityType == fixture.EntityType && entities[i].ID == fixture.ID {
					root = &entities[i]
					break
				}
			}
			if root == nil {
				t.Fatal("root not retained")
			}
			if bytes.Contains(root.Payload, []byte("synthetic-must-remove")) || !bytes.Contains(root.Payload, []byte(`"future_field":{"value":false}`)) {
				t.Fatalf("sensitive or unknown field handling: %s", root.Payload)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(root.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if fixture.Transport == "rest" {
				if string(payload["genres"]) != `["house","ambient"]` || string(payload["tags"]) != "[]" || string(payload["popularity"]) != "0" {
					t.Fatalf("REST presence lost: %s", root.Payload)
				}
			} else {
				if string(payload["genres"]) != "null" || string(payload["tags"]) != `["live",false,0]` || !bytes.Contains(payload["albumOfTrack"], []byte(`"isoString":null`)) || !bytes.Contains(payload["playability"], []byte(`"playable":false`)) {
					t.Fatalf("Web Player presence lost: %s", root.Payload)
				}
			}
		})
	}
}
