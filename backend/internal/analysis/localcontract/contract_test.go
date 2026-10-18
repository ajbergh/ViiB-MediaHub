package localcontract

import "testing"

func TestEventSuppressionPolicySurvivesVersionBumps(t *testing.T) {
	for _, v := range []string{"auto-cues-v2-events", "auto-cues-v2-events-r2", "auto-cues-v3-events", "auto-cues-v10"} {
		if !CueSuppressionIsSlotWide(v) {
			t.Fatalf("deletions lost at version %s", v)
		}
	}
	for _, v := range []string{"auto-cues-v1", "auto-cues-v1-revision", "unknown", "auto-cues-vinvalid"} {
		if CueSuppressionIsSlotWide(v) {
			t.Fatalf("legacy/unknown policy changed: %s", v)
		}
	}
}
