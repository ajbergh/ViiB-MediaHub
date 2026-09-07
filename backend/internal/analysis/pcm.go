package analysis

import "errors"

// DownmixInterleaved averages each interleaved frame into mono. It rejects
// malformed geometry rather than silently selecting channel zero.
func DownmixInterleaved(samples []float32, channels int) ([]float32, error) {
	if channels <= 0 || len(samples)%channels != 0 {
		return nil, errors.New("invalid interleaved PCM geometry")
	}
	frames := len(samples) / channels
	mono := make([]float32, frames)
	for frame := 0; frame < frames; frame++ {
		var sum float32
		for channel := 0; channel < channels; channel++ {
			sum += samples[frame*channels+channel]
		}
		mono[frame] = sum / float32(channels)
	}
	return mono, nil
}

// ResampleLinearMono deterministically changes a mono signal's sample rate.
// The deliberate linear baseline is dependency-free and sufficient for Phase 1
// plumbing; Phase 0 benchmarks still govern any later filter-quality upgrade.
func ResampleLinearMono(samples []float32, sourceRate, targetRate int) ([]float32, error) {
	if sourceRate <= 0 || targetRate <= 0 {
		return nil, errors.New("sample rates must be positive")
	}
	if len(samples) == 0 {
		return []float32{}, nil
	}
	if sourceRate == targetRate {
		return append([]float32(nil), samples...), nil
	}
	outputLength := int((int64(len(samples))*int64(targetRate) + int64(sourceRate) - 1) / int64(sourceRate))
	if outputLength < 1 {
		outputLength = 1
	}
	output := make([]float32, outputLength)
	for i := range output {
		position := float64(i) * float64(sourceRate) / float64(targetRate)
		left := int(position)
		if left >= len(samples)-1 {
			output[i] = samples[len(samples)-1]
			continue
		}
		fraction := float32(position - float64(left))
		output[i] = samples[left]*(1-fraction) + samples[left+1]*fraction
	}
	return output, nil
}
