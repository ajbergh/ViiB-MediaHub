// Validates normalized reference observations before persistence or reuse.
package analysis

import "errors"

// ValidateObservation checks normalized cache/import values using the same
// scalar rules as the network adapter, including canonical musical key labels.
func ValidateObservation(o Observation) error {
	if !finite(o.DurationMilliseconds) || (o.DurationMilliseconds != nil && (*o.DurationMilliseconds <= 0 || o.DurationSeconds == nil || *o.DurationSeconds != *o.DurationMilliseconds/1000)) {
		return errors.New("invalid duration units")
	}
	if len(o.RejectedFields) > 32 {
		return errors.New("too many rejected fields")
	}
	for _, rejection := range o.RejectedFields {
		if len(rejection.Path) > 128 || (rejection.Reason != "invalid_type" && rejection.Reason != "out_of_range" && rejection.Reason != "non_finite" && rejection.Reason != "invalid_artifact") {
			return errors.New("invalid field rejection")
		}
	}
	if len(o.AnalyzerVersion) > 128 || (o.Key != nil && *o.Key < 0) {
		return errors.New("invalid normalized observation")
	}
	normalized, err := normalize(o.TrackID, payload{Track: &track{
		Energy:           o.Energy,
		Danceability:     o.Danceability,
		Acousticness:     o.Acousticness,
		Instrumentalness: o.Instrumentalness,
		Liveness:         o.Liveness,
		Speechiness:      o.Speechiness,
		Valence:          o.Valence,
		Tempo:            o.BPM, TempoConfidence: o.BPMConfidence, Key: o.Key, KeyConfidence: o.KeyConfidence,
		Mode: o.Mode, ModeConfidence: o.ModeConfidence, Loudness: o.LoudnessDB,
		TimeSignature: o.TimeSignature, TimeSignatureConfidence: o.TimeSignatureConfidence, Duration: o.DurationSeconds,
	}}, o.RetrievedAt)
	if len(o.ArtifactCapabilities) > 5 {
		return errors.New("invalid artifact capabilities")
	}
	seen := map[string]bool{}
	for _, name := range o.ArtifactCapabilities {
		switch name {
		case "bars", "beats", "tatums", "sections", "segments":
		default:
			return errors.New("invalid artifact capability")
		}
		if o.SourceEndpoint != "audio_analysis" || seen[name] {
			return errors.New("invalid artifact capability provenance")
		}
		seen[name] = true
	}
	if err != nil {
		var diagnostic *Error
		if !(errors.As(err, &diagnostic) && diagnostic.Code == AnalysisUnavailable && len(o.ArtifactCapabilities) > 0) {
			return err
		}
	}
	if (normalized.Camelot == nil) != (o.Camelot == nil) || (normalized.Camelot != nil && *normalized.Camelot != *o.Camelot) {
		return errors.New("invalid normalized musical key")
	}
	return nil
}
