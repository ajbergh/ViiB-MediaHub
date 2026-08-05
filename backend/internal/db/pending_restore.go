package db

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ApplyPendingRestore applies a validated staged database while the main
// application is stopped. The current database is retained as a rollback copy.
func ApplyPendingRestore(dataDir string) (bool, string, error) {
	pendingDir := filepath.Join(dataDir, "restore-pending")
	pendingDB := filepath.Join(pendingDir, "library.db")
	marker := filepath.Join(pendingDir, "restore.json")
	if _, err := os.Stat(marker); os.IsNotExist(err) { return false, "", nil }
	if err := ValidateSQLiteCopy(pendingDB); err != nil { return false, "", fmt.Errorf("validate pending restore: %w", err) }
	if err := os.MkdirAll(dataDir, 0700); err != nil { return false, "", err }

	currentDB := filepath.Join(dataDir, "library.db")
	rollbackPath := ""
	if _, err := os.Stat(currentDB); err == nil {
		rollbackPath = filepath.Join(dataDir, "library-rollback-"+time.Now().Format("20060102-150405")+".db")
		if err := copyRestoreFile(currentDB, rollbackPath); err != nil { return false, "", fmt.Errorf("create rollback database: %w", err) }
	}

	temporary := currentDB + ".restore-new"
	if err := copyRestoreFile(pendingDB, temporary); err != nil { return false, rollbackPath, err }
	_ = os.Remove(currentDB + "-wal")
	_ = os.Remove(currentDB + "-shm")
	// Windows cannot rename over an existing destination. The rollback copy is
	// durable before this removal, so activation remains recoverable.
	if err := os.Remove(currentDB); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(temporary)
		return false, rollbackPath, fmt.Errorf("remove current database before restore: %w", err)
	}
	if err := os.Rename(temporary, currentDB); err != nil {
		if rollbackPath != "" { _ = copyRestoreFile(rollbackPath, currentDB) }
		_ = os.Remove(temporary)
		return false, rollbackPath, fmt.Errorf("activate restored database: %w", err)
	}
	if err := ValidateSQLiteCopy(currentDB); err != nil {
		if rollbackPath != "" { _ = copyRestoreFile(rollbackPath, currentDB) }
		return false, rollbackPath, fmt.Errorf("validate activated restore: %w", err)
	}
	if err := os.RemoveAll(pendingDir); err != nil { return true, rollbackPath, fmt.Errorf("restore applied but pending marker cleanup failed: %w", err) }
	return true, rollbackPath, nil
}

func copyRestoreFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil { return err }
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil { return err }
	if _, err := io.Copy(output, input); err != nil { output.Close(); return err }
	if err := output.Sync(); err != nil { output.Close(); return err }
	return output.Close()
}
