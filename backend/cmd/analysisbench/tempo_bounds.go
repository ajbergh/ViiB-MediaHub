package main

import (
	"fmt"
	"math"
)

func validateTempoBounds(minimum, maximum float64) error {
	if minimum == 0 && maximum == 0 {
		return nil
	}
	if math.IsNaN(minimum) || math.IsNaN(maximum) || math.IsInf(minimum, 0) || math.IsInf(maximum, 0) || minimum < 20 || maximum > 400 || maximum <= minimum {
		return fmt.Errorf("tempo bounds must be paired, finite and satisfy 20 <= min < max <= 400")
	}
	return nil
}
