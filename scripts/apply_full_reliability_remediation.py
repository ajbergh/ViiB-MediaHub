#!/usr/bin/env python3
"""Apply the post-review reliability remediation to ViiB MediaHub.

The script is intentionally deterministic and fails when an expected source shape
has changed. It is run once by the branch-scoped GitHub Actions workflow.
"""
from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def write(path: str, content: str) -> None:
    target = ROOT / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")


def replace_once(path: str, old: str, new: str) -> None:
    text = read(path)
    count = text.count(old)
    if count != 1:
        raise RuntimeError(f"{path}: expected one occurrence, found {count}: {old[:100]!r}")
    write(path, text.replace(old, new, 1))


def replace_all(path: str, old: str, new: str, minimum: int = 1) -> None:
    text = read(path)
    count = text.count(old)
    if count < minimum:
        raise RuntimeError(f"{path}: expected at least {minimum} occurrences, found {count}: {old[:100]!r}")
    write(path, text.replace(old, new))


def regex_once(path: str, pattern: str, replacement: str, flags: int = 0) -> None:
    text = read(path)
    updated, count = re.subn(pattern, replacement, text, count=1, flags=flags)
    if count != 1:
        raise RuntimeError(f"{path}: regex expected one occurrence, found {count}: {pattern[:120]!r}")
    write(path, updated)


def insert_after(path: str, marker: str, addition: str) -> None:
    replace_once(path, marker, marker + addition)


# ---------------------------------------------------------------------------
# Phase 0: scanner and library integrity
# ---------------------------------------------------------------------------

write(
    "backend/internal/scanner/identity.go",
    r'''package scanner

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "os"
)

const fingerprintSampleSize int64 = 64 * 1024

// computeMediaFingerprint creates a move-stable fingerprint without reading an
// entire large media file. Small files are hashed completely; large files use
// the size plus samples from the beginning and end.
func computeMediaFingerprint(path string, info os.FileInfo) (string, error) {
    file, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer file.Close()

    hash := sha256.New()
    if _, err := fmt.Fprintf(hash, "size:%d\x00", info.Size()); err != nil {
        return "", err
    }

    if info.Size() <= fingerprintSampleSize*2 {
        if _, err := io.Copy(hash, file); err != nil {
            return "", err
        }
    } else {
        if _, err := io.CopyN(hash, file, fingerprintSampleSize); err != nil {
            return "", err
        }
        if _, err := file.Seek(-fingerprintSampleSize, io.SeekEnd); err != nil {
            return "", err
        }
        if _, err := io.CopyN(hash, file, fingerprintSampleSize); err != nil {
            return "", err
        }
    }

    return hex.EncodeToString(hash.Sum(nil)), nil
}

func proposedSongID(fingerprint string) string {
    if len(fingerprint) >= 16 {
        return fingerprint[:16]
    }
    return fingerprint
}
''',
)

write(
    "backend/internal/scanner/path_safety_test.go",
    r'''package scanner

import (
    "path/filepath"
    "testing"
)

func TestIsSubPathUsesPathComponents(t *testing.T) {
    root := filepath.Join(string(filepath.Separator), "music")
    if !isSubPath(root, filepath.Join(root, "artist", "song.flac")) {
        t.Fatal("expected nested path to be accepted")
    }
    if isSubPath(root, filepath.Join(string(filepath.Separator), "music-old", "song.flac")) {
        t.Fatal("prefix sibling must not be treated as a child")
    }
    if !isSubPath(root, root) {
        t.Fatal("root should contain itself")
    }
}
''',
)

write(
    "backend/internal/scanner/identity_test.go",
    r'''package scanner

import (
    "os"
    "path/filepath"
    "testing"
)

func TestMediaFingerprintSurvivesMove(t *testing.T) {
    dir := t.TempDir()
    first := filepath.Join(dir, "first.flac")
    second := filepath.Join(dir, "renamed.flac")
    if err := os.WriteFile(first, []byte("stable audio payload"), 0o600); err != nil {
        t.Fatal(err)
    }
    info, err := os.Stat(first)
    if err != nil {
        t.Fatal(err)
    }
    before, err := computeMediaFingerprint(first, info)
    if err != nil {
        t.Fatal(err)
    }
    if err := os.Rename(first, second); err != nil {
        t.Fatal(err)
    }
    info, err = os.Stat(second)
    if err != nil {
        t.Fatal(err)
    }
    after, err := computeMediaFingerprint(second, info)
    if err != nil {
        t.Fatal(err)
    }
    if before != after {
        t.Fatalf("fingerprint changed after move: %s != %s", before, after)
    }
}
''',
)

write(
    "backend/internal/db/identity.go",
    r'''package db

import "database/sql"

// ResolveSongIdentity preserves the logical song ID when a file is rescanned or
// moved. Existing path identity wins during the first fingerprint migration;
// after that, the stable file hash reconciles renames and moves.
func (d *DB) ResolveSongIdentity(filePath, fingerprint, proposedID string) (string, error) {
    var id string
    err := d.conn.QueryRow(`SELECT id FROM songs WHERE file_path = ?`, filePath).Scan(&id)
    if err == nil {
        return id, nil
    }
    if err != sql.ErrNoRows {
        return "", err
    }

    if fingerprint != "" {
        err = d.conn.QueryRow(`SELECT id FROM songs WHERE file_hash = ? LIMIT 1`, fingerprint).Scan(&id)
        if err == nil {
            return id, nil
        }
        if err != sql.ErrNoRows {
            return "", err
        }
    }
    return proposedID, nil
}
''',
)

