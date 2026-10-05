// Tests and fixtures for media availability behavior.

package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingMediaAccessFailuresAreNeverDeletionEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "song.mp3")
	folders := []ScanFolder{{Path: root}}
	for _, problem := range []error{os.ErrPermission, errors.New("transient network I/O")} {
		if confirmMissingLocalMedia(path, folders, func(string) (os.FileInfo, error) { return nil, problem }, func(string) ([]os.DirEntry, error) { return nil, nil }) {
			t.Fatalf("stat failure allowed delete: %v", problem)
		}
		if confirmMissingLocalMedia(path, folders, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }, func(string) ([]os.DirEntry, error) { return nil, problem }) {
			t.Fatalf("unreadable root allowed delete: %v", problem)
		}
	}
	if confirmMissingLocalMedia(path, folders, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }, func(string) ([]os.DirEntry, error) { return nil, os.ErrNotExist }) {
		t.Fatal("offline root allowed delete")
	}
	if !confirmMissingLocalMedia(path, folders, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }, func(string) ([]os.DirEntry, error) { return nil, nil }) {
		t.Fatal("genuinely absent file on readable root rejected")
	}
}
