#!/usr/bin/env python3
"""Apply final code-review corrections before PR handoff."""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def rewrite(path: str, transform) -> None:
    target = ROOT / path
    before = target.read_text(encoding="utf-8")
    after = transform(before)
    if after != before:
        target.write_text(after, encoding="utf-8")


def fix_scanner(text: str) -> str:
    old_delete = '''\t\t\tpathLower := strings.ToLower(filepath.Clean(existingPath))
\t\t\tisInScanFolder := false
\t\t\tfor folderPath := range deletionSafeFolderPaths {
\t\t\t\tif strings.HasPrefix(pathLower, folderPath) {
\t\t\t\t\tisInScanFolder = true
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}
'''
    new_delete = '''\t\t\tisInScanFolder := false
\t\t\tfor folderPath := range deletionSafeFolderPaths {
\t\t\t\tif isSubPath(folderPath, existingPath) {
\t\t\t\t\tisInScanFolder = true
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}
'''
    if old_delete in text:
        text = text.replace(old_delete, new_delete, 1)

    old_batch_error = '''\t\t\tif err != nil {
\t\t\t\tlogger.Scanner("ERROR saving batch to database: %v", err)
\t\t\t\t// Continue scanning even if save fails
\t\t\t} else {
'''
    new_batch_error = '''\t\t\tif err != nil {
\t\t\t\tlogger.Scanner("ERROR saving batch to database: %v", err)
\t\t\t\tresult.Errors++
\t\t\t\t// Continue discovery, but make the root ineligible for deletion reconciliation.
\t\t\t} else {
'''
    if old_batch_error in text:
        text = text.replace(old_batch_error, new_batch_error, 1)

    text = text.replace('Message:      fmt.Sprintf("Added %d songs", len(songs)),',
                        'Message:      fmt.Sprintf("Library updated: %d added, %d updated", upsert.Inserted, upsert.Updated),')
    text = text.replace('id := proposedSongID(fingerprint)', 'id := proposedSongID(fingerprint, filePath)', 1)
    return text


def fix_scanner_identity(text: str) -> str:
    if '"path/filepath"' not in text:
        text = text.replace('\t"os"\n', '\t"os"\n\t"path/filepath"\n')
    old = '''func proposedSongID(fingerprint string) string {
\tif len(fingerprint) >= 16 {
\t\treturn fingerprint[:16]
\t}
\treturn fingerprint
}
'''
    new = '''// proposedSongID is path-specific so identical files in two live locations
// remain distinct library entries. Move reconciliation reuses the previous ID
// only when the previous path is confirmed absent.
func proposedSongID(fingerprint, filePath string) string {
\thash := sha256.Sum256([]byte(fingerprint + "\\x00" + filepath.Clean(filePath)))
\treturn hex.EncodeToString(hash[:8])
}
'''
    return text.replace(old, new, 1)


def fix_db_identity(text: str) -> str:
    text = text.replace('import "database/sql"', 'import (\n\t"database/sql"\n\t"errors"\n\t"os"\n)')
    old = '''\tif fingerprint != "" {
\t\terr = d.conn.QueryRow(`SELECT id FROM songs WHERE file_hash = ? LIMIT 1`, fingerprint).Scan(&id)
\t\tif err == nil {
\t\t\treturn id, nil
\t\t}
\t\tif err != sql.ErrNoRows {
\t\t\treturn "", err
\t\t}
\t}
'''
    new = '''\tif fingerprint != "" {
\t\tvar previousPath string
\t\terr = d.conn.QueryRow(`SELECT id, file_path FROM songs WHERE file_hash = ? LIMIT 1`, fingerprint).Scan(&id, &previousPath)
\t\tif err == nil {
\t\t\t// Reuse identity only for a confirmed move. If the old path still exists,
\t\t\t// this is a duplicate copy and must receive its own logical ID.
\t\t\tif _, statErr := os.Stat(previousPath); errors.Is(statErr, os.ErrNotExist) {
\t\t\t\treturn id, nil
\t\t\t}
\t\t\treturn proposedID, nil
\t\t}
\t\tif err != sql.ErrNoRows {
\t\t\treturn "", err
\t\t}
\t}
'''
    return text.replace(old, new, 1)


def fix_scanner_tests(text: str) -> str:
    addition = '''
func TestProposedSongIDKeepsLiveDuplicatesDistinct(t *testing.T) {
\tfingerprint := "0123456789abcdef0123456789abcdef"
\tfirst := proposedSongID(fingerprint, filepath.Join("library-a", "song.flac"))
\tsecond := proposedSongID(fingerprint, filepath.Join("library-b", "song.flac"))
\tif first == second {
\t\tt.Fatal("identical content at two paths must not collapse to one song ID")
\t}
\tif first != proposedSongID(fingerprint, filepath.Join("library-a", "song.flac")) {
\t\tt.Fatal("proposed ID must be deterministic")
\t}
}
'''
    if "TestProposedSongIDKeepsLiveDuplicatesDistinct" not in text:
        text += addition
    return text


rewrite("backend/internal/scanner/scanner.go", fix_scanner)
rewrite("backend/internal/scanner/identity.go", fix_scanner_identity)
rewrite("backend/internal/db/identity.go", fix_db_identity)
rewrite("backend/internal/scanner/identity_test.go", fix_scanner_tests)
print("Final review fixes applied")
