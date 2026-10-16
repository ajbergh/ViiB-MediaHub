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

// PreferAudioAttributesV2 overlays only the scalar dimensions supplied by V2.
// The original observations and their per-field origins remain intact.
func PreferAudioAttributesV2(base, v2 *Observation) *Observation {
	if v2 == nil {
		return base
	}
	if base == nil {
		result := *v2
		return &result
	}
	if base.TrackID != v2.TrackID || (base.AccountContext != v2.AccountContext && !base.DurableImport && !v2.DurableImport) {
		return base
	}
	result := *base
	bpmOrigin := v2.BPMOrigin
	if bpmOrigin == nil {
		bpmOrigin = v2
	}
	currentBPM := base.BPMOrigin
	if currentBPM == nil {
		currentBPM = base
	}
	if v2.BPM != nil && bpmOrigin.SourceEndpoint == AttributesV2 && !(currentBPM.SourceEndpoint == AttributesV2 && currentBPM.RetrievedAt.After(bpmOrigin.RetrievedAt)) {
		result.BPM, result.BPMConfidence, result.BPMRetained = v2.BPM, v2.BPMConfidence, v2.BPMRetained
		result.BPMOrigin = v2.BPMOrigin
		if result.BPMOrigin == nil {
			result.BPMOrigin = v2
		}
	}
	keyOrigin := v2.KeyOrigin
	if keyOrigin == nil {
		keyOrigin = v2
	}
	currentKey := base.KeyOrigin
	if currentKey == nil {
		currentKey = base
	}
	if v2.Key != nil && v2.Mode != nil && keyOrigin.SourceEndpoint == AttributesV2 && !(currentKey.SourceEndpoint == AttributesV2 && currentKey.RetrievedAt.After(keyOrigin.RetrievedAt)) {
		result.Key, result.Mode, result.Camelot = v2.Key, v2.Mode, v2.Camelot
		result.KeyConfidence, result.ModeConfidence, result.KeyRetained = v2.KeyConfidence, v2.ModeConfidence, v2.KeyRetained
		result.KeyOrigin = v2.KeyOrigin
		if result.KeyOrigin == nil {
			result.KeyOrigin = v2
		}
	}
	return &result
}
