// Tests and fixtures for spotify scalars behavior.

package db

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDownloadedSpotifyScalarsBecomeEffective(t *testing.T) {
	d, path := evidenceFixture(t)
	o := referenceObservation()
	o.AccountContext = "download-fixture"
	if err := d.ActivateSpotifyMetadataContext(o.AccountContext); err != nil {
		t.Fatal(err)
	}
	o.RetrievedAt = time.Now().UTC()
	o.BPMConfidence = nil
	o.Key, o.Mode = scalarPtr(0), scalarPtr(1) // C major
	o.Camelot = scalarPtr("8B")
	if err := d.PutExternalAnalysis(o, "fixture", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "job", referenceID)
	if _, err := d.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeExternalAnalysis(); err != nil {
		t.Fatal(err)
	}
	fingerprint := scanEvidence(t, d, path, "song")
	record, err := d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	bpm := ResolveEffectiveBPMForSource(EffectiveBPMInputs{Analysis: &record}, fingerprint)
	key := ResolveEffectiveKey(EffectiveKeyInputs{Analysis: &record})
	if bpm.Value == nil || *bpm.Value != *o.BPM || bpm.Source != "spotify" || bpm.SyncAllowed || key.Source != "spotify" || *key.Mode != "major" || *key.Tonic != 0 {
		t.Fatalf("effective values: %+v %+v", bpm, key)
	}
	manual := scalarPtr(130.0)
	if resolved := ResolveEffectiveBPM(EffectiveBPMInputs{Analysis: &record, Override: &TrackAnalysisOverride{BPM: manual, BPMLocked: true}}); resolved.Source != "manual" {
		t.Fatal("manual override lost")
	}
	ids, err := d.ExpandAnalysisSelection(AnalysisSelection{Mode: AnalysisSelectionMissing}, 1, "current")
	if err != nil || len(ids) != 1 {
		t.Fatalf("download excluded from fallback scan: %v %v", ids, err)
	}
}

func TestSpotifyScalarSchemaUpgradePreservesRows(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	var definition string
	if err := d.conn.QueryRow("SELECT sql FROM sqlite_master WHERE name='track_analysis'").Scan(&definition); err != nil {
		t.Fatal(err)
	}
	// Recreate the pre-update schema and a locally measured row.
	if _, err := d.conn.Exec("DROP TABLE track_analysis"); err != nil {
		t.Fatal(err)
	}
	definition = strings.ReplaceAll(definition, "'spotify', ", "")
	if _, err := d.conn.Exec(definition); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`INSERT INTO songs(id,title,artist,album,file_path,added_at) VALUES ('song','S','A','B','song.wav',1);
 INSERT INTO track_analysis(song_id,status,analysis_version,algorithm_version,source_fingerprint,bpm,bpm_source) VALUES ('song','complete',1,'local','source',128.5,'measured')`); err != nil {
		t.Fatal(err)
	}
	trackAnalysisSchemas.Delete(d)
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	record, err := d.GetTrackAnalysis("song")
	if err != nil || record.BPM == nil || *record.BPM != 128.5 || *record.BPMSource != "measured" {
		t.Fatalf("row lost: %+v %v", record, err)
	}
	record.BPMSource = scalarPtr("spotify")
	if err := d.UpsertTrackAnalysis(record); err != nil {
		t.Fatal(err)
	}
}

// A matching recording and file prove download identity, not ownership of an
// unrelated legacy provider cache. Such values cannot become durable facts.
func TestDownloadCompletionExcludesUnownedAndRetiredScalarCaches(t *testing.T) {
	for _, owner := range []string{"", "retired"} {
		t.Run("owner="+owner, func(t *testing.T) {
			d, path := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("current"); err != nil {
				t.Fatal(err)
			}
			o := referenceObservation()
			o.AccountContext = owner
			o.RetrievedAt = time.Now().UTC()
			if err := d.PutExternalAnalysis(o, "historical", time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if cache, err := d.GetExternalAnalysis(referenceID, "audio_features"); err != nil || cache != nil {
				t.Fatalf("ineligible cache reused: %+v %v", cache, err)
			}
			finishEvidence(t, d, path, "job", referenceID)
			var encoded string
			if err := d.conn.QueryRow("SELECT features_json FROM spotify_download_evidence").Scan(&encoded); err != nil {
				t.Fatal(err)
			}
			if encoded != "" {
				t.Fatal("ineligible cache acquired durable download provenance")
			}
			fp := scanEvidence(t, d, path, "song")
			link, err := d.GetSpotifyRecording("song", fp)
			if err != nil || link == nil || link.ExternalID != referenceID {
				t.Fatalf("verified recording identity lost: %+v %v", link, err)
			}
		})
	}
}

func TestVerifiedDownloadRecoversExistingScalarsAndDefersClaims(t *testing.T) {
	for _, status := range []string{TrackAnalysisComplete, TrackAnalysisRunning, TrackAnalysisPending} {
		t.Run(status, func(t *testing.T) {
			d, path := evidenceFixture(t)
			o := referenceObservation()
			o.AccountContext = "download"
			o.RetrievedAt = time.Now().UTC()
			if err := d.ActivateSpotifyMetadataContext(o.AccountContext); err != nil {
				t.Fatal(err)
			}
			if err := d.PutExternalAnalysis(o, "fixture", time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			finishEvidence(t, d, path, "job", referenceID)
			fp := scanEvidence(t, d, path, "song")
			bpm, key, mode, energy, at := 150.0, 0, "major", 7, int64(123)
			original := TrackAnalysis{SongID: "song", Status: status, AnalysisVersion: 1, AlgorithmVersion: "local-current", SourceFingerprint: fp, BPM: &bpm, BPMSource: scalarPtr("spotify"), KeyTonic: &key, KeyMode: &mode, KeySource: scalarPtr("measured"), EnergyLevel: &energy, EnergyLevelConfidence: scalarPtr(0.8), EnergyAlgorithmVersion: scalarPtr("energy-v1"), AnalyzedAt: &at, Local: &LocalScalarObservation{SourceFingerprint: fp, AlgorithmVersion: "local-current", BPM: &bpm, KeyTonic: &key, KeyMode: &mode}}
			if err := d.UpsertTrackAnalysis(original); err != nil {
				t.Fatal(err)
			}
			before, err := d.GetTrackAnalysis("song")
			if err != nil {
				t.Fatal(err)
			}
			if err := d.ReconcileSpotifyDownload(context.Background(), path); err != nil {
				t.Fatal(err)
			}
			after, err := d.GetTrackAnalysis("song")
			if err != nil {
				t.Fatal(err)
			}
			if status != TrackAnalysisComplete {
				if !reflect.DeepEqual(before, after) {
					t.Fatal("recovery replaced active claim")
				}
				return
			}
			if after.SpotifyBindings == nil || !after.SpotifyBindings.BPM.Durable || *after.BPM != *o.BPM || !reflect.DeepEqual(before.Local, after.Local) || *after.EnergyLevel != energy || *after.AnalyzedAt != at || after.AlgorithmVersion != before.AlgorithmVersion || *after.KeyTonic != key {
				t.Fatalf("recovery lost independent facts: %+v", after)
			}
			if err := d.ReconcileSpotifyDownload(context.Background(), path); err != nil {
				t.Fatal(err)
			}
			again, err := d.GetTrackAnalysis("song")
			if err != nil || !reflect.DeepEqual(after, again) {
				t.Fatal("recovery not idempotent")
			}
		})
	}
}
