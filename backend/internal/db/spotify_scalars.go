// Defines spotify scalars functionality for package db.

package db

import (
	"fmt"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

// ApplySpotifyScalars replaces only dimensions present in the provider result.
// Scalar features have no confidence or tempo stability; keep those unknown.
func ApplySpotifyScalars(record *TrackAnalysis, o spotifyanalysis.Observation) {
	if record.SpotifyBindings != nil {
		copy := *record.SpotifyBindings
		record.SpotifyBindings = &copy
	}
	if record.SpotifyBindings != nil {
		if b := record.SpotifyBindings.BPM; b != nil && !b.Durable && o.BPM == nil && (!b.Eligible || (o.AccountContext != "" && o.AccountContext != b.AccountContext) || (o.TrackID != "" && o.TrackID != b.TrackID)) {
			record.BPM = nil
			record.BPMSource = nil
			record.BPMConfidence = nil
			record.BPMAltCandidate = nil
			record.TempoStability = nil
			record.TempoKind = nil
			if l := CurrentLocalScalars(record); l != nil && l.BPM != nil {
				record.BPM = l.BPM
				record.BPMSource = scalarPtr("measured")
				record.BPMConfidence = l.BPMConfidence
				record.BPMAltCandidate = l.BPMAltCandidate
				record.TempoStability = l.TempoStability
				record.TempoKind = l.TempoKind
			}
			record.SpotifyBindings.BPM = nil
		}
		if b := record.SpotifyBindings.Key; b != nil && !b.Durable && (o.Key == nil || o.Mode == nil) && (!b.Eligible || (o.AccountContext != "" && o.AccountContext != b.AccountContext) || (o.TrackID != "" && o.TrackID != b.TrackID)) {
			record.KeyTonic = nil
			record.KeyMode = nil
			record.KeySource = nil
			record.KeyConfidence = nil
			record.CamelotKey = nil
			record.OpenKey = nil
			if l := CurrentLocalScalars(record); l != nil && l.KeyTonic != nil {
				record.KeyTonic = l.KeyTonic
				record.KeyMode = l.KeyMode
				record.KeyConfidence = l.KeyConfidence
				record.KeySource = scalarPtr("measured")
			}
			record.SpotifyBindings.Key = nil
		}
	}
	bpmObservation, keyObservation := o, o
	if o.BPMOrigin != nil {
		bpmObservation = *o.BPMOrigin
	}
	if o.KeyOrigin != nil {
		keyObservation = *o.KeyOrigin
	}
	bpmBinding := bindingForObservation(record, bpmObservation)
	keyBinding := bindingForObservation(record, keyObservation)
	if (bpmBinding != nil || keyBinding != nil) && record.SpotifyBindings == nil {
		record.SpotifyBindings = &SpotifyScalarBindings{}
	}
	if o.BPM != nil {
		if record.SpotifyBindings != nil && !o.BPMRetained {
			record.SpotifyBindings.BPM = bpmBinding
		}
		record.BPM = o.BPM
		record.BPMSource = scalarPtr("spotify")
		record.BPMConfidence = o.BPMConfidence
		record.BPMAltCandidate = nil
		record.TempoStability = nil
		record.TempoKind = scalarPtr("unknown")
	}
	if o.Key != nil && o.Mode != nil {
		if record.SpotifyBindings != nil && !o.KeyRetained {
			record.SpotifyBindings.Key = keyBinding
		}
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
