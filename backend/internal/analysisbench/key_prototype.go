package analysisbench

import "math"

// KeyEstimate is an experimental Phase 0 result. Known is false for silent or
// insufficient PCM; no fallback key is invented.
type KeyEstimate struct {
	Key        string  `json:"key,omitempty"`
	Confidence float64 `json:"confidence"`
	Known      bool    `json:"known"`
}

var prototypeMajorProfile = [...]float64{6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88}
var prototypeMinorProfile = [...]float64{6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17}

// EstimatePrototypeKey creates a simple tonal baseline: direct sinusoidal
// chroma energies at equal-tempered pitch classes, followed by 24
// Krumhansl-style profile scores. It exists solely to validate fixture labels
// and profile plumbing before Phase 3 selects a windowed HPCP/chroma design.
func EstimatePrototypeKey(samples []float32, sampleRate, channels int) KeyEstimate {
	if sampleRate <= 0 || channels <= 0 || len(samples) < sampleRate*channels/2 {
		return KeyEstimate{}
	}
	frames := len(samples) / channels
	chroma := [12]float64{}
	for pitchClass := 0; pitchClass < 12; pitchClass++ {
		for octave := 0; octave < 3; octave++ {
			midi := 48 + pitchClass + octave*12
			frequency := 440 * math.Pow(2, float64(midi-69)/12)
			chroma[pitchClass] += sinusoidEnergy(samples, frames, sampleRate, channels, frequency)
		}
	}
	total := 0.0
	for _, value := range chroma {
		total += value
	}
	if total < 1e-10 {
		return KeyEstimate{}
	}

	bestScore, secondScore := -math.MaxFloat64, -math.MaxFloat64
	bestTonic, bestMinor := 0, false
	for tonic := 0; tonic < 12; tonic++ {
		for _, candidate := range []struct {
			profile [12]float64
			minor   bool
		}{{prototypeMajorProfile, false}, {prototypeMinorProfile, true}} {
			score := 0.0
			for pitchClass := 0; pitchClass < 12; pitchClass++ {
				score += chroma[pitchClass] * candidate.profile[(pitchClass-tonic+12)%12]
			}
			if score > bestScore {
				secondScore = bestScore
				bestScore, bestTonic, bestMinor = score, tonic, candidate.minor
			} else if score > secondScore {
				secondScore = score
			}
		}
	}
	confidence := 0.0
	if bestScore > 0 {
		confidence = math.Max(0, math.Min(1, (bestScore-secondScore)/bestScore))
	}
	mode := "major"
	if bestMinor {
		mode = "minor"
	}
	return KeyEstimate{Key: tonicNames[bestTonic] + " " + mode, Confidence: confidence, Known: true}
}

func sinusoidEnergy(samples []float32, frames, sampleRate, channels int, frequency float64) float64 {
	cosine, sine := 0.0, 0.0
	for frame := 0; frame < frames; frame++ {
		mono := 0.0
		for channel := 0; channel < channels; channel++ {
			mono += float64(samples[frame*channels+channel])
		}
		mono /= float64(channels)
		angle := 2 * math.Pi * frequency * float64(frame) / float64(sampleRate)
		cosine += mono * math.Cos(angle)
		sine += mono * math.Sin(angle)
	}
	return (cosine*cosine + sine*sine) / float64(frames*frames)
}
