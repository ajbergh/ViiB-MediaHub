package analysis

import (
	"encoding/json"
	"math"
	"testing"
)

func TestProviderScoresRetainedWithoutBPMOrKey(t *testing.T) {
	got, err := decodeFixture(`{"track":{"energy":0,"danceability":1,"acousticness":0.2,"instrumentalness":0.3,"liveness":0.4,"speechiness":0.5,"valence":0.6}}`)
	if err != nil {
		t.Fatal(err)
	}
	values := []*float64{got.Energy, got.Danceability, got.Acousticness, got.Instrumentalness, got.Liveness, got.Speechiness, got.Valence}
	for i, want := range []float64{0, 1, 0.2, 0.3, 0.4, 0.5, 0.6} {
		if values[i] == nil || *values[i] != want {
			t.Fatalf("score %d lost", i)
		}
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var restored Observation
	if err = json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if err = ValidateObservation(restored); err != nil {
		t.Fatal(err)
	}
	if restored.Energy == nil || *restored.Energy != 0 || restored.BPM != nil {
		t.Fatal("nullable scores did not round trip")
	}
}

func TestProviderScoreValidation(t *testing.T) {
	for _, name := range []string{"energy", "danceability", "acousticness", "instrumentalness", "liveness", "speechiness", "valence"} {
		for _, value := range []string{"-0.01", "1.01", "\"bad\""} {
			got, err := decodeFixture(`{"track":{"tempo":120,"` + name + `":` + value + `}}`)
			if err != nil || got.BPM == nil || len(got.RejectedFields) != 1 {
				t.Fatalf("valid sibling lost: %+v %v", got, err)
			}
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), -1, 2} {
		if ValidateObservation(Observation{Energy: &value}) == nil {
			t.Fatal("invalid cached score accepted")
		}
	}
	_, err := decodeFixture(`{"track":{"energy":null}}`)
	assertCode(t, err, AnalysisUnavailable)
}
