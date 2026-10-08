package db

import (
	"encoding/json"
	"testing"
	"time"
)

func TestProviderScalarResolutionRejectsNullAndUnknownSemantics(t *testing.T) {
	for _, candidate := range []SpotifyScalarField{
		{Key: "spotify_energy_score", Metric: "spotify_energy", Units: "unit_interval", Value: json.RawMessage(" null ")},
		{Key: "key_mode", Metric: "tonic_and_mode", Units: "pitch_class_and_mode", Value: json.RawMessage(`{"tonic":null,"mode":1}`)},
		{Key: "key_mode", Metric: "tonic_and_mode", Units: "pitch_class_and_mode", Value: json.RawMessage(`{"tonic":0,"mode":null}`)},
		{Key: "unknown", Metric: "unknown", Units: "unknown", Value: json.RawMessage("0")},
	} {
		if got := SelectProviderScalarFields([]SpotifyScalarField{candidate}); len(got) != 0 {
			t.Fatalf("accepted invalid candidate: %+v", candidate)
		}
	}
}

func TestProviderScalarResolutionRetainsZeroAndOrdersPrivateAndDurable(t *testing.T) {
	at := time.Now()
	fresh := SpotifyScalarField{Key: "spotify_energy_score", Metric: "spotify_energy", Units: "unit_interval", Value: json.RawMessage("0"), RetrievedAt: at, Endpoint: "audio_features", DurableImport: true}
	stale := fresh
	stale.Stale = true
	stale.RetrievedAt = at.Add(time.Hour)
	stale.DurableImport = false
	got := SelectProviderScalarFields([]SpotifyScalarField{stale, fresh})
	if len(got) != 1 || !got[0].DurableImport || string(got[0].Value) != "0" {
		t.Fatalf("fresh zero lost: %+v", got)
	}
	detailed := fresh
	detailed.Endpoint = "audio_analysis"
	detailed.DurableImport = false
	for _, fields := range [][]SpotifyScalarField{{fresh, detailed}, {detailed, fresh}} {
		got = SelectProviderScalarFields(fields)
		if len(got) != 1 || got[0].Endpoint != "audio_analysis" {
			t.Fatalf("tie ordering: %+v", got)
		}
	}
}