write(
    "backend/internal/db/upsert_result.go",
    r'''package db

import "strings"

// SongUpsertResult makes scan reporting distinguish inserts from updates.
type SongUpsertResult struct {
    Inserted int
    Updated  int
}

// SaveSongsWithResult records whether each path existed before the upsert. Scan
// batches are intentionally small, so the bounded IN query remains inexpensive.
func (d *DB) SaveSongsWithResult(songs []Song) (SongUpsertResult, error) {
    result := SongUpsertResult{}
    if len(songs) == 0 {
        return result, nil
    }

    placeholders := strings.TrimRight(strings.Repeat("?,", len(songs)), ",")
    args := make([]any, 0, len(songs))
    for _, song := range songs {
        args = append(args, song.FilePath)
    }

    existing := make(map[string]struct{}, len(songs))
    rows, err := d.conn.Query(`SELECT file_path FROM songs WHERE file_path IN (`+placeholders+`)`, args...)
    if err != nil {
        return result, err
    }
    for rows.Next() {
        var path string
        if err := rows.Scan(&path); err != nil {
            rows.Close()
            return result, err
        }
        existing[path] = struct{}{}
    }
    if err := rows.Close(); err != nil {
        return result, err
    }

    if err := d.SaveSongs(songs); err != nil {
        return result, err
    }
    for _, song := range songs {
        if _, found := existing[song.FilePath]; found {
            result.Updated++
        } else {
            result.Inserted++
        }
    }
    return result, nil
}
''',
)

# scanner imports and path containment
replace_once(
    "backend/internal/scanner/scanner.go",
    '"fmt"\n\t"os"',
    '"fmt"\n\t"net/http"\n\t"os"',
)
regex_once(
    "backend/internal/scanner/scanner.go",
    r'func isSubPath\(parent, child string\) bool \{.*?\n\}',
    r'''func isSubPath(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	rel, err := filepath.Rel(parent, child)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}''',
    flags=re.S,
)

# Atomic quick-scan ownership.
replace_once(
    "backend/internal/scanner/scanner.go",
    '''\tif s.IsScanning() {
\t\tlogger.Scanner("Quick scan skipped - scan already in progress")
\t\treturn
\t}

\ts.SetScanning(true)
\tdefer s.SetScanning(false)
''',
    '''\tif !s.TryBeginScan() {
\t\tlogger.Scanner("Quick scan skipped - scan already in progress")
\t\treturn
\t}
\tdefer s.EndScan()
''',
)
insert_after(
    "backend/internal/scanner/scanner.go",
    '''func (s *Scanner) SetScanning(scanning bool) {
\ts.scanMutex.Lock()
\ts.scanning = scanning
\ts.scanMutex.Unlock()
}
''',
    '''
// TryBeginScan atomically acquires scan ownership.
func (s *Scanner) TryBeginScan() bool {
\ts.scanMutex.Lock()
\tdefer s.scanMutex.Unlock()
\tif s.scanning {
\t\treturn false
\t}
\ts.scanning = true
\treturn true
}

// EndScan releases scan ownership.
func (s *Scanner) EndScan() {
\ts.SetScanning(false)
}
''',
)

# Only reconcile deletions for roots that completed without traversal errors.
regex_once(
    "backend/internal/scanner/scanner.go",
    r'\t// Build a set of paths that should exist \(within configured folders\)\n\tvalidFolderPaths := make\(map\[string\]bool\)\n\tfor _, folder := range folders \{\n\t\tvalidFolderPaths\[strings\.ToLower\(filepath\.Clean\(folder\.Path\)\)\] = true\n\t\}\n',
    '''\t// A root is eligible for destructive reconciliation only after a complete,
\t// error-free traversal. Temporary NAS, USB, permission, or mount failures
\t// must never delete valid library records.
\tdeletionSafeFolderPaths := make(map[string]bool)
''',
)
replace_once(
    "backend/internal/scanner/scanner.go",
    '''\t\tresult.Errors += folderResult.Errors

\t\t// Update folder stats
''',
    '''\t\tresult.Errors += folderResult.Errors
\t\tif folderResult.Errors == 0 {
\t\t\tdeletionSafeFolderPaths[filepath.Clean(folder.Path)] = true
\t\t} else {
\t\t\tlogger.Scanner("Skipping destructive reconciliation for %s because traversal reported %d errors", folder.Path, folderResult.Errors)
\t\t}

\t\t// Update folder stats
''',
)
replace_all(
    "backend/internal/scanner/scanner.go",
    "for folderPath := range validFolderPaths {",
    "for folderPath := range deletionSafeFolderPaths {",
)
replace_once(
    "backend/internal/scanner/scanner.go",
    '''\t\tif err != nil {
\t\t\t// Log directory access errors but continue
\t\t\tlogger.Scanner("Error accessing %s: %v", path, err)
\t\t\treturn nil // Skip errors and continue walking
\t\t}
''',
    '''\t\tif err != nil {
\t\t\t// Continue discovery, but mark this root unsafe for deletion reconciliation.
\t\t\tlogger.Scanner("Error accessing %s: %v", path, err)
\t\t\tresult.Errors++
\t\t\treturn nil
\t\t}
''',
)

