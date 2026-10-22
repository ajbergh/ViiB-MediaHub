// Defines v2 stem preview functionality for package api.

package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/stems"
	"github.com/go-chi/chi/v5"
)

// getStemPreviewV2 streams one allow-listed audio artifact from the registered
// package. ServeContent handles HTTP byte ranges while keeping file paths local.
func (a *API) getStemPreviewV2(w http.ResponseWriter, r *http.Request) {
	songID, setID, rawName := chi.URLParam(r, "songID"), chi.URLParam(r, "stemSetID"), chi.URLParam(r, "stemName")
	name := stems.StemName(rawName)
	if !isAllowedStemName(name) {
		stemError(w, r, http.StatusNotFound, "stem_artifact_not_found", "Stem artifact was not found", false)
		return
	}
	song, err := a.db.GetSongByID(songID)
	if errors.Is(err, sql.ErrNoRows) || song == nil && err == nil {
		stemError(w, r, http.StatusNotFound, "song_not_found", "Song was not found", false)
		return
	}
	if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_registry_unavailable", "Unable to load track", true)
		return
	}
	if song.Source == "plex" || strings.TrimSpace(song.FilePath) == "" {
		stemError(w, r, http.StatusConflict, "stem_source_unavailable", "Stem preview requires a local source audio file", false)
		return
	}
	sets, err := a.db.ListStemSets(songID)
	if err != nil {
		stemError(w, r, http.StatusInternalServerError, "stem_registry_unavailable", "Unable to load stem package", true)
		return
	}
	var set *db.StemSet
	for i := range sets {
		if sets[i].ID == setID {
			set = &sets[i]
			break
		}
	}
	if set == nil {
		stemError(w, r, http.StatusNotFound, "stem_set_not_found", "Stem package was not found for this track", false)
		return
	}
	if set.Status != "ready" || set.ManuallyInvalidated {
		stemError(w, r, http.StatusConflict, "stem_set_not_ready", "Stem package is not ready for playback", false)
		return
	}
	root, err := filepath.Abs(set.PackagePath)
	if err != nil || rejectSymlinkPath(root) != nil {
		stemError(w, r, http.StatusConflict, "stem_package_unsafe", "Stem package path is unsafe", false)
		return
	}
	validated, err := validatedStemPackageContext(r.Context(), root)
	if err != nil {
		stemError(w, r, http.StatusConflict, "stem_package_changed", "Stem package failed current validation", false)
		return
	}
	if !registeredPackageMatches(*set, validated.Manifest) {
		stemError(w, r, http.StatusConflict, "stem_package_changed", "Stem package identity differs from the registered package", false)
		return
	}
	if sourceMatches, matchErr := stemSourceMatchesContext(r.Context(), song, validated.Manifest, ""); matchErr != nil || !sourceMatches {
		stemError(w, r, http.StatusConflict, "stem_source_stale", "Stem package no longer matches the local source audio", false)
		return
	}
	artifact, ok := validated.Manifest.Stems[name]
	if !ok {
		stemError(w, r, http.StatusNotFound, "stem_artifact_not_found", "Stem artifact is not part of this package", false)
		return
	}
	mime := "audio/wav"
	switch strings.ToLower(filepath.Ext(artifact.Path)) {
	case ".wav", ".wave":
	case ".mp3":
		mime = "audio/mpeg"
	case ".ogg":
		mime = "audio/ogg"
	default:
		stemError(w, r, http.StatusConflict, "stem_artifact_unsupported", "Stem artifact format is not supported for preview", false)
		return
	}
	if err = rejectSymlinkArtifact(root, artifact.Path); err != nil {
		stemError(w, r, http.StatusConflict, "stem_package_unsafe", "Stem package contains an unsafe artifact path", false)
		return
	}
	path := validated.Files[name]
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		stemError(w, r, http.StatusConflict, "stem_package_unsafe", "Stem artifact path is unsafe", false)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		stemError(w, r, http.StatusConflict, "stem_package_changed", "Stem artifact is unavailable", false)
		return
	}
	defer f.Close()
	fileInfo, err := f.Stat()
	if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Size() != artifact.SizeBytes || !os.SameFile(pathInfo, fileInfo) {
		stemError(w, r, http.StatusConflict, "stem_package_changed", "Stem artifact changed during lookup", false)
		return
	}
	pathAfter, err := os.Lstat(path)
	if err != nil || pathAfter.Mode()&os.ModeSymlink != 0 || !pathAfter.Mode().IsRegular() || !os.SameFile(fileInfo, pathAfter) {
		stemError(w, r, http.StatusConflict, "stem_package_changed", "Stem artifact changed during lookup", false)
		return
	}
	snapshot, snapshotInfo, err := snapshotStemArtifactContext(r.Context(), f, path, artifact)
	if err != nil {
		stemError(w, r, http.StatusConflict, "stem_package_changed", "Stem artifact changed or could not be verified", false)
		return
	}
	defer func() {
		name := snapshot.Name()
		_ = snapshot.Close()
		_ = os.Remove(name)
	}()
	sourceMatches, err := stemSourceMatchesFreshContext(r.Context(), song, validated.Manifest)
	if err != nil || !sourceMatches {
		stemError(w, r, http.StatusConflict, "stem_source_stale", "Stem package no longer matches the current local source audio", false)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Stem-Name", string(name))
	w.Header().Set("X-Sample-Rate", strconv.Itoa(artifact.SampleRate))
	w.Header().Set("X-Channel-Count", strconv.Itoa(artifact.Channels))
	w.Header().Set("X-Frame-Count", strconv.FormatInt(artifact.Frames, 10))
	http.ServeContent(w, r, filepath.Base(artifact.Path), snapshotInfo.ModTime(), snapshot)
}

