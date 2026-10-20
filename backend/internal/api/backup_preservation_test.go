package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestBackupNameConflictPreservesRecoveryPoint(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	a.dataDir = t.TempDir()
	if err := a.db.SetSetting("theme", "before"); err != nil {
		t.Fatal(err)
	}
	create := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		a.createBackupV2(response, httptest.NewRequest(http.MethodPost, "/backups", strings.NewReader(`{"name":"recovery"}`)))
		return response
	}
	first := create()
	if first.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", first.Code, first.Body.String())
	}
	archivePath := filepath.Join(a.backupsDir(), "recovery.zip")
	original, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.SetSetting("theme", "after"); err != nil {
		t.Fatal(err)
	}
	second := create()
	var envelope struct {
		Error V2Error `json:"error"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if second.Code != http.StatusConflict || envelope.Error.Code != "backup_exists" || envelope.Error.Retryable {
		t.Fatalf("collision: %d %s", second.Code, second.Body.String())
	}
	retained, err := os.ReadFile(archivePath)
	if err != nil || !bytes.Equal(retained, original) {
		t.Fatalf("old recovery point changed: %v", err)
	}
	preview := httptest.NewRecorder()
	a.previewRestoreV2(preview, httptest.NewRequest(http.MethodPost, "/restore/preview", strings.NewReader(`{"name":"recovery.zip"}`)))
	if preview.Code != http.StatusOK {
		t.Fatalf("original archive no longer valid: %s", preview.Body.String())
	}
	theme, err := a.db.GetSetting("theme")
	if err != nil || theme != "after" {
		t.Fatal("backup conflict mutated the live database")
	}
}

func TestWriteBackupArchiveConcurrentNoClobber(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source.db")
	payload := bytes.Repeat([]byte("synthetic snapshot"), 1024)
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "recovery.zip")
	const workers = 8
	start := make(chan struct{})
	results := make(chan error, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- writeBackupArchive(destination, source, backupManifest{})
		}()
	}
	close(start)
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !os.IsExist(err) {
			t.Fatalf("unexpected creation error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one writer, got %d", successes)
	}
	archive, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 2 {
		t.Fatalf("incomplete archive: %d members", len(archive.File))
	}
	for _, entry := range archive.File {
		if entry.Name != "library.db" {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil || !bytes.Equal(data, payload) {
			t.Fatalf("winning snapshot corrupted: %v", err)
		}
	}
}

func TestWriteBackupArchiveFailedWriteCleansOnlyNewArchive(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "recovery.zip")
	missingSource := filepath.Join(directory, "missing.db")
	if err := writeBackupArchive(path, missingSource, backupManifest{}); err == nil {
		t.Fatal("missing source accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("incomplete archive retained: %v", err)
	}
	original := []byte("previous recovery point")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeBackupArchive(path, missingSource, backupManifest{}); !os.IsExist(err) {
		t.Fatalf("existing archive not rejected: %v", err)
	}
	retained, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(retained, original) {
		t.Fatalf("existing archive damaged by failed write: %v", err)
	}
}

func TestCreateBackupGeneratedNamesAreDistinct(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	a.dataDir = t.TempDir()
	names := make(map[string]bool)
	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		a.createBackupV2(response, httptest.NewRequest(http.MethodPost, "/backups", nil))
		if response.Code != http.StatusCreated {
			t.Fatalf("create %d: %d %s", i, response.Code, response.Body.String())
		}
		var info backupInfo
		if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
			t.Fatal(err)
		}
		if names[info.Name] {
			t.Fatal("generated backup reused an earlier name")
		}
		names[info.Name] = true
	}
	entries, err := os.ReadDir(a.backupsDir())
	if err != nil || len(entries) != 3 {
		t.Fatalf("expected three retained archives: %v (%d)", err, len(entries))
	}
}
