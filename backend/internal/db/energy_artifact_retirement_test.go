package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis/featurecontract"
)

func TestEnergyArtifactRetirementIsSourceBoundAndAtomic(t *testing.T) {
	for _, oldSource := range []string{"bytes", "other-bytes"} {
		t.Run(oldSource, func(t *testing.T) {
			d, err := New(filepath.Join(t.TempDir(), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.SaveSong(&Song{ID: "song", Title: "Song", FilePath: "song.wav", AddedAt: 1}); err != nil {
				t.Fatal(err)
			}
			old := TrackAnalysisArtifact{ID: "old", SongID: "song", Kind: featurecontract.ArtifactKind, FormatVersion: featurecontract.FormatVersion, AlgorithmVersion: "energy-structure-v1", Encoding: featurecontract.Encoding, Provenance: "measured", SourceFingerprint: oldSource, Data: []byte("old")}
			if err := d.UpsertTrackAnalysisArtifact(old); err != nil {
				t.Fatal(err)
			}
			current := old
			current.ID = "new"
			current.AlgorithmVersion = featurecontract.AlgorithmVersion
			current.SourceFingerprint = "bytes"
			current.Data = []byte("new")
			p := TrackPreparationPublication{RetireLegacyEnergy: true, Analysis: TrackAnalysis{SongID: "song", Status: TrackAnalysisPartial, AnalysisVersion: 1, AlgorithmVersion: "test", SourceFingerprint: "bytes"}, Artifacts: []TrackAnalysisArtifact{current}, Capabilities: []TrackCapabilityStatus{{SongID: "song", SourceFingerprint: "bytes", Capability: "local_features", Version: featurecontract.AlgorithmVersion, State: "available"}}}
			if _, err := d.conn.Exec(`CREATE TRIGGER fail_energy_publication BEFORE INSERT ON track_metadata_capability_status BEGIN SELECT RAISE(ABORT,'late failure'); END`); err != nil {
				t.Fatal(err)
			}
			if err := d.PublishTrackPreparation(p); err == nil {
				t.Fatal("late failure ignored")
			}
			if _, err := d.GetTrackAnalysisArtifact("song", old.Kind, old.FormatVersion, old.AlgorithmVersion); err != nil {
				t.Fatalf("old artifact lost on rollback: %v", err)
			}
			if _, err := d.GetTrackAnalysisArtifact("song", current.Kind, current.FormatVersion, current.AlgorithmVersion); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("new artifact escaped rollback: %v", err)
			}
			if _, err := d.conn.Exec(`DROP TRIGGER fail_energy_publication`); err != nil {
				t.Fatal(err)
			}
			if err := d.PublishTrackPreparation(p); err != nil {
				t.Fatal(err)
			}
			_, err = d.GetTrackAnalysisArtifact("song", old.Kind, old.FormatVersion, old.AlgorithmVersion)
			if oldSource == "bytes" && !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("same-source old artifact retained: %v", err)
			}
			if oldSource != "bytes" && err != nil {
				t.Fatalf("other-source artifact removed: %v", err)
			}
		})
	}
}
