package db

import (
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"testing"
	"time"
)

func TestSpotifyFieldLastGoodFreshnessAndOwnership(t *testing.T) {
	d, _ := evidenceFixture(t)
	o := referenceObservation()
	o.AccountContext = "owner"
	if err := d.ActivateSpotifyMetadataContext(o.AccountContext); err != nil {
		t.Fatal(err)
	}
	first := o.RetrievedAt
	if err := d.PutExternalAnalysis(o, "first", first.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	o.RetrievedAt = first.Add(2 * time.Hour)
	o.Energy = nil
	o.RejectedFields = []spotifyanalysis.FieldRejection{{Path: "energy", Reason: "out_of_range"}}
	o.Valence = nil
	o.BPM = scalarPtr(125.0)
	if err := d.PutExternalAnalysis(o, "second", o.RetrievedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	fields, err := d.GetSpotifyScalarFields(referenceID, o.RetrievedAt)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]SpotifyScalarField{}
	for _, f := range fields {
		seen[f.Key] = f
	}
	energy := seen["spotify_energy_score"]
	tempo := seen["tempo_bpm"]
	if string(energy.Value) != "0" || !energy.Stale || !energy.RetrievedAt.Equal(first) || string(tempo.Value) != "125" || tempo.Stale {
		t.Fatalf("field freshness/last-good: %+v", fields)
	}
	attempts, err := d.GetSpotifyFieldAttempts(referenceID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]SpotifyFieldAttempt{}
	for _, attempt := range attempts {
		states[attempt.Key] = attempt
	}
	if states["spotify_energy_score"].State != "invalid_field" || states["spotify_energy_score"].Reason != "out_of_range" || states["spotify_valence_score"].State != "not_returned" || states["tempo_bpm"].State != "available" {
		t.Fatalf("independent attempts: %+v", attempts)
	}
	if err := d.ActivateSpotifyMetadataContext("replacement"); err != nil {
		t.Fatal(err)
	}
	fields, err = d.GetSpotifyScalarFields(referenceID, o.RetrievedAt)
	if err != nil || len(fields) != 0 {
		t.Fatal("retired owner leaked fields")
	}
	attempts, err = d.GetSpotifyFieldAttempts(referenceID)
	if err != nil || len(attempts) != 0 {
		t.Fatal("retired attempts leaked")
	}
	if err := d.PurgeExternalAnalysis(); err != nil {
		t.Fatal(err)
	}
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	fields, err = d.GetSpotifyScalarFields(referenceID, o.RetrievedAt)
	if err != nil || len(fields) != 0 {
		t.Fatal("purge retained fields")
	}
	attempts, err = d.GetSpotifyFieldAttempts(referenceID)
	if err != nil || len(attempts) != 0 {
		t.Fatal("purge retained attempts")
	}
}
