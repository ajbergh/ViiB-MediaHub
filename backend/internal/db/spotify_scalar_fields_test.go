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

func TestSpotifyPartialFieldAttemptMakesRetainedValueStaleImmediately(t *testing.T) {
	d, _ := evidenceFixture(t)
	o := referenceObservation()
	o.AccountContext = "owner"
	if err := d.ActivateSpotifyMetadataContext(o.AccountContext); err != nil {
		t.Fatal(err)
	}
	first := o.RetrievedAt
	if err := d.PutExternalAnalysis(o, "features-v1", first.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	otherEndpoint := o
	otherEndpoint.SourceEndpoint = "audio_analysis"
	otherEndpoint.BPM = scalarPtr(124.0)
	if err := d.PutExternalAnalysis(otherEndpoint, "analysis-v1", first.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	partialAt := first.Add(10 * time.Minute)
	o.RetrievedAt = partialAt
	o.BPM = nil
	o.Energy = scalarPtr(.75)
	if err := d.PutExternalAnalysis(o, "features-v2", partialAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	fields, err := d.GetSpotifyScalarFields(referenceID, partialAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	byEndpoint := map[string]map[string]SpotifyScalarField{}
	for _, field := range fields {
		if byEndpoint[field.Endpoint] == nil {
			byEndpoint[field.Endpoint] = map[string]SpotifyScalarField{}
		}
		byEndpoint[field.Endpoint][field.Key] = field
	}
	featuresBPM := byEndpoint["audio_features"]["tempo_bpm"]
	analysisBPM := byEndpoint["audio_analysis"]["tempo_bpm"]
	featuresEnergy := byEndpoint["audio_features"]["spotify_energy_score"]
	if !featuresBPM.Stale || featuresBPM.ExpiresAt.Before(partialAt.Add(time.Minute)) {
		t.Fatalf("retained feature BPM should be stale before its original TTL: %+v", featuresBPM)
	}
	if analysisBPM.Stale || string(analysisBPM.Value) != "124" {
		t.Fatalf("partial features response affected other endpoint: %+v", analysisBPM)
	}
	if featuresEnergy.Stale || string(featuresEnergy.Value) != "0.75" {
		t.Fatalf("returned sibling field was not refreshed: %+v", featuresEnergy)
	}

	local := ScalarCandidate{SpotifyScalarField: SpotifyScalarField{Key: "tempo_bpm", Metric: "tempo", Units: "bpm", Value: []byte("119"), AdapterRevision: "local-tempo-v1", RetrievedAt: partialAt}, Source: "local", SourceFingerprint: "source"}
	provider := ScalarCandidate{SpotifyScalarField: featuresBPM, Source: "spotify_private", SourceFingerprint: "source"}
	resolved := ResolveEffectiveScalar("tempo_bpm", "source", []ScalarCandidate{provider, local})
	if resolved.Selected == nil || resolved.Selected.Source != "local" || resolved.LastGood == nil || resolved.LastGood.Source != "spotify_private" {
		t.Fatalf("stale partial-response value suppressed local fallback: %+v", resolved)
	}

	// A late completion from before the partial response cannot make the old
	// endpoint value current again, even when the timestamps share a millisecond.
	late := o
	late.RetrievedAt = partialAt.Add(-time.Nanosecond)
	late.BPM = scalarPtr(130.0)
	if err := d.PutExternalAnalysis(late, "features-late", partialAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	fields, err = d.GetSpotifyScalarFields(referenceID, partialAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range fields {
		if field.Endpoint == "audio_features" && field.Key == "tempo_bpm" && !field.Stale {
			t.Fatalf("late older completion restored freshness: %+v", field)
		}
	}
}
