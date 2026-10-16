//go:build spotify_research

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

func TestMetadataOutput(t *testing.T) {
	const id = "11dFghVXANMlKmJXsNCbNl"
	bpm := 127.625
	var out bytes.Buffer
	err := writeMetadata(context.Background(), id, auth.PinnedWebPlayerContract(), func(_ context.Context, got string) (analysis.Observation, error) {
		if got != id {
			t.Fatalf("track ID = %q", got)
		}
		return analysis.Observation{TrackID: id, BPM: &bpm, SourceEndpoint: "audio_features", DomainPayload: []byte("private-payload"), AccountContext: "private-account"}, nil
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		CacheBypassed bool                 `json:"cacheBypassed"`
		Observation   analysis.Observation `json:"observation"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.CacheBypassed || result.Observation.BPM == nil || *result.Observation.BPM != bpm || result.Observation.SourceEndpoint != "audio_features" {
		t.Fatalf("unexpected metadata: %s", out.String())
	}
	if strings.Contains(out.String(), "private-") {
		t.Fatal("private fields leaked")
	}
}

func TestInputRejectedBeforeAuthentication(t *testing.T) {
	for _, args := range [][]string{nil, {"bad"}, {"11dFghVXANMlKmJXsNCbNl", "extra"}} {
		if err := run(context.Background(), args, io.Discard, io.Discard, ""); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestProviderFailureProducesNoSuccessJSON(t *testing.T) {
	var out bytes.Buffer
	err := writeMetadata(context.Background(), "11dFghVXANMlKmJXsNCbNl", auth.PinnedWebPlayerContract(), func(context.Context, string) (analysis.Observation, error) {
		return analysis.Observation{}, &analysis.Error{Code: analysis.RateLimited, HTTPStatus: http.StatusTooManyRequests}
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "429") || out.Len() != 0 {
		t.Fatalf("error=%v output=%s", err, out.String())
	}
}
