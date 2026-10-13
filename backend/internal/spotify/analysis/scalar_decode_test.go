package analysis

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPartialScalarResponsesPreserveUsableSiblings(t *testing.T) {
	for _, invalid := range []string{`"tempo":"wrong"`, `"key":12`, `"mode":2`, `"time_signature":0`, `"loudness":"wrong"`, `"duration":-1`, `"tempo_confidence":1.2`, `"key_confidence":-1`, `"mode_confidence":true`, `"time_signature_confidence":[]`} {
		got, err := decodeFixture(`{"track":{"energy":0,` + invalid + `}}`)
		if err != nil || got.Energy == nil || *got.Energy != 0 || len(got.RejectedFields) != 1 {
			t.Fatalf("%s: %+v %v", invalid, got, err)
		}
		if err = ValidateObservation(got); err != nil {
			t.Fatalf("normalized partial result invalid: %v", err)
		}
	}
}

func TestFeaturePartialDurationAndIdentity(t *testing.T) {
	for _, duration := range []string{"123456.789", "-1", "\"bad\""} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"id":%q,"energy":0,"key":99,"duration_ms":%s}`, testID, duration)
		}))
		var refreshes atomic.Int32
		got, err := NewClient(testTokens(&refreshes), Options{Enabled: true, AppVersion: "fixture", Client: testHTTP(server)}).FetchFeatures(context.Background(), testID)
		server.Close()
		if err != nil || got.Energy == nil || got.Key != nil {
			t.Fatalf("usable score lost: %+v %v", got, err)
		}
		if duration == "123456.789" {
			if got.DurationMilliseconds == nil || *got.DurationMilliseconds != 123456.789 || got.DurationSeconds == nil || *got.DurationSeconds != 123.456789 {
				t.Fatal("original duration units lost")
			}
		} else if got.DurationMilliseconds != nil || got.DurationSeconds != nil || len(got.RejectedFields) != 2 {
			t.Fatal("invalid duration retained")
		}
		if err = ValidateObservation(got); err != nil {
			t.Fatal(err)
		}
	}
}
