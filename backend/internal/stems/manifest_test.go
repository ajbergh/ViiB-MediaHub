package stems

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cancelOnRead struct {
	cancel context.CancelFunc
	done   bool
}

func (r *cancelOnRead) Read(buffer []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	copy(buffer, []byte("chunk"))
	r.cancel()
	return len("chunk"), nil
}

func TestCopyContextStopsAfterCancellationDuringHashRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelOnRead{cancel: cancel}
	if _, err := copyContext(ctx, io.Discard, reader); err != context.Canceled {
		t.Fatalf("context-aware file copy error=%v, want context canceled", err)
	}
}

func TestValidatePackageDeterministicWAVFixture(t *testing.T) {
	dir := t.TempDir()
	manifest := fixtureManifest(t, dir, LayoutSix)
	first, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(fixtureManifest(t, dir, LayoutSix))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("fixture manifest is not deterministic")
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), data, 0o600); err != nil {
		t.Fatal(err)
	}
	validated, err := ValidatePackage(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(validated.Files) != 6 || validated.Manifest.StemLayout != LayoutSix {
		t.Fatalf("unexpected result: %+v", validated)
	}
}

func TestValidatePlayablePackageRejectsSymlinkedManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := fixtureManifest(t, dir, LayoutFour)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, ManifestFilename)
	targetPath := filepath.Join(dir, "manifest-target.json")
	if err := os.WriteFile(targetPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetPath, manifestPath); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := ValidatePlayablePackageContext(context.Background(), dir); err == nil {
		t.Fatal("expected symlinked manifest to be rejected")
	}
}

func TestValidatePackageRejectsChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	manifest := fixtureManifest(t, dir, LayoutFour)
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifest.Stems[StemVocals].Path), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePackage(dir); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("expected size mismatch, got %v", err)
	}
}

func TestValidateManifestRejectsUnknownVersionAndMissingStem(t *testing.T) {
	m := baseManifest(LayoutFour)
	m.SchemaVersion = 2
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected unknown schema to be rejected")
	}
	m = baseManifest(LayoutFour)
	delete(m.Stems, StemOther)
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected missing canonical stem to be rejected")
	}
}

func TestValidateManifestRejectsUnsafeAndNonWAVPaths(t *testing.T) {
	m := baseManifest(LayoutFour)
	a := m.Stems[StemVocals]
	a.Path = "../outside.wav"
	m.Stems[StemVocals] = a
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
	a.Path = "vocals.flac"
	m.Stems[StemVocals] = a
	if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "WAV only") {
		t.Fatalf("expected FLAC to be rejected, got %v", err)
	}
	a.Path = `C:drive-relative.wav`
	m.Stems[StemVocals] = a
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected drive-relative path to be rejected")
	}
}

func TestValidateManifestRejectsUnsupportedRequiredFeature(t *testing.T) {
	m := baseManifest(LayoutFour)
	m.RequiredFeatures = []string{"flac-stems"}
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected unsupported required feature to be rejected")
	}
}

func TestParseManifestLimit(t *testing.T) {
	if _, err := ParseManifest(strings.NewReader(strings.Repeat(" ", MaxManifestBytes+1))); err == nil {
		t.Fatal("expected oversized manifest to be rejected")
	}
}

func TestParseManifestRequiresTimingAndArtifactFields(t *testing.T) {
	dir := t.TempDir()
	m := fixtureManifest(t, dir, LayoutFour)
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "timing")
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseManifest(bytes.NewReader(data)); err == nil || !strings.Contains(err.Error(), "timing is required") {
		t.Fatalf("expected missing timing error, got %v", err)
	}
}

func fixtureManifest(t *testing.T, dir string, layout Layout) Manifest {
	t.Helper()
	m := baseManifest(layout)
	for _, name := range expectedStems(layout) {
		path := string(name) + ".wav"
		content := deterministicWAV()
		if err := os.WriteFile(filepath.Join(dir, path), content, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		m.Stems[name] = Artifact{Path: path, SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content)), SampleRate: 8000, Channels: 1, Frames: 4, Encoding: "pcm_s16le"}
	}
	return m
}

func baseManifest(layout Layout) Manifest {
	m := Manifest{
		SchemaVersion: 1,
		Source:        Source{Filename: "source.wav", SHA256: strings.Repeat("a", 64), Duration: 0.0005},
		StemLayout:    layout,
		Generator:     Provenance{Name: "Fixture", Version: "1"},
		Model:         Provenance{Name: "Deterministic", Version: "1"},
		Audio:         AudioGeometry{SampleRate: 8000, Channels: 1, Frames: 4},
		Stems:         map[StemName]Artifact{},
	}
	for _, name := range expectedStems(layout) {
		m.Stems[name] = Artifact{Path: string(name) + ".wav", SHA256: strings.Repeat("c", 64), SizeBytes: 1, SampleRate: 8000, Channels: 1, Frames: 4, Encoding: "pcm_s16le"}
	}
	return m
}

func deterministicWAV() []byte {
	// Four fixed signed-PCM samples; RIFF and chunk lengths are fully specified.
	const samples = 8
	dataSize := samples
	b := make([]byte, 44+dataSize)
	copy(b[0:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(b)-8))
	copy(b[8:12], "WAVE")
	copy(b[12:16], "fmt ")
	binary.LittleEndian.PutUint32(b[16:20], 16)
	binary.LittleEndian.PutUint16(b[20:22], 1)
	binary.LittleEndian.PutUint16(b[22:24], 1)
	binary.LittleEndian.PutUint32(b[24:28], 8000)
	binary.LittleEndian.PutUint32(b[28:32], 16000)
	binary.LittleEndian.PutUint16(b[32:34], 2)
	binary.LittleEndian.PutUint16(b[34:36], 16)
	copy(b[36:40], "data")
	binary.LittleEndian.PutUint32(b[40:44], uint32(dataSize))
	for i := 0; i < samples/2; i++ {
		binary.LittleEndian.PutUint16(b[44+i*2:], uint16(int16(i*500-1000)))
	}
	return b
}

func TestParseManifestExplainsStemLab010Manifests(t *testing.T) {
	legacy := `{"schemaVersion":1,"source":{"filename":"a.ogg","sha256":"` + strings.Repeat("a", 64) + `","sizeBytes":1},
		"generator":{"name":"ViiB-StemLab","version":"0.1.0"},"model":{"engine":"demucs","name":"htdemucs_6s","version":"4.0.1","device":"cuda"},
		"audio":{"codec":"wav","sampleRate":44100,"channels":2,"frames":10,"durationSeconds":0.0002},
		"stems":{"vocals":{"file":"vocals.wav","sha256":"` + strings.Repeat("b", 64) + `","sizeBytes":84,"frames":10}}}`
	_, err := ParseManifest(strings.NewReader(legacy))
	if err == nil || !strings.Contains(err.Error(), "viib-stemlab package upgrade") {
		t.Fatalf("legacy StemLab manifest error = %v, want upgrade guidance", err)
	}
}
