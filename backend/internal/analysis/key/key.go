// Package key provides production musical key detection using chromagram analysis
// and Krumhansl-Schmuckler profile matching over the shared STFT adapter.
package key

import (
	"math"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
)

const AlgorithmVersion = "key-v1-chroma-ks"

// maxTonalChromaFlatness is the flattest a chromagram may be before a key is
// refused.
//
// Flatness is the geometric/arithmetic mean ratio of the normalized 12-bin
// chroma vector: 1.0 is perfectly uniform, and approaches 0 as energy
// concentrates into a few pitch classes. Broadband material — percussion,
// noise, applause — spreads energy near-uniformly across all twelve pitch
// classes, so its chromagram carries no tonal information and the 24-profile
// correlation degenerates into ranking numerical noise. Refusing that is not a
// tuned musical judgement; it is declining to read a signal that is not there.
//
// Measured on the Phase 0 synthetic fixtures: additive major/minor triads land
// between 0.0003 and 0.0040, a click track mixed with a quiet triad lands at
// 0.513, while a bare click track lands at 0.848 and white noise at 0.979.
// This bound sits between the loudest genuinely tonal case and the least flat
// atonal one.
//
// Without this gate white noise reported "A minor" at confidence 0.377 —
// higher than every correctly identified triad in the same fixture set, which
// inverts the meaning of confidence. This threshold is provisional and must be
// recalibrated against the Phase 0 labeled corpus.
const maxTonalChromaFlatness = 0.70

const (
	minChromaFrequency = 65.0   // C2
	maxChromaFrequency = 2093.0 // C7
	maxHPCPFrequency   = 3500.0
)

// Options provides an explicit tonality-refusal threshold. The default remains
// deliberately conservative; Phase 0 may sweep an alternate value only on
// its tuning split before evaluating the reserved held-out split.
type Options struct {
	MaxChromaFlatness float64
	// MaxFrequency bounds chroma accumulation. Zero uses the established C7
	// maximum; Phase 0 may test a lower value on tuning data to reduce bright
	// overtone and percussion influence.
	MaxFrequency float64
	Profile      Profile
	Extraction   Extraction
}

// Extraction selects the pitch-class representation. DirectChroma preserves
// the current product path. HPCPPeaks is a Phase 0 candidate independently
// implementing the published spectral-peaks/HPCP design used by established
// offline analysis pipelines; it is not linked to their GPL/AGPL code.
type Extraction string

const (
	ExtractionDirectChroma Extraction = "direct-chroma"
	ExtractionHPCPPeaks    Extraction = "hpcp-peaks"
)

// Profile selects a key-profile family for Phase 0 comparison. Krumhansl is
// the existing default; Temperley is evaluated only through explicit tuning
// configuration until held-out evidence selects a production profile.
type Profile string

const (
	ProfileKrumhansl Profile = "krumhansl"
	ProfileTemperley Profile = "temperley"
)

func DefaultOptions() Options {
	return Options{
		MaxChromaFlatness: maxTonalChromaFlatness,
		MaxFrequency:      maxChromaFrequency,
		Profile:           ProfileKrumhansl,
		Extraction:        ExtractionDirectChroma,
	}
}

// Estimate holds the detected musical key, mode, confidence, and DJ notations.
// Known is false when tonal evidence is insufficient; callers must not invent a key.
type Estimate struct {
	Tonic      int         `json:"tonic"`
	Mode       string      `json:"mode"`
	Key        string      `json:"key"`
	Camelot    string      `json:"camelot"`
	OpenKey    string      `json:"openKey"`
	Confidence float64     `json:"confidence"`
	Chroma     [12]float64 `json:"chroma"`
	// Flatness is the chroma uniformity diagnostic that gates Known. It is
	// retained on refusal too, so Phase 0 calibration can see how far a
	// rejected track sat from the tonality bound.
	Flatness         float64 `json:"flatness"`
	Known            bool    `json:"known"`
	AlgorithmVersion string  `json:"algorithmVersion"`
}

// EstimatePCM estimates musical key from a normalized mono PCM buffer.
func EstimatePCM(samples []float32, sampleRate int) Estimate {
	return EstimatePCMWithOptions(samples, sampleRate, DefaultOptions())
}

