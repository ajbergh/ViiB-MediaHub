// Package key provides production musical key detection using chromagram analysis
// and Krumhansl-Schmuckler profile matching over the shared STFT adapter.
package key

import (
	"math"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
)

const AlgorithmVersion = "key-v1-chroma-ks"

// Estimate holds the detected musical key, mode, confidence, and DJ notations.
// Known is false when tonal evidence is insufficient; callers must not invent a key.
type Estimate struct {
	Tonic            int         `json:"tonic"`
	Mode             string      `json:"mode"`
	Key              string      `json:"key"`
	Camelot          string      `json:"camelot"`
	OpenKey          string      `json:"openKey"`
	Confidence       float64     `json:"confidence"`
	Chroma           [12]float64 `json:"chroma"`
	Known            bool        `json:"known"`
	AlgorithmVersion string      `json:"algorithmVersion"`
}

// EstimatePCM estimates musical key from a normalized mono PCM buffer.
func EstimatePCM(samples []float32, sampleRate int) Estimate {
	acc := NewChromaAccumulator(sampleRate)
	acc.Feed(samples)
	return acc.Estimate()
}

// ChromaAccumulator consumes chunked mono PCM and accumulates frequency energy
// into 12 pitch classes (C through B) using the shared radix-2 STFT.
type ChromaAccumulator struct {
	sampleRate  int
	windowSize  int
	hopSize     int
	stft        *analysis.STFT
	pending     []float32
	chroma      [12]float64
	totalEnergy float64
	windows     int
}

// NewChromaAccumulator constructs a streaming chroma accumulator.
func NewChromaAccumulator(sampleRate int) *ChromaAccumulator {
	if sampleRate <= 0 {
		return &ChromaAccumulator{}
	}
	windowSize := 4096
	if sampleRate >= 40000 {
		windowSize = 8192
	}
	hopSize := windowSize / 4
	stft, _ := analysis.NewSTFT(windowSize, hopSize)
	return &ChromaAccumulator{
		sampleRate: sampleRate,
		windowSize: windowSize,
		hopSize:    hopSize,
		stft:       stft,
	}
}

// Feed buffers PCM chunks and processes complete STFT windows.
func (a *ChromaAccumulator) Feed(samples []float32) {
	if a.stft == nil || a.sampleRate <= 0 {
		return
	}
	a.pending = append(a.pending, samples...)
	if len(a.pending) < a.windowSize {
		return
	}

	// Process available complete STFT frames from pending buffer
	numFrames := (len(a.pending) - a.windowSize) / a.hopSize + 1
	consumedSamples := (numFrames - 1) * a.hopSize + a.windowSize
	toProcess := a.pending[:consumedSamples]

	_ = a.stft.Frames(toProcess, func(spectrum []complex128) error {
		a.accumulateSpectrum(spectrum)
		a.windows++
		return nil
	})

	a.pending = a.pending[numFrames*a.hopSize:]
}

func (a *ChromaAccumulator) accumulateSpectrum(spectrum []complex128) {
	minFreq := 65.0   // C2 (~65.4 Hz)
	maxFreq := 2093.0 // C7 (~2093 Hz)
	binHz := float64(a.sampleRate) / float64(a.windowSize)

	for bin, val := range spectrum {
		freq := float64(bin) * binHz
		if freq < minFreq || freq > maxFreq {
			continue
		}
		mag := math.Hypot(real(val), imag(val))
		if mag < 1e-4 {
			continue
		}
		midi := 12.0*math.Log2(freq/440.0) + 69.0
		pitchClass := (int(math.Round(midi))%12 + 12) % 12
		power := mag * mag
		a.chroma[pitchClass] += power
		a.totalEnergy += power
	}
}

// Estimate correlates accumulated chromagram against 24 major/minor profiles.
func (a *ChromaAccumulator) Estimate() Estimate {
	if a.windows == 0 || a.totalEnergy < 1e-7 {
		return Estimate{AlgorithmVersion: AlgorithmVersion}
	}

	// Normalize chroma vector to unit sum for profile correlation
	var normChroma [12]float64
	for i := 0; i < 12; i++ {
		normChroma[i] = a.chroma[i] / a.totalEnergy
	}

	bestScore := -math.MaxFloat64
	runnerUpScore := -math.MaxFloat64
	bestTonic := 0
	bestMode := ModeMajor

	for tonic := 0; tonic < 12; tonic++ {
		majRot := rotateProfile(krumhanslMajor, tonic)
		majCorr := pearsonCorrelation(normChroma, majRot)
		if majCorr > bestScore {
			runnerUpScore = bestScore
			bestScore = majCorr
			bestTonic = tonic
			bestMode = ModeMajor
		} else if majCorr > runnerUpScore {
			runnerUpScore = majCorr
		}

		minRot := rotateProfile(krumhanslMinor, tonic)
		minCorr := pearsonCorrelation(normChroma, minRot)
		if minCorr > bestScore {
			runnerUpScore = bestScore
			bestScore = minCorr
			bestTonic = tonic
			bestMode = ModeMinor
		} else if minCorr > runnerUpScore {
			runnerUpScore = minCorr
		}
	}

	// Confidence combines absolute correlation fit with winning margin
	margin := 0.0
	if bestScore > 0 {
		denom := math.Max(0.2, math.Abs(bestScore))
		margin = math.Max(0, (bestScore-runnerUpScore)/denom)
	}
	quality := math.Max(0, math.Min(1, (bestScore+1)/2))
	confidence := math.Max(0, math.Min(1, margin*quality))

	return Estimate{
		Tonic:            bestTonic,
		Mode:             bestMode,
		Key:              FormatKey(bestTonic, bestMode),
		Camelot:          Camelot(bestTonic, bestMode),
		OpenKey:          OpenKey(bestTonic, bestMode),
		Confidence:       confidence,
		Chroma:           a.chroma,
		Known:            true,
		AlgorithmVersion: AlgorithmVersion,
	}
}
