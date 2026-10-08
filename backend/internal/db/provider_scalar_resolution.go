package db

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
)

// Selection compares candidates only for the same registered metric and units.
// Freshness wins over recency; stale immutable recording facts remain explicitly
// stale last-good candidates. Equal retrievals prefer detailed analysis for its
// analysis context. All alternatives remain in Fields.
func SelectProviderScalarFields(fields []SpotifyScalarField) []SpotifyScalarField {
	selected := map[string]SpotifyScalarField{}
	for _, candidate := range fields {
		if !validProviderScalarCandidate(candidate) {
			continue
		}
		previous, exists := selected[candidate.Key]
		if exists && !sameProviderScalarSemantics(candidate, previous) {
			continue
		}
		better := !exists || betterProviderScalarCandidate(candidate, previous)
		if better {
			selected[candidate.Key] = candidate
		}
	}
	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]SpotifyScalarField, 0, len(keys))
	for _, key := range keys {
		result = append(result, selected[key])
	}
	return result
}

func sameProviderScalarSemantics(left, right SpotifyScalarField) bool {
	return left.Metric == right.Metric && left.Units == right.Units
}

func validProviderScalarCandidate(candidate SpotifyScalarField) bool {
	if candidate.Key == "" || candidate.Metric == "" || candidate.Units == "" || !json.Valid(candidate.Value) || strings.TrimSpace(string(candidate.Value)) == "null" {
		return false
	}
	if !providerScalarValueMatches(candidate.Key, candidate.Value) {
		return false
	}
	wantMetric, wantUnits, known := providerScalarSemantics(candidate.Key)
	return known && candidate.Metric == wantMetric && candidate.Units == wantUnits
}

func providerScalarSemantics(key string) (metric, units string, known bool) {
	switch key {
	case "tempo_bpm":
		return "tempo", "bpm", true
	case "key_mode":
		return "tonic_and_mode", "pitch_class_and_mode", true
	case "provider_loudness_db":
		return "spotify_track_loudness", "dB", true
	case "time_signature":
		return "measured_meter", "beats_per_bar", true
	case "duration_seconds":
		return "recording_duration", "seconds", true
	case "duration_milliseconds":
		return "recording_duration", "milliseconds", true
	case "spotify_energy_score":
		return "spotify_energy", "unit_interval", true
	case "spotify_danceability_score":
		return "spotify_danceability", "unit_interval", true
	case "spotify_acousticness_score":
		return "spotify_acousticness", "unit_interval", true
	case "spotify_instrumentalness_score":
		return "spotify_instrumentalness", "unit_interval", true
	case "spotify_liveness_score":
		return "spotify_liveness", "unit_interval", true
	case "spotify_speechiness_score":
		return "spotify_speechiness", "unit_interval", true
	case "spotify_valence_score":
		return "spotify_valence", "unit_interval", true
	default:
		return "", "", false
	}
}

func providerScalarValueMatches(key string, raw json.RawMessage) bool {
	var number float64
	switch key {
	case "key_mode":
		var value struct {
			Tonic int `json:"tonic"`
			Mode  int `json:"mode"`
		}
		if !hasJSONObjectFields(raw, "tonic", "mode") || json.Unmarshal(raw, &value) != nil || value.Tonic < 0 || value.Tonic > 11 || value.Mode < 0 || value.Mode > 1 {
			return false
		}
		return true
	case "tempo_bpm", "duration_seconds", "duration_milliseconds":
		return decodeFiniteScalar(raw, &number) && number > 0
	case "time_signature":
		var value int
		return json.Unmarshal(raw, &value) == nil && value > 0 && value <= 32
	case "provider_loudness_db":
		return decodeFiniteScalar(raw, &number)
	case "spotify_energy_score", "spotify_danceability_score", "spotify_acousticness_score", "spotify_instrumentalness_score", "spotify_liveness_score", "spotify_speechiness_score", "spotify_valence_score":
		return decodeFiniteScalar(raw, &number) && number >= 0 && number <= 1
	default:
		return true
	}
}

func decodeFiniteScalar(raw json.RawMessage, value *float64) bool {
	if json.Unmarshal(raw, value) != nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return false
	}
	return true
}

func hasJSONObjectFields(raw json.RawMessage, required ...string) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return false
	}
	for _, key := range required {
		if value, ok := object[key]; !ok || strings.TrimSpace(string(value)) == "null" {
			return false
		}
	}
	return true
}

func betterProviderScalarCandidate(candidate, previous SpotifyScalarField) bool {
	if previous.Stale != candidate.Stale {
		return !candidate.Stale
	}
	return candidate.RetrievedAt.After(previous.RetrievedAt) || (candidate.RetrievedAt.Equal(previous.RetrievedAt) && candidate.Endpoint == "audio_analysis" && previous.Endpoint != "audio_analysis")
}
