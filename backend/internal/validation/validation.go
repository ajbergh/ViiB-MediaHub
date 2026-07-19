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

	// Remove control characters (except newlines and tabs for multiline text)
	var builder strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			builder.WriteRune(r)
		}
	}
	s = builder.String()

	// Trim whitespace
	s = strings.TrimSpace(s)

	// Truncate if too long
	if len(s) > maxLength {
		s = s[:maxLength]
	}

	return s
}

// SanitizeName sanitizes a name field (playlist name, artist name, etc.)
// Removes control characters, trims whitespace, and enforces length limit.
func SanitizeName(name string) string {
	return SanitizeString(name, MaxNameLength)
}

// IsValidID checks if a string is a valid internal ID.
// Valid IDs contain only alphanumeric characters, underscores, and hyphens.
func IsValidID(id string) bool {
	if id == "" || len(id) > MaxIDLength {
		return false
	}
	return IDPattern.MatchString(id)
}

// IsValidSpotifyID checks if a string is a valid Spotify ID.
// Spotify IDs are 22 alphanumeric characters.
func IsValidSpotifyID(id string) bool {
	if id == "" {
		return false
	}
	return SpotifyIDPattern.MatchString(id)
}

// SanitizePath removes potentially dangerous path components.
// This is a basic check - file serving should still verify paths
// are within allowed directories.
func SanitizePath(path string) string {
	if path == "" {
		return ""
	}

	// Remove null bytes (path injection)
	path = strings.ReplaceAll(path, "\x00", "")

	// Normalize the path to resolve any . or .. components
	path = filepath.Clean(path)

	// Reject paths that still contain .. after cleaning
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return ""
		}
	}

	// Truncate if too long
	if len(path) > MaxPathLength {
		path = path[:MaxPathLength]
	}

	return path
}

// StripSQLKeywords removes common SQL keywords from input.
// This is defense-in-depth - parameterized queries are the primary protection.
// Note: This is NOT a substitute for parameterized queries!
func StripSQLKeywords(s string) string {
	// Common SQL injection patterns (case-insensitive)
	patterns := []string{
		"--",     // SQL comment
		";",      // Statement terminator (be careful with legitimate use)
		"/*",     // Block comment start
		"*/",     // Block comment end
		"'OR'",   // Common injection
		"'AND'",  // Common injection
		"UNION",  // UNION injection
		"SELECT", // SELECT injection
		"INSERT", // INSERT injection
		"UPDATE", // UPDATE injection
		"DELETE", // DELETE injection
		"DROP",   // DROP injection
		"EXEC",   // EXEC injection
		"xp_",    // SQL Server extended procedures
	}

	result := s
	lower := strings.ToLower(s)
	for _, pattern := range patterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			// Replace with empty string
			result = strings.ReplaceAll(result, pattern, "")
			result = strings.ReplaceAll(result, strings.ToLower(pattern), "")
			result = strings.ReplaceAll(result, strings.ToUpper(pattern), "")
		}
	}
	return result
}

// ValidateIntRange ensures an integer is within the specified range.
// Returns the value clamped to min/max if out of range.
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
// Sensitive values may be configured, but are never returned to the renderer.
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

// IsSensitiveSettingKey reports whether a setting is write-only through the API.
// Sensitive values may be configured, but are never returned to the renderer.
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
// This prevents arbitrary setting access via the API.
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
