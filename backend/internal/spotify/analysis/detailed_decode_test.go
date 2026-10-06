package analysis

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDetailedArtifactsRetainFullObjectsAndIsolateArrays(t *testing.T) {
	body := `{"meta":{"analyzer_version":"fixture","unknown":false,"token":"secret"},"track":{"tempo":120,"duration":10,"sample_count":441000},"beats":[{"start":0,"duration":1,"confidence":0}],"sections":[{"start":0,"duration":10,"tempo":120,"key":0,"mode":0,"unknown":7}],"segments":[{"start":0,"duration":1,"loudness_start":-10,"loudness_max":-5,"loudness_end":-9,"loudness_max_time":0.2,"pitches":[0,0,0,0,0,0,0,0,0,0,0,0],"timbre":[0,0,0,0,0,0,0,0,0,0,0,0]}]}`
	got, err := decodeFixture(body)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(got.DomainPayload)
	for _, field := range []string{"sample_count", "loudness_start", "loudness_max_time", "pitches", "timbre", `"unknown":false`, `"unknown":7`} {
		if !strings.Contains(raw, field) {
			t.Fatalf("lost %s", field)
		}
	}
	if strings.Contains(raw, "secret") || len(got.ArtifactCapabilities) != 3 {
		t.Fatal("artifact sanitization invalid")
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "pitches") {
		t.Fatal("arrays leaked into scalar projection")
	}
	broken := strings.Replace(body, `"start":0,"duration":1,"confidence":0`, `"start":9,"duration":2,"confidence":0`, 1)
	got, err = decodeFixture(broken)
	if err != nil || got.BPM == nil || len(got.ArtifactCapabilities) != 2 || len(got.RejectedFields) != 1 || got.RejectedFields[0].Path != "beats" {
		t.Fatalf("siblings lost: %+v %v", got, err)
	}
	if strings.Contains(string(got.DomainPayload), `"beats"`) {
		t.Fatal("invalid array retained")
	}
}
func TestDetailedOnlyResponseIsUsable(t *testing.T) {
	got, err := decodeFixture(`{"track":{},"beats":[{"start":0,"duration":1}]}`)
	if err != nil || got.BPM != nil || len(got.ArtifactCapabilities) != 1 {
		t.Fatalf("detailed-only lost: %+v %v", got, err)
	}
	if err = ValidateObservation(got); err != nil {
		t.Fatal(err)
	}
}

func TestDetailedIdentityMismatchIsFatal(t *testing.T) {
	_, err := decodeFixture(`{"track":{"id":"different","tempo":120},"beats":[{"start":0,"duration":1}]}`)
	assertCode(t, err, ProviderChanged)
}