// snapshotStemArtifact copies a verified artifact into a private temporary
// file before range serving. The response therefore remains bound to the exact
// bytes whose digest was checked, even if the package is replaced mid-stream.
func snapshotStemArtifact(source *os.File, sourcePath string, artifact stems.Artifact) (*os.File, os.FileInfo, error) {
	return snapshotStemArtifactContext(context.Background(), source, sourcePath, artifact)
}
func snapshotStemArtifactContext(ctx context.Context, source *os.File, sourcePath string, artifact stems.Artifact) (*os.File, os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	before, err := source.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() || before.Size() != artifact.SizeBytes {
		return nil, nil, fmt.Errorf("stem artifact size or file type changed")
	}
	pathBefore, err := os.Lstat(sourcePath)
	if err != nil || pathBefore.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, pathBefore) {
		return nil, nil, fmt.Errorf("stem artifact path changed before snapshot")
	}
	temp, err := os.CreateTemp("", "viib-stem-preview-*")
	if err != nil {
		return nil, nil, err
	}
	ok := false
	defer func() {
		if !ok {
			name := temp.Name()
			_ = temp.Close()
			_ = os.Remove(name)
		}
	}()
	if _, err = source.Seek(0, io.SeekStart); err != nil {
		return nil, nil, err
	}
	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, h), stemContextReader{ctx: ctx, reader: source})
	if err != nil {
		return nil, nil, err
	}
	after, err := source.Stat()
	if err != nil {
		return nil, nil, err
	}
	pathAfter, pathErr := os.Lstat(sourcePath)
	if pathErr != nil || pathAfter.Mode()&os.ModeSymlink != 0 || !os.SameFile(after, pathAfter) || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, nil, fmt.Errorf("stem artifact changed while taking snapshot")
	}
	if written != artifact.SizeBytes || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), artifact.SHA256) {
		return nil, nil, fmt.Errorf("stem artifact checksum mismatch")
	}
	if _, err = temp.Seek(0, io.SeekStart); err != nil {
		return nil, nil, err
	}
	info, err := temp.Stat()
	if err != nil {
		return nil, nil, err
	}
	ok = true
	return temp, info, nil
}

func isAllowedStemName(name stems.StemName) bool {
	switch name {
	case stems.StemVocals, stems.StemDrums, stems.StemBass, stems.StemGuitar, stems.StemPiano, stems.StemOther:
		return true
	default:
		return false
	}
}
