package db

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastDirectoryCredentialsLifecycleAndEncryption(t *testing.T) {
	t.Setenv("VIIB_PODCAST_INDEX_API_KEY", "")
	t.Setenv("VIIB_PODCAST_INDEX_API_SECRET", "")
	t.Setenv("PODCAST_INDEX_API_KEY", "")
	t.Setenv("PODCAST_INDEX_API_SECRET", "")
	dbPath := filepath.Join(t.TempDir(), "directory.db")
	d, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	status, err := d.GetPodcastDirectoryStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Configured || status.Provider != "none" || status.KeyHint != "" {
		t.Fatalf("unexpected initial status: %+v", status)
	}
	if _, err := d.GetPodcastDirectoryCredentials(ctx); !errors.Is(err, podcast.ErrDirectoryNotConfigured) {
		t.Fatalf("expected ErrDirectoryNotConfigured, got %v", err)
	}

	if err := d.SetPodcastDirectoryCredentials(ctx, "   ", "secret"); err == nil {
		t.Fatal("accepted empty apiKey")
	}
	if err := d.SetPodcastDirectoryCredentials(ctx, "key", strings.Repeat("x", 257)); err == nil {
		t.Fatal("accepted oversized apiSecret")
	}

	const rawKey = "pi-key-abcdef1234"
	const rawSecret = "super-secret-value-xyz"
	if err := d.SetPodcastDirectoryCredentials(ctx, " "+rawKey+" ", " "+rawSecret+" "); err != nil {
		t.Fatal(err)
	}

	// Raw DB column must be encrypted at rest (enc:v2:) and never hold plaintext.
	var storedRaw string
	if err := d.conn.QueryRow("SELECT value FROM settings WHERE key = ?", podcastIndexCredentialsKey).Scan(&storedRaw); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(storedRaw, "enc:v2:") {
		t.Fatalf("expected enc:v2: encrypted value, got %q", storedRaw)
	}
	if strings.Contains(storedRaw, rawKey) || strings.Contains(storedRaw, rawSecret) {
		t.Fatal("plaintext credentials found in settings row")
	}

	status, err = d.GetPodcastDirectoryStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.Provider != "podcast_index" || status.KeyHint != "••••1234" {
		t.Fatalf("unexpected configured status: %+v", status)
	}
	if strings.Contains(status.KeyHint, rawSecret) || strings.Contains(status.KeyHint, rawKey) {
		t.Fatalf("status leaked secret or full key: %+v", status)
	}

	creds, err := d.GetPodcastDirectoryCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if creds.APIKey != rawKey || creds.APISecret != rawSecret {
		t.Fatalf("decrypted credentials mismatch: %+v", creds)
	}

	// Short keys (<= 4 chars) must be completely masked so full key is never exposed.
	if err := d.SetPodcastDirectoryCredentials(ctx, "abcd", rawSecret); err != nil {
		t.Fatal(err)
	}
	shortStatus, err := d.GetPodcastDirectoryStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if shortStatus.KeyHint != "••••" {
		t.Fatalf("expected full masking for <=4 char key, got %q", shortStatus.KeyHint)
	}

	// Restore longer key, reopen DB, and verify persistence.
	if err := d.SetPodcastDirectoryCredentials(ctx, rawKey, rawSecret); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	reloadedStatus, err := reopened.GetPodcastDirectoryStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reloadedStatus.Configured || reloadedStatus.Provider != "podcast_index" || reloadedStatus.KeyHint != "••••1234" {
		t.Fatalf("reopened status mismatch: %+v", reloadedStatus)
	}
	reloadedCreds, err := reopened.GetPodcastDirectoryCredentials(ctx)
	if err != nil || reloadedCreds.APIKey != rawKey || reloadedCreds.APISecret != rawSecret {
		t.Fatalf("reopened credentials mismatch: %+v %v", reloadedCreds, err)
	}

	if err := reopened.DeletePodcastDirectoryCredentials(ctx); err != nil {
		t.Fatal(err)
	}
	afterDelete, err := reopened.GetPodcastDirectoryStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterDelete.Configured || afterDelete.Provider != "none" || afterDelete.KeyHint != "" {
		t.Fatalf("status after delete: %+v", afterDelete)
	}
	if _, err := reopened.GetPodcastDirectoryCredentials(ctx); !errors.Is(err, podcast.ErrDirectoryNotConfigured) {
		t.Fatalf("expected ErrDirectoryNotConfigured after delete, got %v", err)
	}
}

func TestPodcastDirectoryEmbeddedFallbackAndUserOverride(t *testing.T) {
	t.Setenv("VIIB_PODCAST_INDEX_API_KEY", "built-in-key-9999")
	t.Setenv("VIIB_PODCAST_INDEX_API_SECRET", "built-in-secret")
	t.Setenv("PODCAST_INDEX_API_KEY", "")
	t.Setenv("PODCAST_INDEX_API_SECRET", "")

	d, err := New(filepath.Join(t.TempDir(), "embedded.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()

	status, err := d.GetPodcastDirectoryStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.Source != "embedded" || status.KeyHint != "Built-in" {
		t.Fatalf("expected embedded status, got %+v", status)
	}
	creds, err := d.GetPodcastDirectoryCredentials(ctx)
	if err != nil || creds.APIKey != "built-in-key-9999" || creds.APISecret != "built-in-secret" {
		t.Fatalf("expected embedded creds, got %+v %v", creds, err)
	}

	// Explicit user override takes precedence and reverts cleanly on delete.
	if err := d.SetPodcastDirectoryCredentials(ctx, "user-key-1234", "user-secret"); err != nil {
		t.Fatal(err)
	}
	userStatus, err := d.GetPodcastDirectoryStatus(ctx)
	if err != nil || !userStatus.Configured || userStatus.Source != "user" || userStatus.KeyHint != "••••1234" {
		t.Fatalf("expected user override status, got %+v %v", userStatus, err)
	}
	userCreds, err := d.GetPodcastDirectoryCredentials(ctx)
	if err != nil || userCreds.APIKey != "user-key-1234" {
		t.Fatalf("expected user creds override, got %+v %v", userCreds, err)
	}

	if err := d.DeletePodcastDirectoryCredentials(ctx); err != nil {
		t.Fatal(err)
	}
	reverted, err := d.GetPodcastDirectoryStatus(ctx)
	if err != nil || !reverted.Configured || reverted.Source != "embedded" || reverted.KeyHint != "Built-in" {
		t.Fatalf("expected revert to embedded status, got %+v %v", reverted, err)
	}
}
