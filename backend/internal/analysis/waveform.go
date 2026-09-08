package analysis

import (
	"context"
	"errors"
	"math"
)

// DefaultWaveformResolution is the number of source frames summarized by one
// overview peak. At 44.1 kHz it is roughly 5.8 ms per peak, which matches the
// resolution the DJ waveform cache and its consumers were built around.
const DefaultWaveformResolution = 256

// WaveformOverview is the compact amplitude overview of a decoded track. It is
// the online accumulator output described by the shared analysis pipeline: the
// full-resolution PCM is never retained, only one peak per resolution window.
type WaveformOverview struct {
	SampleRate int
	Resolution int
	Frames     int64
	Peaks      []float64
}

// Duration reports decoded length in seconds. It is derived from the frames
// actually decoded rather than from a container-header estimate, so a
// truncated or mis-tagged file reports what was really read.
func (o WaveformOverview) Duration() float64 {
	if o.SampleRate <= 0 {
		return 0
	}
	return float64(o.Frames) / float64(o.SampleRate)
}

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
	for _, sample := range samples {
		magnitude := math.Abs(float64(sample))
		if magnitude > a.current {
			a.current = magnitude
		}
		a.filled++
		if a.filled == a.resolution {
			a.peaks = append(a.peaks, a.current)
			a.current, a.filled = 0, 0
		}
	}
	a.frames += int64(len(samples))
}

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
	accumulator := NewPeakAccumulator(resolution)
	sampleRate := 0
	err := StreamMonoFile(ctx, registry, path, func(chunk MonoChunk) error {
		if sampleRate == 0 {
			sampleRate = chunk.SampleRate
		}
		if sampleRate != chunk.SampleRate {
			return errors.New("analysis stream sample rate changed")
		}
		accumulator.Feed(chunk.Samples)
		return nil
	})
	if err != nil {
		return WaveformOverview{}, err
	}
	if sampleRate <= 0 {
		return WaveformOverview{}, errors.New("decoder produced no audio")
	}
	return accumulator.Overview(sampleRate), nil
}
