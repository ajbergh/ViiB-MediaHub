package validation

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		expected  string
	}{
		{"empty", "", 100, ""},
		{"normal", "hello world", 100, "hello world"},
		{"whitespace", "  hello  ", 100, "hello"},
		{"truncate", "hello world", 5, "hello"},
		{"control chars", "hello\x00world", 100, "helloworld"},
		{"newlines preserved", "hello\nworld", 100, "hello\nworld"},
		{"tabs preserved", "hello\tworld", 100, "hello\tworld"},
		{"unicode", "こんにちは", 100, "こんにちは"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeString(tc.input, tc.maxLength)
			if result != tc.expected {
				t.Errorf("SanitizeString(%q, %d) = %q, expected %q",
					tc.input, tc.maxLength, result, tc.expected)
			}
		})
	}
}

func TestIsValidID(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		{"", false},
		{"abc123", true},
		{"ABC-123_xyz", true},
		{"song_12345", true},
		{"pl_1234567890", true},
		{"has space", false},
		{"has.dot", false},
		{"has/slash", false},
		{"has'quote", false},
		{strings.Repeat("a", 101), false}, // Too long
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			result := IsValidID(tc.id)
			if result != tc.expected {
				t.Errorf("IsValidID(%q) = %v, expected %v", tc.id, result, tc.expected)
			}
		})
	}
}

func TestIsValidSpotifyID(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		{"", false},
		{"4iV5W9uYEdYUVa79Axb7Rh", true},        // Valid 22 char
		{"37i9dQZF1DXcBWIGoYBM5M", true},        // Valid 22 char
		{"short", false},                        // Too short
		{"toolongtoolongtoolongtoolong", false}, // Too long
		{"has-dashes-in-it-xyz", false},         // Invalid chars
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			result := IsValidSpotifyID(tc.id)
			if result != tc.expected {
				t.Errorf("IsValidSpotifyID(%q) = %v, expected %v", tc.id, result, tc.expected)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"normal", "/path/to/file.mp3", "/path/to/file.mp3"},
		{"null byte", "/path/to\x00/file.mp3", "/path/to/file.mp3"},
		{"windows path", "C:\\Users\\Music\\song.mp3", "C:\\Users\\Music\\song.mp3"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizePath(tc.input)
			expected := tc.expected
			if expected != "" {
				expected = filepath.Clean(expected)
			}
			if result != expected {
				t.Errorf("SanitizePath(%q) = %q, expected %q", tc.input, result, expected)
			}
		})
	}
}

func TestStripSQLKeywords(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		hasSQL bool
	}{
		{"normal", "hello world", false},
		{"sql comment", "test--comment", true},
		{"union", "test UNION select", true},
		{"drop", "DROP TABLE users", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := StripSQLKeywords(tc.input)
			if tc.hasSQL && result == tc.input {
				t.Errorf("StripSQLKeywords(%q) should have stripped SQL keywords", tc.input)
			}
			if !tc.hasSQL && result != tc.input {
				t.Errorf("StripSQLKeywords(%q) = %q, should be unchanged", tc.input, result)
			}
		})
	}
}

func TestValidateIntRange(t *testing.T) {
	tests := []struct {
		value, min, max, expected int
	}{
		{5, 1, 10, 5},
		{0, 1, 10, 1},
		{15, 1, 10, 10},
		{-5, 0, 100, 0},
	}

	for _, tc := range tests {
		result := ValidateIntRange(tc.value, tc.min, tc.max)
		if result != tc.expected {
			t.Errorf("ValidateIntRange(%d, %d, %d) = %d, expected %d",
				tc.value, tc.min, tc.max, result, tc.expected)
		}
	}
}

func TestIsValidSettingKey(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"concurrent_downloads", true},
		{"spotify_conversion_workers", true},
		{"spotify_download_rescan_threshold", true},
		{"spotify_download_path", true},
		{"spotify_auto_convert_ogg_to_mp3", true},
		{"random_key", false},
		{"spotify_credentials", false}, // Sensitive, not in allowed list
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			result := IsValidSettingKey(tc.key)
			if result != tc.expected {
				t.Errorf("IsValidSettingKey(%q) = %v, expected %v", tc.key, result, tc.expected)
			}
		})
	}
}
