package features

import (
	"bytes"
	"compress/gzip"
	"math"
	"testing"
)

func TestCurrentEventContractRejectsMislabeledAndMalformedPayloads(t *testing.T) {
	for _, payload := range []string{
		`{"energy":[{"time":0,"value":0.5}]}`,
		`{"energy":[{"time":0,"value":0.5}],"eventVersion":"future","durationSeconds":20}`,
		`{"energy":[{"time":0,"value":0.5}],"eventVersion":"native-novelty-v1"}`,
		`{"energy":[{"time":0,"value":0.5}],"eventVersion":"native-novelty-v1","durationSeconds":-20}`,
		`{"energy":[{"time":0,"value":0.5}],"eventVersion":"native-novelty-v1","durationSeconds":20,"events":[{"position":21,"kind":"start","strength":0.7}]}`,
		`{"energy":[{"time":0,"value":0.5}],"eventVersion":"native-novelty-v1","durationSeconds":20,"events":[{"position":1,"kind":"chorus","strength":0.7}]}`,
	} {
		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		if _, err := w.Write([]byte(payload)); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeCurrentBounded(b.Bytes(), MaxStructureStatusArtifactBytes); err == nil {
			t.Fatalf("malformed current payload accepted: %s", payload)
		}
		if _, err := Decode(b.Bytes()); err != nil {
			t.Fatalf("compatibility decode changed: %v", err)
		}
	}
	for _, duration := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		r := Result{EventVersion: EventVersion, DurationSeconds: duration}
		if err := r.ValidateNativeEvents(); err == nil {
			t.Fatalf("bad decoded duration accepted: %v", duration)
		}
	}
}

func TestCurrentEventContractAllowsSilentAbstention(t *testing.T) {
	r := Result{EventVersion: EventVersion, DurationSeconds: 20, Energy: []EnergyPoint{{Time: 0, Value: 0}}}
	b, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCurrentBounded(b, MaxStructureStatusArtifactBytes); err != nil {
		t.Fatalf("silent abstention rejected: %v", err)
	}
}
