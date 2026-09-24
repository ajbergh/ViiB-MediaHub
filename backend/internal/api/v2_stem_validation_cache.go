package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/ajbergh/viib-mediahub/internal/stems"
)

var stemValidationCache = struct {
	sync.RWMutex
	entries map[string]stemValidationCacheEntry
}{entries: make(map[string]stemValidationCacheEntry)}

type stemValidationCacheEntry struct {
	signature  string
	validation stems.Validation
}

const maxStemValidationCacheEntries = 64

// validatedStemPackage fully validates and hashes a package when its current
// manifest/artifact signatures change. Frame requests after that only stat the
// files and verify their WAV geometry while reading the requested byte range.
func validatedStemPackage(packagePath string) (stems.Validation, error) {
	root, err := filepath.Abs(packagePath)
	if err != nil {
		return stems.Validation{}, err
	}
	root = filepath.Clean(root)
	before, manifest, err := stemPackageSignature(root)
	if err != nil {
		return stems.Validation{}, err
	}
	key := strings.ToLower(root)
	stemValidationCache.RLock()
	cached, ok := stemValidationCache.entries[key]
	stemValidationCache.RUnlock()
	if ok && cached.signature == before {
		return cached.validation, nil
	}
	validation, err := stems.ValidatePackage(root)
	if err != nil {
		return stems.Validation{}, err
	}
	if !reflect.DeepEqual(validation.Manifest, manifest) {
		return stems.Validation{}, fmt.Errorf("stem manifest changed during validation")
	}
	after, _, err := stemPackageSignature(root)
	if err != nil {
		return stems.Validation{}, err
	}
	if after != before {
		return stems.Validation{}, fmt.Errorf("stem package changed during validation")
	}
	stemValidationCache.Lock()
	if _, exists := stemValidationCache.entries[key]; !exists && len(stemValidationCache.entries) >= maxStemValidationCacheEntries {
		for oldKey := range stemValidationCache.entries {
			delete(stemValidationCache.entries, oldKey)
			break
		}
	}
	stemValidationCache.entries[key] = stemValidationCacheEntry{signature: after, validation: validation}
	stemValidationCache.Unlock()
	return validation, nil
}

func stemPackageSignature(root string) (string, stems.Manifest, error) {
	var zero stems.Manifest
	if err := rejectSymlinkPath(root); err != nil {
		return "", zero, err
	}
	manifestPath := filepath.Join(root, stems.ManifestFilename)
	info, err := os.Lstat(manifestPath)
	if err != nil {
		return "", zero, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", zero, fmt.Errorf("manifest is not a regular file")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", zero, err
	}
	if len(data) > stems.MaxManifestBytes {
		return "", zero, fmt.Errorf("manifest exceeds size limit")
	}
	manifest, err := stems.ParseManifest(bytes.NewReader(data))
	if err != nil {
		return "", zero, err
	}
	if err = stems.ValidateManifest(manifest); err != nil {
		return "", zero, err
	}
	h := sha256.New()
	_, _ = h.Write(data)
	names := make([]string, 0, len(manifest.Stems))
	for name := range manifest.Stems {
		names = append(names, string(name))
	}
	sort.Strings(names)
	for _, name := range names {
		artifact := manifest.Stems[stems.StemName(name)]
		if err = rejectSymlinkArtifact(root, artifact.Path); err != nil {
			return "", zero, err
		}
		path := filepath.Join(root, filepath.FromSlash(artifact.Path))
		stat, statErr := os.Stat(path)
		if statErr != nil {
			return "", zero, statErr
		}
		if !stat.Mode().IsRegular() {
			return "", zero, fmt.Errorf("stem artifact is not a regular file")
		}
		_, _ = fmt.Fprintf(h, "\x00%s\x00%d\x00%d\x00%s", name, stat.Size(), stat.ModTime().UnixNano(), strings.ToLower(artifact.SHA256))
	}
	return hex.EncodeToString(h.Sum(nil)), manifest, nil
}
