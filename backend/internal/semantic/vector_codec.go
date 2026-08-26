package semantic

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const normalizedVectorTolerance = 1e-4

// NormalizeL2 returns a newly allocated unit vector. It is the single
// normalization gate used by both durable and in-memory vector write paths.
func NormalizeL2(vector []float32) ([]float32, error) {
	if err := ValidateVector(vector); err != nil {
		return nil, err
	}
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	magnitude := math.Sqrt(sum)
	if magnitude == 0 || math.IsNaN(magnitude) || math.IsInf(magnitude, 0) {
		return nil, errors.New("vector has zero or invalid magnitude")
	}
	result := make([]float32, len(vector))
	for index, value := range vector {
		result[index] = float32(float64(value) / magnitude)
	}
	return result, nil
}

// ValidateVector rejects the values that would make cosine ordering undefined.
func ValidateVector(vector []float32) error {
	if len(vector) == 0 {
		return errors.New("vector is empty")
	}
	var sum float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errors.New("vector contains NaN or infinity")
		}
		sum += float64(value) * float64(value)
	}
	if sum == 0 {
		return errors.New("vector has zero magnitude")
	}
	return nil
}

// EncodeVector validates and L2-normalizes vector before producing the little-
// endian float32 BLOB stored in SQLite. It never mutates the caller's slice.
func EncodeVector(vector []float32) ([]byte, error) {
	normalized, err := NormalizeL2(vector)
	if err != nil {
		return nil, err
	}
	encoded := make([]byte, len(normalized)*4)
	for index, value := range normalized {
		binary.LittleEndian.PutUint32(encoded[index*4:], math.Float32bits(value))
	}
	return encoded, nil
}

// DecodeVector validates the byte layout and requires the persisted vector to
// already be normalized. Loading must never silently normalize a corrupt or
// stale row because that could mix incomparable vector spaces in an arena.
func DecodeVector(encoded []byte, expectedDimensions int) ([]float32, error) {
	if len(encoded) == 0 || len(encoded)%4 != 0 {
		return nil, errors.New("embedding BLOB must contain complete float32 values")
	}
	dimensions := len(encoded) / 4
	if expectedDimensions > 0 && dimensions != expectedDimensions {
		return nil, fmt.Errorf("embedding dimensions %d do not match expected %d", dimensions, expectedDimensions)
	}
	vector := make([]float32, dimensions)
	for index := range vector {
		vector[index] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[index*4:]))
	}
	if err := ValidateVector(vector); err != nil {
		return nil, err
	}
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	if math.Abs(math.Sqrt(sum)-1) > normalizedVectorTolerance {
		return nil, errors.New("persisted vector is not L2-normalized")
	}
	return vector, nil
}
