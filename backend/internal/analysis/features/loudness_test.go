package features

import (
	"math"
	"reflect"
	"testing"
)

func tone(rate, frames int, frequency, amplitude, phase float64) []float32 {
	samples := make([]float32, frames)
	for index := range samples {
		samples[index] = float32(amplitude * math.Sin(2*math.Pi*frequency*float64(index)/float64(rate)+phase))
	}
	return samples
}

func interleave(mono []float32, channels int) []float32 {
	result := make([]float32, len(mono)*channels)
	for index, sample := range mono {
		for channel := 0; channel < channels; channel++ {
			result[index*channels+channel] = sample
		}
	}
	return result
}

func requireMeasurement(t *testing.T, accumulator *BS1770Accumulator) BS1770Result {
	t.Helper()
	result := accumulator.Result()
	if result.Standard != BS1770Standard || result.Algorithm != BS1770AlgorithmVersion {
		t.Fatalf("unexpected measurement metadata: %#v", result)
	}
	return result
}

func TestBS1770SilenceAndAbsoluteGate(t *testing.T) {
	for _, amplitude := range []float64{0, .0001} {
		accumulator, err := NewBS1770Accumulator(48000, 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := accumulator.Feed(tone(48000, 48000, 997, amplitude, 0)); err != nil {
			t.Fatal(err)
		}
		result := requireMeasurement(t, accumulator)
		if result.IntegratedLUFS != nil {
			t.Fatalf("silence / below-gate signal produced integrated loudness %v", *result.IntegratedLUFS)
		}
		if result.LoudnessStatus != "below-absolute-gate" {
			t.Fatalf("loudness status = %q", result.LoudnessStatus)
		}
		if amplitude == 0 && (result.TruePeakDBTP != nil || result.TruePeakStatus != "silence") {
			t.Fatalf("silence true peak = %#v / %q", result.TruePeakDBTP, result.TruePeakStatus)
		}
	}
}

func TestBS1770KnownToneAndChannelWeighting(t *testing.T) {
	filter := newKWeightFilter(48000)
	if math.Abs(filter.shelf.b0-1.53512485958697) > 1e-13 || math.Abs(filter.shelf.b1+2.69169618940638) > 1e-13 || math.Abs(filter.shelf.b2-1.19839281085285) > 1e-13 || math.Abs(filter.shelf.a1+1.69065929318241) > 1e-13 || math.Abs(filter.shelf.a2-.73248077421585) > 1e-13 || filter.highpass.b0 != 1 || filter.highpass.b1 != -2 || filter.highpass.b2 != 1 || math.Abs(filter.highpass.a1+1.99004745483398) > 1e-13 || math.Abs(filter.highpass.a2-.99007225036621) > 1e-13 {
		t.Fatalf("48 kHz K-weighting coefficients differ from BS.1770-5 Annex 1: %#v", filter)
	}
	mono, _ := NewBS1770Accumulator(48000, 1)
	if err := mono.Feed(tone(48000, 48000*2, 997, 1, 0)); err != nil {
		t.Fatal(err)
	}
	monoResult := requireMeasurement(t, mono)
	if monoResult.IntegratedLUFS == nil || math.Abs(*monoResult.IntegratedLUFS-(-3.01)) > .08 {
		t.Fatalf("997 Hz full-scale mono tone loudness = %#v, want -3.01 LKFS", monoResult.IntegratedLUFS)
	}
	if monoResult.Layout != "mono" || monoResult.Weighting != "M=1" {
		t.Fatalf("mono weighting metadata = %#v", monoResult)
	}
	otherRate, _ := NewBS1770Accumulator(44100, 1)
	if err := otherRate.Feed(tone(44100, 44100*2, 997, 1, 0)); err != nil {
		t.Fatal(err)
	}
	otherRateResult := requireMeasurement(t, otherRate)
	if otherRateResult.IntegratedLUFS == nil || math.Abs(*otherRateResult.IntegratedLUFS-(-3.01)) > .08 {
		t.Fatalf("997 Hz full-scale tone at 44.1 kHz = %v LKFS, want -3.01", valueOrNaN(otherRateResult.IntegratedLUFS))
	}

	stereo, _ := NewBS1770Accumulator(48000, 2)
	if err := stereo.Feed(interleave(tone(48000, 48000*2, 997, 1, 0), 2)); err != nil {
		t.Fatal(err)
	}
	stereoResult := requireMeasurement(t, stereo)
	if stereoResult.IntegratedLUFS == nil || math.Abs((*stereoResult.IntegratedLUFS-*monoResult.IntegratedLUFS)-3.0103) > .02 {
		t.Fatalf("identical stereo should sum channel power (+3.0103 LU): mono=%v stereo=%v", *monoResult.IntegratedLUFS, stereoResult.IntegratedLUFS)
	}
	if stereoResult.Layout != "stereo" || stereoResult.Weighting != "L=1;R=1" {
		t.Fatalf("stereo weighting metadata = %#v", stereoResult)
	}
}

func TestBS1770DiscardsIncompleteFinalGatingBlock(t *testing.T) {
	const rate = 48000
	samples := tone(rate, int(float64(rate)*.45), 997, .5, .17)
	complete, _ := NewBS1770Accumulator(rate, 1)
	if err := complete.Feed(samples[:int(float64(rate)*.4)]); err != nil {
		t.Fatal(err)
	}
	withIncompleteTail, _ := NewBS1770Accumulator(rate, 1)
	if err := withIncompleteTail.Feed(samples); err != nil {
		t.Fatal(err)
	}
	first, second := requireMeasurement(t, complete), requireMeasurement(t, withIncompleteTail)
	if first.IntegratedLUFS == nil || second.IntegratedLUFS == nil || *first.IntegratedLUFS != *second.IntegratedLUFS {
		t.Fatalf("partial final block affected integrated loudness: complete=%v plus tail=%v", first.IntegratedLUFS, second.IntegratedLUFS)
	}
}

func TestBS1770RelativeGateExcludesQuietBlocks(t *testing.T) {
	const rate = 48000
	loud := tone(rate, rate*8, 997, .4, 0)
	quiet := tone(rate, rate*8, 997, .004, 0)
	baseline, _ := NewBS1770Accumulator(rate, 1)
	if err := baseline.Feed(loud); err != nil {
		t.Fatal(err)
	}
	mixture, _ := NewBS1770Accumulator(rate, 1)
	if err := mixture.Feed(append(append([]float32(nil), loud...), quiet...)); err != nil {
		t.Fatal(err)
	}
	baseResult, mixtureResult := requireMeasurement(t, baseline), requireMeasurement(t, mixture)
	if baseResult.IntegratedLUFS == nil || mixtureResult.IntegratedLUFS == nil || math.Abs(*baseResult.IntegratedLUFS-*mixtureResult.IntegratedLUFS) > .15 {
		t.Fatalf("relative gate failed to exclude quiet blocks: loud=%.6f mixture=%.6f", valueOrNaN(baseResult.IntegratedLUFS), valueOrNaN(mixtureResult.IntegratedLUFS))
	}
}

func valueOrNaN(value *float64) float64 {
	if value == nil {
		return math.NaN()
	}
	return *value
}

func TestBS1770FourTimesTruePeakFindsInterSampleOvers(t *testing.T) {
	const rate = 48000
	samples := tone(rate, 2048, rate/4, .8, math.Pi/4)
	samplePeak := 0.0
	for _, sample := range samples {
		if peak := math.Abs(float64(sample)); peak > samplePeak {
			samplePeak = peak
		}
	}
	accumulator, _ := NewBS1770Accumulator(rate, 1)
	if err := accumulator.Feed(samples); err != nil {
		t.Fatal(err)
	}
	result := requireMeasurement(t, accumulator)
	if result.TruePeakDBTP == nil || 20*math.Log10(math.Pow(10, *result.TruePeakDBTP/20)/samplePeak) < 2.7 || math.Abs(*result.TruePeakDBTP-20*math.Log10(.8)) > .15 {
		t.Fatalf("four-times FIR did not detect expected inter-sample overs: sample=%g dBTP=%v", samplePeak, result.TruePeakDBTP)
	}
}

func TestBS1770ChunkBoundariesAndArtifactAreDeterministic(t *testing.T) {
	samples := tone(44100, 44100*2, 1000, .5, .23)
	whole, _ := NewBS1770Accumulator(44100, 1)
	if err := whole.Feed(samples); err != nil {
		t.Fatal(err)
	}
	split, _ := NewBS1770Accumulator(44100, 1)
	for start := 0; start < len(samples); {
		end := start + 137
		if end > len(samples) {
			end = len(samples)
		}
		if err := split.Feed(samples[start:end]); err != nil {
			t.Fatal(err)
		}
		start = end
	}
	first, second := requireMeasurement(t, whole), requireMeasurement(t, split)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("chunk boundaries changed measurement: %#v / %#v", first, second)
	}
	encoded, err := EncodeBS1770(first)
	if err != nil {
		t.Fatal(err)
	}
	encodedAgain, err := EncodeBS1770(first)
	if err != nil || !reflect.DeepEqual(encoded, encodedAgain) {
		t.Fatalf("artifact encoding is not deterministic: %v", err)
	}
	decoded, err := DecodeBS1770(encoded)
	if err != nil || !reflect.DeepEqual(decoded, first) {
		t.Fatalf("artifact round trip = %#v, %v; want %#v", decoded, err, first)
	}
}

func TestBS1770DoesNotInferSurroundLayout(t *testing.T) {
	accumulator, _ := NewBS1770Accumulator(48000, 6)
	if err := accumulator.Feed(make([]float32, 48000*6)); err != nil {
		t.Fatal(err)
	}
	result := requireMeasurement(t, accumulator)
	if result.IntegratedLUFS != nil || result.LoudnessStatus != "unsupported-channel-layout" || result.TruePeakDBTP != nil {
		t.Fatalf("multichannel input was measured without speaker layout: %#v", result)
	}
}