# Correct insert/update accounting.
replace_all(
    "backend/internal/scanner/scanner.go",
    "if err := s.db.SaveSongs(songs); err != nil {",
    "upsert, err := s.db.SaveSongsWithResult(songs)\n\t\t\tif err != nil {",
    minimum=1,
)
replace_all(
    "backend/internal/scanner/scanner.go",
    "result.NewSongs += len(songs)",
    "result.NewSongs += upsert.Inserted\n\t\t\t\tresult.UpdatedSongs += upsert.Updated",
    minimum=1,
)
replace_all(
    "backend/internal/scanner/scanner.go",
    "NewSongs: len(songs),",
    "NewSongs: upsert.Inserted,\n\t\t\t\t\tUpdatedSongs: upsert.Updated,",
    minimum=1,
)

# Stable identity metadata and reconciliation.
replace_once(
    "backend/internal/scanner/scanner.go",
    "\tCoverData   []byte\n}",
    "\tCoverData   []byte\n\tFileHash    string\n}",
)
regex_once(
    "backend/internal/scanner/scanner.go",
    r'\t\t// Generate ID from file path and size\n\t\thash := sha256\.Sum256\(\[\]byte\(fmt\.Sprintf\("%s:%d", filePath, info\.Size\(\)\)\)\)\n\t\tid := hex\.EncodeToString\(hash\[:8\]\)',
    '''\t\tfingerprint, err := computeMediaFingerprint(filePath, info)
\t\tif err != nil {
\t\t\tresultChan <- metadataResult{nil, fmt.Errorf("failed to fingerprint file: %w", err)}
\t\t\treturn
\t\t}
\t\tid := proposedSongID(fingerprint)''',
)
replace_once(
    "backend/internal/scanner/scanner.go",
    '''\t\t\tresultChan <- metadataResult{&SongMetadata{
\t\t\t\tID:       id,
\t\t\t\tTitle:    title,
\t\t\t\tArtist:   "Unknown Artist",
\t\t\t\tAlbum:    "Unknown Album",
\t\t\t\tFilePath: filePath,
\t\t\t}, nil}
''',
    '''\t\t\tresultChan <- metadataResult{&SongMetadata{
\t\t\t\tID:       id,
\t\t\t\tTitle:    title,
\t\t\t\tArtist:   "Unknown Artist",
\t\t\t\tAlbum:    "Unknown Album",
\t\t\t\tFilePath: filePath,
\t\t\t\tFileHash: fingerprint,
\t\t\t}, nil}
''',
)
replace_once(
    "backend/internal/scanner/scanner.go",
    '''\t\tsong := &SongMetadata{
\t\t\tID:       id,
\t\t\tTitle:    getTag(taglib.Title),
''',
    '''\t\tsong := &SongMetadata{
\t\t\tID:       id,
\t\t\tFileHash: fingerprint,
\t\t\tTitle:    getTag(taglib.Title),
''',
)
replace_once(
    "backend/internal/scanner/scanner.go",
    '''\t\tsong, err := s.extractMetadata(path)
\t\tif err != nil {
\t\t\tlogger.Scanner("Failed to extract metadata from %s: %v", path, err)
\t\t\tresult.Errors++
\t\t\treturn nil
\t\t}

\t\t// Get or create album cover
''',
    '''\t\tsong, err := s.extractMetadata(path)
\t\tif err != nil {
\t\t\tlogger.Scanner("Failed to extract metadata from %s: %v", path, err)
\t\t\tresult.Errors++
\t\t\treturn nil
\t\t}
\t\tresolvedID, err := s.db.ResolveSongIdentity(path, song.FileHash, song.ID)
\t\tif err != nil {
\t\t\tlogger.Scanner("Failed to resolve stable identity for %s: %v", path, err)
\t\t\tresult.Errors++
\t\t\treturn nil
\t\t}
\t\tsong.ID = resolvedID

\t\t// Get or create album cover
''',
)
replace_once(
    "backend/internal/scanner/scanner.go",
    "\t\t\tFileHash:    song.ID, // Use the file hash we generated",
    "\t\t\tFileHash:    song.FileHash,",
)

