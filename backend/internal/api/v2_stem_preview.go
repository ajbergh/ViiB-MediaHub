package api

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/stems"
	"github.com/go-chi/chi/v5"
)

// getStemPreviewV2 streams one allow-listed WAV artifact from the registered
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
	validated, err := validatedStemPackage(root)
	if err != nil {
		stemError(w, r, http.StatusConflict, "stem_package_changed", "Stem package failed current validation", false)
		return
	}
	if !registeredPackageMatches(*set, validated.Manifest) {
		stemError(w, r, http.StatusConflict, "stem_package_changed", "Stem package identity differs from the registered package", false)
		return
	}
	if !stemSourceMatches(song, validated.Manifest) {
		stemError(w, r, http.StatusConflict, "stem_source_stale", "Stem package no longer matches the local source audio", false)
		return
	}
	artifact, ok := validated.Manifest.Stems[name]
	if !ok {
		stemError(w, r, http.StatusNotFound, "stem_artifact_not_found", "Stem artifact is not part of this package", false)
		return
	}
	if !strings.EqualFold(filepath.Ext(artifact.Path), ".wav") && !strings.EqualFold(filepath.Ext(artifact.Path), ".wave") {
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
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Stem-Name", string(name))
	w.Header().Set("X-Sample-Rate", strconv.Itoa(artifact.SampleRate))
	w.Header().Set("X-Channel-Count", strconv.Itoa(artifact.Channels))
	w.Header().Set("X-Frame-Count", strconv.FormatInt(artifact.Frames, 10))
	http.ServeContent(w, r, "stem.wav", fileInfo.ModTime(), f)
}

func isAllowedStemName(name stems.StemName) bool {
	switch name {
	case stems.StemVocals, stems.StemDrums, stems.StemBass, stems.StemGuitar, stems.StemPiano, stems.StemOther:
		return true
	default:
		return false
	}
}
