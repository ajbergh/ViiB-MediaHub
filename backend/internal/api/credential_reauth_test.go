// Tests HTTP handler behavior when stored legacy enc:v1 credentials cannot be
// decrypted (crypto.ErrReauthRequired) across Plex, Spotify, semantic
// embedding, and generic settings endpoints, and verifies recovery on re-auth.
package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/crypto"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

func encryptStrandedLegacyV1ForTest(t *testing.T, plaintext string) string {
	t.Helper()
	wrongKey := make([]byte, 32)
	for i := range wrongKey {
		wrongKey[i] = byte(0xA0 + i)
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

func writeRawSettingForTest(t *testing.T, dbPath, key, rawValue string) {
	t.Helper()
	rawDB, err := sql.Open("viib_sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()
	if _, err := rawDB.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, rawValue,
	); err != nil {
		t.Fatalf("write raw setting %s: %v", key, err)
	}
}

func TestAPIHandlersReportReauthRequiredOnStrandedLegacyV1Credentials(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "library.db")

	// Seed database with stranded enc:v1: secrets before opening API.
	seedDB, err := db.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = seedDB.Close()

	strandedCiphertext := encryptStrandedLegacyV1ForTest(t, "secret-credential-payload")
	writeRawSettingForTest(t, dbPath, db.PlexCredentialsSettingKey, strandedCiphertext)
	writeRawSettingForTest(t, dbPath, spotifyCookieSetting, strandedCiphertext)
	writeRawSettingForTest(t, dbPath, semantic.SemanticEmbeddingProviderSetting, semantic.EmbeddingProviderOpenAI)
	writeRawSettingForTest(t, dbPath, semantic.SemanticEmbeddingAPIKeySetting, strandedCiphertext)
	writeRawSettingForTest(t, dbPath, "gemini_api_key", strandedCiphertext)

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("reopen db with stranded credentials: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	apiHandler := New(database, dir)
	t.Cleanup(apiHandler.Close)

	// 1. Plex GET /config reports reauthRequired=true and authenticated=false without 500
	t.Run("Plex config and auth status surface reauth requirement", func(t *testing.T) {
		plexRouter := apiHandler.PlexRoutes()

		req := httptest.NewRequest(http.MethodGet, "/config", nil)
		rec := httptest.NewRecorder()
		plexRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /plex/config status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
		}
		var cfg map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg["authenticated"] != false || cfg["reauthRequired"] != true {
			t.Fatalf("expected authenticated=false and reauthRequired=true, got %+v", cfg)
		}
		if strings.Contains(rec.Body.String(), "secret-credential-payload") || strings.Contains(rec.Body.String(), "enc:v1:") {
			t.Fatal("Plex config response leaked secret or raw ciphertext")
		}

		statusReq := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
		statusRec := httptest.NewRecorder()
		plexRouter.ServeHTTP(statusRec, statusReq)
		if statusRec.Code != http.StatusUnauthorized {
			t.Fatalf("GET /plex/auth/status status = %d, want 401 (body=%s)", statusRec.Code, statusRec.Body.String())
		}
		var errPayload struct {
			Error V2Error `json:"error"`
		}
		if err := json.Unmarshal(statusRec.Body.Bytes(), &errPayload); err != nil {
			t.Fatal(err)
		}
		if errPayload.Error.Code != "plex_reauth_required" {
			t.Fatalf("expected error code plex_reauth_required, got %+v", errPayload.Error)
		}
	})

	// 2. Spotify GET /spotify/auth/status reports authRequired=true with clear reauth message
	t.Run("Spotify auth status surfaces reauth message", func(t *testing.T) {
		apiRouter := apiHandler.Routes()
		req := httptest.NewRequest(http.MethodGet, "/spotify/auth/status", nil)
		rec := httptest.NewRecorder()
		apiRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /spotify/auth/status status = %d, want 200", rec.Code)
		}
		var status spotifySessionStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if status.Connected || !status.AuthRequired || !strings.Contains(status.Message, "sign in to Spotify again") {
			t.Fatalf("unexpected Spotify auth status on stranded cookie: %+v", status)
		}
	})

	// 3. Semantic GET /semantic/settings reports needs_configuration with reauth reason and allows replacement
	t.Run("Semantic settings surface reauth reason and allow saving new key", func(t *testing.T) {
		apiRouter := apiHandler.Routes()
		req := httptest.NewRequest(http.MethodGet, "/semantic/settings", nil)
		rec := httptest.NewRecorder()
		apiRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /semantic/settings status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
		}
		var semResp semanticSettingsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &semResp); err != nil {
			t.Fatal(err)
		}
		if semResp.APIKeyConfigured || semResp.Status != "needs_configuration" || !strings.Contains(semResp.Reason, "re-enter the API key") {
			t.Fatalf("unexpected semantic settings response on stranded key: %+v", semResp)
		}
	})

	// 4. Generic sensitive setting GET /settings/gemini_api_key reports reauthRequired=true and allows POST replacement
	t.Run("Generic sensitive setting reports reauthRequired and recovers on POST", func(t *testing.T) {
		apiRouter := apiHandler.Routes()
		req := httptest.NewRequest(http.MethodGet, "/settings/gemini_api_key", nil)
		rec := httptest.NewRecorder()
		apiRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /settings/gemini_api_key status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
		}
		var settingResp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &settingResp); err != nil {
			t.Fatal(err)
		}
		if settingResp["configured"] != false || settingResp["reauthRequired"] != true {
			t.Fatalf("unexpected setting response on stranded key: %+v", settingResp)
		}

		// Replace with a new key via POST /settings/gemini_api_key
		postReq := httptest.NewRequest(http.MethodPost, "/settings/gemini_api_key", strings.NewReader(`{"value":"new-gemini-secret"}`))
		postReq.Header.Set("Content-Type", "application/json")
		postRec := httptest.NewRecorder()
		apiRouter.ServeHTTP(postRec, postReq)
		if postRec.Code != http.StatusOK {
			t.Fatalf("POST /settings/gemini_api_key status = %d, want 200", postRec.Code)
		}

		// Verify DB now holds enc:v2: and decrypts cleanly
		decrypted, err := database.GetSetting("gemini_api_key")
		if err != nil || decrypted != "new-gemini-secret" {
			t.Fatalf("GetSetting(gemini_api_key) after recovery = %q, err=%v", decrypted, err)
		}
		var rawStored string
		rawDB, _ := sql.Open("viib_sqlite", dbPath)
		defer rawDB.Close()
		_ = rawDB.QueryRow(`SELECT value FROM settings WHERE key = 'gemini_api_key'`).Scan(&rawStored)
		if !crypto.IsCurrentEncrypted(rawStored) {
			t.Fatalf("expected enc:v2: prefix after recovery, got %q", rawStored)
		}
	})
}
