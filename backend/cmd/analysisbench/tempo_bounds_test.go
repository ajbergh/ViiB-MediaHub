package main

import (
	"math"
	"testing"
)

func TestValidateTempoBounds(t *testing.T) {
	for _, bounds := range [][2]float64{{0, 0}, {40, 240}, {20, 400}} {
		if err := validateTempoBounds(bounds[0], bounds[1]); err != nil {
			t.Errorf("%v: %v", bounds, err)
		}
	}
	for _, bounds := range [][2]float64{{0, 240}, {40, 0}, {80, 80}, {100, 90}, {19, 200}, {40, 401}, {math.NaN(), 240}, {40, math.Inf(1)}} {
		if err := validateTempoBounds(bounds[0], bounds[1]); err == nil {
			t.Errorf("accepted invalid bounds %v", bounds)
		}
	}
}
