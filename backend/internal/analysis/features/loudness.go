package features

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"math"
)

const (
	BS1770ArtifactKind      = "loudness-bs1770"
	BS1770FormatVersion     = 1
	BS1770LoudnessAlgorithm = "itu-r-bs1770-5-annex1-k-weight-gating-v1"
	BS1770TruePeakAlgorithm = "itu-r-bs1770-5-annex2-4x-fir-v1"
	BS1770AlgorithmVersion  = BS1770LoudnessAlgorithm + ";" + BS1770TruePeakAlgorithm
	BS1770Standard          = "ITU-R BS.1770-5"
	BS1770Encoding          = "gzip-json-v1"
	MaxBS1770ArtifactBytes  = 2 << 20
)

// BS1770Result is a separately versioned measurement artifact. Nil values
// mean no finite measurement is available; legacy proxy aliases remain in the
// energy-structure artifact and are not copied here.
type BS1770Result struct {
	IntegratedLUFS    *float64 `json:"integratedLufsBs1770,omitempty"`
	TruePeakDBTP      *float64 `json:"truePeakDbtp,omitempty"`
	Standard          string   `json:"standard"`
	Algorithm         string   `json:"algorithmVersion"`
	LoudnessAlgorithm string   `json:"loudnessAlgorithmVersion"`
	TruePeakAlgorithm string   `json:"truePeakAlgorithmVersion"`
	Layout            string   `json:"channelLayout"`
	Weighting         string   `json:"channelWeighting"`
	LoudnessStatus    string   `json:"loudnessStatus"`
	TruePeakStatus    string   `json:"truePeakStatus"`
}

func EncodeBS1770(result BS1770Result) ([]byte, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if err := json.NewEncoder(writer).Encode(result); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

func DecodeBS1770(data []byte) (BS1770Result, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return BS1770Result{}, err
	}
	defer reader.Close()
	var result BS1770Result
	if err := json.NewDecoder(io.LimitReader(reader, 2<<20)).Decode(&result); err != nil {
		return BS1770Result{}, err
	}
	if result.Standard != BS1770Standard || result.Algorithm != BS1770AlgorithmVersion || result.LoudnessAlgorithm != BS1770LoudnessAlgorithm || result.TruePeakAlgorithm != BS1770TruePeakAlgorithm {
		return BS1770Result{}, errors.New("unsupported BS.1770 artifact metadata")
	}
	return result, nil
}

// BS1770Accumulator implements the BS.1770-5 Annex 1 K-weighting and
// integrated gating path for declared mono and stereo channel layouts, and
// the Annex 2 four-phase FIR true-peak estimator per input channel. Surround
// channel layouts are not inferred from channel count and are marked
// unavailable for integrated loudness until decoders expose channel layout.
type BS1770Accumulator struct {
	sampleRate int
	channels   int
	supported  bool
	filters    []kWeightFilter
	blockSize  int
	stepSize   int
	blockRing  [][]float64
	blockSum   []float64
	blocks     [][2]float64
	frames     int64
	peak       float64
	tpHistory  [][]float64
	tpPosition []int
	finalized  bool
	result     BS1770Result
}

func NewBS1770Accumulator(sampleRate, channels int) (*BS1770Accumulator, error) {
	if sampleRate <= 0 || channels <= 0 {
		return nil, errors.New("BS.1770 analysis requires positive sample rate and channel count")
	}
	a := &BS1770Accumulator{sampleRate: sampleRate, channels: channels, supported: channels <= 2, peak: 0}
	a.tpHistory = make([][]float64, channels)
	a.tpPosition = make([]int, channels)
	for channel := range a.tpHistory {
		a.tpHistory[channel] = make([]float64, len(truePeakCoefficients[0]))
	}
	if a.supported && sampleRate >= 8000 {
		a.blockSize = int(math.Round(float64(sampleRate) * .4))
		a.stepSize = int(math.Round(float64(sampleRate) * .1))
		if a.blockSize > 0 && a.stepSize > 0 {
			a.filters = make([]kWeightFilter, channels)
			a.blockRing = make([][]float64, channels)
			a.blockSum = make([]float64, channels)
			for channel := 0; channel < channels; channel++ {
				a.filters[channel] = newKWeightFilter(sampleRate)
				a.blockRing[channel] = make([]float64, a.blockSize)
			}
		} else {
			a.supported = false
		}
	} else {
		a.supported = false
	}
	return a, nil
}

