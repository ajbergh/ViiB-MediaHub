package stems

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const packageDirectorySuffix = ".viibstems"

const (
	maxLibraryWalkDepth   = 16
	maxLibraryWalkEntries = 100_000
)

// CandidateSource identifies how a stem package was found. Adjacent packages
// always precede packages found in configured library directories.
type CandidateSource string

const (
	CandidateAdjacent CandidateSource = "adjacent"
	CandidateLibrary  CandidateSource = "library"
)

// PackageCandidate contains a fully validated package and its discovery path.
// Validation is retained so callers can use the already checked file paths.
type PackageCandidate struct {
	Path                  string
	Source                CandidateSource
	Validation            Validation
	SourceIdentityChecked bool
	SourceIdentityMatches bool
}

// DiscoveryResult keeps valid candidates and rejected package diagnostics.
// Candidate order is deterministic: adjacent first, then library paths in
// lexical order.
type DiscoveryResult struct {
	Candidates []PackageCandidate
	Rejected   []RejectedPackage
}

type RejectedPackage struct {
	Path   string
	Source CandidateSource
	Err    error
}

// DiscoverPackages inspects the source-adjacent package and every bounded nested
// .viibstems package under each configured library directory. An adjacent package
// is named after the source without its media extension, for example
// song.flac -> song.viibstems. Package validity is always checked by
// ValidatePackage; discovery alone does not establish source identity.
func DiscoverPackages(sourcePath string, libraryDirs []string) DiscoveryResult {
	result, _ := DiscoverPackagesContext(context.Background(), sourcePath, libraryDirs)
	return result
}

// DiscoverPackagesContext is the cancellable variant of DiscoverPackages.
func DiscoverPackagesContext(ctx context.Context, sourcePath string, libraryDirs []string) (DiscoveryResult, error) {
	result := DiscoveryResult{}
	adjacent := adjacentPackagePath(sourcePath)
	seen := make(map[string]bool)
	appendCandidate := func(path string, source CandidateSource) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			result.Rejected = append(result.Rejected, RejectedPackage{Path: path, Source: source, Err: err})
			return nil
		}
		abs = filepath.Clean(abs)
		key := pathKey(abs)
		if seen[key] {
			return nil
		}
		seen[key] = true
		info, err := os.Stat(abs)
		if err != nil {
			result.Rejected = append(result.Rejected, RejectedPackage{Path: abs, Source: source, Err: err})
			return nil
		}
		if !info.IsDir() {
			result.Rejected = append(result.Rejected, RejectedPackage{Path: abs, Source: source, Err: errors.New("package candidate is not a directory")})
			return nil
		}
		validation, err := ValidatePlayablePackageContext(ctx, abs)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			result.Rejected = append(result.Rejected, RejectedPackage{Path: abs, Source: source, Err: err})
			return nil
		}
		result.Candidates = append(result.Candidates, PackageCandidate{Path: abs, Source: source, Validation: validation})
		return nil
	}

	if adjacent != "" {
		if err := appendCandidate(adjacent, CandidateAdjacent); err != nil {
			return result, err
		}
	}

	libraryDiscovery, err := DiscoverLibraryPackagesContext(ctx, libraryDirs)
	if err != nil {
		return result, err
	}
	result.Rejected = append(result.Rejected, libraryDiscovery.Rejected...)
	for _, candidate := range libraryDiscovery.Candidates {
		key := pathKey(filepath.Clean(candidate.Path))
		if !seen[key] {
			seen[key] = true
			candidate.Source = CandidateLibrary
			result.Candidates = append(result.Candidates, candidate)
		}
	}
	return result, nil
}

// DiscoverLibraryPackages walks each configured root once, without following
// symlinked directories. A `.viibstems` directory is a leaf package, and its
// contents are left to ValidatePackage. Depth and entry limits bound accidental
// scans of very large roots. Results are validated once and sorted
// deterministically.
func DiscoverLibraryPackages(libraryDirs []string) DiscoveryResult {
	result, _ := DiscoverLibraryPackagesContext(context.Background(), libraryDirs)
	return result
}

