package analysis

import (
	"errors"
	"math"
	"math/bits"
)

// STFT frames mono PCM with a Hann window and radix-2 forward transform.
// It is intentionally a deterministic shared primitive; callers decide which
// compact summaries to retain after consuming its spectra.
type STFT struct {
	WindowSize int
	HopSize    int
	window     []float64
}

func NewSTFT(windowSize, hopSize int) (*STFT, error) {
	if windowSize < 2 || windowSize&(windowSize-1) != 0 || hopSize <= 0 || hopSize > windowSize {
		return nil, errors.New("STFT requires power-of-two window and bounded hop")
	}
	window := make([]float64, windowSize)
	for i := range window {
		window[i] = .5 - .5*math.Cos(2*math.Pi*float64(i)/float64(windowSize-1))
	}
	return &STFT{WindowSize: windowSize, HopSize: hopSize, window: window}, nil
}

// Frames invokes consume for each complete window. The returned spectra contain
// the non-redundant real-signal bins [0, windowSize/2].
func (s *STFT) Frames(samples []float32, consume func([]complex128) error) error {
	if consume == nil {
		return errors.New("STFT consumer is required")
	}
	for start := 0; start+s.WindowSize <= len(samples); start += s.HopSize {
		input := make([]complex128, s.WindowSize)
		for i := range input {
			input[i] = complex(float64(samples[start+i])*s.window[i], 0)
		}
		spectrum := radix2FFT(input)
		if err := consume(spectrum[:s.WindowSize/2+1]); err != nil {
			return err
		}
	}
	return nil
}

func radix2FFT(values []complex128) []complex128 {
	result := append([]complex128(nil), values...)
	width := bits.Len(uint(len(result))) - 1
	for i := range result {
		reversed := int(bits.Reverse(uint(i)) >> (bits.UintSize - width))
		if reversed > i {
			result[i], result[reversed] = result[reversed], result[i]
		}
	}
	for size := 2; size <= len(result); size *= 2 {
		half := size / 2
		root := complex(math.Cos(-2*math.Pi/float64(size)), math.Sin(-2*math.Pi/float64(size)))
		for start := 0; start < len(result); start += size {
			weight := complex(1, 0)
			for offset := 0; offset < half; offset++ {
				even, odd := result[start+offset], weight*result[start+offset+half]
				result[start+offset], result[start+offset+half] = even+odd, even-odd
				weight *= root
			}
		}
	}
	return result
}
