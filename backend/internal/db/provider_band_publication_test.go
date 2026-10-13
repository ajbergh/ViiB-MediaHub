package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
)

func TestProviderBandSelectionDeletesLocalOutputOnlyWithSuccessfulOwnedPublication(t *testing.T) {
	for _, scenario := range []string{"success", "late-failure", "lost-claim", "other-source", "conflicting-output"} {
		t.Run(scenario, func(t *testing.T) {
			d, err := New(filepath.Join(t.TempDir(), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.SaveSong(&Song{ID: "song", Title: "Song", FilePath: "song.wav"}); err != nil {
				t.Fatal(err)
			}
			record := TrackAnalysis{SongID: "song", Status: TrackAnalysisPartial, SourceFingerprint: "bytes", AnalysisVersion: 1, AlgorithmVersion: "v1"}
			if err := d.UpsertTrackAnalysis(record); err != nil {
				t.Fatal(err)
			}
			artifact := TrackAnalysisArtifact{ID: "song:bands", SongID: "song", Kind: threeband.Kind, FormatVersion: threeband.FormatVersion, AlgorithmVersion: threeband.AlgorithmVersion, Encoding: threeband.Encoding, Provenance: "measured", SourceFingerprint: "bytes", Data: []byte("retained-local-output")}
			if scenario == "other-source" {
				artifact.SourceFingerprint = "new-bytes"
			}
			if err := d.UpsertTrackAnalysisArtifact(artifact); err != nil {
				t.Fatal(err)
			}
			token, claimed, err := d.ClaimTrackAnalysisLease("song", "bytes", 1, "v1")
			if err != nil || !claimed {
				t.Fatalf("claim: %v %v", claimed, err)
			}
			publication := TrackPreparationPublication{Analysis: record, ClaimToken: token, Capabilities: []TrackCapabilityStatus{{SongID: "song", SourceFingerprint: "bytes", Capability: threeband.Kind, Version: threeband.AlgorithmVersion, State: "available", Reason: "provider_three_band"}}}
			switch scenario {
			case "late-failure":
				if _, err := d.conn.Exec(`CREATE TRIGGER fail_band_publication BEFORE INSERT ON track_metadata_capability_status BEGIN SELECT RAISE(ABORT,'late test failure'); END`); err != nil {
					t.Fatal(err)
				}
			case "lost-claim":
				if _, err := d.conn.Exec(`UPDATE track_analysis SET analyzed_at=0 WHERE song_id='song'`); err != nil {
					t.Fatal(err)
				}
				if _, claimed, err := d.ClaimTrackAnalysisLease("song", "bytes", 1, "v1"); err != nil || !claimed {
					t.Fatalf("replacement claim: %v %v", claimed, err)
				}
			case "conflicting-output":
				publication.Artifacts = []TrackAnalysisArtifact{artifact}
			}
			err = d.PublishTrackPreparation(publication)
			if scenario == "lost-claim" && !errors.Is(err, ErrTrackAnalysisLeaseLost) {
				t.Fatalf("late worker accepted: %v", err)
			}
			failure := scenario == "late-failure" || scenario == "lost-claim" || scenario == "conflicting-output"
			if failure && err == nil {
				t.Fatal("invalid publication accepted")
			}
			if !failure && err != nil {
				t.Fatal(err)
			}
			retained, readErr := d.GetTrackAnalysisArtifact("song", threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion)
			if scenario == "success" {
				if !errors.Is(readErr, sql.ErrNoRows) {
					t.Fatal("successful provider selection retained duplicate local output")
				}
			} else if readErr != nil || string(retained.Data) != string(artifact.Data) || retained.SourceFingerprint != artifact.SourceFingerprint {
				t.Fatalf("fallback artifact deleted by failed/cross-source publication: %+v %v", retained, readErr)
			}
			if scenario == "late-failure" {
				var retainedToken string
				if err := d.conn.QueryRow(`SELECT claim_token FROM track_analysis WHERE song_id='song'`).Scan(&retainedToken); err != nil || retainedToken != token {
					t.Fatalf("rollback lost generation: %q %v", retainedToken, err)
				}
				if _, err := d.conn.Exec(`DROP TRIGGER fail_band_publication`); err != nil {
					t.Fatal(err)
				}
				if err := d.PublishTrackPreparation(publication); err != nil {
					t.Fatal(err)
				}
				if _, err := d.GetTrackAnalysisArtifact("song", threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion); !errors.Is(err, sql.ErrNoRows) {
					t.Fatal("successful retry retained duplicate local output")
				}
			}
		})
	}
}

func TestUnavailableProviderBandRetirementIsOwnedAndAtomic(t *testing.T) {
	for _, scenario := range []string{"success", "late-failure", "lost-claim", "local-status"} {
		t.Run(scenario, func(t *testing.T) {
			d, err := New(filepath.Join(t.TempDir(), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.SaveSong(&Song{ID: "song", Title: "Song", FilePath: "song.wav"}); err != nil {
				t.Fatal(err)
			}
			record := TrackAnalysis{SongID: "song", Status: TrackAnalysisPartial, SourceFingerprint: "bytes", AnalysisVersion: 1, AlgorithmVersion: "v1"}
			if err := d.UpsertTrackAnalysis(record); err != nil {
				t.Fatal(err)
			}
			status := TrackCapabilityStatus{SongID: "song", SourceFingerprint: "bytes", Capability: threeband.Kind, Version: threeband.AlgorithmVersion, State: "available", Reason: "provider_three_band"}
			if scenario == "local-status" {
				status.Reason = ""
			}
			if err := d.PutTrackCapabilityStatuses([]TrackCapabilityStatus{status}); err != nil {
				t.Fatal(err)
			}
			token, claimed, err := d.ClaimTrackAnalysisLease("song", "bytes", 1, "v1")
			if err != nil || !claimed {
				t.Fatalf("claim: %v %v", claimed, err)
			}
			publication := TrackPreparationPublication{Analysis: record, ClaimToken: token, ClearProviderThreeBand: true}
			if scenario == "late-failure" {
				publication.Capabilities = []TrackCapabilityStatus{{SongID: "song", SourceFingerprint: "bytes", Capability: "test", Version: "v1", State: "available"}}
				if _, err := d.conn.Exec(`CREATE TRIGGER fail_retirement BEFORE INSERT ON track_metadata_capability_status BEGIN SELECT RAISE(ABORT,'late failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "lost-claim" {
				if _, err := d.conn.Exec(`UPDATE track_analysis SET analyzed_at=0 WHERE song_id='song'`); err != nil {
					t.Fatal(err)
				}
				if _, claimed, err := d.ClaimTrackAnalysisLease("song", "bytes", 1, "v1"); err != nil || !claimed {
					t.Fatalf("new claim: %v %v", claimed, err)
				}
			}
			err = d.PublishTrackPreparation(publication)
			failure := scenario == "late-failure" || scenario == "lost-claim"
			if failure && err == nil || !failure && err != nil {
				t.Fatalf("publication result: %v", err)
			}
			states, err := d.GetTrackCapabilityStatuses("song", "bytes")
			if err != nil {
				t.Fatal(err)
			}
			got, present := states[threeband.Kind]
			if scenario == "success" {
				if present {
					t.Fatal("successful retirement kept provider claim")
				}
			} else if !present || got.State != status.State || got.Reason != status.Reason {
				t.Fatalf("failed retirement or local output lost status: %+v", states)
			}
		})
	}
}
