// Package localcontract owns local estimator identities shared by analysis
// implementations and the capability registry without importing either.
package localcontract

import (
	"strconv"
	"strings"
)

const (
	TempoAlgorithmVersion     = "tempo-v3-multifeature-half-bpm"
	KeyAlgorithmVersion       = "key-v2-hpcp-ks"
	AmplitudeAlgorithmVersion = "amplitude-peaks-v1-bounded"
	ThreeBandAlgorithmVersion = "onepole-band-peaks-v1"
	CueGeneratorVersion       = "auto-cues-v2-events"
)

// Event selection changes the semantic role at a pad. A deletion made against
// the old percentage/role policy must not be undone by renaming the new event.
// Saving a manual cue or explicitly clearing suppression still restores a slot.
func CueSuppressionIsSlotWide(generatorVersion string) bool {
	// All numbered successors to the percentage-based v1 policy keep deleted
	// pads suppressed, including future algorithm/revision suffixes.
	if !strings.HasPrefix(generatorVersion, "auto-cues-v") {
		return false
	}
	major := strings.SplitN(strings.TrimPrefix(generatorVersion, "auto-cues-v"), "-", 2)[0]
	version, err := strconv.Atoi(major)
	return err == nil && version >= 2
}
