// Package waveformartifact defines source-bound local amplitude artifact bytes.
package waveformartifact

import (
	"encoding/binary"
	"errors"
	"math"
)

const (
	Kind             = "local_amplitude"
	FormatVersion    = 1
	AlgorithmVersion = "amplitude-peaks-v1-bounded"
	Encoding         = "amplitude-f32-le-v1"
	MaxPeaks         = 1000000
	HeaderBytes      = 24
	MaxBytes         = HeaderBytes + 4*MaxPeaks
)

var ErrInvalid = errors.New("invalid local waveform artifact")

type Overview struct {
	SampleRate int
	Resolution int
	Frames     int64
	Peaks      []float64
}

func (o Overview) Duration() float64 {
	if o.SampleRate <= 0 {
		return 0
	}
	return float64(o.Frames) / float64(o.SampleRate)
}
func (o Overview) Validate() error {
	if o.SampleRate <= 0 || o.SampleRate > 768000 || o.Resolution <= 0 || o.Resolution > math.MaxInt32 || o.Frames <= 0 || len(o.Peaks) == 0 || len(o.Peaks) > MaxPeaks {
		return ErrInvalid
	}
	count := o.Frames / int64(o.Resolution)
	if o.Frames%int64(o.Resolution) != 0 {
		count++
	}
	if count != int64(len(o.Peaks)) {
		return ErrInvalid
	}
	for _, peak := range o.Peaks {
		if peak < 0 || math.IsNaN(peak) || math.IsInf(peak, 0) || peak > math.MaxFloat32 {
			return ErrInvalid
		}
	}
	return nil
}
func Encode(o Overview) ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	raw := make([]byte, HeaderBytes+len(o.Peaks)*4)
	copy(raw, "WVP1")
	binary.LittleEndian.PutUint32(raw[4:], uint32(o.SampleRate))
	binary.LittleEndian.PutUint32(raw[8:], uint32(o.Resolution))
	binary.LittleEndian.PutUint64(raw[12:], uint64(o.Frames))
	binary.LittleEndian.PutUint32(raw[20:], uint32(len(o.Peaks)))
	for i, value := range o.Peaks {
		binary.LittleEndian.PutUint32(raw[HeaderBytes+i*4:], math.Float32bits(float32(value)))
	}
	return raw, nil
}
func Decode(raw []byte) (Overview, error) {
	if len(raw) < HeaderBytes || len(raw) > MaxBytes || string(raw[:4]) != "WVP1" {
		return Overview{}, ErrInvalid
	}
	count := binary.LittleEndian.Uint32(raw[20:])
	frames := binary.LittleEndian.Uint64(raw[12:])
	if count > MaxPeaks || len(raw) != HeaderBytes+int(count)*4 || frames > math.MaxInt64 {
		return Overview{}, ErrInvalid
	}
	o := Overview{SampleRate: int(binary.LittleEndian.Uint32(raw[4:])), Resolution: int(binary.LittleEndian.Uint32(raw[8:])), Frames: int64(frames), Peaks: make([]float64, int(count))}
	for i := range o.Peaks {
		o.Peaks[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[HeaderBytes+i*4:])))
	}
	if err := o.Validate(); err != nil {
		return Overview{}, err
	}
	return o, nil
}