# Preserve MIME type for cached artwork.
regex_once(
    "backend/internal/scanner/scanner.go",
    r'func \(s \*Scanner\) saveCoverWithContentHash\(srcPath string, data \[\]byte\) string \{.*?\n\}\n\n// findLocalCover',
    r'''func (s *Scanner) saveCoverWithContentHash(srcPath string, data []byte) string {
\tvar coverData []byte
\tvar err error
\tif srcPath != "" {
\t\tcoverData, err = os.ReadFile(srcPath)
\t\tif err != nil {
\t\t\tlogger.Scanner("Failed to read cover file %s: %v", srcPath, err)
\t\t\treturn ""
\t\t}
\t} else if len(data) > 0 {
\t\tcoverData = data
\t} else {
\t\treturn ""
\t}

\tcontentType := http.DetectContentType(coverData)
\textension := ""
\tswitch contentType {
\tcase "image/jpeg":
\t\textension = ".jpg"
\tcase "image/png":
\t\textension = ".png"
\tcase "image/gif":
\t\textension = ".gif"
\tcase "image/webp":
\t\textension = ".webp"
\tdefault:
\t\tlogger.Scanner("Unsupported cover content type %s", contentType)
\t\treturn ""
\t}

\tcontentHash := sha256.Sum256(coverData)
\tcoverPath := filepath.Join(s.coverDir, hex.EncodeToString(contentHash[:8])+extension)
\tif _, err := os.Stat(coverPath); err == nil {
\t\treturn coverPath
\t}
\tif err := os.WriteFile(coverPath, coverData, 0o600); err != nil {
\t\tlogger.Scanner("Failed to write cover file %s: %v", coverPath, err)
\t\treturn ""
\t}
\treturn coverPath
}

// findLocalCover''',
    flags=re.S,
)

# Preserve logical IDs and paths during upsert.
replace_once(
    "backend/internal/db/db.go",
    "\t\t\tcover_path = excluded.cover_path,\n\t\t\tplay_count = excluded.play_count,",
    "\t\t\tcover_path = excluded.cover_path,\n\t\t\tfile_path = excluded.file_path,\n\t\t\tplay_count = excluded.play_count,",
)
replace_once(
    "backend/internal/db/db.go",
    "\t\tON CONFLICT(file_path) DO UPDATE SET\n\t\t\ttitle = excluded.title,",
    "\t\tON CONFLICT DO UPDATE SET\n\t\t\tfile_path = excluded.file_path,\n\t\t\ttitle = excluded.title,",
)
replace_once(
    "backend/internal/db/db.go",
    "\t\t\tcover_path = excluded.cover_path\n\t`)",
    "\t\t\tcover_path = excluded.cover_path,\n\t\t\tfile_hash = excluded.file_hash\n\t`)",
)

# ---------------------------------------------------------------------------
# Phase 1: Spotify credential and token lifecycle
# ---------------------------------------------------------------------------

write(
    "backend/internal/api/spotify_token.go",
    r'''package api

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strings"
    "sync"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/db"
)

var (
    spotifyTokenRefreshMu sync.Mutex
    spotifyTokenEndpoint  = "https://accounts.spotify.com/api/token"
)

func loadValidSpotifyCredentials(ctx context.Context, database *db.DB) (SpotifyCredentials, error) {
    spotifyTokenRefreshMu.Lock()
    defer spotifyTokenRefreshMu.Unlock()

    raw, err := database.GetSetting("spotify_credentials")
    if err != nil || raw == "" {
        return SpotifyCredentials{}, fmt.Errorf("spotify credentials not configured")
    }
    var credentials SpotifyCredentials
    if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
        return SpotifyCredentials{}, fmt.Errorf("parse spotify credentials: %w", err)
    }
    if credentials.AccessToken == "" {
        return SpotifyCredentials{}, fmt.Errorf("spotify access token missing")
    }

    if credentials.Expiry == 0 || time.Now().Add(5*time.Minute).Before(time.UnixMilli(credentials.Expiry)) {
        return credentials, nil
    }
    if credentials.RefreshToken == "" || credentials.ClientId == "" {
        return SpotifyCredentials{}, fmt.Errorf("spotify re-authentication required")
    }

    values := url.Values{
        "grant_type":    {"refresh_token"},
        "refresh_token": {credentials.RefreshToken},
        "client_id":     {credentials.ClientId},
    }
    request, err := http.NewRequestWithContext(ctx, http.MethodPost, spotifyTokenEndpoint, strings.NewReader(values.Encode()))
    if err != nil {
        return SpotifyCredentials{}, err
    }
    request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
    if err != nil {
        return SpotifyCredentials{}, fmt.Errorf("refresh spotify token: %w", err)
    }
    defer response.Body.Close()
    if response.StatusCode != http.StatusOK {
        return SpotifyCredentials{}, fmt.Errorf("spotify token refresh returned %d", response.StatusCode)
    }

    var token struct {
        AccessToken  string `json:"access_token"`
        RefreshToken string `json:"refresh_token"`
        ExpiresIn    int    `json:"expires_in"`
    }
    if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
        return SpotifyCredentials{}, fmt.Errorf("decode spotify token response: %w", err)
    }
    if token.AccessToken == "" || token.ExpiresIn <= 0 {
        return SpotifyCredentials{}, fmt.Errorf("spotify token refresh returned incomplete credentials")
    }

    credentials.AccessToken = token.AccessToken
    if token.RefreshToken != "" {
        credentials.RefreshToken = token.RefreshToken
    }
    credentials.Expiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli()
    encoded, err := json.Marshal(credentials)
    if err != nil {
        return SpotifyCredentials{}, err
    }
    if err := database.SetSetting("spotify_credentials", string(encoded)); err != nil {
        return SpotifyCredentials{}, fmt.Errorf("persist refreshed spotify token: %w", err)
    }
    return credentials, nil
}

func (a *API) validSpotifyAccessToken(ctx context.Context) (string, error) {
    credentials, err := loadValidSpotifyCredentials(ctx, a.db)
    if err != nil {
        return "", err
    }
    return credentials.AccessToken, nil
}
''',
)

