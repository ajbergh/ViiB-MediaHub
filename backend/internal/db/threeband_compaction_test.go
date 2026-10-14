package db

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"path/filepath"
	"testing"
)

func TestThreeBandCompactionPreservesIdentityAndLegacyData(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	acc, _ := threeband.New(48000)
	if err := acc.Feed([]float32{0, .25, -.5}); err != nil {
		t.Fatal(err)
	}
	overview, _ := acc.Result()
	legacy, _ := json.Marshal(overview)
	for _, id := range []string{"a", "b", "corrupt"} {
		if err := d.SaveSong(&Song{ID: id, Title: id, FilePath: id + ".wav"}); err != nil {
			t.Fatal(err)
		}
		data := legacy
		if id == "corrupt" {
			data = []byte("broken")
		}
		if err := d.UpsertTrackAnalysisArtifact(TrackAnalysisArtifact{ID: id, SongID: id, Kind: threeband.Kind, FormatVersion: threeband.FormatVersion, AlgorithmVersion: threeband.AlgorithmVersion, Encoding: threeband.LegacyEncoding, Provenance: "measured", SourceFingerprint: "fp", CreatedAt: 123, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.CompactLegacyThreeBandArtifacts(canceled, 128); err == nil {
		t.Fatal("cancellation ignored")
	}
	if n, err := d.CompactLegacyThreeBandArtifacts(t.Context(), 1); err != nil || n != 1 {
		t.Fatalf("bounded compaction: %d %v", n, err)
	}
	b, err := d.GetTrackAnalysisArtifact("b", threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion)
	if err != nil || !bytes.Equal(b.Data, legacy) {
		t.Fatal("batch limit ignored")
	}
	if n, err := d.CompactLegacyThreeBandArtifacts(t.Context(), 128); err != nil || n != 1 {
		t.Fatalf("remaining compaction: %d %v", n, err)
	}
	for _, id := range []string{"a", "b"} {
		got, err := d.GetTrackAnalysisArtifact(id, threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion)
		if err != nil || got.Encoding != threeband.Encoding || got.SourceFingerprint != "fp" || got.Provenance != "measured" || got.CreatedAt != 123 || got.ID != id || len(got.Data) >= len(legacy) {
			t.Fatalf("metadata changed: %+v %v", got, err)
		}
		decoded, err := threeband.Decode(got.Data)
		if err != nil || decoded.Frames != overview.Frames {
			t.Fatal("invalid converted data", err)
		}
	}
	bad, _ := d.GetTrackAnalysisArtifact("corrupt", threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion)
	if bad.Encoding != threeband.LegacyEncoding || string(bad.Data) != "broken" {
		t.Fatal("corrupt record overwritten")
	}
	if n, err := d.CompactLegacyThreeBandArtifacts(t.Context(), 128); err != nil || n != 0 {
		t.Fatal("compaction not idempotent", n, err)
	}
}