// DiscoverLibraryPackagesContext walks and validates packages while observing
// cancellation between directory entries and artifact reads.
func DiscoverLibraryPackagesContext(ctx context.Context, libraryDirs []string) (DiscoveryResult, error) {
	result := DiscoveryResult{}
	seen := make(map[string]bool)
	appendCandidate := func(path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			result.Rejected = append(result.Rejected, RejectedPackage{Path: path, Source: CandidateLibrary, Err: err})
			return nil
		}
		abs = filepath.Clean(abs)
		key := pathKey(abs)
		if seen[key] {
			return nil
		}
		seen[key] = true
		validation, err := ValidatePlayablePackageContext(ctx, abs)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			result.Rejected = append(result.Rejected, RejectedPackage{Path: abs, Source: CandidateLibrary, Err: err})
			return nil
		}
		result.Candidates = append(result.Candidates, PackageCandidate{Path: abs, Source: CandidateLibrary, Validation: validation})
		return nil
	}

	for _, library := range libraryDirs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		root, err := filepath.Abs(filepath.Clean(library))
		if err != nil {
			result.Rejected = append(result.Rejected, RejectedPackage{Path: library, Source: CandidateLibrary, Err: err})
			continue
		}
		rootInfo, err := os.Lstat(root)
		if err != nil {
			result.Rejected = append(result.Rejected, RejectedPackage{Path: root, Source: CandidateLibrary, Err: fmt.Errorf("read stem library: %w", err)})
			continue
		}
		if !rootInfo.IsDir() || rootInfo.Mode()&fs.ModeSymlink != 0 {
			result.Rejected = append(result.Rejected, RejectedPackage{Path: root, Source: CandidateLibrary, Err: errors.New("stem library root must be a real directory")})
			continue
		}
		entries := 0
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries++
			if walkErr != nil {
				return walkErr
			}
			if entries > maxLibraryWalkEntries {
				return fmt.Errorf("stem library entry limit (%d) exceeded", maxLibraryWalkEntries)
			}
			if path == root || !entry.IsDir() {
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if strings.EqualFold(filepath.Ext(entry.Name()), packageDirectorySuffix) {
				if err := appendCandidate(path); err != nil {
					return err
				}
				return filepath.SkipDir
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			depth := strings.Count(rel, string(filepath.Separator)) + 1
			if depth >= maxLibraryWalkDepth {
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			result.Rejected = append(result.Rejected, RejectedPackage{Path: root, Source: CandidateLibrary, Err: fmt.Errorf("walk stem library: %w", err)})
		}
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		a, b := filepath.Clean(result.Candidates[i].Path), filepath.Clean(result.Candidates[j].Path)
		if strings.EqualFold(a, b) {
			return a < b
		}
		return strings.ToLower(a) < strings.ToLower(b)
	})
	return result, nil
}

// ResolvePackage discovers and validates packages, then returns the first
// candidate whose manifest source SHA-256 matches the complete source file.
// Adjacent packages have priority over configured Stem Libraries. Invalid
// packages and packages for other source files are skipped.
func ResolvePackage(sourcePath string, libraryDirs []string, hashes *SourceHashCache) (PackageCandidate, error) {
	if hashes == nil {
		hashes = NewSourceHashCache()
	}
	discovery := DiscoverPackages(sourcePath, libraryDirs)
	if len(discovery.Candidates) == 0 {
		if len(discovery.Rejected) > 0 {
			return PackageCandidate{}, fmt.Errorf("no valid stem package found (%d rejected candidate(s))", len(discovery.Rejected))
		}
		return PackageCandidate{}, errors.New("no stem package found")
	}
	sourceHash, err := hashes.SHA256(sourcePath)
	if err != nil {
		return PackageCandidate{}, fmt.Errorf("hash source audio: %w", err)
	}
	for _, candidate := range discovery.Candidates {
		if strings.EqualFold(candidate.Validation.Manifest.Source.SHA256, sourceHash) {
			return candidate, nil
		}
	}
	return PackageCandidate{}, errors.New("no stem package matches the source audio SHA-256")
}

func adjacentPackagePath(sourcePath string) string {
	if sourcePath == "" {
		return ""
	}
	ext := filepath.Ext(sourcePath)
	base := strings.TrimSuffix(sourcePath, ext)
	return base + packageDirectorySuffix
}

func pathKey(path string) string {
	if os.PathSeparator == '\\' {
		return strings.ToLower(path)
	}
	return path
}

type sourceHashEntry struct {
	size    int64
	modTime int64
	hash    string
}

// SourceHashCache lazily hashes full source files and reuses the digest while
// path, size, and modification time remain unchanged. A changed signature
// replaces that path's prior entry.
type SourceHashCache struct {
	mu      sync.Mutex
	entries map[string]sourceHashEntry
}

func NewSourceHashCache() *SourceHashCache {
	return &SourceHashCache{entries: make(map[string]sourceHashEntry)}
}

func (c *SourceHashCache) SHA256(path string) (string, error) {
	return c.SHA256Context(context.Background(), path)
}

// SHA256Context hashes a source file with cancellation checks between reads.
func (c *SourceHashCache) SHA256Context(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if c == nil {
		return "", errors.New("nil source hash cache")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve source path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("source audio is not a regular file")
	}
	key := pathKey(filepath.Clean(abs))
	size, modTime := info.Size(), info.ModTime().UnixNano()
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]sourceHashEntry)
	}
	if cached, ok := c.entries[key]; ok && cached.size == size && cached.modTime == modTime {
		c.mu.Unlock()
		return cached.hash, nil
	}
	c.mu.Unlock()

	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, copyErr := copyContext(ctx, h, f)
	closeErr := f.Close()
	if copyErr != nil {
		return "", fmt.Errorf("read source audio: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close source audio: %w", closeErr)
	}
	got := hex.EncodeToString(h.Sum(nil))
	// Re-stat before caching so a file changed while it was being read does not
	// poison the cache with a digest under the earlier metadata signature.
	after, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if after.Size() != size || after.ModTime().UnixNano() != modTime {
		return "", errors.New("source audio changed while hashing")
	}
	c.mu.Lock()
	c.entries[key] = sourceHashEntry{size: size, modTime: modTime, hash: got}
	c.mu.Unlock()
	return got, nil
}