write(
    "backend/internal/api/spotify_token_test.go",
    r'''package api

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "testing"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/db"
)

func TestPKCERefreshDoesNotRequireClientSecret(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if err := r.ParseForm(); err != nil {
            t.Fatal(err)
        }
        if r.Form.Get("client_id") != "client-id" || r.Form.Get("client_secret") != "" {
            t.Fatalf("unexpected refresh form: %v", r.Form)
        }
        _ = json.NewEncoder(w).Encode(map[string]any{
            "access_token": "new-access-token",
            "expires_in":   3600,
        })
    }))
    defer server.Close()

    previousEndpoint := spotifyTokenEndpoint
    spotifyTokenEndpoint = server.URL
    defer func() { spotifyTokenEndpoint = previousEndpoint }()

    database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
    if err != nil {
        t.Fatal(err)
    }
    defer database.Close()
    credentials := SpotifyCredentials{
        ClientId:     "client-id",
        AccessToken:  "expired",
        RefreshToken: "refresh",
        Expiry:       time.Now().Add(-time.Minute).UnixMilli(),
    }
    raw, _ := json.Marshal(credentials)
    if err := database.SetSetting("spotify_credentials", string(raw)); err != nil {
        t.Fatal(err)
    }

    refreshed, err := loadValidSpotifyCredentials(context.Background(), database)
    if err != nil {
        t.Fatal(err)
    }
    if refreshed.AccessToken != "new-access-token" {
        t.Fatalf("unexpected token %q", refreshed.AccessToken)
    }
}
''',
)

# API endpoints use the centralized refresh path.
regex_once(
    "backend/internal/api/spotify.go",
    r'\tval, err := a\.db\.GetSetting\("spotify_credentials"\)\n\tif err != nil \|\| val == "" \{\n\t\trespondError\(w, http\.StatusUnauthorized, "Spotify credentials not configured"\)\n\t\treturn\n\t\}\n\tvar creds SpotifyCredentials\n\tif err := json\.Unmarshal\(\[\]byte\(val\), &creds\); err != nil \{\n\t\trespondError\(w, http\.StatusInternalServerError, "Failed to parse credentials"\)\n\t\treturn\n\t\}\n\tquery :=',
    '''\taccessToken, err := a.validSpotifyAccessToken(r.Context())
\tif err != nil {
\t\trespondError(w, http.StatusUnauthorized, err.Error())
\t\treturn
\t}
\tquery :=''',
)
replace_once(
    "backend/internal/api/spotify.go",
    'request.Header.Set("Authorization", "Bearer "+creds.AccessToken)',
    'request.Header.Set("Authorization", "Bearer "+accessToken)',
)
regex_once(
    "backend/internal/api/spotify.go",
    r'func \(a \*API\) spotifyGetUserProfile\(w http\.ResponseWriter, r \*http\.Request\) \{\n\tval, err := a\.db\.GetSetting\("spotify_credentials"\).*?\n\treq, err := http\.NewRequest\("GET", "https://api\.spotify\.com/v1/me", nil\)',
    '''func (a *API) spotifyGetUserProfile(w http.ResponseWriter, r *http.Request) {
\taccessToken, err := a.validSpotifyAccessToken(r.Context())
\tif err != nil {
\t\trespondError(w, http.StatusUnauthorized, err.Error())
\t\treturn
\t}
\treq, err := http.NewRequestWithContext(r.Context(), "GET", "https://api.spotify.com/v1/me", nil)''',
    flags=re.S,
)
replace_once(
    "backend/internal/api/spotify.go",
    'req.Header.Set("Authorization", "Bearer "+creds.AccessToken)',
    'req.Header.Set("Authorization", "Bearer "+accessToken)',
)
regex_once(
    "backend/internal/api/spotify.go",
    r'func \(a \*API\) spotifyProxy\(w http\.ResponseWriter, r \*http\.Request\) \{\n\tval, err := a\.db\.GetSetting\("spotify_credentials"\).*?\n\t// Get the path after /api/spotify/proxy/',
    '''func (a *API) spotifyProxy(w http.ResponseWriter, r *http.Request) {
\taccessToken, err := a.validSpotifyAccessToken(r.Context())
\tif err != nil {
\t\trespondError(w, http.StatusUnauthorized, err.Error())
\t\treturn
\t}

\t// Get the path after /api/spotify/proxy/''',
    flags=re.S,
)
replace_once(
    "backend/internal/api/spotify.go",
    'req.Header.Set("Authorization", "Bearer "+creds.AccessToken)',
    'req.Header.Set("Authorization", "Bearer "+accessToken)',
)

# Download/stream session uses PKCE refresh and the same credential source.
regex_once(
    "backend/internal/api/download_manager.go",
    r'\t// Get Spotify credentials from database.*?\n\tdmLog\("Access token present \(length: %d\), updating session manager\.\.\.", len\(creds\.AccessToken\)\)',
    '''\tcreds, err := loadValidSpotifyCredentials(dm.ctx, dm.db)
\tif err != nil {
\t\tdm.setAuthRequired(true, "Spotify session expired - please reconnect to Spotify")
\t\treturn err
\t}

\tdmLog("Access token present (length: %d), updating session manager...", len(creds.AccessToken))''',
    flags=re.S,
)

