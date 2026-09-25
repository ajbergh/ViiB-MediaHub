// Package stems defines and validates the ViiB Stem Package v1 interchange format.
// It intentionally contains no stem-generation runtime.
package stems

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	ManifestFilename = "manifest.json"
	MaxManifestBytes = 1 << 20
)

type Layout string

const (
	LayoutFour Layout = "four"
	LayoutSix  Layout = "six"
)

type StemName string

const (
	StemVocals StemName = "vocals"
	StemDrums  StemName = "drums"
	StemBass   StemName = "bass"
	StemGuitar StemName = "guitar"
	StemPiano  StemName = "piano"
	StemOther  StemName = "other"
)

type Manifest struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	RequiredFeatures []string              `json:"requiredFeatures,omitempty"`
	Source           Source                `json:"source"`
	StemLayout       Layout                `json:"stemLayout"`
	Generator        Provenance            `json:"generator"`
	Model            Provenance            `json:"model"`
	Audio            AudioGeometry         `json:"audio"`
	Timing           Timing                `json:"timing"`
	Stems            map[StemName]Artifact `json:"stems"`
}

type Source struct {
	Filename    string  `json:"filename"`
	SHA256      string  `json:"sha256"`
	AudioSHA256 string  `json:"audioSha256,omitempty"`
	Duration    float64 `json:"duration"`
}

type Provenance struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type AudioGeometry struct {
	SampleRate int   `json:"sampleRate"`
	Channels   int   `json:"channels"`
	Frames     int64 `json:"frames"`
}

// Timing records source-decoder compensation applied by the package producer.
// Both values are non-negative counts in source/output sample frames.
type Timing struct {
	DecoderDelayFrames int64 `json:"decoderDelayFrames"`
	StartTrimFrames    int64 `json:"startTrimFrames"`
}

type Artifact struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"sizeBytes"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
	Frames     int64  `json:"frames"`
	Encoding   string `json:"encoding"`
}

type Validation struct {
	Manifest Manifest
	Files    map[StemName]string // absolute, validated file paths
}

// ParseManifest reads a bounded JSON manifest. Unknown optional fields are
// ignored for forward-compatible v1 extensions; RequiredFeatures lets a writer
// explicitly declare capabilities that an older reader must reject.
func ParseManifest(r io.Reader) (Manifest, error) {
	var m Manifest
	data, err := io.ReadAll(io.LimitReader(r, MaxManifestBytes+1))
	if err != nil {
		return m, fmt.Errorf("read stem manifest: %w", err)
	}
	if len(data) > MaxManifestBytes {
		return m, fmt.Errorf("stem manifest exceeds %d bytes", MaxManifestBytes)
	}
	if err := requireManifestFields(data); err != nil {
		return m, err
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("decode stem manifest: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return m, errors.New("stem manifest contains multiple JSON values")
		}
		return m, fmt.Errorf("decode trailing stem manifest data: %w", err)
	}
	return m, nil
}

func requireManifestFields(data []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("decode stem manifest: %w", err)
	}
	if top == nil {
		return errors.New("stem manifest must be a JSON object")
	}
	if err := requireFields("manifest", top, "schemaVersion", "source", "stemLayout", "generator", "model", "audio", "timing", "stems"); err != nil {
		return err
	}
	var source, generator, model, audio, timing map[string]json.RawMessage
	for key, target := range map[string]*map[string]json.RawMessage{"source": &source, "generator": &generator, "model": &model, "audio": &audio, "timing": &timing} {
		if err := json.Unmarshal(top[key], target); err != nil {
			return fmt.Errorf("manifest.%s must be an object", key)
		}
	}
	if err := requireFields("manifest.source", source, "filename", "sha256", "duration"); err != nil {
		return err
	}
	if err := requireFields("manifest.generator", generator, "name", "version"); err != nil {
		return err
	}
	if err := requireFields("manifest.model", model, "name", "version"); err != nil {
		return err
	}
	if err := requireFields("manifest.audio", audio, "sampleRate", "channels", "frames"); err != nil {
		return err
	}
	if err := requireFields("manifest.timing", timing, "decoderDelayFrames", "startTrimFrames"); err != nil {
		return err
	}
	var stems map[string]json.RawMessage
	if err := json.Unmarshal(top["stems"], &stems); err != nil || stems == nil {
		return errors.New("manifest.stems must be an object")
	}
	for name, raw := range stems {
		var artifact map[string]json.RawMessage
		if err := json.Unmarshal(raw, &artifact); err != nil {
			return fmt.Errorf("manifest.stems.%s must be an object", name)
		}
		if err := requireFields("manifest.stems."+name, artifact, "path", "sha256", "sizeBytes", "sampleRate", "channels", "frames", "encoding"); err != nil {
			return err
		}
	}
	return nil
}

