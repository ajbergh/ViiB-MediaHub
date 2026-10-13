// Tests and fixtures for crypto behavior.

package crypto

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useTempKeyStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := ConfigureKeyStore(dir); err != nil {
		t.Fatalf("ConfigureKeyStore: %v", err)
	}
	t.Cleanup(func() {
		_ = ConfigureKeyStore("")
		keyMu.Lock()
		cachedLegacyKey = nil
		executablePathFn = os.Executable
		keyMu.Unlock()
	})
	return dir
}

func TestEncryptDecrypt(t *testing.T) {
	useTempKeyStore(t)
	testCases := []struct {
		name      string
		plaintext string
	}{
		{"empty string", ""},
		{"simple text", "hello world"},
		{"json data", `{"accessToken":"abc123","refreshToken":"xyz789"}`},
		{"unicode", "こんにちは世界 🎵"},
		{"special chars", `!@#$%^&*()_+-=[]{}|;':",.<>?/~` + "`"},
		{"long text", strings.Repeat("a", 10000)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := Encrypt(tc.plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Empty string should return empty
			if tc.plaintext == "" {
				if encrypted != "" {
					t.Errorf("Expected empty string, got %q", encrypted)
				}
				return
			}

			// Should have v2 encryption prefix
			if !strings.HasPrefix(encrypted, encryptionPrefixV2) {
				t.Errorf("Expected prefix %q, got %q", encryptionPrefixV2, encrypted[:min(len(encrypted), 10)])
			}

			// Encrypted should be different from plaintext
			if encrypted == tc.plaintext {
				t.Error("Encrypted value should differ from plaintext")
			}

			// Decrypt
			decrypted, err := Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			// Should match original
			if decrypted != tc.plaintext {
				t.Errorf("Decrypted %q does not match original %q", decrypted, tc.plaintext)
			}
		})
	}
}

func TestDecryptUnencryptedValue(t *testing.T) {
	useTempKeyStore(t)
	// Unencrypted values should be returned as-is for backward compatibility
	plaintext := "plain text value"
	result, err := Decrypt(plaintext)
	if err != nil {
		t.Fatalf("Decrypt of unencrypted value failed: %v", err)
	}
	if result != plaintext {
		t.Errorf("Expected %q, got %q", plaintext, result)
	}
}

func TestIsEncrypted(t *testing.T) {
	useTempKeyStore(t)
	encrypted, _ := Encrypt("test")
	legacyEncrypted, _ := encryptLegacyV1("test")

	if !IsEncrypted(encrypted) || !IsCurrentEncrypted(encrypted) || IsLegacyEncrypted(encrypted) {
		t.Error("v2 encrypted value classification failed")
	}

	if !IsEncrypted(legacyEncrypted) || IsCurrentEncrypted(legacyEncrypted) || !IsLegacyEncrypted(legacyEncrypted) {
		t.Error("v1 encrypted value classification failed")
	}

	if IsEncrypted("plain text") {
		t.Error("IsEncrypted should return false for plain text")
	}

	if IsEncrypted("") {
		t.Error("IsEncrypted should return false for empty string")
	}
}

func TestIsSensitiveKey(t *testing.T) {
	testCases := []struct {
		key      string
		expected bool
	}{
		{"spotify_credentials", true},
		{"spotify_webplayer_session", true},
		{"plex_credentials", true},
		{"semantic_embedding_api_key", true},
		{"gemini_api_key", true},
		{"llm_api_key", true},
		{"lastfm_api_key", true},
		{"lastfm_shared_secret", true},
		{"lastfm_session_key", true},
		{"concurrent_downloads", false},
		{"spotify_download_path", false},
		{"random_setting", false},
	}

	for _, tc := range testCases {
		t.Run(tc.key, func(t *testing.T) {
			result := IsSensitiveKey(tc.key)
			if result != tc.expected {
				t.Errorf("IsSensitiveKey(%q) = %v, expected %v", tc.key, result, tc.expected)
			}
		})
	}
}