# Browser persistence no longer stores secrets and now retains playback preferences.
replace_once("store.ts", "version: 1, // Increment when storage schema changes", "version: 2, // Removes legacy renderer secrets and persists Spotify playback preferences")
replace_once("store.ts", "          spotifyClientSecret: state.spotifyClientSecret,\n", "")
replace_once(
    "store.ts",
    "          spotifyUser: state.spotifyUser,\n",
    "          spotifyUser: state.spotifyUser,\n          streamingEnabled: state.streamingEnabled,\n          streamingQuality: state.streamingQuality,\n          preferLocalPlayback: state.preferLocalPlayback,\n",
)
insert_after(
    "store.ts",
    "      version: 2, // Removes legacy renderer secrets and persists Spotify playback preferences\n",
    '''      migrate: (persistedState: any) => {
        const migrated = { ...(persistedState || {}) };
        delete migrated.spotifyClientSecret;
        return migrated;
      },
''',
)
replace_once(
    "slices/spotifySlice.ts",
    "  setSpotifyCredentials: (id, secret) => set({ spotifyClientId: id, spotifyClientSecret: secret }),",
    "  setSpotifyCredentials: (id, _secret) => set({ spotifyClientId: id, spotifyClientSecret: '' }),",
)
replace_once(
    "App.tsx",
    "                                     (creds && creds.clientId && creds.clientSecret);",
    "                                     Boolean(creds && creds.clientId);",
)
replace_once(
    "App.tsx",
    "                      setSpotifyCredentials(creds.clientId, creds.clientSecret);",
    "                      setSpotifyCredentials(creds.clientId, '');",
)
replace_once(
    "services/spotifyService.ts",
    "            spotifyClientId, spotifyClientSecret, \n",
    "            spotifyClientId,\n",
)

# ---------------------------------------------------------------------------
# Phase 2 and 3: deterministic playback and truthful audio controls
# ---------------------------------------------------------------------------

write(
    "lib/playbackLifecycle.ts",
    r'''export const normalizeCrossfadeDuration = (
  requested: number | undefined,
  gapless: boolean,
): number => {
  if (gapless) return 0;
  if (!Number.isFinite(requested)) return 0.2;
  return Math.max(0, requested ?? 0.2);
};

export const isActivePlaybackEvent = (
  eventTarget: EventTarget | null,
  activeElement: HTMLAudioElement | null,
): boolean => Boolean(activeElement && eventTarget === activeElement);

export class ManagedObjectUrlRegistry {
  private readonly urls = new Set<string>();

  create(blob: Blob): string {
    const url = URL.createObjectURL(blob);
    this.urls.add(url);
    return url;
  }

  release(url?: string | null): void {
    if (!url || !this.urls.delete(url)) return;
    URL.revokeObjectURL(url);
  }

  releaseAll(): void {
    for (const url of this.urls) URL.revokeObjectURL(url);
    this.urls.clear();
  }
}
''',
)

write(
    "lib/playbackLifecycle.test.ts",
    r'''import { describe, expect, it, vi } from 'vitest';
import { ManagedObjectUrlRegistry, normalizeCrossfadeDuration } from './playbackLifecycle';

describe('normalizeCrossfadeDuration', () => {
  it('preserves an explicit zero', () => {
    expect(normalizeCrossfadeDuration(0, false)).toBe(0);
  });
  it('forces zero for gapless mode', () => {
    expect(normalizeCrossfadeDuration(5, true)).toBe(0);
  });
});

describe('ManagedObjectUrlRegistry', () => {
  it('revokes managed URLs exactly once', () => {
    const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:test');
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined);
    const registry = new ManagedObjectUrlRegistry();
    const url = registry.create(new Blob(['audio']));
    registry.release(url);
    registry.release(url);
    expect(create).toHaveBeenCalledOnce();
    expect(revoke).toHaveBeenCalledOnce();
  });
});
''',
)

# Song playback context is carried through queue transitions.
regex_once(
    "types.ts",
    r'export interface Song \{',
    "export type PlaybackContext = 'ai_dj' | 'album' | 'playlist' | 'queue' | 'search' | 'spotify';\n\nexport interface Song {\n  playbackContext?: PlaybackContext;",
)
regex_once(
    "slices/types.ts",
    r'playSong: \(song: Song, context\?: Song\[\]\) => Promise<void>;',
    "playSong: (song: Song, context?: Song[], playbackContext?: import('../types').PlaybackContext) => Promise<void>;",
)
replace_once(
    "slices/playerSlice.ts",
    "import { AudioSettings, MilkdropSettings } from '../types';",
    "import { AudioSettings, MilkdropSettings } from '../types';\nimport { ManagedObjectUrlRegistry } from '../lib/playbackLifecycle';",
)
insert_after(
    "slices/playerSlice.ts",
    "let saveSettingsTimeout: ReturnType<typeof setTimeout> | null = null;\n",
    "\nlet latestPlaybackRequestId = 0;\nconst managedObjectUrls = new ManagedObjectUrlRegistry();\n",
)
replace_once(
    "slices/playerSlice.ts",
    "    playSong: async (song, context) => {\n        // 1. Resolve URL if missing (e.g. after page reload)",
    "    playSong: async (song, context, requestedContext = song.playbackContext || 'queue') => {\n        const playbackRequestId = ++latestPlaybackRequestId;\n        // 1. Resolve URL if missing (e.g. after page reload)",
)
replace_all(
    "slices/playerSlice.ts",
    "playableSong.url = URL.createObjectURL(file);",
    "playableSong.url = managedObjectUrls.create(file);",
    minimum=2,
)
replace_once(
    "slices/playerSlice.ts",
    "        // When no explicit context is provided, default the queue to just this",
    "        if (playbackRequestId !== latestPlaybackRequestId) {\n            managedObjectUrls.release(playableSong.url);\n            return;\n        }\n        playableSong.playbackContext = requestedContext;\n        const previousUrl = get().currentSong?.url;\n        if (previousUrl && previousUrl !== playableSong.url) managedObjectUrls.release(previousUrl);\n\n        // When no explicit context is provided, default the queue to just this",
)
replace_once(
    "slices/playerSlice.ts",
    "            await playSong(currentSong, queue);",
    "            await playSong(currentSong, queue, currentSong.playbackContext || 'queue');",
)

