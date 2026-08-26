package semantic

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestVectorCodecRoundTripAndNormalization(t *testing.T) {
	encoded, err := EncodeVector([]float32{3, 4})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeVector(encoded, 2)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if math.Abs(float64(decoded[0]-0.6)) > 1e-6 || math.Abs(float64(decoded[1]-0.8)) > 1e-6 {
		t.Fatalf("decoded = %v, want normalized [0.6 0.8]", decoded)
	}
	reencoded, err := EncodeVector(decoded)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if string(encoded) != string(reencoded) {
		t.Fatalf("vector BLOB is not bit-stable: %x != %x", encoded, reencoded)
	}
}

func TestVectorCodecRejectsInvalidVectors(t *testing.T) {
	for _, vector := range [][]float32{nil, {}, {0, 0}, {float32(math.NaN())}, {float32(math.Inf(1))}} {
		if _, err := EncodeVector(vector); err == nil {
			t.Fatalf("EncodeVector(%v) succeeded", vector)
		}
	}
	malformed := [][]byte{{1, 2, 3}, make([]byte, 4), make([]byte, 8)}
	binary.LittleEndian.PutUint32(malformed[1], math.Float32bits(0))
	binary.LittleEndian.PutUint32(malformed[2], math.Float32bits(2))
	binary.LittleEndian.PutUint32(malformed[2][4:], math.Float32bits(2))
	for _, blob := range malformed {
		if _, err := DecodeVector(blob, 0); err == nil {
			t.Fatalf("DecodeVector(%x) succeeded", blob)
		}
	}
}

func TestDecodeVectorValidatesDimensionsAndStoredNormalization(t *testing.T) {
	encoded, err := EncodeVector([]float32{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeVector(encoded, 3); err == nil {
		t.Fatal("dimension mismatch was accepted")
	}
	raw := make([]byte, 8)
	binary.LittleEndian.PutUint32(raw, math.Float32bits(2))
	binary.LittleEndian.PutUint32(raw[4:], math.Float32bits(2))
	if _, err := DecodeVector(raw, 2); err == nil {
		t.Fatal("unnormalized persisted vector was accepted")
	}
}