func TestMigrateUnencryptedValueAndLegacyV1Upgrade(t *testing.T) {
	useTempKeyStore(t)

	// Test with plain value
	value := "plain secret"
	encrypted, migrated, err := MigrateUnencryptedValue(value)
	if err != nil {
		t.Fatalf("MigrateUnencryptedValue failed: %v", err)
	}
	if !migrated {
		t.Error("Expected migration to occur")
	}
	if !IsCurrentEncrypted(encrypted) {
		t.Errorf("Result should be enc:v2:, got %q", encrypted)
	}

	// Test with already v2 encrypted value
	encrypted2, migrated2, err := MigrateUnencryptedValue(encrypted)
	if err != nil {
		t.Fatalf("MigrateUnencryptedValue failed on encrypted input: %v", err)
	}
	if migrated2 {
		t.Error("Should not migrate already v2 encrypted value")
	}
	if encrypted2 != encrypted {
		t.Error("Should return same value for already v2 encrypted input")
	}

	// Test with legacy enc:v1: value -> upgrades to enc:v2:
	legacyCiphertext, err := encryptLegacyV1("legacy-secret-value")
	if err != nil {
		t.Fatalf("encryptLegacyV1: %v", err)
	}
	upgraded, migratedV1, err := MigrateUnencryptedValue(legacyCiphertext)
	if err != nil {
		t.Fatalf("MigrateUnencryptedValue on v1 failed: %v", err)
	}
	if !migratedV1 || !IsCurrentEncrypted(upgraded) {
		t.Fatalf("Expected v1 ciphertext to upgrade to enc:v2:, got migrated=%v value=%q", migratedV1, upgraded)
	}
	decrypted, err := Decrypt(upgraded)
	if err != nil || decrypted != "legacy-secret-value" {
		t.Fatalf("Decrypt(upgraded) = %q, err=%v", decrypted, err)
	}

	// Test with empty value
	empty, migrated3, err := MigrateUnencryptedValue("")
	if err != nil {
		t.Fatalf("MigrateUnencryptedValue failed on empty: %v", err)
	}
	if migrated3 {
		t.Error("Should not migrate empty value")
	}
	if empty != "" {
		t.Error("Should return empty for empty input")
	}
}

func TestV2SurvivesExecutableRelocationAndRestart(t *testing.T) {
	dir := useTempKeyStore(t)

	keyMu.Lock()
	executablePathFn = func() (string, error) { return `C:\Program Files\ViiB\ViiB-MediaHub.exe`, nil }
	cachedLegacyKey = nil
	cachedV2Key = nil
	keyMu.Unlock()

	ciphertext, err := Encrypt("relocation-resilient-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, masterKeyFileName)); err != nil {
		t.Fatalf("expected master key file to be created: %v", err)
	}

	// Simulate moving the executable to a new path and restarting the process
	// (clearing in-memory key caches while keeping the same data directory).
	keyMu.Lock()
	executablePathFn = func() (string, error) { return `D:\Apps\Moved\ViiB-MediaHub.exe`, nil }
	cachedLegacyKey = nil
	cachedV2Key = nil
	cachedV2KeyDir = ""
	keyMu.Unlock()

	decrypted, err := Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt after binary relocation failed: %v", err)
	}
	if decrypted != "relocation-resilient-secret" {
		t.Fatalf("Decrypt after relocation = %q, want %q", decrypted, "relocation-resilient-secret")
	}
}

func TestUnresolvableLegacyV1ReportsErrReauthRequired(t *testing.T) {
	useTempKeyStore(t)

	keyMu.Lock()
	executablePathFn = func() (string, error) { return `/opt/old-location/viib`, nil }
	cachedLegacyKey = nil
	keyMu.Unlock()

	legacyCiphertext, err := encryptLegacyV1("stranded-v1-secret")
	if err != nil {
		t.Fatalf("encryptLegacyV1: %v", err)
	}

	// Relocate binary before v1->v2 migration has occurred.
	keyMu.Lock()
	executablePathFn = func() (string, error) { return `/usr/local/bin/viib`, nil }
	cachedLegacyKey = nil
	keyMu.Unlock()

	plain, err := Decrypt(legacyCiphertext)
	if err == nil {
		t.Fatalf("expected error decrypting stranded v1 secret, got %q", plain)
	}
	if !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("expected ErrReauthRequired, got %v", err)
	}
	if strings.Contains(err.Error(), "stranded-v1-secret") {
		t.Fatal("error leaked plaintext")
	}
}

func TestEncryptionDeterminism(t *testing.T) {
	useTempKeyStore(t)
	// Each encryption should produce different ciphertext (due to random nonce)
	plaintext := "same input"
	encrypted1, _ := Encrypt(plaintext)
	encrypted2, _ := Encrypt(plaintext)

	if encrypted1 == encrypted2 {
		t.Error("Encryption should be non-deterministic (random nonce)")
	}

	// But both should decrypt to the same value
	decrypted1, _ := Decrypt(encrypted1)
	decrypted2, _ := Decrypt(encrypted2)

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Both should decrypt to original plaintext")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