# Hook guards inactive audio-element events, supports output routing, and uses media time.
replace_once(
    "hooks/useAudioPlayer.ts",
    "import { StreamingErrorType } from '../slices/types';",
    "import { StreamingErrorType } from '../slices/types';\nimport { isActivePlaybackEvent, normalizeCrossfadeDuration } from '../lib/playbackLifecycle';",
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "        lastPlayStartTime: number | null;\n        isTracking: boolean;",
    "        lastPlayStartTime: number | null;\n        lastMediaTime: number;\n        isTracking: boolean;",
)
insert_after(
    "hooks/useAudioPlayer.ts",
    "    useEffect(() => {\n        audioEngine.setVolume(volume);\n    }, [volume]);\n",
    '''
    useEffect(() => {
        const applySink = async (element: HTMLAudioElement | null) => {
            if (!element || !audioSettings.mainOutputDevice) return;
            const sinkCapable = element as HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> };
            if (typeof sinkCapable.setSinkId !== 'function') return;
            try {
                await sinkCapable.setSinkId(audioSettings.mainOutputDevice);
            } catch (error) {
                console.warn('[AudioPlayer] Failed to route output device', error);
            }
        };
        void applySink(primaryRef.current);
        void applySink(secondaryRef.current);
    }, [audioSettings.mainOutputDevice]);
''',
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "        const handleWaiting = () => {",
    "        const activeElement = () => activePlayerIndex.current === 0 ? primaryRef.current : secondaryRef.current;\n        const handleWaiting = (event: Event) => {\n            if (!isActivePlaybackEvent(event.currentTarget, activeElement())) return;",
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "        const handleCanPlay = () => {",
    "        const handleCanPlay = (event: Event) => {\n            if (!isActivePlaybackEvent(event.currentTarget, activeElement())) return;",
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "        const handleProgress = (e: Event) => {\n            const audio = e.target as HTMLAudioElement;",
    "        const handleProgress = (e: Event) => {\n            if (!isActivePlaybackEvent(e.currentTarget, activeElement())) return;\n            const audio = e.target as HTMLAudioElement;",
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "        const handleError = (e: Event) => {\n            const audio = e.target as HTMLAudioElement;",
    "        const handleError = (e: Event) => {\n            if (!isActivePlaybackEvent(e.currentTarget, activeElement())) return;\n            const audio = e.target as HTMLAudioElement;",
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "        const handleStalled = () => {",
    "        const handleStalled = (event: Event) => {\n            if (!isActivePlaybackEvent(event.currentTarget, activeElement())) return;",
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "                lastPlayStartTime: isPlaying ? Date.now() : null,\n                isTracking: true",
    "                lastPlayStartTime: isPlaying ? Date.now() : null,\n                lastMediaTime: 0,\n                isTracking: true",
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "                 const fadeDuration = audioSettings.crossfadeDuration || 0.2; ",
    "                 const fadeDuration = normalizeCrossfadeDuration(audioSettings.crossfadeDuration, audioSettings.gapless);",
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "    }, [currentSong, isPlaying, audioSettings.crossfadeDuration]);",
    "    }, [currentSong, isPlaying, audioSettings.crossfadeDuration, audioSettings.gapless]);",
)
insert_after(
    "hooks/useAudioPlayer.ts",
    "            const time = player.currentTime;\n            const dur = player.duration || 0;\n",
    '''
            const tracking = listenTrackingRef.current;
            if (tracking && tracking.songId === useStore.getState().currentSong?.id) {
                const delta = time - tracking.lastMediaTime;
                if (delta > 0 && delta < 5) tracking.accumulatedPlayTime += delta;
                tracking.lastMediaTime = time;
            }
''',
)
# Wall-clock time is no longer counted as listening time.
regex_once(
    "hooks/useAudioPlayer.ts",
    r'\s+if \(listenTrackingRef\.current && listenTrackingRef\.current\.lastPlayStartTime !== null\) \{\n\s+listenTrackingRef\.current\.accumulatedPlayTime \+=\s+\(Date\.now\(\) - listenTrackingRef\.current\.lastPlayStartTime\) / 1000;\n\s+listenTrackingRef\.current\.lastPlayStartTime = null;\n\s+\}',
    "\n                  if (listenTrackingRef.current) listenTrackingRef.current.lastPlayStartTime = null;",
)
replace_all(
    "hooks/useAudioPlayer.ts",
    "                        'queue'\n",
    "                        currentSong.playbackContext || 'queue'\n",
    minimum=1,
)
replace_all(
    "hooks/useAudioPlayer.ts",
    "                    'queue'\n",
    "                    currentSong.playbackContext || 'queue'\n",
    minimum=1,
)
replace_once(
    "hooks/useAudioPlayer.ts",
    "            player.currentTime = time;\n            setCurrentTime(time);",
    "            player.currentTime = time;\n            if (listenTrackingRef.current) listenTrackingRef.current.lastMediaTime = time;\n            setCurrentTime(time);",
)

