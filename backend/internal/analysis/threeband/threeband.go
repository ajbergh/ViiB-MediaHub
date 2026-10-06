// Package threeband measures local filtered peak envelopes. These filters and
// units do not reproduce Spotify's native three-band representation.
package threeband

import (
	"encoding/json"
	"errors"
	"math"
)

const Kind = "local_three_band_estimate"
const AlgorithmVersion = "onepole-band-peaks-v1"
const FormatVersion = 1
const Encoding = "local-threeband-json-v1"
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
func Encode(o Overview) ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(o)
	if len(raw) > MaxBytes {
		return nil, errors.New("three-band artifact too large")
	}
	return raw, err
}
func Decode(raw []byte) (Overview, error) {
	var o Overview
	if len(raw) > MaxBytes {
		return o, errors.New("three-band artifact too large")
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return o, err
	}
	return o, o.Validate()
}
