// Package crypto provides versioned encryption utilities for ViiB MediaHub.
//
// This package implements AES-256-GCM encryption for sensitive settings such as:
//   - Spotify session cookies and legacy OAuth credentials
//   - Plex Media Server tokens and device private keys
//   - Configured LLM, semantic embedding, and Last.FM API keys and secrets
//   - Values explicitly passed through the encryption helpers
//
// Versioning & Key Management:
//   - enc:v2: (current default) encrypts with a random 256-bit master key stored
//     in the application's data directory (credentials.key, mode 0600). On Windows,
//     the master key is protected at rest by Windows DPAPI (CryptProtectData bound
//     to the current user profile). On other platforms (or if OS protection is
//     unavailable), the random master key is wrapped with AES-256-GCM using a
//     relocation-safe machine/user key that intentionally excludes os.Executable(),
//     so moving or upgrading the binary never strands stored credentials.
//   - enc:v1: (legacy) derives a key from machine identifiers including
//     os.Executable(). Decrypt and DecryptAndUpgrade transparently read enc:v1:
//     values when the legacy key still matches and migrate them to enc:v2:.
//     When an enc:v1: value cannot be decrypted (for example after the binary
//     was relocated prior to upgrading to enc:v2:), Decrypt returns an error
//     wrapping ErrReauthRequired so callers can report a clear reauthentication
//     path without disclosing ciphertext or panicking.
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
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// legacyEncryptionPrefix identifies legacy v1 executable-bound ciphertexts.
	legacyEncryptionPrefix = "enc:v1:"

	// encryptionPrefixV2 identifies current v2 relocation-safe ciphertexts.
	encryptionPrefixV2 = "enc:v2:"

	// masterKeyFileName is the 0600 keyfile inside the configured data directory.
	masterKeyFileName = "credentials.key"

	// keyFileWrapPrefix identifies a relocation-safe AES-GCM wrapped master key.
	keyFileWrapPrefix = "viib-key:v2:aesgcm:"

	// keyFileOSPrefix identifies an OS-protected (e.g. Windows DPAPI) master key.
	keyFileOSPrefix = "viib-key:v2:os:"

	// keyLength is the AES-256 key length in bytes
	keyLength = 32

	// nonceLength is the GCM nonce length in bytes
	nonceLength = 12

	// pbkdf2Iterations is the number of PBKDF2 iterations for key derivation
	pbkdf2Iterations = 100000
)

// ErrReauthRequired indicates that a stored credential could not be decrypted
// with the available installation key material (for example, a legacy enc:v1
// secret written from a different executable path or copied from another machine)
// and the user must reauthenticate or re-enter the credential.
var ErrReauthRequired = errors.New("stored credential cannot be decrypted; reauthentication required")

// sensitiveKeys is the list of setting keys that should be encrypted
var sensitiveKeys = map[string]bool{
	"spotify_credentials":       true,
	"spotify_webplayer_session": true,
	"gemini_api_key":            true,
	"llm_api_key":               true,
	"lastfm_api_key":            true,
	"lastfm_shared_secret":      true,
	"lastfm_session_key":        true,
}

var (
	keyMu            sync.Mutex
	keyStoreDir      string
	cachedV2Key      []byte
	cachedV2KeyDir   string
	cachedLegacyKey  []byte
	executablePathFn = os.Executable
)

// IsSensitiveKey returns true if the given setting key should be encrypted.
//
// The sensitiveKeys map is the authoritative list for this package; it covers
// Spotify cookie/OAuth sessions, Plex credentials, Gemini/generic LLM keys,
// semantic embedding keys, and Last.FM secrets.
func IsSensitiveKey(key string) bool {
	return sensitiveKeys[key]
}

// SensitiveKeys returns a sorted snapshot of all registered sensitive setting keys.
func SensitiveKeys() []string {
	keys := make([]string, 0, len(sensitiveKeys))
	for k, ok := range sensitiveKeys {
		if ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// ConfigureKeyStore sets the directory where the v2 master key file
// (credentials.key) is stored and loaded. Passing an empty string resets to the
// default user config directory (or process-local fallback in tests).
func ConfigureKeyStore(dataDir string) error {
	cleaned := strings.TrimSpace(dataDir)
	if cleaned != "" {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return fmt.Errorf("resolve key store directory: %w", err)
		}
		cleaned = filepath.Clean(abs)
		if err := os.MkdirAll(cleaned, 0700); err != nil {
			return fmt.Errorf("create key store directory: %w", err)
		}
	}

	keyMu.Lock()
	defer keyMu.Unlock()
	if keyStoreDir != cleaned {
		keyStoreDir = cleaned
		cachedV2Key = nil
		cachedV2KeyDir = ""
	}
	return nil
}

func resolveKeyStoreDirLocked() string {
	if keyStoreDir != "" {
		return keyStoreDir
	}
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		dir := filepath.Join(configDir, "ViiB-MediaHub")
		if os.MkdirAll(dir, 0700) == nil {
			return dir
		}
	}
	return ""
}

