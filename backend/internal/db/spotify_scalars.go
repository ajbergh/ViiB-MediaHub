package db

import (
	"fmt"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

// ApplySpotifyScalars replaces only dimensions present in the provider result.
// Scalar features have no confidence or tempo stability; keep those unknown.
func ApplySpotifyScalars(record *TrackAnalysis, o spotifyanalysis.Observation) {
	if o.BPM != nil {
		record.BPM = o.BPM
		record.BPMSource = scalarPtr("spotify")
		record.BPMConfidence = o.BPMConfidence
		record.BPMAltCandidate = nil
		record.TempoStability = nil
		record.TempoKind = scalarPtr("unknown")
	}
	if o.Key != nil && o.Mode != nil {
		mode := "major"
		tonic := *o.Key
		suffix, openSuffix := "B", "d"
		if *o.Mode == 0 {
			mode = "minor"
			tonic = (tonic + 3) % 12
			suffix, openSuffix = "A", "m"
		}
		record.KeyTonic = o.Key
		record.KeyMode = &mode
		record.KeySource = scalarPtr("spotify")
		record.KeyConfidence = o.KeyConfidence
		record.CamelotKey = scalarPtr(fmt.Sprintf("%d%s", ((tonic*7+7)%12)+1, suffix))
		record.OpenKey = scalarPtr(fmt.Sprintf("%d%s", ((tonic*7)%12)+1, openSuffix))
	}
}

func scalarPtr[T any](v T) *T { return &v }
