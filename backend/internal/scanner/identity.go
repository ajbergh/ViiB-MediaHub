package scanner

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
