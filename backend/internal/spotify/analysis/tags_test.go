// Tests and fixtures for tags behavior.

package analysis

import "testing"

func TestInitialKeyPreservesSpotifyModeAndUnknown(t *testing.T) {
	tonic, mode := 9, 0
	o := Observation{Key: &tonic, Mode: &mode}
	if o.InitialKey() != "Am" {
		t.Fatal(o.InitialKey())
	}
	tonic, mode = 0, 1
	if o.InitialKey() != "C" {
		t.Fatal(o.InitialKey())
	}
	o.Mode = nil
	if o.InitialKey() != "" {
		t.Fatal("unknown mode became major")
	}
}
