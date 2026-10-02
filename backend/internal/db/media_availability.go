package db

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PathWithinRoot compares components rather than string prefixes.
func PathWithinRoot(root, path string) bool {
	root, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
		path = strings.ToLower(path)
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ConfirmMissingLocalMedia fails closed when ownership or filesystem access is
// uncertain. Requiring readable roots and parents also protects unmounted shares.
func (d *DB) ConfirmMissingLocalMedia(path string) bool {
	if !filepath.IsAbs(path) || strings.Contains(path, "://") {
		return false
	}
	folders, err := d.GetScanFolders()
	if err != nil {
		return false
	}
	return confirmMissingLocalMedia(path, folders, os.Stat, os.ReadDir)
}

// Filesystem operations are parameters so permission and transient I/O failures
// can be tested consistently on every supported OS.
func confirmMissingLocalMedia(path string, folders []ScanFolder, stat func(string) (os.FileInfo, error), readDir func(string) ([]os.DirEntry, error)) bool {
	var owner string
	for _, folder := range folders {
		if PathWithinRoot(folder.Path, path) && len(folder.Path) > len(owner) {
			owner = folder.Path
		}
	}
	if owner == "" {
		return false
	}
	if _, err := readDir(owner); err != nil {
		return false
	}
	if _, err := readDir(filepath.Dir(path)); err != nil {
		return false
	}
	_, err := stat(path)
	return os.IsNotExist(err)
}
