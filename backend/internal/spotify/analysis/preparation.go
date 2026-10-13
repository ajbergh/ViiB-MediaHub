package analysis

// PreparationScalars selects compatibility dimensions without changing either
// original endpoint observation. Key and mode always come from the same result.
// Callers must establish freshness, recording identity and account eligibility.
func PreparationScalars(features, detailed *Observation) *Observation {
	if features == nil {
		return detailed
	}
	result := *features
	if detailed == nil || features.TrackID != detailed.TrackID || features.AccountContext != detailed.AccountContext {
		return &result
	}
	if detailed.ProviderBeatGrid != nil {
		result.ProviderBeatGrid = detailed.ProviderBeatGrid
	}
	if result.BPM == nil && detailed.BPM != nil {
		result.BPM = detailed.BPM
		result.BPMConfidence = detailed.BPMConfidence
		result.BPMRetained = detailed.BPMRetained
		result.BPMOrigin = detailed
	}
	if (result.Key == nil || result.Mode == nil) && detailed.Key != nil && detailed.Mode != nil {
		result.Key, result.Mode, result.Camelot = detailed.Key, detailed.Mode, detailed.Camelot
		result.KeyConfidence, result.ModeConfidence = detailed.KeyConfidence, detailed.ModeConfidence
		result.KeyRetained = detailed.KeyRetained
		result.KeyOrigin = detailed
	}
	return &result
}
