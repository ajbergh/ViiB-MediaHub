//go:build spotify_research

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Read-only output verification for an isolated app run; no HTTP or cookie input.
// Root restriction prevents accidentally traversing the user's real media library.
func verifyAppDownloadArtifacts(ctx context.Context, root string, out io.Writer) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return errDownloadPath
	}
	temporary, err := filepath.Abs(os.TempDir())
	if err != nil || !strings.EqualFold(filepath.Dir(root), temporary) || !strings.HasPrefix(filepath.Base(root), "viib-cookie-app-") {
		return errDownloadPath
	}
	for _, path := range []string{root, filepath.Join(root, "data"), filepath.Join(root, "data", "spotify_downloads")} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errDownloadPath
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	type artifact struct {
		Format string          `json:"format"`
		PCM    pcmVerification `json:"pcm"`
	}
	files := []artifact{}
	directory := filepath.Join(root, "data", "spotify_downloads")
	err = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errDownloadPath
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errDownloadPath
		}
		if entry.IsDir() {
			if entry.Name() == ".cache" {
				return filepath.SkipDir
			}
			return nil
		}
		format := strings.ToLower(filepath.Ext(path))
		if format != ".ogg" && format != ".mp3" {
			return nil
		}
		if len(files) >= 32 {
			return errDownloadLimit
		}
		var pcm pcmVerification
		var err error
		if format == ".ogg" {
			pcm, err = verifyProbeOgg(ctx, path)
		} else {
			pcm, err = verifyProbeMP3(ctx, path)
		}
		if err != nil {
			return errIncompleteAudio
		}
		files = append(files, artifact{strings.TrimPrefix(format, "."), pcm})
		return nil
	})
	if err != nil {
		return &probeFailure{stage: "app_audio_verification", cause: err}
	}
	if len(files) == 0 {
		return errors.New("isolated app contains no completed audio files")
	}
	return json.NewEncoder(out).Encode(struct {
		FullyDecoded bool       `json:"fullyDecoded"`
		Files        []artifact `json:"files"`
	}{true, files})
}
