package key

import "fmt"

// Tonic pitch classes 0..11.
const (
	TonicC  = 0
	TonicCs = 1
	TonicD  = 2
	TonicDs = 3
	TonicE  = 4
	TonicF  = 5
	TonicFs = 6
	TonicG  = 7
	TonicGs = 8
	TonicA  = 9
	TonicAs = 10
	TonicB  = 11
)

const (
	ModeMajor = "major"
	ModeMinor = "minor"
)

var pitchClasses = [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
var pitchClassesFlat = [...]string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}

// Camelot returns the Camelot wheel notation (e.g. "8B" for C major, "8A" for A minor).
func Camelot(tonic int, mode string) string {
	tonic = ((tonic % 12) + 12) % 12
	if mode == ModeMinor {
		// Minor key shares Camelot number with its relative major (tonic + 3).
		relMajor := (tonic + 3) % 12
		number := ((relMajor*7 + 7) % 12) + 1
		return fmt.Sprintf("%dA", number)
	}
	number := ((tonic*7 + 7) % 12) + 1
	return fmt.Sprintf("%dB", number)
}

// OpenKey returns the Open Key notation (e.g. "1d" for C major, "1m" for A minor).
func OpenKey(tonic int, mode string) string {
	tonic = ((tonic % 12) + 12) % 12
	if mode == ModeMinor {
		relMajor := (tonic + 3) % 12
		number := ((relMajor * 7) % 12) + 1
		return fmt.Sprintf("%dm", number)
	}
	number := ((tonic * 7) % 12) + 1
	return fmt.Sprintf("%dd", number)
}

// FormatKey returns the full traditional key name (e.g. "C major", "A minor", "F# minor").
func FormatKey(tonic int, mode string) string {
	tonic = ((tonic % 12) + 12) % 12
	name := pitchClasses[tonic]
	if mode == ModeMinor && (tonic == 1 || tonic == 3 || tonic == 8 || tonic == 10) {
		name = pitchClassesFlat[tonic]
	}
	if mode == ModeMinor {
		return name + " " + ModeMinor
	}
	return name + " " + ModeMajor
}

// ShortKeyName returns the compact key name (e.g. "C", "Am", "F#m").
func ShortKeyName(tonic int, mode string) string {
	tonic = ((tonic % 12) + 12) % 12
	name := pitchClasses[tonic]
	if mode == ModeMinor && (tonic == 1 || tonic == 3 || tonic == 8 || tonic == 10) {
		name = pitchClassesFlat[tonic]
	}
	if mode == ModeMinor {
		return name + "m"
	}
	return name
}

// HarmonicRelation classifies the relationship between two Camelot keys.
// Returns ("same"|"adjacent"|"relative"|"other"|"unknown", isCompatible).
func HarmonicRelation(camelotA, camelotB string) (string, bool) {
	if camelotA == "" || camelotB == "" || camelotA == "N/A" || camelotB == "N/A" {
		return "unknown", false
	}
	if camelotA == camelotB {
		return "same", true
	}
	var numA, numB int
	var letA, letB byte
	if _, err := fmt.Sscanf(camelotA, "%d%c", &numA, &letA); err != nil {
		return "unknown", false
	}
	if _, err := fmt.Sscanf(camelotB, "%d%c", &numB, &letB); err != nil {
		return "unknown", false
	}
	if numA < 1 || numA > 12 || numB < 1 || numB > 12 {
		return "unknown", false
	}
	// Same number, opposite mode (e.g. 8A and 8B) = relative major/minor
	if numA == numB && letA != letB {
		return "relative", true
	}
	// Same mode, adjacent number on 12-position wheel (±1)
	if letA == letB {
		diff := (numA - numB + 12) % 12
		if diff == 1 || diff == 11 {
			return "adjacent", true
		}
	}
	return "other", false
}
