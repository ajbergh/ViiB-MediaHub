package stems

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func codecPackages(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "fixtures", "stems", "package-v1-*.viibstems"))
	if err != nil || len(paths) != 8 {
		t.Fatalf("missing shared codec fixtures: %d %v", len(paths), err)
	}
	return paths
}

func TestCompressedStemLabInteroperability(t *testing.T) {
	for _, path := range codecPackages(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			validation, err := ValidatePlayablePackage(path)
			if err != nil {
				t.Fatal(err)
			}
			a := validation.Manifest.Stems[StemVocals]
			if a.Frames != 44117 {
				t.Fatalf("gapless frames=%d", a.Frames)
			}
			full, err := ReadPCMFrames(validation.Files[StemVocals], a, 0, a.Frames)
			if err != nil {
				t.Fatal(err)
			}
			if len(full) != int(a.Frames)*a.Channels {
				t.Fatal("wrong decoded length")
			}
			for _, peak := range []int{512, 11025, 40000} {
				position := peak - 50
				amplitude := float64(0)
				for i := peak - 50; i < peak+50; i++ {
					if v := math.Abs(float64(full[i*a.Channels])); v > amplitude {
						amplitude = v
						position = i
					}
				}
				if position != peak || amplitude < .25 {
					t.Fatalf("impulse %d decoded at %d (amplitude %f)", peak, position, amplitude)
				}
			}
			// Compare random access with a full sequential decode, including the final samples.
			for _, start := range []int64{0, 400, 10970, 20000, 39950, a.Frames - 64} {
				samples, err := ReadPCMFrames(validation.Files[StemVocals], a, start, 64)
				if err != nil {
					t.Fatal(err)
				}
				for i, sample := range samples {
					if math.Abs(float64(sample-full[int(start)*a.Channels+i])) > .003 {
						t.Fatalf("seek drift at frame %d sample %d: %f vs %f", start, i, sample, full[int(start)*a.Channels+i])
					}
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := ReadPCMFramesContext(ctx, validation.Files[StemVocals], a, 0, 1); err != context.Canceled {
				t.Fatalf("cancel error=%v", err)
			}
		})
	}
}

func TestCompressedPackagesRejectInvalidArtifacts(t *testing.T) {
	for _, path := range codecPackages(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			valid, err := ValidatePackage(path)
			if err != nil {
				t.Fatal(err)
			}
			m := valid.Manifest
			m.RequiredFeatures = nil
			if err := ValidateManifest(m); err == nil {
				t.Fatal("missing capability accepted")
			}
			m.RequiredFeatures = []string{CompressedStemsFeature, "unknown"}
			if err := ValidateManifest(m); err == nil {
				t.Fatal("unknown capability accepted")
			}
			m = valid.Manifest
			a := m.Stems[StemVocals]
			a.Encoding = "pcm_s16le"
			m.Stems[StemVocals] = a
			if err := ValidateManifest(m); err == nil {
				t.Fatal("extension/codec mismatch accepted")
			}
			// Copy encoded bytes so corruption never modifies shared fixtures.
			for _, kind := range []string{"checksum", "truncated", "frames", "payload"} {
				t.Run(kind, func(t *testing.T) {
					v, err := ValidatePackage(path)
					if err != nil {
						t.Fatal(err)
					}
					m := v.Manifest
					dir := t.TempDir()
					for name, artifact := range m.Stems {
						b, err := os.ReadFile(v.Files[name])
						if err != nil {
							t.Fatal(err)
						}
						if name == StemVocals {
							switch kind {
							case "checksum":
								b[len(b)/2] ^= 1
							case "truncated":
								b = b[:len(b)/2]
							case "payload":
								for i := range b {
									b[i] = 0
								}
							case "frames":
								m.Audio.Frames--
								m.Source.Duration = float64(m.Audio.Frames) / float64(m.Audio.SampleRate)
							}
						}
						if kind == "truncated" || kind == "payload" {
							sum := sha256.Sum256(b)
							artifact.SizeBytes = int64(len(b))
							artifact.SHA256 = hex.EncodeToString(sum[:])
						}
						if kind == "frames" {
							artifact.Frames = m.Audio.Frames
						}
						m.Stems[name] = artifact
						if err = os.WriteFile(filepath.Join(dir, artifact.Path), b, 0600); err != nil {
							t.Fatal(err)
						}
					}
					// Set all declared geometries consistently to exercise decoder validation.
					if kind == "frames" {
						for name, a := range m.Stems {
							a.Frames = m.Audio.Frames
							m.Stems[name] = a
						}
					}
					b, _ := json.Marshal(m)
					if err = os.WriteFile(filepath.Join(dir, ManifestFilename), b, 0600); err != nil {
						t.Fatal(err)
					}
					if _, err = ValidatePackage(dir); err == nil {
						t.Fatalf("%s accepted", kind)
					}
				})
			}
		})
	}
}

func TestMP3RejectsMissingGaplessMetadata(t *testing.T) {
	for _, path := range codecPackages(t) {
		if !strings.Contains(path, "mp3") {
			continue
		}
		validation, err := ValidatePackage(path)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(validation.Files[StemVocals])
		if err != nil {
			t.Fatal(err)
		}
		// Removing the Xing/Info tag must not silently fall back to shifted decoding.
		s := strings.ReplaceAll(strings.ReplaceAll(string(b), "Xing", "xxxx"), "Info", "xxxx")
		file := filepath.Join(t.TempDir(), "stem.mp3")
		if err = os.WriteFile(file, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err = inspectAudio(context.Background(), file, "mp3"); err == nil {
			t.Fatal("missing gapless metadata accepted")
		}
	}
}
