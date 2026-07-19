// Package validation provides input validation utilities for ViiB MediaHub API.
//
// This package provides sanitization and validation functions to ensure
// user input is safe before processing. While the database layer uses
// parameterized queries (which prevent SQL injection), these utilities
// provide defense-in-depth by:
//
//   - Sanitizing strings for safe display
//   - Validating input lengths to prevent DoS
//   - Stripping or escaping potentially dangerous characters
//   - Providing safe defaults for missing values
//
// Usage:
//
//	name := validation.SanitizeString(userInput, 255)
//	if !validation.IsValidID(id) {
//	    return error
//	}
package validation

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const (
	// MaxStringLength is the default maximum length for text inputs
	MaxStringLength = 1000

	// MaxNameLength is the maximum length for names (playlists, etc.)
	MaxNameLength = 255

	// MaxPathLength is the maximum length for file paths
	MaxPathLength = 4096

	// MaxIDLength is the maximum length for IDs
	MaxIDLength = 100
)

// IDPattern matches valid internal IDs (alphanumeric with underscores and hyphens)
var IDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// SpotifyIDPattern matches valid Spotify IDs (22 alphanumeric characters)
var SpotifyIDPattern = regexp.MustCompile(`^[a-zA-Z0-9]{22}$`)

// SanitizeString removes control characters and trims whitespace.
// If the string exceeds maxLength, it is truncated.
// Returns an empty string if input is nil-equivalent or only whitespace.
func SanitizeString(s string, maxLength int) string {
	if s == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			builder.WriteRune(r)
		}
	}
	s = strings.TrimSpace(builder.String())

	if len(s) > maxLength {
		s = s[:maxLength]
	}
	return s
}

// SanitizeName sanitizes a name field (playlist name, artist name, etc.)
func SanitizeName(name string) string {
	return SanitizeString(name, MaxNameLength)
}

// IsValidID checks if a string is a valid internal ID.
func IsValidID(id string) bool {
	if id == "" || len(id) > MaxIDLength {
		return false
	}
	return IDPattern.MatchString(id)
}

// IsValidSpotifyID checks if a string is a valid Spotify ID.
func IsValidSpotifyID(id string) bool {
	if id == "" {
		return false
	}
	return SpotifyIDPattern.MatchString(id)
}

// SanitizePath removes potentially dangerous path components.
// File serving must still verify that paths remain within allowed directories.
func SanitizePath(path string) string {
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\x00", "")
	path = filepath.Clean(path)
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return ""
		}
	}
	if len(path) > MaxPathLength {
		path = path[:MaxPathLength]
	}
	return path
}

// StripSQLKeywords removes common SQL keywords from input.
// Parameterized queries remain the primary SQL-injection protection.
func StripSQLKeywords(s string) string {
	patterns := []string{
		"--", ";", "/*", "*/", "'OR'", "'AND'", "UNION", "SELECT",
		"INSERT", "UPDATE", "DELETE", "DROP", "EXEC", "xp_",
	}
	result := s
	lower := strings.ToLower(s)
	for _, pattern := range patterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			result = strings.ReplaceAll(result, pattern, "")
			result = strings.ReplaceAll(result, strings.ToLower(pattern), "")
			result = strings.ReplaceAll(result, strings.ToUpper(pattern), "")
		}
	}
	return result
}

// ValidateIntRange ensures an integer is within the specified range.
func ValidateIntRange(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// IsSensitiveSettingKey reports whether a setting is write-only through the API.
func IsSensitiveSettingKey(key string) bool {
	sensitive := map[string]bool{
		"gemini_api_key":       true,
		"llm_api_key":          true,
		"lastfm_api_key":       true,
		"lastfm_shared_secret": true,
		"lastfm_session_key":   true,
	}
	return sensitive[key]
}

// IsValidSettingKey checks if a setting key is in the allowed list.
func IsValidSettingKey(key string) bool {
	allowedKeys := map[string]bool{
		"concurrent_downloads":     true,
		"spotify_download_path":    true,
		"gemini_api_key":           true,
		"theme":                    true,
		"volume":                   true,
		"shuffle":                  true,
		"repeat":                   true,
		"equalizer_enabled":        true,
		"equalizer_preset":         true,
		"visualizer_enabled":       true,
		"auto_scan_on_startup":     true,
		"background_genre_enrich":  true,
		"background_mood_analysis": true,
		"llm_provider":             true,
		"llm_model":                true,
		"llm_api_key":              true,
		"llm_base_url":             true,
		"lastfm_api_key":           true,
		"lastfm_shared_secret":     true,
		"lastfm_enabled":           true,
		"lastfm_username":          true,
		"lastfm_session_key":       true,
		"lastfm_last_sync":         true,
		"enrichment_source":        true,
		"audio_settings":           true,
	}
	return allowedKeys[key]
}
