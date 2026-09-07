package analysisbench

import (
	"fmt"
	"math"
	"math/bits"

	"gonum.org/v1/gonum/dsp/fourier"
)

// Radix2FFT is the small in-house Phase 0 comparator. It is iterative,
// allocation-bounded, and intentionally isolated in analysisbench until a
// measured dependency decision is recorded. It returns forward coefficients.
func Radix2FFT(input []complex128) ([]complex128, error) {
	length := len(input)
	if length == 0 || length&(length-1) != 0 {
		return nil, fmt.Errorf("FFT input length %d is not a positive power of two", length)
	}
	output := append([]complex128(nil), input...)
	width := bits.Len(uint(length)) - 1
	for index := 0; index < length; index++ {
		reversed := int(bits.Reverse(uint(index)) >> (bits.UintSize - width))
		if reversed > index {
			output[index], output[reversed] = output[reversed], output[index]
		}
	}
	for size := 2; size <= length; size *= 2 {
		half := size / 2
		step := complexUnitRoot(size)
		for start := 0; start < length; start += size {
			weight := complex(1, 0)
			for offset := 0; offset < half; offset++ {
				even := output[start+offset]
				odd := weight * output[start+offset+half]
				output[start+offset] = even + odd
				output[start+offset+half] = even - odd
				weight *= step
			}
		}
	}
	return output, nil
}

// GonumFFT invokes the candidate external implementation with matching forward
// transform semantics. A new plan is intentionally created per call here; the
// benchmark includes that cost because analysis jobs create one per geometry.
func GonumFFT(input []complex128) []complex128 {
	transform := fourier.NewCmplxFFT(len(input))
	return transform.Coefficients(make([]complex128, len(input)), input)
}

func complexUnitRoot(size int) complex128 {
	angle := -2 * math.Pi / float64(size)
	return complex(math.Cos(angle), math.Sin(angle))
}