// getV2Key returns the 32-byte AES-256 key used for enc:v2: encryption.
// When a key store directory is available, it loads or atomically creates
// credentials.key (protected via OS secret protection such as Windows DPAPI, or
// wrapped with the relocation-safe v2 machine/user key).
func getV2Key() ([]byte, error) {
	keyMu.Lock()
	defer keyMu.Unlock()

	dir := resolveKeyStoreDirLocked()
	if len(cachedV2Key) == keyLength && cachedV2KeyDir == dir {
		out := make([]byte, keyLength)
		copy(out, cachedV2Key)
		return out, nil
	}

	if dir == "" {
		fallback := deriveRelocationSafeWrapKey()
		cachedV2Key = fallback
		cachedV2KeyDir = ""
		out := make([]byte, keyLength)
		copy(out, fallback)
		return out, nil
	}

	keyPath := filepath.Join(dir, masterKeyFileName)
	if raw, err := os.ReadFile(keyPath); err == nil {
		key, parseErr := parseMasterKeyFile(strings.TrimSpace(string(raw)))
		if parseErr != nil {
			return nil, parseErr
		}
		cachedV2Key = key
		cachedV2KeyDir = dir
		out := make([]byte, keyLength)
		copy(out, key)
		return out, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read credential master key: %w", err)
	}

	masterKey := make([]byte, keyLength)
	if _, err := io.ReadFull(rand.Reader, masterKey); err != nil {
		return nil, fmt.Errorf("generate credential master key: %w", err)
	}

	serialized, err := formatMasterKeyFile(masterKey)
	if err != nil {
		return nil, err
	}

	tmpPath := keyPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(serialized+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("write credential master key: %w", err)
	}
	if err := os.Rename(tmpPath, keyPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("commit credential master key: %w", err)
	}

	cachedV2Key = masterKey
	cachedV2KeyDir = dir
	out := make([]byte, keyLength)
	copy(out, masterKey)
	return out, nil
}

func formatMasterKeyFile(masterKey []byte) (string, error) {
	if protected, scheme, ok := protectOSMasterKey(masterKey); ok && len(protected) > 0 {
		return keyFileOSPrefix + scheme + ":" + base64.StdEncoding.EncodeToString(protected), nil
	}
	wrapKey := deriveRelocationSafeWrapKey()
	ciphertext, err := sealAESGCM(wrapKey, masterKey)
	if err != nil {
		return "", fmt.Errorf("wrap credential master key: %w", err)
	}
	return keyFileWrapPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func parseMasterKeyFile(contents string) ([]byte, error) {
	switch {
	case strings.HasPrefix(contents, keyFileOSPrefix):
		rest := strings.TrimPrefix(contents, keyFileOSPrefix)
		scheme, encoded, ok := strings.Cut(rest, ":")
		if !ok || scheme == "" || encoded == "" {
			return nil, fmt.Errorf("%w: malformed OS-protected master key record", ErrReauthRequired)
		}
		payload, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid base64 in master key record: %v", ErrReauthRequired, err)
		}
		return unprotectOSMasterKey(scheme, payload)

	case strings.HasPrefix(contents, keyFileWrapPrefix):
		encoded := strings.TrimPrefix(contents, keyFileWrapPrefix)
		payload, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid base64 in wrapped master key: %v", ErrReauthRequired, err)
		}
		wrapKey := deriveRelocationSafeWrapKey()
		plain, err := openAESGCM(wrapKey, payload)
		if err != nil || len(plain) != keyLength {
			return nil, fmt.Errorf("%w: master key unwrap failed", ErrReauthRequired)
		}
		return plain, nil

	default:
		return nil, fmt.Errorf("%w: unrecognized credential master key format", ErrReauthRequired)
	}
}

// deriveRelocationSafeWrapKey derives a 256-bit key bound to the machine and
// user account, intentionally excluding os.Executable() so relocating or
// upgrading the application binary never invalidates the key.
func deriveRelocationSafeWrapKey() []byte {
	salt := getMachineSaltV2()
	passphrase := getRelocationSafePassphrase()
	return pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, keyLength, sha256.New)
}

