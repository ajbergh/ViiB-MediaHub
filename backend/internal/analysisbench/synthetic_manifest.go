package analysisbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SyntheticCorpusManifest produces a label-only manifest for generated WAV
// fixtures. It deliberately excludes fixtures with no comparable BPM/key/
// unknown label (for example a tempo ramp or a bare tuning tone), and labels
// every included entry as synthetic so this transport aid cannot be mistaken
// for the lawful real-audio Phase 0 corpus.
func SyntheticCorpusManifest(fixtures []PCMFixture, audioDirectory string) (CorpusManifest, error) {
	if audioDirectory == "" {
		return CorpusManifest{}, fmt.Errorf("synthetic audio directory is required")
	}
	manifest := CorpusManifest{Version: "phase0-synthetic-v1", EvidenceClass: EvidenceSyntheticCI}
	for _, fixture := range fixtures {
		if fixture.Name == "" {
			return CorpusManifest{}, fmt.Errorf("synthetic fixture name is required")
		}
		if fixture.Expected.BPM == nil && fixture.Expected.Key == "" && !fixture.Expected.IsUnknown {
			continue
		}
		track := CorpusTrack{
			ID:          fixture.Name,
			Path:        filepath.Join(audioDirectory, fixture.Name+".wav"),
			License:     "generated",
			LabelSource: "deterministic fixture generator",
			Genre:       "synthetic",
			Notes:       "Generated regression fixture; not Phase 0 real-audio corpus evidence.",
		}
		if len(manifest.Tracks)%3 == 0 {
			track.Split = SplitHeldOut
		} else {
			track.Split = SplitTuning
		}
		if fixture.Expected.IsUnknown {
			track.ExpectedUnknown = true
		} else {
			track.ExpectedBPM = fixture.Expected.BPM
			track.ExpectedKey = fixture.Expected.Key
			if fixture.Expected.BPM != nil {
				track.AcceptedMetricBPM = []float64{*fixture.Expected.BPM}
			}
		}
		manifest.Tracks = append(manifest.Tracks, track)
	}
	if err := manifest.Validate(); err != nil {
		return CorpusManifest{}, fmt.Errorf("build synthetic corpus manifest: %w", err)
	}
	return manifest, nil
}

// WriteSyntheticCorpusManifest writes a generated-fixture manifest once. Like
// WriteFixturesWAV, it refuses to overwrite an existing artifact so an
// exported browser baseline stays auditable.
func WriteSyntheticCorpusManifest(path, audioDirectory string, fixtures []PCMFixture) (CorpusManifest, error) {
	if path == "" {
		return CorpusManifest{}, fmt.Errorf("synthetic manifest path is required")
	}
	manifest, err := SyntheticCorpusManifest(fixtures, audioDirectory)
	if err != nil {
		return CorpusManifest{}, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return CorpusManifest{}, fmt.Errorf("create synthetic manifest %q: %w", path, err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(manifest)
	closeErr := file.Close()
	if writeErr != nil {
		return CorpusManifest{}, fmt.Errorf("write synthetic manifest %q: %w", path, writeErr)
	}
	if closeErr != nil {
		return CorpusManifest{}, fmt.Errorf("close synthetic manifest %q: %w", path, closeErr)
	}
	return manifest, nil
}
