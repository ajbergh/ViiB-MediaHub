package api

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// DJ queue metadata and sequencing must use the same admitted observations.
// This is a bounded scalar projection; it opens no audio or analysis artifacts.
func (a *API) resolveDJQueueAnalysis(fingerprints map[string]string, provider map[string][]db.SpotifyScalarField) (map[string]TrackAnalysisFeatureResponse, map[string]int, error) {
	analyses, err := a.db.ListTrackAnalysis()
	if err != nil {
		return nil, nil, err
	}
	overrides, err := a.db.ListTrackAnalysisOverrides()
	if err != nil {
		return nil, nil, err
	}
	manualFields, err := a.db.ListTrackMetadataOverrides()
	if err != nil {
		return nil, nil, err
	}
	for id, fields := range manualFields {
		override := overrides[id]
		override.Fields = fields
		overrides[id] = override
	}
	byID := make(map[string]db.TrackAnalysis, len(analyses))
	for _, row := range analyses {
		byID[row.SongID] = row
	}
	features := make(map[string]TrackAnalysisFeatureResponse, len(fingerprints))
	bpms := make(map[string]int, len(fingerprints))
	for id, fingerprint := range fingerprints {
		row := byID[id]
		row.SongID = id
		if row.Status == "" {
			row.Status = db.TrackAnalysisPending
		}
		feature := trackAnalysisFeatureResponseWithCurrentSource(row, overrides[id], fingerprint)
		feature.EffectiveFields = db.ResolveAnalysisScalarFields(row, overrides[id], fingerprint, provider[id])
		applyEffectiveScalarCompatibilityFields(&feature, feature.EffectiveFields)
		features[id] = feature
		if feature.BPM != nil && *feature.BPM > 0 && !math.IsNaN(*feature.BPM) && !math.IsInf(*feature.BPM, 0) {
			bpms[id] = int(math.Round(*feature.BPM))
		}
	}
	return features, bpms, nil
}

func sameDJQueueAnalysis(first, second TrackAnalysisFeatureResponse) bool {
	a, errA := json.Marshal(first)
	b, errB := json.Marshal(second)
	return errA == nil && errB == nil && string(a) == string(b)
}

type djQueueSong struct {
	db.Song
	Analysis TrackAnalysisFeatureResponse `json:"analysis"`
}

func (a *API) validateDJQueuePublication(fingerprints map[string]string, provider map[string][]db.SpotifyScalarField, queue map[string]TrackAnalysisFeatureResponse, bpms map[string]int) error {
	ids := make(map[string]struct{}, len(fingerprints))
	for id := range fingerprints {
		ids[id] = struct{}{}
	}
	latest, err := a.currentAnalysisSourceFingerprints(sortedSongIDs(ids))
	if err != nil {
		return err
	}
	for id, fingerprint := range fingerprints {
		if latest[id] != fingerprint {
			return fmt.Errorf("DJ candidate source changed during assembly")
		}
	}
	validated := provider
	if len(provider) > 0 {
		validated, err = a.db.RevalidateSpotifyScalarCandidateBatch(fingerprints, provider)
		if err != nil {
			return err
		}
	}
	features, currentBPM, err := a.resolveDJQueueAnalysis(fingerprints, validated)
	if err != nil {
		return err
	}
	if len(currentBPM) != len(bpms) {
		return fmt.Errorf("DJ effective tempo changed during assembly")
	}
	for id, bpm := range bpms {
		if currentBPM[id] != bpm {
			return fmt.Errorf("DJ effective tempo changed during assembly")
		}
	}
	for id, feature := range queue {
		if !sameDJQueueAnalysis(feature, features[id]) {
			return fmt.Errorf("DJ queue analysis changed during assembly")
		}
	}
	return nil
}