func getMachineSaltV2() []byte {
	var identifiers []string
	switch runtime.GOOS {
	case "windows":
		if name := os.Getenv("COMPUTERNAME"); name != "" {
			identifiers = append(identifiers, name)
		}
		if user := os.Getenv("USERNAME"); user != "" {
			identifiers = append(identifiers, user)
		}
		if productID := os.Getenv("PROCESSOR_IDENTIFIER"); productID != "" {
			identifiers = append(identifiers, productID)
		}
	case "darwin":
		if host, err := os.Hostname(); err == nil {
			identifiers = append(identifiers, host)
		}
		if user := os.Getenv("USER"); user != "" {
			identifiers = append(identifiers, user)
		}
		if home := os.Getenv("HOME"); home != "" {
			identifiers = append(identifiers, home)
		}
	default:
		if host, err := os.Hostname(); err == nil {
			identifiers = append(identifiers, host)
		}
		if user := os.Getenv("USER"); user != "" {
			identifiers = append(identifiers, user)
		}
		if data, err := os.ReadFile("/etc/machine-id"); err == nil {
			identifiers = append(identifiers, strings.TrimSpace(string(data)))
		}
	}
	identifiers = append(identifiers, "viib-mediahub-v2")
	sum := sha256.Sum256([]byte(strings.Join(identifiers, "::")))
	return sum[:]
}

func getRelocationSafePassphrase() string {
	var parts []string
	if host, err := os.Hostname(); err == nil {
		parts = append(parts, host)
	}
	if home, err := os.UserHomeDir(); err == nil {
		parts = append(parts, home)
	}
	if configDir, err := os.UserConfigDir(); err == nil {
		parts = append(parts, configDir)
	}
	parts = append(parts, "viib-mediahub-keywrap-v2")
	return strings.Join(parts, "||")
}

// getLegacyV1Key derives the legacy v1 encryption key (which includes the
// executable path) for backward-compatible decryption and migration of enc:v1:
// values.
func getLegacyV1Key() []byte {
	keyMu.Lock()
	defer keyMu.Unlock()
	if len(cachedLegacyKey) == keyLength {
		out := make([]byte, keyLength)
		copy(out, cachedLegacyKey)
		return out
	}
	salt := getMachineSalt()
	passphrase := getMachinePassphrase()
	cachedLegacyKey = pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, keyLength, sha256.New)
	out := make([]byte, keyLength)
	copy(out, cachedLegacyKey)
	return out
}

// getMachineSalt generates the legacy v1 cryptographic salt.
func getMachineSalt() []byte {
	var identifiers []string

	switch runtime.GOOS {
	case "windows":
		if name := os.Getenv("COMPUTERNAME"); name != "" {
			identifiers = append(identifiers, name)
		}
		if user := os.Getenv("USERNAME"); user != "" {
			identifiers = append(identifiers, user)
		}
		if productID := os.Getenv("PROCESSOR_IDENTIFIER"); productID != "" {
			identifiers = append(identifiers, productID)
		}
	case "darwin":
		if host, err := os.Hostname(); err == nil {
			identifiers = append(identifiers, host)
		}
		if user := os.Getenv("USER"); user != "" {
			identifiers = append(identifiers, user)
		}
		if home := os.Getenv("HOME"); home != "" {
			identifiers = append(identifiers, home)
		}
	default:
		if host, err := os.Hostname(); err == nil {
			identifiers = append(identifiers, host)
		}
		if user := os.Getenv("USER"); user != "" {
			identifiers = append(identifiers, user)
		}
		if data, err := os.ReadFile("/etc/machine-id"); err == nil {
			identifiers = append(identifiers, strings.TrimSpace(string(data)))
		}
	}

	identifiers = append(identifiers, "viib-mediahub-v1")

	combined := strings.Join(identifiers, "::")
	hash := sha256.Sum256([]byte(combined))
	return hash[:]
}

// getMachinePassphrase generates the legacy v1 passphrase (which includes executable path).
func getMachinePassphrase() string {
	var parts []string

	if host, err := os.Hostname(); err == nil {
		parts = append(parts, host)
	}

	if exe, err := executablePathFn(); err == nil {
		parts = append(parts, exe)
	}

	if home, err := os.UserHomeDir(); err == nil {
		parts = append(parts, home)
	}

	parts = append(parts, "viib-mediahub-secret-2024")

	return strings.Join(parts, "||")
}

