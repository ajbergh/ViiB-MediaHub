// Package crypto provides encryption utilities for ViiB MediaHub.
//
// This package implements AES-256-GCM encryption for sensitive data such as:
//   - Spotify OAuth credentials (access tokens, refresh tokens, client secrets)
//   - Gemini API keys
//   - Other sensitive configuration values
//
// The encryption key is derived from machine-specific information to ensure
// that encrypted data can only be decrypted on the same machine where it was
// encrypted. This provides protection against database file theft.
//
// Key Derivation:
//   - Uses PBKDF2 with SHA-256 to derive a 256-bit key
//   - Salt is derived from machine-specific identifiers
//   - Provides defense against rainbow table attacks
//
// Encryption Format:
//   - Uses AES-256-GCM for authenticated encryption
//   - Nonce is prepended to ciphertext and stored together
//   - Base64 encoded for safe storage in database text fields
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// encryptionPrefix is prepended to encrypted values to identify them
	encryptionPrefix = "enc:v1:"

	// keyLength is the AES-256 key length in bytes
	keyLength = 32

	// nonceLength is the GCM nonce length in bytes
	nonceLength = 12

	// pbkdf2Iterations is the number of PBKDF2 iterations for key derivation
	pbkdf2Iterations = 100000
)

// sensitiveKeys is the list of setting keys that should be encrypted
var sensitiveKeys = map[string]bool{
	"spotify_credentials":  true,
	"gemini_api_key":       true,
	"llm_api_key":          true,
	"lastfm_api_key":       true,
	"lastfm_shared_secret": true,
	"lastfm_session_key":   true,
}

var (
	derivedKey []byte
	keyOnce    sync.Once
	keyErr     error
)

// IsSensitiveKey returns true if the given setting key should be encrypted.
//
// Currently protected keys:
//   - "spotify_credentials": Contains OAuth tokens and client secrets
//   - "gemini_api_key": Contains the Google Gemini AI API key
//
// To add new sensitive keys, add them to the sensitiveKeys map.
func IsSensitiveKey(key string) bool {
	return sensitiveKeys[key]
}

// getEncryptionKey derives a machine-specific encryption key using PBKDF2.
// The key is cached after first derivation for performance.
func getEncryptionKey() ([]byte, error) {
	keyOnce.Do(func() {
		salt := getMachineSalt()
		passphrase := getMachinePassphrase()

		derivedKey = pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, keyLength, sha256.New)
	})

	if keyErr != nil {
		return nil, keyErr
	}
	return derivedKey, nil
}

// getMachineSalt generates a cryptographic salt based on machine-specific identifiers.
// This ensures the same salt is generated on the same machine, binding encrypted
// data to the specific installation.
//
// Identifiers used vary by OS:
//   - Windows: COMPUTERNAME, USERNAME, PROCESSOR_IDENTIFIER
//   - macOS: hostname, USER, HOME directory
//   - Linux: hostname, USER, /etc/machine-id
//
// The identifiers are hashed with SHA-256 to produce a fixed 32-byte salt.
func getMachineSalt() []byte {
	// Combine multiple machine identifiers for uniqueness
	var identifiers []string

	// Add OS-specific identifiers
	switch runtime.GOOS {
	case "windows":
		// Use COMPUTERNAME and USERNAME
		if name := os.Getenv("COMPUTERNAME"); name != "" {
			identifiers = append(identifiers, name)
		}
		if user := os.Getenv("USERNAME"); user != "" {
			identifiers = append(identifiers, user)
		}
		// Add Windows product ID if available
		if productID := os.Getenv("PROCESSOR_IDENTIFIER"); productID != "" {
			identifiers = append(identifiers, productID)
		}
	case "darwin":
		// Use hostname and user
		if host, err := os.Hostname(); err == nil {
			identifiers = append(identifiers, host)
		}
		if user := os.Getenv("USER"); user != "" {
			identifiers = append(identifiers, user)
		}
		// Add home directory for additional uniqueness
		if home := os.Getenv("HOME"); home != "" {
			identifiers = append(identifiers, home)
		}
	default:
		// Linux and other Unix-like systems
		if host, err := os.Hostname(); err == nil {
			identifiers = append(identifiers, host)
		}
		if user := os.Getenv("USER"); user != "" {
			identifiers = append(identifiers, user)
		}
		// Try to read machine-id
		if data, err := os.ReadFile("/etc/machine-id"); err == nil {
			identifiers = append(identifiers, strings.TrimSpace(string(data)))
		}
	}

	// Add application identifier for namespace separation
	identifiers = append(identifiers, "viib-mediahub-v1")

	// Hash all identifiers together to create a fixed-length salt
	combined := strings.Join(identifiers, "::")
	hash := sha256.Sum256([]byte(combined))
	return hash[:]
}

