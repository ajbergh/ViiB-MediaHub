package key

import "math"

// Key profiles from Krumhansl & Schmuckler (probe tone hierarchy).
// Index 0 = tonic, 1 = minor second, ..., 11 = major seventh.
var krumhanslMajor = [12]float64{6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88}
var krumhanslMinor = [12]float64{6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17}

// Temperley profiles are a second tonal hierarchy used for Phase 0 profile
// comparison. Indexing follows the same tonic-relative pitch-class order as
// the Krumhansl profiles above.
var temperleyMajor = [12]float64{.748, .060, .488, .082, .670, .460, .096, .715, .104, .366, .057, .400}
var temperleyMinor = [12]float64{.712, .084, .474, .618, .049, .460, .105, .747, .404, .067, .133, .330}

func tonalProfiles(profile Profile) ([12]float64, [12]float64) {
	if profile == ProfileTemperley {
		return temperleyMajor, temperleyMinor
	}
	return krumhanslMajor, krumhanslMinor
}

// rotateProfile shifts a 12-element profile by the specified pitch class offset.
func rotateProfile(profile [12]float64, shift int) [12]float64 {
	var result [12]float64
	for i := 0; i < 12; i++ {
		result[i] = profile[(i-shift+12)%12]
	}
	return result
}

// pearsonCorrelation computes the Pearson correlation coefficient between two 12-element vectors.
func pearsonCorrelation(a [12]float64, b [12]float64) float64 {
	sumA, sumB := 0.0, 0.0
	for i := 0; i < 12; i++ {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / 12.0
	meanB := sumB / 12.0

	numerator := 0.0
	denomA := 0.0
	denomB := 0.0
	for i := 0; i < 12; i++ {
		diffA := a[i] - meanA
		diffB := b[i] - meanB
		numerator += diffA * diffB
		denomA += diffA * diffA
		denomB += diffB * diffB
	}
	denom := math.Sqrt(denomA * denomB)
	if denom < 1e-12 {
		return 0.0
	}
	return numerator / denom
}
