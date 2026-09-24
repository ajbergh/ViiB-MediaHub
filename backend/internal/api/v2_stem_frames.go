package api

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/stems"
	"github.com/go-chi/chi/v5"
)

const maxStemFrameResponseBytes int64 = 4 << 20

var dj4StemOrder = []stems.StemName{stems.StemVocals, stems.StemDrums, stems.StemBass}
var sixStemOrder = []stems.StemName{stems.StemVocals, stems.StemDrums, stems.StemBass, stems.StemGuitar, stems.StemPiano, stems.StemOther}

func (a *API) getStemFramesV2(w http.ResponseWriter, r *http.Request) {
	songID, setID := chi.URLParam(r, "songID"), chi.URLParam(r, "stemSetID")
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
		stemError(w, r, http.StatusConflict, "stem_source_unavailable", "Stem playback requires a local source audio file", false)
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

	start, ok := parseFrameQuery(r, "startFrame")
	if !ok {
		stemError(w, r, http.StatusBadRequest, "invalid_start_frame", "startFrame must be a non-negative integer", false)
		return
	}
	count, ok := parseFrameQuery(r, "frameCount")
	if !ok || count == 0 {
		stemError(w, r, http.StatusBadRequest, "invalid_frame_count", "frameCount must be a positive integer", false)
		return
	}
	layout, format := r.URL.Query().Get("layout"), r.URL.Query().Get("format")
	if layout != "dj4" && layout != "six" {
		stemError(w, r, http.StatusBadRequest, "invalid_stem_layout", "layout must be dj4 or six", false)
		return
	}
	if format != "f32le" && format != "s16le" {
		stemError(w, r, http.StatusBadRequest, "invalid_stem_format", "format must be f32le or s16le", false)
		return
	}
	if start > set.Frames || count > set.Frames-start {
		stemError(w, r, http.StatusRequestedRangeNotSatisfiable, "frame_range_invalid", "Requested frame range exceeds the package", false)
		return
	}
	stemCount := int64(4)
	if layout == "six" {
		stemCount = 6
	}
	bytesPerSample := int64(2)
	if format == "f32le" {
		bytesPerSample = 4
	}
	channels := int64(set.Channels)
	if channels != 1 && channels != 2 {
		stemError(w, r, http.StatusConflict, "stem_geometry_invalid", "Stem package has unsupported channel geometry", false)
		return
	}
	if count > maxStemFrameResponseBytes/(stemCount*channels*bytesPerSample) {
		stemError(w, r, http.StatusRequestEntityTooLarge, "frame_request_too_large", "Requested frame block exceeds the 4 MiB response limit", false)
		return
	}
	if _, err = os.Stat(song.FilePath); err != nil {
		stemError(w, r, http.StatusConflict, "stem_source_unavailable", "The local source audio file is unavailable", false)
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
	for _, artifact := range validated.Manifest.Stems {
		if rejectSymlinkArtifact(root, artifact.Path) != nil {
			stemError(w, r, http.StatusConflict, "stem_package_unsafe", "Stem package contains an unsafe artifact path", false)
			return
		}
	}
	groups, order, err := requestedStemGroups(validated.Manifest, layout)
	if err != nil {
		stemError(w, r, http.StatusConflict, "stem_layout_unavailable", err.Error(), false)
		return
	}
	pcm := make(map[stems.StemName][]float32, len(validated.Manifest.Stems))
	for _, name := range order {
		artifact := validated.Manifest.Stems[name]
		samples, readErr := stems.ReadPCMFrames(validated.Files[name], artifact, start, count)
		if readErr != nil {
			stemError(w, r, http.StatusConflict, "stem_package_changed", "Stem artifact changed or could not be read", false)
			return
		}
		pcm[name] = samples
	}
	packed, err := packStemFrameResponse(groups, order, pcm, int(channels), format, count)
	if err != nil {
		stemError(w, r, http.StatusConflict, "stem_frames_unavailable", "Unable to pack aligned stem frames", false)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Start-Frame", strconv.FormatInt(start, 10))
	w.Header().Set("X-Frame-Count", strconv.FormatInt(count, 10))
	w.Header().Set("X-Sample-Rate", strconv.Itoa(set.SampleRate))
	w.Header().Set("X-Channel-Count", strconv.Itoa(len(groups)*int(channels)))
	w.Header().Set("X-Layout", layout)
	w.Header().Set("X-Format", format)
	orderNames := make([]string, len(groups))
	for i, n := range groups {
		orderNames[i] = n
	}
	w.Header().Set("X-Channel-Order", strings.Join(orderNames, ","))
	w.Header().Set("Content-Length", strconv.Itoa(len(packed)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(packed)
}

func parseFrameQuery(r *http.Request, key string) (int64, bool) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	return v, err == nil && v >= 0
}

func registeredPackageMatches(set db.StemSet, manifest stems.Manifest) bool {
	if set.Status != "ready" || !strings.EqualFold(set.SourceAudioHash, manifest.Source.SHA256) || set.Layout != string(manifest.StemLayout) || set.SampleRate != manifest.Audio.SampleRate || set.Channels != manifest.Audio.Channels || set.Frames != manifest.Audio.Frames || set.ManifestSchemaVersion != manifest.SchemaVersion {
		return false
	}
	registered := make(map[string]db.StemArtifact, len(set.Stems))
	for _, a := range set.Stems {
		registered[a.Name] = a
	}
	if len(registered) != len(manifest.Stems) {
		return false
	}
	for name, a := range manifest.Stems {
		saved, ok := registered[string(name)]
		if !ok || saved.RelativePath != a.Path || !strings.EqualFold(saved.SHA256, a.SHA256) || saved.SizeBytes != a.SizeBytes || saved.SampleRate != a.SampleRate || saved.Channels != a.Channels || saved.Frames != a.Frames || saved.Encoding != a.Encoding {
			return false
		}
	}
	return true
}

// rejectSymlinkPath rejects a symlink or reparse component anywhere in the
// registered absolute package path, including the package root itself.
func rejectSymlinkPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(abs)
	rest := strings.TrimPrefix(abs, volume)
	current := volume + string(os.PathSeparator)
	for _, part := range strings.FieldsFunc(rest, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path component")
		}
	}
	return nil
}
func rejectSymlinkArtifact(root, relative string) error {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, `\`) || strings.Contains(relative, ":") {
		return errors.New("unsafe artifact path")
	}
	current := root
	for _, part := range strings.Split(relative, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("unsafe artifact path")
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("artifact symlink is not allowed")
		}
	}
	return nil
}

// requestedStemGroups returns output bus names plus the contributing package
// stems. `dj4` on a six-stem package sums guitar, piano and other into music.
func requestedStemGroups(m stems.Manifest, layout string) ([]string, []stems.StemName, error) {
	if layout == "six" {
		if m.StemLayout != stems.LayoutSix {
			return nil, nil, errors.New("six layout requires a six-stem package")
		}
		names := append([]stems.StemName(nil), sixStemOrder...)
		return []string{"vocals", "drums", "bass", "guitar", "piano", "other"}, names, nil
	}
	if m.StemLayout == stems.LayoutFour {
		return []string{"vocals", "drums", "bass", "music"}, []stems.StemName{stems.StemVocals, stems.StemDrums, stems.StemBass, stems.StemOther}, nil
	}
	if m.StemLayout == stems.LayoutSix {
		return []string{"vocals", "drums", "bass", "music"}, append([]stems.StemName(nil), sixStemOrder...), nil
	}
	return nil, nil, errors.New("unsupported package stem layout")
}

func packStemFrameResponse(groups []string, order []stems.StemName, pcm map[stems.StemName][]float32, channels int, format string, frameCount int64) ([]byte, error) {
	if channels < 1 || channels > 2 {
		return nil, errors.New("unsupported channel count")
	}
	totalSamples := frameCount * int64(len(groups)) * int64(channels)
	bytesPer := int64(4)
	if format == "s16le" {
		bytesPer = 2
	}
	if totalSamples < 0 || totalSamples > int64(int(^uint(0)>>1))/bytesPer {
		return nil, errors.New("packed stem data is too large")
	}
	out := make([]byte, int(totalSamples*bytesPer))
	writeIndex := 0
	for frame := int64(0); frame < frameCount; frame++ {
		for groupIndex, group := range groups {
			var contributing []stems.StemName
			if group == "music" && len(order) == 6 {
				contributing = []stems.StemName{stems.StemGuitar, stems.StemPiano, stems.StemOther}
			} else {
				contributing = []stems.StemName{order[groupIndex]}
			}
			for channel := 0; channel < channels; channel++ {
				var sample float32
				for _, name := range contributing {
					data, ok := pcm[name]
					if !ok || int64(len(data)) <= frame*int64(channels)+int64(channel) {
						return nil, errors.New("stem frame range is incomplete")
					}
					sample += data[int(frame*int64(channels)+int64(channel))]
				}
				if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
					return nil, errors.New("packed stem sample is not finite")
				}
				if format == "f32le" {
					binary.LittleEndian.PutUint32(out[writeIndex:writeIndex+4], math.Float32bits(sample))
					writeIndex += 4
				} else if format == "s16le" {
					var value int16
					if sample <= -1 {
						value = -32768
					} else if sample >= 1 {
						value = 32767
					} else {
						value = int16(math.Round(float64(sample) * 32767))
					}
					binary.LittleEndian.PutUint16(out[writeIndex:writeIndex+2], uint16(value))
					writeIndex += 2
				} else {
					return nil, errors.New("unsupported output format")
				}
			}
		}
	}
	return out, nil
}