// getMachinePassphrase generates a passphrase based on machine identifiers.
// Uses different identifiers than getMachineSalt for defense in depth.
// The passphrase is stretched via PBKDF2 to derive the encryption key.
//
// Components:
//   - Hostname
//   - Executable path (prevents key reuse if app is moved)
//   - User home directory
//   - Static application secret
func getMachinePassphrase() string {
	var parts []string

	// Use different combinations than salt for security
	if host, err := os.Hostname(); err == nil {
		parts = append(parts, host)
	}

	// Add executable path for additional binding
	if exe, err := os.Executable(); err == nil {
		parts = append(parts, exe)
	}

	// Add user home directory
	if home, err := os.UserHomeDir(); err == nil {
		parts = append(parts, home)
	}

	// Add a static application secret
	parts = append(parts, "viib-mediahub-secret-2024")

	return strings.Join(parts, "||")
}

// Encrypt encrypts plaintext using AES-256-GCM with the machine-derived key.
// Returns a base64-encoded string prefixed with "enc:v1:" for identification.
//
// The encryption process:
//  1. Derives a 256-bit key from machine-specific identifiers using PBKDF2
//  2. Generates a random 12-byte nonce for each encryption
//  3. Encrypts using AES-256-GCM (provides both confidentiality and integrity)
//  4. Prepends the nonce to the ciphertext
//  5. Base64 encodes and adds the "enc:v1:" prefix
//
// Security properties:
//   - Authenticated encryption: tampering is detected
//   - Random nonce: same plaintext produces different ciphertext
//   - Machine-bound: data can only be decrypted on the same machine
//
// Returns empty string for empty input (no encryption needed).
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	key, err := getEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("failed to get encryption key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate a random nonce
	nonce := make([]byte, nonceLength)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and append nonce to beginning of ciphertext
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode as base64 and add prefix
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return encryptionPrefix + encoded, nil
}

// Decrypt decrypts a value that was encrypted with Encrypt.
// Returns the original plaintext.
//
// Backward Compatibility:
// If the value does not have the "enc:v1:" prefix, it is assumed to be
// unencrypted plaintext and is returned unchanged. This allows seamless
// migration from older versions that stored secrets in plaintext.
//
// The decryption process:
//  1. Checks for "enc:v1:" prefix (returns unchanged if not present)
//  2. Base64 decodes the ciphertext
//  3. Extracts the 12-byte nonce from the beginning
//  4. Derives the same key from machine identifiers
//  5. Decrypts and verifies integrity using AES-256-GCM
//
// Returns error if:
//   - Ciphertext is corrupted or tampered with
//   - Attempting to decrypt on a different machine
//   - Base64 encoding is invalid
func Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// Check if this is an encrypted value
	if !strings.HasPrefix(ciphertext, encryptionPrefix) {
		// Not encrypted, return as-is (for backward compatibility)
		return ciphertext, nil
	}

	// Remove prefix and decode base64
	encoded := strings.TrimPrefix(ciphertext, encryptionPrefix)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	if len(data) < nonceLength {
		return "", errors.New("ciphertext too short")
	}

	key, err := getEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("failed to get encryption key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce and ciphertext
	nonce := data[:nonceLength]
	ciphertextBytes := data[nonceLength:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted returns true if the value appears to be encrypted.
// Checks for the "enc:v1:" prefix that is added by the Encrypt function.
// This is a heuristic check - it does not verify the encryption is valid.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, encryptionPrefix)
}

// MigrateUnencryptedValue checks if a value needs encryption and encrypts it.
// Used during database migration to encrypt existing plaintext secrets.
//
// Returns:
//   - encrypted: The encrypted value (or original if already encrypted/empty)
//   - migrated: true if encryption was performed, false otherwise
//   - error: Any error during encryption
//
// This function is idempotent - calling it multiple times on the same value
// will only encrypt once.
func MigrateUnencryptedValue(value string) (string, bool, error) {
	if value == "" {
		return "", false, nil
	}

	// Already encrypted
	if IsEncrypted(value) {
		return value, false, nil
	}

	// Encrypt the value
	encrypted, err := Encrypt(value)
	if err != nil {
		return "", false, err
	}

	return encrypted, true, nil
}
