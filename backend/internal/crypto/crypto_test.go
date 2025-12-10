package crypto

import (
	"strings"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
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

			// Should have encryption prefix
			if !strings.HasPrefix(encrypted, encryptionPrefix) {
				t.Errorf("Expected prefix %q, got %q", encryptionPrefix, encrypted[:min(len(encrypted), 10)])
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
	encrypted, _ := Encrypt("test")

	if !IsEncrypted(encrypted) {
		t.Error("IsEncrypted should return true for encrypted value")
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
		{"gemini_api_key", true},
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

func TestMigrateUnencryptedValue(t *testing.T) {
	// Test with plain value
	value := "plain secret"
	encrypted, migrated, err := MigrateUnencryptedValue(value)
	if err != nil {
		t.Fatalf("MigrateUnencryptedValue failed: %v", err)
	}
	if !migrated {
		t.Error("Expected migration to occur")
	}
	if !IsEncrypted(encrypted) {
		t.Error("Result should be encrypted")
	}

	// Test with already encrypted value
	encrypted2, migrated2, err := MigrateUnencryptedValue(encrypted)
	if err != nil {
		t.Fatalf("MigrateUnencryptedValue failed on encrypted input: %v", err)
	}
	if migrated2 {
		t.Error("Should not migrate already encrypted value")
	}
	if encrypted2 != encrypted {
		t.Error("Should return same value for already encrypted input")
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

func TestEncryptionDeterminism(t *testing.T) {
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