// EstimatePCMWithOptions evaluates PCM using an explicit Phase 0 tonality
// setting. Product callers use EstimatePCM and therefore retain the default.
func EstimatePCMWithOptions(samples []float32, sampleRate int, options Options) Estimate {
	acc := NewChromaAccumulatorWithOptions(sampleRate, options)
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
	options     Options
	pending     []float32
	chroma      [12]float64
	totalEnergy float64
	hpcp        [36]float64
	hpcpEnergy  float64
	windows     int
}

// NewChromaAccumulator constructs a streaming chroma accumulator.
func NewChromaAccumulator(sampleRate int) *ChromaAccumulator {
	return NewChromaAccumulatorWithOptions(sampleRate, DefaultOptions())
}

// NewChromaAccumulatorWithOptions constructs a streaming chroma accumulator
// with an explicit Phase 0 calibration setting.
func NewChromaAccumulatorWithOptions(sampleRate int, options Options) *ChromaAccumulator {
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
		options:    options,
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
	numFrames := (len(a.pending)-a.windowSize)/a.hopSize + 1
	consumedSamples := (numFrames-1)*a.hopSize + a.windowSize
	toProcess := a.pending[:consumedSamples]

	_ = a.stft.Frames(toProcess, func(spectrum []complex128) error {
		a.accumulateSpectrum(spectrum)
		a.windows++
		return nil
	})

	a.pending = a.pending[numFrames*a.hopSize:]
}

