package db

import (
	"strings"
	"testing"
	"time"
)

func TestDownloadedSpotifyScalarsBecomeEffective(t *testing.T) {
	d, path := evidenceFixture(t)
	o := referenceObservation()
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
