package analysisbench

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestRadix2FFTMatchesGonumAtAnalysisGeometry(t *testing.T) {
	input := benchmarkSignal(8192)
	actual, err := Radix2FFT(input)
	if err != nil {
		t.Fatalf("Radix2FFT() error = %v", err)
	}
	want := GonumFFT(input)
	for index := range want {
		if difference := cmplx.Abs(actual[index] - want[index]); difference > 1e-8 {
			t.Fatalf("coefficient %d differs by %g: got %v want %v", index, difference, actual[index], want[index])
		}
	}
}

func TestRadix2FFTRejectsUnsupportedLengths(t *testing.T) {
	for _, length := range []int{0, 3, 8191} {
		if _, err := Radix2FFT(make([]complex128, length)); err == nil {
			t.Fatalf("Radix2FFT accepted length %d", length)
		}
	}
}

func BenchmarkRadix2FFT8192(b *testing.B) {
	input := benchmarkSignal(8192)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := Radix2FFT(input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGonumFFT8192(b *testing.B) {
	input := benchmarkSignal(8192)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = GonumFFT(input)
	}
}

func benchmarkSignal(length int) []complex128 {
	result := make([]complex128, length)
	for index := range result {
		time := float64(index) / 44100
		result[index] = complex(
			0.7*math.Sin(2*math.Pi*440*time)+0.2*math.Sin(2*math.Pi*1760*time),
			0.1*math.Cos(2*math.Pi*120*time),
		)
	}
	return result
}