func (a *ChromaAccumulator) accumulateSpectrum(spectrum []complex128) {
	if a.options.Extraction == ExtractionHPCPPeaks {
		a.accumulateHPCP(spectrum)
		return
	}
	maxFreq := a.options.MaxFrequency
	if maxFreq <= 0 || maxFreq > maxChromaFrequency {
		maxFreq = maxChromaFrequency
	}
	binHz := float64(a.sampleRate) / float64(a.windowSize)

	for bin, val := range spectrum {
		freq := float64(bin) * binHz
		if freq < minChromaFrequency || freq > maxFreq {
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

type spectralPeak struct {
	frequency float64
	magnitude float64
}

// accumulateHPCP independently follows the high-level design of established
// offline key pipelines: retain interpolated local spectral peaks, project
// them into a high-resolution harmonic pitch-class profile using cosine
// weights, normalize per frame, then aggregate. This is deliberately not a
// port of any external implementation.
func (a *ChromaAccumulator) accumulateHPCP(spectrum []complex128) {
	maxFrequency := a.options.MaxFrequency
	if maxFrequency <= 0 || maxFrequency > maxHPCPFrequency {
		maxFrequency = maxHPCPFrequency
	}
	binHz := float64(a.sampleRate) / float64(a.windowSize)
	var peaks [60]spectralPeak
	count := 0

	for bin := 1; bin+1 < len(spectrum); bin++ {
		frequency := float64(bin) * binHz
		if frequency < 25 || frequency > maxFrequency {
			continue
		}
		left := math.Hypot(real(spectrum[bin-1]), imag(spectrum[bin-1]))
		center := math.Hypot(real(spectrum[bin]), imag(spectrum[bin]))
		right := math.Hypot(real(spectrum[bin+1]), imag(spectrum[bin+1]))
		if center < 1e-4 || center < left || center < right {
			continue
		}
		denominator := left - 2*center + right
		offset := 0.0
		if math.Abs(denominator) > 1e-12 {
			offset = math.Max(-0.5, math.Min(0.5, 0.5*(left-right)/denominator))
		}
		peak := spectralPeak{frequency: (float64(bin) + offset) * binHz, magnitude: center}
		if count < len(peaks) {
			peaks[count] = peak
			count++
			continue
		}
		lowest := 0
		for index := 1; index < len(peaks); index++ {
			if peaks[index].magnitude < peaks[lowest].magnitude {
				lowest = index
			}
		}
		if peak.magnitude > peaks[lowest].magnitude {
			peaks[lowest] = peak
		}
	}

	var frame [36]float64
	for index := 0; index < count; index++ {
		peak := peaks[index]
		midi := 12*math.Log2(peak.frequency/440) + 69
		centre := midi * 3
		nearest := int(math.Round(centre))
		for delta := -1; delta <= 1; delta++ {
			bin := nearest + delta
			distance := math.Abs(float64(bin)-centre) / 3
			if distance > 0.5 {
				continue
			}
			weight := math.Cos(math.Pi * distance)
			if weight > 0 {
				frame[(bin%36+36)%36] += peak.magnitude * weight
			}
		}
	}

	frameEnergy := 0.0
	for _, value := range frame {
		frameEnergy += value
	}
	if frameEnergy <= 1e-7 {
		return
	}
	for index, value := range frame {
		a.hpcp[index] += value / frameEnergy
	}
	a.hpcpEnergy += 1
}

// Estimate correlates accumulated chromagram against 24 major/minor profiles.
func (a *ChromaAccumulator) Estimate() Estimate {
	if a.windows == 0 || a.totalEnergy < 1e-7 {
		if a.options.Extraction != ExtractionHPCPPeaks || a.hpcpEnergy < 1e-7 {
			return Estimate{AlgorithmVersion: AlgorithmVersion}
		}
	}
	chroma := a.chroma
	totalEnergy := a.totalEnergy
	if a.options.Extraction == ExtractionHPCPPeaks {
		chroma, totalEnergy = collapseHPCP(a.hpcp)
	}
	if totalEnergy < 1e-7 {
		return Estimate{AlgorithmVersion: AlgorithmVersion}
	}

	// Normalize chroma vector to unit sum for profile correlation
	var normChroma [12]float64
	for i := 0; i < 12; i++ {
		normChroma[i] = chroma[i] / totalEnergy
	}

	// Refuse material with no tonal centre before correlating. A near-uniform
	// chromagram fits every profile about equally badly, so whichever of the 24
	// wins is an artifact rather than a reading of the music.
	flatness := chromaFlatness(normChroma)
	maximumFlatness := a.options.MaxChromaFlatness
	if maximumFlatness <= 0 {
		maximumFlatness = maxTonalChromaFlatness
	}
	if flatness > maximumFlatness {
		return Estimate{Chroma: chroma, Flatness: flatness, AlgorithmVersion: AlgorithmVersion}
	}

	majorProfile, minorProfile := tonalProfiles(a.options.Profile)
	bestScore := -math.MaxFloat64
	runnerUpScore := -math.MaxFloat64
	bestTonic := 0
	bestMode := ModeMajor

	for tonic := 0; tonic < 12; tonic++ {
		majRot := rotateProfile(majorProfile, tonic)
		majCorr := pearsonCorrelation(normChroma, majRot)
		if majCorr > bestScore {
			runnerUpScore = bestScore
			bestScore = majCorr
			bestTonic = tonic
			bestMode = ModeMajor
		} else if majCorr > runnerUpScore {
			runnerUpScore = majCorr
		}

		minRot := rotateProfile(minorProfile, tonic)
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
		Chroma:           chroma,
		Flatness:         flatness,
		Known:            true,
		AlgorithmVersion: AlgorithmVersion,
	}
}

// collapseHPCP estimates the common tuning-bin offset then folds 36
// high-resolution bins into a 12-bin PCP. A shared offset makes a slightly
// sharp or flat recording contribute to its nearest tempered pitch class.
func collapseHPCP(hpcp [36]float64) ([12]float64, float64) {
	offsetEnergy := [3]float64{}
	for index, value := range hpcp {
		offsetEnergy[index%3] += value
	}
	offset := 0
	if offsetEnergy[1] > offsetEnergy[offset] {
		offset = 1
	}
	if offsetEnergy[2] > offsetEnergy[offset] {
		offset = 2
	}

	var chroma [12]float64
	for index, value := range hpcp {
		midi := int(math.Round(float64(index-offset) / 3))
		chroma[(midi%12+12)%12] += value
	}
	total := 0.0
	for _, value := range chroma {
		total += value
	}
	return chroma, total
}

// chromaFlatness returns the geometric/arithmetic mean ratio of a normalized
// chroma vector. It is the standard spectral-flatness measure applied to pitch
// classes: 1.0 for a uniform distribution, approaching 0 as energy concentrates.
func chromaFlatness(norm [12]float64) float64 {
	logSum, sum := 0.0, 0.0
	for _, value := range norm {
		// Clamp before the log so an empty pitch class cannot produce -Inf.
		logSum += math.Log(math.Max(value, 1e-12))
		sum += value
	}
	if sum <= 0 {
		return 1
	}
	return math.Exp(logSum/12) / (sum / 12)
}