// Feed consumes borrowed interleaved PCM synchronously. Chunk boundaries do
// not reset either the K-weight filters or the oversampling history.
func (a *BS1770Accumulator) Feed(interleaved []float32) error {
	if a == nil {
		return errors.New("nil BS.1770 accumulator")
	}
	if a.finalized {
		return errors.New("BS.1770 accumulator already finalized")
	}
	if a.channels <= 0 || len(interleaved)%a.channels != 0 {
		return errors.New("invalid BS.1770 interleaved PCM geometry")
	}
	for offset := 0; offset < len(interleaved); offset += a.channels {
		var powers [2]float64
		for channel := 0; channel < a.channels; channel++ {
			value := float64(interleaved[offset+channel])
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return errors.New("BS.1770 input contains non-finite PCM")
			}
			a.feedTruePeak(channel, value)
			if a.supported {
				filtered := a.filters[channel].process(value)
				powers[channel] = filtered * filtered
			}
		}
		if a.supported {
			index := int(a.frames % int64(a.blockSize))
			for channel := 0; channel < a.channels; channel++ {
				if a.frames >= int64(a.blockSize) {
					a.blockSum[channel] -= a.blockRing[channel][index]
				}
				a.blockRing[channel][index] = powers[channel]
				a.blockSum[channel] += powers[channel]
			}
			if a.frames+1 >= int64(a.blockSize) && (a.frames+1-int64(a.blockSize))%int64(a.stepSize) == 0 {
				var block [2]float64
				for channel := 0; channel < a.channels; channel++ {
					block[channel] = a.blockSum[channel] / float64(a.blockSize)
				}
				a.blocks = append(a.blocks, block)
			}
		}
		a.frames++
	}
	return nil
}

func (a *BS1770Accumulator) Result() BS1770Result {
	if a.finalized {
		return a.result
	}
	// Flush the interpolation FIR with zero-valued samples so peaks near the
	// end of a finite source receive the same filter support as interior peaks.
	for channel := 0; channel < a.channels; channel++ {
		for range truePeakCoefficients[0] {
			a.feedTruePeak(channel, 0)
		}
	}
	a.finalized = true
	layout, weighting := "unsupported", "unavailable-layout"
	if a.channels == 1 {
		layout, weighting = "mono", "M=1"
	} else if a.channels == 2 {
		layout, weighting = "stereo", "L=1;R=1"
	}
	result := BS1770Result{Standard: BS1770Standard, Algorithm: BS1770AlgorithmVersion, LoudnessAlgorithm: BS1770LoudnessAlgorithm, TruePeakAlgorithm: BS1770TruePeakAlgorithm, Layout: layout, Weighting: weighting, LoudnessStatus: "unsupported-channel-layout", TruePeakStatus: "available"}
	if a.supported {
		result.LoudnessStatus = "insufficient-duration"
		if len(a.blocks) > 0 {
			absolutePower := 0.0
			absoluteCount := 0
			for _, block := range a.blocks {
				power := channelPower(block, a.channels)
				if blockLoudness(power) > -70 {
					absolutePower += power
					absoluteCount++
				}
			}
			if absoluteCount == 0 {
				result.LoudnessStatus = "below-absolute-gate"
			} else {
				absoluteLoudness := blockLoudness(absolutePower / float64(absoluteCount))
				relativeThreshold := absoluteLoudness - 10
				gatedPower := 0.0
				gatedCount := 0
				for _, block := range a.blocks {
					power := channelPower(block, a.channels)
					loudness := blockLoudness(power)
					if loudness > -70 && loudness > relativeThreshold {
						gatedPower += power
						gatedCount++
					}
				}
				if gatedCount == 0 {
					result.LoudnessStatus = "no-blocks-after-gating"
				} else {
					value := blockLoudness(gatedPower / float64(gatedCount))
					result.IntegratedLUFS = &value
					result.LoudnessStatus = "available"
				}
			}
		}
	} else if a.channels <= 2 {
		result.LoudnessStatus = "unsupported-sample-rate"
	}
	if a.peak <= 0 {
		result.TruePeakStatus = "silence"
	} else {
		value := 20 * math.Log10(a.peak)
		result.TruePeakDBTP = &value
	}
	a.result = result
	return result
}

func (a *BS1770Accumulator) feedTruePeak(channel int, value float64) {
	if magnitude := math.Abs(value); magnitude > a.peak {
		a.peak = magnitude
	}
	position := a.tpPosition[channel]
	history := a.tpHistory[channel]
	history[position] = value
	for phase := range truePeakCoefficients {
		var sample float64
		for tap, coefficient := range truePeakCoefficients[phase] {
			index := position - tap
			if index < 0 {
				index += len(history)
			}
			sample += coefficient * history[index]
		}
		if magnitude := math.Abs(sample); magnitude > a.peak {
			a.peak = magnitude
		}
	}
	position++
	if position == len(history) {
		position = 0
	}
	a.tpPosition[channel] = position
}

