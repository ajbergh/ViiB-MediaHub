// Accumulates amplitude peaks and generates compact waveform overviews from decoded PCM.

package analysis

import (
	"context"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"io"
	"math"
	"os"
)

// DefaultWaveformResolution is the number of source frames summarized by one
// overview peak. At 44.1 kHz it is roughly 5.8 ms per peak, which matches the
// resolution the DJ waveform cache and its consumers were built around.
const DefaultWaveformResolution = 256

// WaveformOverview is the compact amplitude overview of a decoded track. It is
// the online accumulator output described by the shared analysis pipeline: the
// full-resolution PCM is never retained, only one peak per resolution window.
type WaveformOverview = waveformartifact.Overview

// PeakAccumulator reduces streamed mono PCM into fixed-width absolute peaks.
// Feed may be called with arbitrarily sized chunks: window state carries across
// calls, so peak boundaries depend only on the resolution and never on how the
// decoder happened to chunk its output.
type PeakAccumulator struct {
	resolution int
	current    float64
	filled     int
	frames     int64
	peaks      []float64
	err        error
}

// NewPeakAccumulator constructs an accumulator. A non-positive resolution
// falls back to the default rather than dividing by zero.
func NewPeakAccumulator(resolution int) *PeakAccumulator {
	if resolution <= 0 {
		resolution = DefaultWaveformResolution
	}
	return &PeakAccumulator{resolution: resolution}
}

// Feed accumulates one bounded mono chunk.
func (a *PeakAccumulator) Feed(samples []float32) {
	if a.err != nil {
		return
	}
	for _, sample := range samples {
		if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
			a.err = waveformartifact.ErrInvalid
			return
		}
		magnitude := math.Abs(float64(sample))
		if magnitude > a.current {
			a.current = magnitude
		}
		a.filled++
		if a.filled == a.resolution {
			a.peaks = append(a.peaks, a.current)
			a.current, a.filled = 0, 0
			if len(a.peaks) == waveformartifact.MaxPeaks {
				if a.resolution > math.MaxInt32/2 {
					a.err = waveformartifact.ErrInvalid
					return
				}
				for i := 0; i < len(a.peaks)/2; i++ {
					a.peaks[i] = math.Max(a.peaks[i*2], a.peaks[i*2+1])
				}
				a.peaks = a.peaks[:len(a.peaks)/2]
				a.resolution *= 2
			}
		}
	}
	a.frames += int64(len(samples))
}

func (a *PeakAccumulator) Err() error { return a.err }

// Overview flushes any partial trailing window and returns the result. A track
// shorter than one window still yields one peak, so a valid short file is not
// reported as an empty waveform.
func (a *PeakAccumulator) Overview(sampleRate int) WaveformOverview {
	peaks := a.peaks
	if a.filled > 0 {
		peaks = append(append([]float64(nil), peaks...), a.current)
	}
	if peaks == nil {
		peaks = []float64{}
	}
	return WaveformOverview{SampleRate: sampleRate, Resolution: a.resolution, Frames: a.frames, Peaks: peaks}
}

// GenerateWaveformOverview decodes one audio file through the shared decoder
// registry and returns its amplitude overview. It returns ErrUnsupportedCodec
// when no backend decoder is registered for the format, which callers surface
// as an explicit capability gap instead of a fabricated waveform.
func GenerateWaveformOverview(ctx context.Context, registry *DecoderRegistry, path string, resolution int) (WaveformOverview, error) {
	return GenerateWaveformOverviewWithOpener(ctx, registry, path, func() (io.ReadCloser, error) {
		return os.Open(path)
	}, resolution)
}

// GenerateWaveformOverviewWithOpener uses the same decoder and peak calculation
// for local files and authenticated remote streams.
func GenerateWaveformOverviewWithOpener(ctx context.Context, registry *DecoderRegistry, name string, open func() (io.ReadCloser, error), resolution int) (WaveformOverview, error) {
	accumulator := NewPeakAccumulator(resolution)
	sampleRate := 0
	err := StreamMonoFileWithOpener(ctx, registry, name, open, func(chunk MonoChunk) error {
		if sampleRate == 0 {
			sampleRate = chunk.SampleRate
		}
		if sampleRate != chunk.SampleRate {
			return errors.New("analysis stream sample rate changed")
		}
		accumulator.Feed(chunk.Samples)
		return accumulator.Err()
	})
	if err != nil {
		return WaveformOverview{}, err
	}
	if sampleRate <= 0 {
		return WaveformOverview{}, errors.New("decoder produced no audio")
	}
	return accumulator.Overview(sampleRate), nil
}
