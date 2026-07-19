package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// proposedSongID is path-specific so identical files in two live locations
// remain distinct library entries. Move reconciliation reuses the previous ID
// only when the previous path is confirmed absent.
func proposedSongID(fingerprint, filePath string) string {
	hash := sha256.Sum256([]byte(fingerprint + "\x00" + filepath.Clean(filePath)))
	return hex.EncodeToString(hash[:8])
}