func requireFields(object string, values map[string]json.RawMessage, fields ...string) error {
	for _, field := range fields {
		if _, ok := values[field]; !ok {
			return fmt.Errorf("%s.%s is required", object, field)
		}
	}
	return nil
}

// ValidatePackage validates the manifest, contained paths, file hashes/sizes,
// and WAV PCM geometry. It follows symlinks only when their resolved target is
// still inside the package directory; callers should retain the returned paths
// and avoid re-resolving untrusted manifest values when serving audio.
func ValidatePackage(packageDir string) (Validation, error) {
	return ValidatePackageContext(context.Background(), packageDir)
}

// ValidatePackageContext validates a package and checks cancellation between
// artifact reads. Existing callers can use ValidatePackage when cancellation
// is not needed.
func ValidatePackageContext(ctx context.Context, packageDir string) (Validation, error) {
	var result Validation
	if err := ctx.Err(); err != nil {
		return result, err
	}
	root, err := filepath.Abs(packageDir)
	if err != nil {
		return result, fmt.Errorf("resolve package directory: %w", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return result, fmt.Errorf("stat package directory: %w", err)
	}
	if !rootInfo.IsDir() {
		return result, errors.New("package path is not a directory")
	}
	manifestPath := filepath.Join(root, ManifestFilename)
	f, err := os.Open(manifestPath)
	if err != nil {
		return result, fmt.Errorf("open stem manifest: %w", err)
	}
	m, parseErr := ParseManifest(f)
	closeErr := f.Close()
	if parseErr != nil {
		return result, parseErr
	}
	if closeErr != nil {
		return result, fmt.Errorf("close stem manifest: %w", closeErr)
	}
	if err := ValidateManifest(m); err != nil {
		return result, err
	}
	result = Validation{Manifest: m, Files: make(map[StemName]string, len(m.Stems))}
	for name, artifact := range m.Stems {
		if err := ctx.Err(); err != nil {
			return Validation{}, err
		}
		full, err := resolveContained(root, artifact.Path)
		if err != nil {
			return Validation{}, fmt.Errorf("stem %q path: %w", name, err)
		}
		info, err := os.Stat(full)
		if err != nil {
			return Validation{}, fmt.Errorf("stem %q stat: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return Validation{}, fmt.Errorf("stem %q is not a regular file", name)
		}
		if info.Size() != artifact.SizeBytes {
			return Validation{}, fmt.Errorf("stem %q size mismatch: manifest %d, file %d", name, artifact.SizeBytes, info.Size())
		}
		gotHash, err := fileSHA256Context(ctx, full)
		if err != nil {
			return Validation{}, fmt.Errorf("stem %q checksum: %w", name, err)
		}
		if !strings.EqualFold(gotHash, artifact.SHA256) {
			return Validation{}, fmt.Errorf("stem %q checksum mismatch", name)
		}
		geometry, encoding, err := inspectWAV(full)
		if err != nil {
			return Validation{}, fmt.Errorf("stem %q WAV validation: %w", name, err)
		}
		if encoding != artifact.Encoding || geometry.SampleRate != artifact.SampleRate || geometry.Channels != artifact.Channels || geometry.Frames != artifact.Frames {
			return Validation{}, fmt.Errorf("stem %q WAV geometry does not match manifest", name)
		}
		if geometry != m.Audio {
			return Validation{}, fmt.Errorf("stem %q geometry differs from package audio geometry", name)
		}
		result.Files[name] = full
	}
	return result, nil
}

// ValidatePlayablePackage applies the same strict no-symlink policy as stem
// playback before returning package metadata suitable for registry readiness.
func ValidatePlayablePackage(packageDir string) (Validation, error) {
	return ValidatePlayablePackageContext(context.Background(), packageDir)
}

// ValidatePlayablePackageContext validates a package under the strict path
// policy used by playback and remains cancellable during artifact hashing.
func ValidatePlayablePackageContext(ctx context.Context, packageDir string) (Validation, error) {
	root, err := filepath.Abs(packageDir)
	if err != nil {
		return Validation{}, err
	}
	if err = RejectSymlinkPath(root); err != nil {
		return Validation{}, fmt.Errorf("unsafe package path: %w", err)
	}
	validated, err := ValidatePackageContext(ctx, root)
	if err != nil {
		return Validation{}, err
	}
	for _, artifact := range validated.Manifest.Stems {
		if err = ctx.Err(); err != nil {
			return Validation{}, err
		}
		if err = RejectSymlinkArtifact(root, artifact.Path); err != nil {
			return Validation{}, fmt.Errorf("unsafe stem artifact path: %w", err)
		}
	}
	return validated, nil
}

func ValidateManifest(m Manifest) error {
	if m.SchemaVersion != 1 {
		return fmt.Errorf("unsupported stem package schemaVersion %d", m.SchemaVersion)
	}
	for _, feature := range m.RequiredFeatures {
		return fmt.Errorf("unsupported required stem package feature %q", feature)
	}
	if m.Source.Filename == "" || strings.ContainsAny(m.Source.Filename, "\\/\x00") || filepath.Base(m.Source.Filename) != m.Source.Filename {
		return errors.New("source.filename must be a plain filename")
	}
	if !validSHA256(m.Source.SHA256) {
		return errors.New("source.sha256 must be a 64-character hexadecimal SHA-256")
	}
	if m.Source.AudioSHA256 != "" && !validSHA256(m.Source.AudioSHA256) {
		return errors.New("source.audioSha256 must be a 64-character hexadecimal SHA-256")
	}
	if !finitePositive(m.Source.Duration) || m.Audio.SampleRate <= 0 || m.Audio.Channels != 1 && m.Audio.Channels != 2 || m.Audio.Frames <= 0 {
		return errors.New("source duration and audio geometry must be positive (one or two channels)")
	}
	if m.Generator.Name == "" || m.Generator.Version == "" || m.Model.Name == "" || m.Model.Version == "" {
		return errors.New("generator and model name/version are required")
	}
	if m.Timing.DecoderDelayFrames < 0 || m.Timing.StartTrimFrames < 0 {
		return errors.New("timing compensation frame counts cannot be negative")
	}
	if math.Abs(float64(m.Audio.Frames)/float64(m.Audio.SampleRate)-m.Source.Duration) > 1.0/float64(m.Audio.SampleRate)+0.001 {
		return errors.New("source duration does not match audio frame count")
	}
	required := expectedStems(m.StemLayout)
	if required == nil {
		return fmt.Errorf("unsupported stemLayout %q", m.StemLayout)
	}
	if len(m.Stems) != len(required) {
		return fmt.Errorf("stemLayout %q requires exactly %d stems", m.StemLayout, len(required))
	}
	for _, name := range required {
		a, ok := m.Stems[name]
		if !ok {
			return fmt.Errorf("required stem %q is missing", name)
		}
		if err := validateArtifact(name, a, m.Audio); err != nil {
			return err
		}
	}
	for name := range m.Stems {
		if !containsStem(required, name) {
			return fmt.Errorf("stem %q is not part of layout %q", name, m.StemLayout)
		}
	}
	return nil
}

func validateArtifact(name StemName, a Artifact, audio AudioGeometry) error {
	if a.Path == "" || strings.ContainsRune(a.Path, '\x00') || strings.Contains(a.Path, ":") || filepath.IsAbs(a.Path) || filepath.VolumeName(a.Path) != "" || strings.Contains(a.Path, `\`) {
		return fmt.Errorf("stem %q path must be a relative slash-separated path", name)
	}
	for _, component := range strings.Split(a.Path, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("stem %q path contains an unsafe component", name)
		}
	}
	if ext := strings.ToLower(filepath.Ext(a.Path)); ext != ".wav" && ext != ".wave" {
		return fmt.Errorf("stem %q uses unsupported encoding extension %q; v1 currently accepts WAV only", name, ext)
	}
	if !validSHA256(a.SHA256) || a.SizeBytes <= 0 {
		return fmt.Errorf("stem %q requires a SHA-256 checksum and positive sizeBytes", name)
	}
	if a.Encoding != "pcm_s16le" && a.Encoding != "float32le" {
		return fmt.Errorf("stem %q encoding %q is not supported; use pcm_s16le or float32le WAV", name, a.Encoding)
	}
	if a.SampleRate != audio.SampleRate || a.Channels != audio.Channels || a.Frames != audio.Frames {
		return fmt.Errorf("stem %q declared geometry differs from package audio geometry", name)
	}
	return nil
}

func resolveContained(root, relative string) (string, error) {
	parts := strings.Split(filepath.FromSlash(relative), string(filepath.Separator))
	current := root
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(root, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				return "", errors.New("resolved path escapes package directory")
			}
			current = resolved
		}
	}
	return current, nil
}

func fileSHA256(path string) (string, error) {
	return fileSHA256Context(context.Background(), path)
}

func fileSHA256Context(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := copyContext(ctx, h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 64*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

func inspectWAV(path string) (AudioGeometry, string, error) {
	var geometry AudioGeometry
	f, err := os.Open(path)
	if err != nil {
		return geometry, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return geometry, "", err
	}
	if info.Size() < 12 {
		return geometry, "", errors.New("file is shorter than a RIFF/WAVE header")
	}
	var riff [12]byte
	if _, err := io.ReadFull(f, riff[:]); err != nil {
		return geometry, "", err
	}
	if string(riff[:4]) != "RIFF" || string(riff[8:]) != "WAVE" {
		return geometry, "", errors.New("not a RIFF/WAVE file")
	}
	riFFEnd := int64(binary.LittleEndian.Uint32(riff[4:8])) + 8
	if riFFEnd > info.Size() || riFFEnd < 12 {
		return geometry, "", errors.New("invalid RIFF size")
	}
	var haveFormat, haveData bool
	var format, bits, blockAlign uint16
	var dataSize uint32
	for {
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return geometry, "", err
		}
		if pos == riFFEnd {
			break
		}
		if pos+8 > riFFEnd {
			return geometry, "", errors.New("truncated WAV chunk header")
		}
		var chunk [8]byte
		if _, err := io.ReadFull(f, chunk[:]); err != nil {
			return geometry, "", err
		}
		size := binary.LittleEndian.Uint32(chunk[4:])
		chunkEnd := pos + 8 + int64(size) + int64(size%2)
		if chunkEnd > riFFEnd {
			return geometry, "", errors.New("WAV chunk exceeds RIFF boundary")
		}
		switch string(chunk[:4]) {
		case "fmt ":
			if size < 16 {
				return geometry, "", errors.New("WAV fmt chunk is shorter than 16 bytes")
			}
			var b [16]byte
			if _, err := io.ReadFull(f, b[:]); err != nil {
				return geometry, "", err
			}
			format = binary.LittleEndian.Uint16(b[0:2])
			geometry.Channels = int(binary.LittleEndian.Uint16(b[2:4]))
			geometry.SampleRate = int(binary.LittleEndian.Uint32(b[4:8]))
			blockAlign = binary.LittleEndian.Uint16(b[12:14])
			bits = binary.LittleEndian.Uint16(b[14:16])
			haveFormat = true
		case "data":
			if haveData {
				return geometry, "", errors.New("multiple WAV data chunks are unsupported")
			}
			dataSize, haveData = size, true
		}
		if _, err := f.Seek(chunkEnd, io.SeekStart); err != nil {
			return geometry, "", err
		}
	}
	if !haveFormat || !haveData {
		return geometry, "", errors.New("WAV requires fmt and data chunks")
	}
	if geometry.SampleRate <= 0 || geometry.Channels != 1 && geometry.Channels != 2 {
		return geometry, "", errors.New("invalid WAV sample rate or channel count")
	}
	bytesPerSample := uint16(0)
	encoding := ""
	switch {
	case format == 1 && bits == 16:
		bytesPerSample, encoding = 2, "pcm_s16le"
	case format == 3 && bits == 32:
		bytesPerSample, encoding = 4, "float32le"
	default:
		return geometry, "", fmt.Errorf("unsupported WAV format %d/%d-bit; v1 allows PCM16 and float32", format, bits)
	}
	expectedAlign := uint16(geometry.Channels) * bytesPerSample
	if blockAlign != expectedAlign || dataSize == 0 || dataSize%uint32(blockAlign) != 0 {
		return geometry, "", errors.New("WAV data is empty or not frame aligned")
	}
	geometry.Frames = int64(dataSize / uint32(blockAlign))
	return geometry, encoding, nil
}

func validSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func finitePositive(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }

func expectedStems(layout Layout) []StemName {
	switch layout {
	case LayoutFour:
		return []StemName{StemVocals, StemDrums, StemBass, StemOther}
	case LayoutSix:
		return []StemName{StemVocals, StemDrums, StemBass, StemGuitar, StemPiano, StemOther}
	default:
		return nil
	}
}

func containsStem(names []StemName, target StemName) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}