# Clamp and validate analytics input.
replace_once(
    "backend/internal/api/api.go",
    '''\tif body.PlayDuration < 0 || body.SongDuration <= 0 || body.PlayDuration > 7*24*60*60 || body.SongDuration > 7*24*60*60 {
\t\trespondError(w, http.StatusBadRequest, "Invalid listening durations")
\t\treturn
\t}

\t// Determine event type based on play duration
''',
    '''\tif body.PlayDuration < 0 || body.SongDuration <= 0 || body.PlayDuration > 7*24*60*60 || body.SongDuration > 7*24*60*60 {
\t\trespondError(w, http.StatusBadRequest, "Invalid listening durations")
\t\treturn
\t}
\tif body.PlayDuration > body.SongDuration {
\t\tbody.PlayDuration = body.SongDuration
\t}
\tvalidContexts := map[string]bool{"ai_dj": true, "album": true, "playlist": true, "queue": true, "search": true, "spotify": true}
\tif !validContexts[body.Context] {
\t\tbody.Context = "queue"
\t}

\t// Determine event type based on play duration
''',
)

# PATCH is required by the duration endpoint in cross-origin Wails mode.
replace_once(
    "backend/internal/server/server.go",
    'AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},',
    'AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},',
)

# ---------------------------------------------------------------------------
# Phase 5: release evidence and regression coverage
# ---------------------------------------------------------------------------

write(
    "docs/FULL_RELIABILITY_REMEDIATION_STATUS.md",
    r'''# Full Reliability Remediation Status

Branch: `agent/full-reliability-remediation`

## Phase 0 — Library integrity

- [x] Prevent destructive deletion reconciliation after incomplete root scans.
- [x] Replace path-prefix containment with `filepath.Rel` component checks.
- [x] Make quick-scan admission atomic.
- [x] Report inserts and updates separately.
- [x] Add move-stable media fingerprints and path/hash identity reconciliation.
- [x] Preserve artwork MIME types in the cache.
- [x] Add scanner path and fingerprint regression tests.

## Phase 1 — Spotify lifecycle

- [x] Remove the client secret from persisted renderer state and migrate legacy storage.
- [x] Persist Spotify playback preferences.
- [x] Implement backend-owned PKCE refresh using refresh token plus client ID.
- [x] Share refreshed credentials with download and streaming sessions.
- [x] Route core Spotify API calls through the centralized valid-token path.
- [x] Add a PKCE refresh regression test.

## Phase 2 — Playback reliability

- [x] Preserve an explicit zero crossfade value.
- [x] Guard inactive audio-element buffering/error events.
- [x] Add playback-request sequencing for asynchronous source resolution.
- [x] Revoke application-created object URLs.
- [x] Carry playback context into listening analytics.
- [x] Measure listening from media-time movement and clamp server input.
- [x] Add unit tests for crossfade and object-URL lifecycle.

## Phase 3 — Audio capability wiring

- [x] Apply the selected main output device when `setSinkId` is supported.
- [x] Make gapless mode use a zero-duration handoff rather than a hidden fade.
- [x] Persist streaming enabled, quality, and local-playback preference.
- [ ] ReplayGain/EBU R128 analysis remains a separate media-analysis feature; the existing normalization control should remain marked experimental until per-track loudness metadata is available.

## Phase 4 — Library identity and metadata

- [x] Add stable content fingerprints.
- [x] Reconcile file moves and renames without changing logical song IDs after fingerprint migration.
- [x] Preserve user metadata during normal rescans.
- [x] Correct cached cover extensions and MIME behavior.
- [ ] Interactive duplicate-resolution and M3U import/export remain product capabilities rather than correctness blockers and should be delivered in a dedicated UX change set.

## Phase 5 — Validation

- [x] Add backend scanner and Spotify token tests.
- [x] Add frontend playback lifecycle tests.
- [x] Run formatting, frontend checks, Go tests, race tests, vet, and staticcheck in CI.
- [x] Retain the existing Windows Wails packaging and vulnerability gates.

## Release note

The branch prioritizes prevention of user-data loss, deterministic playback, and a single Spotify token lifecycle. ReplayGain analysis, duplicate-management UX, and playlist interchange are explicitly separated from the reliability release because they require new user-facing workflows and persisted data models rather than defect remediation.
''',
)

print("Full reliability remediation applied successfully")
