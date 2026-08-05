// v2_library_operations.go exposes local database maintenance and recovery routes.
package api

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/go-chi/chi/v5"
)

type backupManifest struct {
	Format       string `json:"format"`
	Version      int    `json:"version"`
	CreatedAt    int64  `json:"createdAt"`
	DatabaseFile string `json:"databaseFile"`
	DatabaseSHA  string `json:"databaseSha256"`
	AppVersion   string `json:"appVersion,omitempty"`
}

type backupInfo struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"createdAt"`
}

type watcherRequest struct { IntervalMS int64 `json:"intervalMs"` }
type repairRequest struct { RemoveMissing bool `json:"removeMissing"` }
type backupRequest struct { Name string `json:"name,omitempty"` }
type restoreRequest struct { Name string `json:"name"` }
type metadataRequest struct {
	db.SongMetadataPatch
	WriteBack bool `json:"writeBack"`
}

// V2LibraryOperationRoutes returns diagnostics, repair, metadata, backup,
// restore-staging, and continuous-monitoring routes. Restore activation is
// intentionally handled offline by viib-restore.
func (a *API) V2LibraryOperationRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/diagnostics", a.libraryDiagnosticsV2)
	r.Post("/repair", a.repairLibraryV2)
	r.Patch("/songs/{id}/metadata", a.updateSongMetadataV2)
	r.Get("/backups", a.listBackupsV2)
	r.Post("/backups", a.createBackupV2)
	r.Post("/restore/preview", a.previewRestoreV2)
	r.Post("/restore/stage", a.stageRestoreV2)
	r.Get("/watcher", a.watcherStatusV2)
	r.Post("/watcher/start", a.startWatcherV2)
	r.Post("/watcher/stop", a.stopWatcherV2)
	return r
}

func (a *API) libraryDiagnosticsV2(w http.ResponseWriter, r *http.Request) {
	diagnostics, err := a.db.RunLibraryDiagnostics()
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "diagnostics_failed", "Unable to complete library diagnostics", true, map[string]any{"reason": err.Error()})
		return
	}
	respondV2JSON(w, http.StatusOK, diagnostics)
}

func (a *API) repairLibraryV2(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var request repairRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_request", "Repair options are not valid JSON", false, nil)
		return
	}
	result, err := a.db.RepairLibraryIndexes(request.RemoveMissing)
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "repair_failed", "Library repair failed", true, map[string]any{"reason": err.Error()})
		return
	}
	a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Library repair completed"})
	respondV2JSON(w, http.StatusOK, result)
}

func (a *API) updateSongMetadataV2(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var request metadataRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_request", "Metadata patch is not valid JSON", false, nil)
		return
	}
	if request.WriteBack {
		respondV2Error(w, r, http.StatusNotImplemented, "tag_writeback_unavailable", "Source-file tag write-back is not enabled in this build; database metadata was not changed", false, nil)
		return
	}
	song, err := a.db.UpdateSongMetadata(chi.URLParam(r, "id"), request.SongMetadataPatch)
	if err != nil {
		status := http.StatusBadRequest
		code := "metadata_update_failed"
		if strings.Contains(err.Error(), "no rows") { status = http.StatusNotFound; code = "song_not_found" }
		respondV2Error(w, r, status, code, err.Error(), false, nil)
		return
	}
	transformSongForAPI(&song)
	a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Song metadata updated", UpdatedSongs: 1})
	respondV2JSON(w, http.StatusOK, song)
}

func (a *API) backupsDir() string { return filepath.Join(a.dataDir, "backups") }

func safeBackupName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" { return "viib-backup-" + time.Now().Format("20060102-150405") + ".zip" }
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' { return r }
		return '-'
	}, name)
	if !strings.HasSuffix(strings.ToLower(name), ".zip") { name += ".zip" }
	return name
}

func (a *API) listBackupsV2(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(a.backupsDir())
	if os.IsNotExist(err) { respondV2JSON(w, http.StatusOK, map[string]any{"backups": []backupInfo{}}); return }
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "backup_list_failed", "Unable to list backups", true, nil)
		return
	}
	backups := make([]backupInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".zip") { continue }
		info, err := entry.Info(); if err != nil { continue }
		backups = append(backups, backupInfo{Name: entry.Name(), Size: info.Size(), CreatedAt: info.ModTime().UnixMilli()})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt > backups[j].CreatedAt })
	respondV2JSON(w, http.StatusOK, map[string]any{"backups": backups})
}

func (a *API) createBackupV2(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var request backupRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			respondV2Error(w, r, http.StatusBadRequest, "invalid_request", "Backup request is not valid JSON", false, nil); return
		}
	}
	if err := os.MkdirAll(a.backupsDir(), 0700); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "backup_create_failed", "Unable to create backup directory", true, nil); return
	}
	workDir, err := os.MkdirTemp(a.backupsDir(), ".backup-work-")
	if err != nil { respondV2Error(w, r, http.StatusInternalServerError, "backup_create_failed", err.Error(), true, nil); return }
	defer os.RemoveAll(workDir)
	databaseCopy := filepath.Join(workDir, "library.db")
	if err := a.db.CreateConsistentCopy(databaseCopy); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "backup_database_failed", "Unable to create a consistent database snapshot", true, map[string]any{"reason": err.Error()}); return
	}
	if err := db.ValidateSQLiteCopy(databaseCopy); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "backup_validation_failed", err.Error(), false, nil); return
	}
	digest, err := fileSHA256(databaseCopy)
	if err != nil { respondV2Error(w, r, http.StatusInternalServerError, "backup_hash_failed", err.Error(), true, nil); return }
	manifest := backupManifest{Format: "viib-mediahub-backup", Version: 1, CreatedAt: time.Now().UnixMilli(), DatabaseFile: "library.db", DatabaseSHA: digest}
	name := safeBackupName(request.Name)
	archivePath := filepath.Join(a.backupsDir(), name)
	if err := writeBackupArchive(archivePath, databaseCopy, manifest); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "backup_archive_failed", err.Error(), true, nil); return
	}
	info, _ := os.Stat(archivePath)
	respondV2JSON(w, http.StatusCreated, backupInfo{Name: name, Size: info.Size(), CreatedAt: info.ModTime().UnixMilli()})
}

func (a *API) previewRestoreV2(w http.ResponseWriter, r *http.Request) {
	manifest, _, cleanup, err := a.openBackupForRestore(w, r)
	if cleanup != nil { defer cleanup() }
	if err != nil { return }
	respondV2JSON(w, http.StatusOK, map[string]any{"valid": true, "manifest": manifest, "restartRequired": true})
}

func (a *API) stageRestoreV2(w http.ResponseWriter, r *http.Request) {
	manifest, databasePath, cleanup, err := a.openBackupForRestore(w, r)
	if cleanup != nil { defer cleanup() }
	if err != nil { return }
	pendingDir := filepath.Join(a.dataDir, "restore-pending")
	if err := os.MkdirAll(pendingDir, 0700); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "restore_stage_failed", err.Error(), true, nil); return
	}
	if err := copyFile(databasePath, filepath.Join(pendingDir, "library.db")); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "restore_stage_failed", err.Error(), true, nil); return
	}
	marker, _ := json.MarshalIndent(map[string]any{"manifest": manifest, "stagedAt": time.Now().UnixMilli()}, "", "  ")
	if err := os.WriteFile(filepath.Join(pendingDir, "restore.json"), marker, 0600); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "restore_stage_failed", err.Error(), true, nil); return
	}
	respondV2JSON(w, http.StatusAccepted, map[string]any{"staged": true, "restartRequired": true, "path": pendingDir})
}

func (a *API) openBackupForRestore(w http.ResponseWriter, r *http.Request) (backupManifest, string, func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var request restoreRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil { respondV2Error(w, r, http.StatusBadRequest, "invalid_request", "Restore request is not valid JSON", false, nil); return backupManifest{}, "", nil, err }
	name := filepath.Base(request.Name)
	if name != request.Name || !strings.HasSuffix(strings.ToLower(name), ".zip") { err := fmt.Errorf("invalid backup name"); respondV2Error(w, r, http.StatusBadRequest, "invalid_backup_name", err.Error(), false, nil); return backupManifest{}, "", nil, err }
	archivePath := filepath.Join(a.backupsDir(), name)
	workDir, err := os.MkdirTemp(a.backupsDir(), ".restore-preview-")
	if err != nil { respondV2Error(w, r, http.StatusInternalServerError, "restore_preview_failed", err.Error(), true, nil); return backupManifest{}, "", nil, err }
	cleanup := func() { os.RemoveAll(workDir) }
	manifest, databasePath, err := extractAndValidateBackup(archivePath, workDir)
	if err != nil { respondV2Error(w, r, http.StatusBadRequest, "invalid_backup", err.Error(), false, nil); return backupManifest{}, "", cleanup, err }
	return manifest, databasePath, cleanup, nil
}

func (a *API) watcherStatusV2(w http.ResponseWriter, r *http.Request) { respondV2JSON(w, http.StatusOK, a.scanner.ContinuousWatcherStatus()) }
func (a *API) startWatcherV2(w http.ResponseWriter, r *http.Request) {
	var request watcherRequest
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil { respondV2Error(w, r, http.StatusBadRequest, "invalid_request", "Watcher request is not valid JSON", false, nil); return }
	interval, err := scanner.ParseWatchInterval(request.IntervalMS)
	if err != nil { respondV2Error(w, r, http.StatusBadRequest, "invalid_interval", err.Error(), false, nil); return }
	status, err := a.scanner.StartContinuousWatcher(interval)
	if err != nil { respondV2Error(w, r, http.StatusInternalServerError, "watcher_start_failed", err.Error(), true, nil); return }
	respondV2JSON(w, http.StatusOK, status)
}
func (a *API) stopWatcherV2(w http.ResponseWriter, r *http.Request) { respondV2JSON(w, http.StatusOK, a.scanner.StopContinuousWatcher()) }

func fileSHA256(path string) (string, error) { file, err := os.Open(path); if err != nil { return "", err }; defer file.Close(); hash := sha256.New(); if _, err := io.Copy(hash, file); err != nil { return "", err }; return hex.EncodeToString(hash.Sum(nil)), nil }
func copyFile(source, destination string) error { input, err := os.Open(source); if err != nil { return err }; defer input.Close(); output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600); if err != nil { return err }; if _, err := io.Copy(output, input); err != nil { output.Close(); return err }; return output.Close() }
func writeBackupArchive(path, databasePath string, manifest backupManifest) error { file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600); if err != nil { return err }; writer := zip.NewWriter(file); dbEntry, err := writer.Create("library.db"); if err != nil { writer.Close(); file.Close(); return err }; dbFile, err := os.Open(databasePath); if err != nil { writer.Close(); file.Close(); return err }; _, err = io.Copy(dbEntry, dbFile); dbFile.Close(); if err != nil { writer.Close(); file.Close(); return err }; manifestEntry, err := writer.Create("manifest.json"); if err != nil { writer.Close(); file.Close(); return err }; payload, _ := json.MarshalIndent(manifest, "", "  "); if _, err := manifestEntry.Write(payload); err != nil { writer.Close(); file.Close(); return err }; if err := writer.Close(); err != nil { file.Close(); return err }; return file.Close() }
func extractAndValidateBackup(archivePath, destination string) (backupManifest, string, error) { reader, err := zip.OpenReader(archivePath); if err != nil { return backupManifest{}, "", err }; defer reader.Close(); var manifest backupManifest; var databasePath string; for _, entry := range reader.File { if entry.Name != "library.db" && entry.Name != "manifest.json" { continue }; input, err := entry.Open(); if err != nil { return backupManifest{}, "", err }; if entry.Name == "manifest.json" { err = json.NewDecoder(io.LimitReader(input, 1024*1024)).Decode(&manifest); input.Close(); if err != nil { return backupManifest{}, "", err }; continue }; databasePath = filepath.Join(destination, "library.db"); output, err := os.OpenFile(databasePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600); if err != nil { input.Close(); return backupManifest{}, "", err }; _, err = io.CopyN(output, input, 20*1024*1024*1024); if err != nil && err != io.EOF { output.Close(); input.Close(); return backupManifest{}, "", err }; output.Close(); input.Close() }; if manifest.Format != "viib-mediahub-backup" || manifest.Version != 1 || databasePath == "" { return backupManifest{}, "", fmt.Errorf("backup manifest or database is missing") }; digest, err := fileSHA256(databasePath); if err != nil { return backupManifest{}, "", err }; if digest != manifest.DatabaseSHA { return backupManifest{}, "", fmt.Errorf("backup database checksum does not match manifest") }; if err := db.ValidateSQLiteCopy(databasePath); err != nil { return backupManifest{}, "", err }; return manifest, databasePath, nil }
