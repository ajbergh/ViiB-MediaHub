package analysis

import (
	"fmt"
	"math"
	"time"
)

func finite(value *float64) bool {
	return value == nil || (!math.IsNaN(*value) && !math.IsInf(*value, 0))
}
func confidence(value *float64) bool {
	return finite(value) && (value == nil || (*value >= 0 && *value <= 1))
}

func normalize(id string, data payload, now time.Time) (Observation, error) {
	t := data.Track
	if t == nil {
		return Observation{}, failure(ProviderChanged, 0)
	}
	if !finite(t.Tempo) || (t.Tempo != nil && *t.Tempo <= 0) ||
		!finite(t.Duration) || (t.Duration != nil && *t.Duration <= 0) ||
		!finite(t.Loudness) || !confidence(t.TempoConfidence) || !confidence(t.KeyConfidence) ||
		!confidence(t.ModeConfidence) || !confidence(t.TimeSignatureConfidence) ||
		(t.Key != nil && (*t.Key < -1 || *t.Key > 11)) ||
		(t.Mode != nil && (*t.Mode < 0 || *t.Mode > 1)) ||
		(t.TimeSignature != nil && *t.TimeSignature <= 0) {
		return Observation{}, failure(ProviderChanged, 0)
	}
	// An unknown pitch class must not become C through Go's numeric zero value.
	if t.Key != nil && *t.Key == -1 {
		t.Key = nil
	}
	if t.Tempo == nil && (t.Key == nil || t.Mode == nil) {
		return Observation{}, failure(AnalysisUnavailable, 0)
	}
	for _, intervals := range [][]interval{data.Bars, data.Beats, data.Tatums, data.Sections} {
		if len(intervals) > 20000 {
			return Observation{}, failure(ProviderChanged, 0)
		}
		for _, value := range intervals {
			if !validInterval(value, t.Duration) {
				return Observation{}, failure(ProviderChanged, 0)
			}
		}
	}
	if len(data.Segments) > 40000 {
		return Observation{}, failure(ProviderChanged, 0)
	}
	for _, value := range data.Segments {
		if !validInterval(value.interval, t.Duration) {
			return Observation{}, failure(ProviderChanged, 0)
		}
		for _, vector := range [][]float64{value.Pitches, value.Timbre} {
			if vector != nil && len(vector) != 12 {
				return Observation{}, failure(ProviderChanged, 0)
			}
			for _, number := range vector {
				if !finite(&number) {
					return Observation{}, failure(ProviderChanged, 0)
				}
			}
		}
	}
	observation := Observation{
		SourceEndpoint: "audio_analysis",
		TrackID:        id, Source: "spotify_internal", RetrievedAt: now.UTC(), AnalyzerVersion: data.Meta.AnalyzerVersion,
		BPM: t.Tempo, BPMConfidence: t.TempoConfidence, Key: t.Key, KeyConfidence: t.KeyConfidence,
		Mode: t.Mode, ModeConfidence: t.ModeConfidence, LoudnessDB: t.Loudness,
		TimeSignature: t.TimeSignature, TimeSignatureConfidence: t.TimeSignatureConfidence, DurationSeconds: t.Duration,
	}
	if t.Key != nil && t.Mode != nil {
		// Keep scalar notation independent of the local DSP/database service.
		tonic := *t.Key
		suffix := "B"
		if *t.Mode == 0 {
			tonic = (tonic + 3) % 12
			suffix = "A"
		}
		camelot := fmt.Sprintf("%d%s", ((tonic*7+7)%12)+1, suffix)
		observation.Camelot = &camelot
	}
	return observation, nil
}

func validInterval(value interval, duration *float64) bool {
	if value.Start == nil || value.Duration == nil || !finite(value.Start) || !finite(value.Duration) ||
		*value.Start < 0 || *value.Duration < 0 || !confidence(value.Confidence) {
		return false
	}
	end := *value.Start + *value.Duration
	if math.IsInf(end, 0) {
		return false
	}
	// A small tolerance accommodates the provider's rounded interval times.
	return duration == nil || end <= *duration+0.001
}
