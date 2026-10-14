package threeband

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestChunkingTailAndArtifact(t *testing.T) {
	samples := make([]float32, 48007)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 1000 * float64(i) / 48000))
	}
	a, _ := New(48000)
	if err := a.Feed(samples); err != nil {
		t.Fatal(err)
	}
	want, err := a.Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []int{1, 7, 1024} {
		b, _ := New(48000)
		for start := 0; start < len(samples); start += chunk {
			if err := b.Feed(samples[start:min(start+chunk, len(samples))]); err != nil {
				t.Fatal(err)
			}
		}
		got, err := b.Result()
		if err != nil || !reflect.DeepEqual(want, got) {
			t.Fatal("chunk-dependent envelopes")
		}
	}
	raw, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	quantized := want
	quantized.Low = append([]float64(nil), want.Low...)
	quantized.Mid = append([]float64(nil), want.Mid...)
	quantized.High = append([]float64(nil), want.High...)
	for _, band := range [][]float64{quantized.Low, quantized.Mid, quantized.High} {
		for i := range band {
			band[i] = float64(float32(band[i]))
		}
	}
	if !reflect.DeepEqual(quantized, got) {
		t.Fatal("artifact round trip")
	}
	legacy, _ := json.Marshal(want)
	if len(raw)*3 >= len(legacy) {
		t.Fatalf("insufficient reduction: binary=%d JSON=%d", len(raw), len(legacy))
	}
	if decoded, err := Decode(legacy); err != nil || !reflect.DeepEqual(want, decoded) {
		t.Fatal("legacy round trip", err)
	}
	for _, bad := range [][]byte{raw[:4], append(append([]byte(nil), raw...), 0)} {
		if _, err := Decode(bad); err == nil {
			t.Fatal("malformed artifact accepted")
		}
	}
	bad := append([]byte(nil), raw...)
	binary.LittleEndian.PutUint32(bad[20:], MaxWindows+1)
	if _, err := Decode(bad); err == nil {
		t.Fatal("unbounded count accepted")
	}
	if want.Frames != 48007 || len(want.Low) != 51 {
		t.Fatal("tail lost")
	}
	if _, err := Decode(raw[:len(raw)-1]); err == nil {
		t.Fatal("truncated artifact accepted")
	}
}
func TestBandsSilenceAndInvalidPCM(t *testing.T) {
	for _, frequency := range []float64{50, 1000, 15000} {
		a, _ := New(48000)
		samples := make([]float32, 48000)
		for i := range samples {
			samples[i] = float32(math.Sin(2 * math.Pi * frequency * float64(i) / 48000))
		}
		_ = a.Feed(samples)
		o, _ := a.Result()
		v := [3]float64{o.Low[20], o.Mid[20], o.High[20]}
		winner := 0
		for i := 1; i < 3; i++ {
			if v[i] > v[winner] {
				winner = i
			}
		}
		expected := map[float64]int{50: 0, 1000: 1, 15000: 2}[frequency]
		if winner != expected {
			t.Fatalf("frequency %v bands %v", frequency, v)
		}
	}
	a, _ := New(48000)
	_ = a.Feed([]float32{0, 0, 0})
	o, err := a.Result()
	if err != nil || o.Low[0] != 0 || o.Mid[0] != 0 || o.High[0] != 0 {
		t.Fatal("silence invented signal")
	}
	if a.Feed([]float32{float32(math.NaN())}) == nil {
		t.Fatal("invalid PCM accepted")
	}
}

func TestBoundedEnvelopePreservesFramesAndTail(t *testing.T) {
	a, _ := New(48000)
	a.o.Resolution = 1
	samples := make([]float32, MaxWindows+1)
	samples[len(samples)-1] = .75
	if err := a.Feed(samples); err != nil {
		t.Fatal(err)
	}
	o, err := a.Result()
	if err != nil || o.Frames != int64(len(samples)) || o.Resolution != 2 || len(o.Low) != MaxWindows/2+1 || o.High[len(o.High)-1] == 0 {
		t.Fatal("bounded merge lost duration/tail")
	}
}
