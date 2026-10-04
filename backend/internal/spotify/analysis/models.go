// Package analysis retrieves optional Spotify reference observations.
// It does not select effective local BPM/key or control DJ timing.
package analysis

import "time"

type track struct {
	Tempo                   *float64 `json:"tempo"`
	TempoConfidence         *float64 `json:"tempo_confidence"`
	Key                     *int     `json:"key"`
	KeyConfidence           *float64 `json:"key_confidence"`
	Mode                    *int     `json:"mode"`
	ModeConfidence          *float64 `json:"mode_confidence"`
	TimeSignature           *int     `json:"time_signature"`
	TimeSignatureConfidence *float64 `json:"time_signature_confidence"`
	Loudness                *float64 `json:"loudness"`
	Duration                *float64 `json:"duration"`
}
type interval struct {
	Start      *float64 `json:"start"`
	Duration   *float64 `json:"duration"`
	Confidence *float64 `json:"confidence"`
}
type segment struct {
	interval
	Pitches []float64 `json:"pitches"`
	Timbre  []float64 `json:"timbre"`
}
type payload struct {
	Track *track `json:"track"`
	Meta  struct {
		AnalyzerVersion string `json:"analyzer_version"`
	} `json:"meta"`
	Bars     []interval `json:"bars"`
	Beats    []interval `json:"beats"`
	Tatums   []interval `json:"tatums"`
	Sections []interval `json:"sections"`
	Segments []segment  `json:"segments"`
}

// Observation contains provider facts, never local effective values.
// Nullable fields distinguish unknown values from numeric zero.
type Observation struct {
	SourceEndpoint          string    `json:"sourceEndpoint"`
	TrackID                 string    `json:"trackId"`
	Source                  string    `json:"source"`
	RetrievedAt             time.Time `json:"retrievedAt"`
	AnalyzerVersion         string    `json:"analyzerVersion,omitempty"`
	BPM                     *float64  `json:"bpm"`
	BPMConfidence           *float64  `json:"bpmConfidence"`
	Key                     *int      `json:"key"`
	KeyConfidence           *float64  `json:"keyConfidence"`
	Mode                    *int      `json:"mode"`
	ModeConfidence          *float64  `json:"modeConfidence"`
	Camelot                 *string   `json:"camelot"`
	LoudnessDB              *float64  `json:"loudnessDb"`
	TimeSignature           *int      `json:"timeSignature"`
	TimeSignatureConfidence *float64  `json:"timeSignatureConfidence"`
	DurationSeconds         *float64  `json:"durationSeconds"`
}
