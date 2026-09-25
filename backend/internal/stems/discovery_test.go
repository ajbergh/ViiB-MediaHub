package stems

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolvePackagePrefersAdjacentOverLibrary(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "track.wav")
	sourceBytes := []byte("canonical source audio")
	if err := os.WriteFile(source, sourceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	sourceHash := sha256Hex(sourceBytes)
	adjacent := filepath.Join(root, "track.viibstems")
	library := filepath.Join(root, "library")
	if err := os.Mkdir(library, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDiscoveryFixture(t, adjacent, sourceHash)
	writeDiscoveryFixture(t, filepath.Join(library, "z-package.viibstems"), sourceHash)
	writeDiscoveryFixture(t, filepath.Join(library, "a-package.viibstems"), sourceHash)

	candidate, err := ResolvePackage(source, []string{library}, NewSourceHashCache())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(candidate.Path) != filepath.Clean(adjacent) || candidate.Source != CandidateAdjacent {
		t.Fatalf("expected adjacent package to win, got %+v", candidate)
	}
}

func TestResolvePackageUsesDeterministicLibraryOrderAndIgnoresWrongSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "track.wav")
	sourceBytes := []byte("source")
	if err := os.WriteFile(source, sourceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(root, "library")
	if err := os.Mkdir(library, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDiscoveryFixture(t, filepath.Join(library, "z-package.viibstems"), sha256Hex(sourceBytes))
	writeDiscoveryFixture(t, filepath.Join(library, "b-package.viibstems"), sha256Hex([]byte("different source")))
	writeDiscoveryFixture(t, filepath.Join(library, "a-package.viibstems"), sha256Hex(sourceBytes))

	candidate, err := ResolvePackage(source, []string{library}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(library, "a-package.viibstems")
	if filepath.Clean(candidate.Path) != filepath.Clean(want) || candidate.Source != CandidateLibrary {
		t.Fatalf("expected lexically first matching library package %q, got %+v", want, candidate)
	}
}

func TestDiscoverPackagesRetainsInvalidPackageDiagnostics(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "track.wav")
	if err := os.WriteFile(source, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	badAdjacent := filepath.Join(root, "track.viibstems")
	if err := os.Mkdir(badAdjacent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badAdjacent, ManifestFilename), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := DiscoverPackages(source, nil)
	if len(result.Candidates) != 0 || len(result.Rejected) != 1 {
		t.Fatalf("expected invalid package diagnostic, got %+v", result)
	}
}

func TestDiscoverLibraryPackagesFindsNestedPackagesAndTreatsPackagesAsLeaves(t *testing.T) {
	library := t.TempDir()
	nested := filepath.Join(library, "Artist", "Album", "album-stems.viibstems")
	writeDiscoveryFixture(t, nested, sha256Hex([]byte("source")))
	// A nested folder inside a package is package data, never another package root.
	writeDiscoveryFixture(t, filepath.Join(nested, "nested.viibstems"), sha256Hex([]byte("nested")))

	result := DiscoverLibraryPackages([]string{library})
	if len(result.Candidates) != 1 {
		t.Fatalf("expected one validated leaf package, got %d candidates and %d rejected: %+v", len(result.Candidates), len(result.Rejected), result)
	}
	if filepath.Clean(result.Candidates[0].Path) != filepath.Clean(nested) {
		t.Fatalf("candidate path = %q, want %q", result.Candidates[0].Path, nested)
	}
}

func TestSourceHashCacheUsesPathSizeAndMtimeSignature(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.wav")
	first := []byte("first")
	if err := os.WriteFile(path, first, 0o600); err != nil {
		t.Fatal(err)
	}
	cache := NewSourceHashCache()
	hash1, err := cache.SHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	hash1Again, err := cache.SHA256(path)
	if err != nil || hash1Again != hash1 {
		t.Fatalf("expected cached digest %q, got %q, err %v", hash1, hash1Again, err)
	}
	second := []byte("other") // Same size; changed mtime invalidates the cache.
	if err := os.WriteFile(path, second, 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	hash2, err := cache.SHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if hash2 != sha256Hex(second) || hash2 == hash1 {
		t.Fatalf("expected changed content digest %q, got %q", sha256Hex(second), hash2)
	}
}

func writeDiscoveryFixture(t *testing.T, dir, sourceHash string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fixtureManifest(t, dir, LayoutFour)
	manifest.Source.SHA256 = sourceHash
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