func channelPower(block [2]float64, channels int) float64 {
	if channels == 1 {
		return block[0]
	}
	return block[0] + block[1]
}

func blockLoudness(power float64) float64 {
	if power <= 0 {
		return math.Inf(-1)
	}
	return -0.691 + 10*math.Log10(power)
}

type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	x1, x2     float64
	y1, y2     float64
}

func (b *biquad) process(x float64) float64 {
	y := b.b0*x + b.b1*b.x1 + b.b2*b.x2 - b.a1*b.y1 - b.a2*b.y2
	b.x2, b.x1 = b.x1, x
	b.y2, b.y1 = b.y1, y
	return y
}

type kWeightFilter struct{ shelf, highpass biquad }

func (f *kWeightFilter) process(sample float64) float64 {
	return f.highpass.process(f.shelf.process(sample))
}

// The 48 kHz coefficients below are the values printed in BS.1770-5 Annex 1.
// For other PCM rates, inverse-bilinear-transform to the analog prototype and
// bilinear-transform again at the target rate, preserving the specified
// frequency response instead of applying 48 kHz digital coefficients blindly.
func newKWeightFilter(sampleRate int) kWeightFilter {
	shelf := biquad{b0: 1.53512485958697, b1: -2.69169618940638, b2: 1.19839281085285, a1: -1.69065929318241, a2: .73248077421585}
	highpass := biquad{b0: 1, b1: -2, b2: 1, a1: -1.99004745483398, a2: .99007225036621}
	if sampleRate != 48000 {
		shelf = transformBiquadRate(shelf, 48000, sampleRate)
		highpass = transformBiquadRate(highpass, 48000, sampleRate)
	}
	return kWeightFilter{
		shelf: shelf, highpass: highpass,
	}
}

func transformBiquadRate(base biquad, baseRate, targetRate int) biquad {
	kBase, kTarget := 2*float64(baseRate), 2*float64(targetRate)
	analogNumerator := [3]float64{
		base.b0 + base.b1 + base.b2,
		2 * (base.b0 - base.b2) / kBase,
		(base.b0 - base.b1 + base.b2) / (kBase * kBase),
	}
	analogDenominator := [3]float64{
		1 + base.a1 + base.a2,
		2 * (1 - base.a2) / kBase,
		(1 - base.a1 + base.a2) / (kBase * kBase),
	}
	digitalNumerator := [3]float64{
		analogNumerator[0] + analogNumerator[1]*kTarget + analogNumerator[2]*kTarget*kTarget,
		2*analogNumerator[0] - 2*analogNumerator[2]*kTarget*kTarget,
		analogNumerator[0] - analogNumerator[1]*kTarget + analogNumerator[2]*kTarget*kTarget,
	}
	digitalDenominator := [3]float64{
		analogDenominator[0] + analogDenominator[1]*kTarget + analogDenominator[2]*kTarget*kTarget,
		2*analogDenominator[0] - 2*analogDenominator[2]*kTarget*kTarget,
		analogDenominator[0] - analogDenominator[1]*kTarget + analogDenominator[2]*kTarget*kTarget,
	}
	a0 := digitalDenominator[0]
	return biquad{b0: digitalNumerator[0] / a0, b1: digitalNumerator[1] / a0, b2: digitalNumerator[2] / a0, a1: digitalDenominator[1] / a0, a2: digitalDenominator[2] / a0}
}

var truePeakCoefficients = [4][12]float64{
	{.001708984375, .010986328125, -.0196533203125, .033203125, -.0594482421875, .1373291015625, .97216796875, -.102294921875, .047607421875, -.026611328125, .014892578125, -.00830078125},
	{-.0291748046875, .029296875, -.0517578125, .089111328125, -.16650390625, .465087890625, .77978515625, -.2003173828125, .1015625, -.0582275390625, .0330810546875, -.0189208984375},
	{-.0189208984375, .0330810546875, -.0582275390625, .1015625, -.2003173828125, .77978515625, .465087890625, -.16650390625, .089111328125, -.0517578125, .029296875, -.0291748046875},
	{-.00830078125, .014892578125, -.026611328125, .047607421875, -.102294921875, .97216796875, .1373291015625, -.0594482421875, .033203125, -.0196533203125, .010986328125, .001708984375},
}
