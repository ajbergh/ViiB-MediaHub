package analysisbench

import (
	"math"
	"testing"
)

func TestStructureResultRequiresValidInteriorBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		available  bool
		duration   float64
		boundaries []float64
		valid      bool
	}{
		{"legacy_missing", false, 0, nil, true},
		{"one_region", true, 10, nil, true},
		{"interior_regions", true, 10, []float64{2, 7}, true},
		{"missing_availability", false, 10, []float64{2}, false},
		{"missing_duration", true, 0, nil, false},
		{"nan_duration", true, math.NaN(), nil, false},
		{"nan_without_structure", false, math.NaN(), nil, false},
		{"start_is_not_interior", true, 10, []float64{0}, false},
		{"end_is_not_interior", true, 10, []float64{10}, false},
		{"duplicate", true, 10, []float64{2, 2}, false},
		{"unsorted", true, 10, []float64{7, 2}, false},
		{"nan_boundary", true, 10, []float64{math.NaN()}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ResultSet{Algorithm: "test", Results: []DetectorResult{{ID: "track", StructureAvailable: tc.available, AudioDurationSeconds: tc.duration, StructureBoundaries: tc.boundaries}}}
			if err := result.Validate(); (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
