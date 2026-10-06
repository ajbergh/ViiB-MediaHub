// Package featurecontract owns artifact identities without decoder or database
// dependencies, so storage selection and DSP share the same version contract.
package featurecontract

const (
	ArtifactKind                    = "energy-structure"
	FormatVersion                   = 1
	AlgorithmVersion                = "energy-structure-v1"
	EnergyLevelAlgorithmVersion     = "energy-level-v1-fixed-reference"
	Encoding                        = "gzip-json-v1"
	MaxStructureStatusArtifactBytes = 16 << 20
	BS1770ArtifactKind              = "loudness-bs1770"
	BS1770FormatVersion             = 1
	BS1770LoudnessAlgorithm         = "itu-r-bs1770-5-annex1-k-weight-gating-v1"
	BS1770TruePeakAlgorithm         = "itu-r-bs1770-5-annex2-4x-fir-v1"
	BS1770AlgorithmVersion          = BS1770LoudnessAlgorithm + ";" + BS1770TruePeakAlgorithm
	BS1770Encoding                  = "gzip-json-v1"
	MaxBS1770ArtifactBytes          = 2 << 20
)
