package beatgrid

import (
	"errors"
	"math"
)

// AlgorithmVersion identifies the phase detector and beat-timestamp rules
// used to produce a persisted artifact.
const AlgorithmVersion = "beatgrid-v1-phase-v1"

// TempoAnchor is an explicit local-tempo change.  Its Time is also a beat;
// callers can use anchors from an editor without converting a dynamic grid
// back through a rounded global BPM.
type TempoAnchor struct {
	Time float64
	BPM  float64
}

// BuildDynamic constructs an arbitrary (including variable-tempo) grid from
// user or detector anchors.  Grid stores the resulting beat times explicitly,
// which is both more accurate for Sync and safer to persist than asking a
// later playback client to reproduce interpolation rules.
func BuildDynamic(anchors []TempoAnchor, duration float64, beatsPerBar int) (Grid, error) {
	if !finitePositive(duration) || beatsPerBar < 1 || beatsPerBar > 32 {
		return Grid{}, errors.New("invalid dynamic beatgrid geometry")
	}
	if len(anchors) == 0 {
		return Grid{}, errors.New("dynamic beatgrid requires at least one tempo anchor")
	}
	for i, anchor := range anchors {
		if !finiteNonNegative(anchor.Time) || anchor.Time >= duration || !finitePositive(anchor.BPM) || (i > 0 && anchor.Time <= anchors[i-1].Time) {
			return Grid{}, errors.New("tempo anchors must be finite, ascending, and within track duration")
		}
	}

	grid := Grid{Beats: make([]float64, 0), DownbeatIndices: make([]int, 0)}
	beat := anchors[0].Time
	anchorIndex := 0
	for beat < duration {
		if len(grid.Beats) >= maxBeats {
			return Grid{}, errors.New("beatgrid exceeds maximum supported beat count")
		}
		grid.Beats = append(grid.Beats, beat)
		if (len(grid.Beats)-1)%beatsPerBar == 0 {
			grid.DownbeatIndices = append(grid.DownbeatIndices, len(grid.Beats)-1)
		}

		// An anchor takes effect at its declared beat.  If an imported anchor
		// lies between calculated beats, make it an exact beat rather than
		// silently drifting it to the nearest global-tempo value.
		next := beat + 60/anchors[anchorIndex].BPM
		if anchorIndex+1 < len(anchors) && anchors[anchorIndex+1].Time <= next {
			anchorIndex++
			next = anchors[anchorIndex].Time
		}
		if next <= beat {
			return Grid{}, errors.New("tempo anchor does not advance beatgrid")
		}
		beat = next
	}
	return grid, grid.Validate()
}

// PhaseAccumulator derives a stable first beat from an onset-energy stream.
// It intentionally keeps only a low-rate flux envelope, never full PCM, and
// resolves phase after the tempo estimator has selected BPM.
type PhaseAccumulator struct {
	sampleRate int
	window     int
	filled     int
	energy     float64
	flux       []float64
	previous   float64
}

// NewPhaseAccumulator uses 5 ms windows, matching the tempo onset envelope.
func NewPhaseAccumulator(sampleRate int) *PhaseAccumulator {
	return &PhaseAccumulator{sampleRate: sampleRate, window: max(1, sampleRate/200)}
}

// Feed accepts bounded mono PCM chunks.
func (a *PhaseAccumulator) Feed(samples []float32) {
	for _, sample := range samples {
		a.energy += float64(sample) * float64(sample)
		a.filled++
		if a.filled != a.window {
			continue
		}
		value := math.Sqrt(a.energy / float64(a.window))
		if value > a.previous {
			a.flux = append(a.flux, value-a.previous)
		} else {
			a.flux = append(a.flux, 0)
		}
		a.previous, a.energy, a.filled = value, 0, 0
	}
}

// Build creates a phase-aligned straight grid.  The circular weighted mean
// makes the result independent of decoder chunk boundaries and avoids the
// legacy zero-offset assumption.
func (a *PhaseAccumulator) Build(bpm, duration float64, beatsPerBar int) (Grid, error) {
	if a.sampleRate <= 0 || len(a.flux) < 2 {
		return Grid{}, errors.New("insufficient onset evidence for beat phase")
	}
	if !finitePositive(bpm) {
		return Grid{}, errors.New("beat phase requires positive BPM")
	}
	interval := 60 / bpm
	stepSeconds := float64(a.window) / float64(a.sampleRate)
	var x, y float64
	for index, flux := range a.flux {
		if flux == 0 {
			continue
		}
		angle := 2 * math.Pi * (float64(index) * stepSeconds / interval)
		x += flux * math.Cos(angle)
		y += flux * math.Sin(angle)
	}
	if math.Hypot(x, y) <= 1e-12 {
		return Grid{}, errors.New("no periodic onset evidence for beat phase")
	}
	phase := math.Atan2(y, x) * interval / (2 * math.Pi)
	if phase < 0 {
		phase += interval
	}
	return BuildStraight(bpm, phase, duration, beatsPerBar)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
