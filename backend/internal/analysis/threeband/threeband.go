// Package threeband measures local filtered peak envelopes. These filters and
// units do not reproduce Spotify's native three-band representation.
package threeband

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"

	"github.com/ajbergh/viib-mediahub/internal/analysis/localcontract"
)

const Kind = "local_three_band_estimate"
const AlgorithmVersion = localcontract.ThreeBandAlgorithmVersion
const FormatVersion = 1

// FormatVersion describes the logical envelope; encoding identifies its bytes.
const Encoding = "local-threeband-f32-le-v1"
const LegacyEncoding = "local-threeband-json-v1"
const HeaderBytes = 24

func SupportsEncoding(encoding string) bool {
	return encoding == Encoding || encoding == LegacyEncoding
}

const MaxWindows = 100000
const MaxBytes = 8 << 20

type Overview struct {
	SampleRate    int       `json:"sampleRate"`
	Frames        int64     `json:"frames"`
	Resolution    int       `json:"resolution"`
	Low           []float64 `json:"low"`
	Mid           []float64 `json:"mid"`
	High          []float64 `json:"high"`
	Units         string    `json:"units"`
	Filter        string    `json:"filter"`
	Normalization string    `json:"normalization"`
}

type Accumulator struct {
	o                        Overview
	low, upper, aLow, aUpper float64
	peak                     [3]float64
	filled                   int
	err                      error
}

func New(sampleRate int) (*Accumulator, error) {
	if sampleRate < 10000 || sampleRate > 768000 {
		return nil, errors.New("local three-band sample rate unsupported")
	}
	a := &Accumulator{o: Overview{SampleRate: sampleRate, Resolution: max(1, sampleRate/50), Units: "filtered_pcm_absolute_peak", Filter: "one_pole_250_4000_hz_residual_v1", Normalization: "none_equal_channel_mono"}}
	a.aLow = 1 - math.Exp(-2*math.Pi*250/float64(sampleRate))
	a.aUpper = 1 - math.Exp(-2*math.Pi*4000/float64(sampleRate))
	return a, nil
}

func (a *Accumulator) Feed(samples []float32) error {
	if a.err != nil {
		return a.err
	}
	for _, sample := range samples {
		x := float64(sample)
		if math.IsNaN(x) || math.IsInf(x, 0) {
			a.err = errors.New("non-finite three-band PCM")
			return a.err
		}
		a.low += a.aLow * (x - a.low)
		a.upper += a.aUpper * (x - a.upper)
		for i, v := range [3]float64{a.low, a.upper - a.low, x - a.upper} {
			a.peak[i] = math.Max(a.peak[i], math.Abs(v))
		}
		a.filled++
		a.o.Frames++
		if a.filled == a.o.Resolution {
			a.flush()
		}
	}
	return nil
}
func (a *Accumulator) flush() {
	a.o.Low = append(a.o.Low, a.peak[0])
	a.o.Mid = append(a.o.Mid, a.peak[1])
	a.o.High = append(a.o.High, a.peak[2])
	a.peak = [3]float64{}
	a.filled = 0
	if len(a.o.Low) == MaxWindows {
		for _, band := range [][]float64{a.o.Low, a.o.Mid, a.o.High} {
			for i := 0; i < len(band)/2; i++ {
				band[i] = math.Max(band[2*i], band[2*i+1])
			}
		}
		a.o.Low = a.o.Low[:MaxWindows/2]
		a.o.Mid = a.o.Mid[:MaxWindows/2]
		a.o.High = a.o.High[:MaxWindows/2]
		a.o.Resolution *= 2
	}
}
func (a *Accumulator) Result() (Overview, error) {
	if a.err != nil {
		return Overview{}, a.err
	}
	o := a.o
	o.Low = append([]float64(nil), o.Low...)
	o.Mid = append([]float64(nil), o.Mid...)
	o.High = append([]float64(nil), o.High...)
	if a.filled > 0 {
		o.Low = append(o.Low, a.peak[0])
		o.Mid = append(o.Mid, a.peak[1])
		o.High = append(o.High, a.peak[2])
	}
	return o, o.Validate()
}
func (o Overview) Validate() error {
	if o.SampleRate < 10000 || o.SampleRate > 768000 || o.Frames <= 0 || o.Resolution <= 0 || o.Resolution > math.MaxInt32 || len(o.Low) == 0 || len(o.Low) > MaxWindows || len(o.Mid) != len(o.Low) || len(o.High) != len(o.Low) || o.Units != "filtered_pcm_absolute_peak" || o.Filter != "one_pole_250_4000_hz_residual_v1" || o.Normalization != "none_equal_channel_mono" {
		return errors.New("invalid local three-band artifact")
	}
	if (o.Frames-1)/int64(o.Resolution)+1 != int64(len(o.Low)) {
		return errors.New("invalid three-band frame count")
	}
	for _, band := range [][]float64{o.Low, o.Mid, o.High} {
		for _, v := range band {
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) || v > math.MaxFloat32 {
				return errors.New("invalid three-band peak")
			}
		}
	}
	return nil
}

// Encode uses bounded float32 peaks, matching source PCM precision.
func Encode(o Overview) ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	raw := make([]byte, HeaderBytes+12*len(o.Low))
	copy(raw, "TBP1")
	binary.LittleEndian.PutUint32(raw[4:], uint32(o.SampleRate))
	binary.LittleEndian.PutUint64(raw[8:], uint64(o.Frames))
	binary.LittleEndian.PutUint32(raw[16:], uint32(o.Resolution))
	binary.LittleEndian.PutUint32(raw[20:], uint32(len(o.Low)))
	offset := HeaderBytes
	for _, band := range [][]float64{o.Low, o.Mid, o.High} {
		for _, value := range band {
			binary.LittleEndian.PutUint32(raw[offset:], math.Float32bits(float32(value)))
			offset += 4
		}
	}
	return raw, nil
}

// Decode retains legacy JSON compatibility without reopening the audio source.
func Decode(raw []byte) (Overview, error) {
	var o Overview
	if len(raw) > MaxBytes {
		return o, errors.New("three-band artifact too large")
	}
	if len(raw) >= 4 && string(raw[:4]) == "TBP1" {
		if len(raw) < HeaderBytes {
			return o, errors.New("truncated three-band header")
		}
		count := binary.LittleEndian.Uint32(raw[20:])
		frames := binary.LittleEndian.Uint64(raw[8:])
		if count == 0 || count > MaxWindows || len(raw) != HeaderBytes+12*int(count) || frames > math.MaxInt64 {
			return o, errors.New("invalid three-band payload size")
		}
		o = Overview{SampleRate: int(binary.LittleEndian.Uint32(raw[4:])), Frames: int64(frames), Resolution: int(binary.LittleEndian.Uint32(raw[16:])), Low: make([]float64, int(count)), Mid: make([]float64, int(count)), High: make([]float64, int(count)), Units: "filtered_pcm_absolute_peak", Filter: "one_pole_250_4000_hz_residual_v1", Normalization: "none_equal_channel_mono"}
		offset := HeaderBytes
		for _, band := range [][]float64{o.Low, o.Mid, o.High} {
			for i := range band {
				band[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[offset:])))
				offset += 4
			}
		}
	} else if err := json.Unmarshal(raw, &o); err != nil {
		return o, err
	}
	return o, o.Validate()
}
