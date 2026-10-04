package analysis

import "errors"

// ValidateObservation checks normalized cache/import values using the same
// scalar rules as the network adapter, including canonical musical key labels.
func ValidateObservation(o Observation) error {
	if len(o.AnalyzerVersion) > 128 || (o.Key != nil && *o.Key < 0) {
		return errors.New("invalid normalized observation")
	}
	normalized, err := normalize(o.TrackID, payload{Track: &track{
		Tempo: o.BPM, TempoConfidence: o.BPMConfidence, Key: o.Key, KeyConfidence: o.KeyConfidence,
		Mode: o.Mode, ModeConfidence: o.ModeConfidence, Loudness: o.LoudnessDB,
		TimeSignature: o.TimeSignature, TimeSignatureConfidence: o.TimeSignatureConfidence, Duration: o.DurationSeconds,
	}}, o.RetrievedAt)
	if err != nil {
		return err
	}
	if (normalized.Camelot == nil) != (o.Camelot == nil) || (normalized.Camelot != nil && *normalized.Camelot != *o.Camelot) {
		return errors.New("invalid normalized musical key")
	}
	return nil
}
