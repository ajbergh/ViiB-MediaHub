// Tests versioned credential key storage (enc:v2), automatic migration from
// plaintext and legacy enc:v1 settings across restarts, and ErrReauthRequired
// reporting for unreadable legacy entries.
package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/crypto"
)

func encryptFakeLegacyV1WithWrongKey(t *testing.T, plaintext string) string {
	t.Helper()
	wrongKey := make([]byte, 32)
	for i := range wrongKey {
		wrongKey[i] = byte(i + 1)
	}
	block, err := aes.NewCipher(wrongKey)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatal(err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:v1:" + base64.StdEncoding.EncodeToString(sealed)
}

func TestCredentialMigrationUpgradesAllSensitiveSettingsToV2(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "library.db")

	database, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Write plaintext rows directly to simulate pre-v2 / legacy rows for all
	// sensitive settings (including plex_credentials and semantic_embedding_api_key).
	for _, key := range crypto.SensitiveKeys() {
		rawPlain := "secret-for-" + key
		if _, err := database.conn.Exec(
			`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			key, rawPlain,
		); err != nil {
			t.Fatalf("insert raw setting %s: %v", key, err)
		}
	}
	_ = database.Close()

	// Reopen database -> migrateUnencryptedSettings runs automatically on startup.
	reopened, err := New(dbPath)
	if err != nil {
		t.Fatalf("Reopen New: %v", err)
	}
	defer reopened.Close()

	for _, key := range crypto.SensitiveKeys() {
		var rawStored string
		if err := reopened.conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&rawStored); err != nil {
			t.Fatalf("scan raw %s: %v", key, err)
		}
		if !crypto.IsCurrentEncrypted(rawStored) {
			t.Fatalf("setting %s raw value = %q, want enc:v2: prefix", key, rawStored)
		}
		if strings.Contains(rawStored, "secret-for-") {
			t.Fatalf("setting %s raw value leaked plaintext", key)
		}
		decrypted, err := reopened.GetSetting(key)
		if err != nil {
			t.Fatalf("GetSetting(%s): %v", key, err)
		}
		if decrypted != "secret-for-"+key {
			t.Fatalf("GetSetting(%s) = %q, want %q", key, decrypted, "secret-for-"+key)
		}
	}
}

func TestUnresolvableLegacyV1SettingDoesNotBlockStartupAndReportsReauth(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "library.db")

	database, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	strandedV1 := encryptFakeLegacyV1WithWrongKey(t, "stranded-plex-secret")
	if _, err := database.conn.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		PlexCredentialsSettingKey, strandedV1,
	); err != nil {
		t.Fatalf("insert stranded v1: %v", err)
	}
	if err := database.SetSetting("llm_api_key", "valid-v2-llm-key"); err != nil {
		t.Fatalf("SetSetting llm_api_key: %v", err)
	}
	_ = database.Close()

	// Reopening the database must succeed even when a legacy v1 row is unreadable.
	reopened, err := New(dbPath)
	if err != nil {
		t.Fatalf("Reopen with stranded v1 row failed: %v", err)
	}
	defer reopened.Close()

	// Readable v2 settings continue working normally.
	llmKey, err := reopened.GetSetting("llm_api_key")
	if err != nil || llmKey != "valid-v2-llm-key" {
		t.Fatalf("GetSetting(llm_api_key) = %q, err=%v", llmKey, err)
	}

	// Reading the stranded v1 setting returns ErrReauthRequired without disclosing ciphertext.
	plexVal, err := reopened.GetSetting(PlexCredentialsSettingKey)
	if err == nil || plexVal != "" {
		t.Fatalf("expected error for stranded v1 setting, got val=%q err=%v", plexVal, err)
	}
	if !errors.Is(err, crypto.ErrReauthRequired) {
		t.Fatalf("expected ErrReauthRequired, got %v", err)
	}

	// Re-authenticating (writing a new secret) replaces the stranded row with enc:v2:.
	if err := reopened.SetSetting(PlexCredentialsSettingKey, `{"accountToken":"fresh-token"}`); err != nil {
		t.Fatalf("SetSetting replacement: %v", err)
	}
	recovered, err := reopened.GetSetting(PlexCredentialsSettingKey)
	if err != nil || recovered != `{"accountToken":"fresh-token"}` {
		t.Fatalf("recovered setting = %q, err=%v", recovered, err)
	}
}

func TestCredentialKeysRemainBoundToDatabase(t *testing.T) {
	first, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := first.SetSetting("llm_api_key", "first-secret"); err != nil {
		t.Fatal(err)
	}
	second, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := second.SetSetting("llm_api_key", "second-secret"); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		database *DB
		want     string
	}{{first, "first-secret"}, {second, "second-secret"}} {
		got, err := entry.database.GetSetting("llm_api_key")
		if err != nil || got != entry.want {
			t.Fatalf("credential changed after opening another database: got %q, err %v", got, err)
		}
		if err := entry.database.SetSettingsBatch(map[string]string{"gemini_api_key": entry.want}); err != nil {
			t.Fatal(err)
		}
		got, err = entry.database.GetSetting("gemini_api_key")
		if err != nil || got != entry.want {
			t.Fatalf("batch credential: got %q, err %v", got, err)
		}
	}
}
