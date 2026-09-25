package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/stems"
)

func normalizedLocalRoot(path string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = filepath.Clean(resolved)
	}
	return abs, nil
}

func rootPathContains(parent, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	if os.PathSeparator == '\\' {
		rel = strings.ToLower(rel)
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func localRootsOverlap(first, second string) bool {
	a, err := normalizedLocalRoot(first)
	if err != nil {
		return false
	}
	b, err := normalizedLocalRoot(second)
	if err != nil {
		return false
	}
	if os.PathSeparator == '\\' && strings.EqualFold(a, b) {
		return true
	}
	return rootPathContains(a, b) || rootPathContains(b, a)
}

func validateStemLibraryRootsAgainstMusic(musicFolders []db.ScanFolder, stemRoots []string) error {
	for _, stemRoot := range stemRoots {
		if err := stems.RejectSymlinkPath(stemRoot); err != nil {
			return fmt.Errorf("Stem Library root must not contain symlinks: %s", stemRoot)
		}
		info, err := os.Stat(stemRoot)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("Stem Library root must be an existing directory: %s", stemRoot)
		}
		for _, music := range musicFolders {
			if localRootsOverlap(music.Path, stemRoot) {
				return fmt.Errorf("Stem Library root %q overlaps Music Folder %q; choose separate folders", stemRoot, music.Path)
			}
		}
	}
	return nil
}

func (a *API) validateMusicFolderAgainstStemRoots(musicRoot string) error {
	locations, err := a.db.ListStemLocations()
	if err != nil {
		return err
	}
	for _, location := range locations {
		if location.Enabled && localRootsOverlap(musicRoot, location.Path) {
			return fmt.Errorf("Music Folder %q overlaps Stem Library root %q; choose separate folders", musicRoot, location.Path)
		}
	}
	return nil
}

func (a *API) validateStemLibraryScanRoots(requested []string) ([]string, error) {
	locations, err := a.db.ListStemLocations()
	if err != nil {
		return nil, fmt.Errorf("unable to load configured Stem Library locations")
	}
	enabled := make([]db.StemLocation, 0, len(locations))
	for _, location := range locations {
		if location.Enabled {
			enabled = append(enabled, location)
		}
	}
	musicFolders, err := a.db.GetScanFolders()
	if err != nil {
		return nil, fmt.Errorf("unable to load Music Folders")
	}
	configuredPaths := make([]string, 0, len(enabled))
	for _, location := range enabled {
		configuredPaths = append(configuredPaths, location.Path)
	}
	if err = validateStemLibraryRootsAgainstMusic(musicFolders, configuredPaths); err != nil {
		return nil, err
	}
	if len(requested) == 0 || len(requested) != len(enabled) {
		return nil, fmt.Errorf("Stem Library scan roots must exactly match the currently enabled Stem Library locations")
	}
	requestedKeys := make(map[string]bool, len(requested))
	for _, root := range requested {
		abs, normalizeErr := normalizedLocalRoot(root)
		if normalizeErr != nil {
			return nil, fmt.Errorf("invalid Stem Library scan root")
		}
		if requestedKeys[pathKeyForOS(abs)] {
			return nil, fmt.Errorf("duplicate Stem Library scan root")
		}
		requestedKeys[pathKeyForOS(abs)] = true
	}
	paths := make([]string, 0, len(enabled))
	for _, location := range enabled {
		if err = stems.RejectSymlinkPath(location.Path); err != nil {
			return nil, fmt.Errorf("configured Stem Library root is unsafe")
		}
		abs, normalizeErr := normalizedLocalRoot(location.Path)
		if normalizeErr != nil || !requestedKeys[pathKeyForOS(abs)] {
			return nil, fmt.Errorf("Stem Library scan roots no longer match the currently enabled Stem Library locations")
		}
		info, statErr := os.Stat(abs)
		if statErr != nil || !info.IsDir() {
			return nil, fmt.Errorf("configured Stem Library root is unavailable")
		}
		paths = append(paths, abs)
	}
	return paths, nil
}

func pathKeyForOS(path string) string {
	if os.PathSeparator == '\\' {
		return strings.ToLower(filepath.Clean(path))
	}
	return filepath.Clean(path)
}
