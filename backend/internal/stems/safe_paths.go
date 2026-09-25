package stems

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RejectSymlinkPath rejects a symlink in any component of an absolute path.
// This is the package-root policy used by playback.
func RejectSymlinkPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(abs)
	rest := strings.TrimPrefix(abs, volume)
	current := volume + string(os.PathSeparator)
	for _, part := range strings.FieldsFunc(rest, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path component %q", current)
		}
	}
	return nil
}

// RejectSymlinkArtifact rejects a symlink in a package-relative artifact path.
func RejectSymlinkArtifact(root, relative string) error {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, `\`) || strings.Contains(relative, ":") {
		return errors.New("unsafe artifact path")
	}
	current := root
	for _, part := range strings.Split(relative, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("unsafe artifact path")
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("artifact symlink is not allowed")
		}
	}
	return nil
}
