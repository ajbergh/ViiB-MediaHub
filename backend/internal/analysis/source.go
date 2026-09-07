package analysis

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// ResolvedSource is a canonical song source plus the inexpensive revision
// material used to invalidate a prior analysis result.
type ResolvedSource struct {
	SongID      string
	Name        string
	Path        string
	Fingerprint string
	Size        int64
	Mtime       int64
}

// ResolveLocalSource resolves one canonical local song. Plex streams require a
// dedicated authenticated adapter and are intentionally not guessed from the
// file_path field.
func ResolveLocalSource(database *db.DB, songID string) (ResolvedSource, error) {
	song, err := database.GetSongByID(songID)
	if err != nil {
		return ResolvedSource{}, err
	}
	if song == nil || strings.TrimSpace(song.FilePath) == "" {
		return ResolvedSource{}, fmt.Errorf("song %q has no local analysis source", songID)
	}
	if song.Source == "plex" {
		return ResolvedSource{}, fmt.Errorf("song %q requires a Plex source adapter", songID)
	}
	info, err := os.Stat(song.FilePath)
	if err != nil {
		return ResolvedSource{}, fmt.Errorf("stat local source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ResolvedSource{}, fmt.Errorf("local source is not a regular file")
	}
	mtime := info.ModTime().UnixMilli()
	identity := song.FileHash
	if identity == "" {
		identity = "path:" + filepath.Clean(song.FilePath)
	}
	return ResolvedSource{SongID: song.ID, Name: filepath.Base(song.FilePath), Path: song.FilePath, Size: info.Size(), Mtime: mtime,
		Fingerprint: identity + ":" + strconv.FormatInt(info.Size(), 10) + ":" + strconv.FormatInt(mtime, 10)}, nil
}

// Open provides a new source stream. Decoders own and close the returned file.
func (source ResolvedSource) Open() (io.ReadCloser, error) { return os.Open(source.Path) }