func sealAESGCM(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, nonceLength)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func openAESGCM(key, data []byte) ([]byte, error) {
	if len(data) < nonceLength {
		return nil, fmt.Errorf("%w: ciphertext too short", ErrReauthRequired)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := data[:nonceLength]
	ciphertextBytes := data[nonceLength:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decrypt: %v", ErrReauthRequired, err)
	}
	return plaintext, nil
}

// encryptLegacyV1 encrypts plaintext with the legacy v1 executable-bound key.
// Retained for migration and relocation tests.
func encryptLegacyV1(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	ciphertext, err := sealAESGCM(getLegacyV1Key(), []byte(plaintext))
	if err != nil {
		return "", err
	}
	return legacyEncryptionPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Encrypt encrypts plaintext using AES-256-GCM with the v2 credential key.
// Returns a base64-encoded string prefixed with "enc:v2:" for identification.
// Returns empty string for empty input (no encryption needed).
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key, err := getV2Key()
	if err != nil {
		return "", fmt.Errorf("failed to get encryption key: %w", err)
	}
	ciphertext, err := sealAESGCM(key, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return encryptionPrefixV2 + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a value that was encrypted with Encrypt ("enc:v2:") or the
// legacy v1 scheme ("enc:v1:").
//
// Backward Compatibility:
//   - Plaintext without an "enc:v1:" or "enc:v2:" prefix is returned unchanged.
//   - "enc:v1:" values are decrypted with the legacy v1 key (and fallback to
//     relocation-safe v2 if applicable).
//   - If an encrypted value cannot be decrypted (e.g., corrupted ciphertext or
//     an enc:v1: secret stranded by binary relocation), the returned error wraps
//     ErrReauthRequired.
func Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	switch {
	case strings.HasPrefix(ciphertext, encryptionPrefixV2):
		encoded := strings.TrimPrefix(ciphertext, encryptionPrefixV2)
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", fmt.Errorf("%w: failed to decode base64: %v", ErrReauthRequired, err)
		}
		key, err := getV2Key()
		if err != nil {
			return "", fmt.Errorf("failed to get encryption key: %w", err)
		}
		plaintext, err := openAESGCM(key, data)
		if err != nil {
			return "", err
		}
		return string(plaintext), nil

	case strings.HasPrefix(ciphertext, legacyEncryptionPrefix):
		encoded := strings.TrimPrefix(ciphertext, legacyEncryptionPrefix)
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", fmt.Errorf("%w: failed to decode base64: %v", ErrReauthRequired, err)
		}
		plaintext, err := openAESGCM(getLegacyV1Key(), data)
		if err != nil {
			return "", fmt.Errorf("%w: legacy enc:v1 credential cannot be decrypted after executable relocation or machine change; please reauthenticate", ErrReauthRequired)
		}
		return string(plaintext), nil

	default:
		// Not encrypted, return as-is (for backward compatibility)
		return ciphertext, nil
	}
}

// DecryptAndUpgrade decrypts ciphertext and, if it was stored in plaintext or
// legacy "enc:v1:" format, re-encrypts it into "enc:v2:" format.
//
// Returns:
//   - plaintext: The decrypted value
//   - upgradedCiphertext: The new "enc:v2:" value when upgraded is true
//   - upgraded: true when the caller should persist upgradedCiphertext
//   - err: Any error during decryption or re-encryption (wraps ErrReauthRequired on key mismatch)
func DecryptAndUpgrade(ciphertext string) (plaintext string, upgradedCiphertext string, upgraded bool, err error) {
	if ciphertext == "" {
		return "", "", false, nil
	}
	plaintext, err = Decrypt(ciphertext)
	if err != nil {
		return "", "", false, err
	}
	if IsCurrentEncrypted(ciphertext) {
		return plaintext, ciphertext, false, nil
	}
	upgradedCiphertext, err = Encrypt(plaintext)
	if err != nil {
		return plaintext, "", false, err
	}
	return plaintext, upgradedCiphertext, true, nil
}

// IsEncrypted returns true if the value has any recognized encryption prefix
// ("enc:v1:" or "enc:v2:").
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, encryptionPrefixV2) || strings.HasPrefix(value, legacyEncryptionPrefix)
}

// IsCurrentEncrypted returns true if the value uses the current "enc:v2:" format.
func IsCurrentEncrypted(value string) bool {
	return strings.HasPrefix(value, encryptionPrefixV2)
}

// IsLegacyEncrypted returns true if the value uses the legacy "enc:v1:" format.
func IsLegacyEncrypted(value string) bool {
	return strings.HasPrefix(value, legacyEncryptionPrefix)
}

// MigrateUnencryptedValue checks if a value is plaintext or legacy "enc:v1:"
// and upgrades it to "enc:v2:".
//
// Returns:
//   - encrypted: The "enc:v2:" encrypted value (or original if already "enc:v2:" or empty)
//   - migrated: true if encryption or v1->v2 upgrade was performed, false otherwise
//   - error: Any error during decryption or encryption
func MigrateUnencryptedValue(value string) (string, bool, error) {
	if value == "" {
		return "", false, nil
	}
	if IsCurrentEncrypted(value) {
		return value, false, nil
	}
	_, upgraded, migrated, err := DecryptAndUpgrade(value)
	if err != nil {
		return "", false, err
	}
	return upgraded, migrated, nil
}

