package analysisbench

import (
	"encoding/json"
	"fmt"
	"os"
)

// BenchmarkEnvironment captures portable run metadata required by roadmap
// §13.1. CPU model is optional because not every operating system exposes it.
type BenchmarkEnvironment struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	CPUModel     string `json:"cpuModel,omitempty"`
	CPUCount     int    `json:"cpuCount"`
	GoVersion    string `json:"goVersion"`
}

// ThroughputMetrics separates decode/downmix/stream work from DSP wall time.
// Bitrate is intentionally omitted: the shipped decoder interfaces do not
// expose a trustworthy container bitrate, so guessing would corrupt evidence.
type ThroughputMetrics struct {
	Environment            BenchmarkEnvironment `json:"environment"`
	Tracks                 []TrackTiming        `json:"tracks"`
	AudioSeconds           float64              `json:"audioSeconds"`
	WallSeconds            float64              `json:"wallSeconds"`
	DecodeAndStreamSeconds float64              `json:"decodeAndStreamSeconds"`
	DSPSeconds             float64              `json:"dspSeconds"`
	TotalRealtimeMultiple  *float64             `json:"totalRealtimeMultiple,omitempty"`
	DecodeRealtimeMultiple *float64             `json:"decodeRealtimeMultiple,omitempty"`
	DSPRealtimeMultiple    *float64             `json:"dspRealtimeMultiple,omitempty"`
}

// TrackTiming retains codec and decoded geometry alongside the separate timing
// totals, so decoder errors cannot be mistaken for detector errors.
type TrackTiming struct {
	ID                     string  `json:"id"`
	Codec                  string  `json:"codec"`
	SampleRate             int     `json:"sampleRate,omitempty"`
	SourceChannels         int     `json:"sourceChannels,omitempty"`
	AudioSeconds           float64 `json:"audioSeconds"`
	DeclaredAudioSeconds   float64 `json:"declaredAudioSeconds,omitempty"`
	WallSeconds            float64 `json:"wallSeconds"`
	DecodeAndStreamSeconds float64 `json:"decodeAndStreamSeconds"`
	DSPSeconds             float64 `json:"dspSeconds"`
	Error                  string  `json:"error,omitempty"`
}

// CodecProbe is an unlabeled decoder/analyzer smoke measurement. It never
// contributes to corpus accuracy or Phase 0 readiness; it exists to keep a
// codec failure visibly separate from an absent ground-truth label.
type CodecProbe struct {
	Name            string      `json:"name"`
	Codec           string      `json:"codec"`
	Algorithm       string      `json:"algorithm"`
	Status          string      `json:"status"`
	BPM             *float64    `json:"bpm,omitempty"`
	TempoConfidence *float64    `json:"tempoConfidence,omitempty"`
	Key             string      `json:"key,omitempty"`
	KeyConfidence   *float64    `json:"keyConfidence,omitempty"`
	Timing          TrackTiming `json:"timing"`
	Error           string      `json:"error,omitempty"`
	ErrorMessage    string      `json:"errorMessage,omitempty"`
}

// WriteResultSet writes benchmark output without overwriting a previous run.
// Keeping each raw result immutable prevents a favorable run replacing an
// unfavorable one after the fact.
func WriteResultSet(path string, resultSet ResultSet) error {
	if err := resultSet.Validate(); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create result set: %w", err)
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(resultSet); err != nil {
		return fmt.Errorf("encode result set: %w", err)
	}
	return nil
}
