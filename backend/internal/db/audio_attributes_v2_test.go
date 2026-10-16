package db

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func TestV2CacheMigrationPreservesLegacyRows(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	o := referenceObservation()
	o.AccountContext = "v2-owner"
	if err = d.ActivateSpotifyMetadataContext(o.AccountContext); err != nil {
		t.Fatal(err)
	}
	if err = d.PutExternalAnalysis(o, "legacy", o.RetrievedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	status := ExternalAnalysisStatus{Code: spotifyanalysis.NotFound, CheckedAt: o.RetrievedAt, RetryAt: o.RetrievedAt.Add(time.Hour)}
	if err = d.PutExternalAnalysisStatus(referenceID, "audio_analysis", status); err != nil {
		t.Fatal(err)
	}
	// Recreate the pre-V2 tables, including the ownership columns added by
	// earlier migrations. This exercises an upgrade with real retained rows.
	for _, table := range []string{"external_track_analysis", "external_track_analysis_status"} {
		var sql string
		if err = d.conn.QueryRow("SELECT sql FROM sqlite_master WHERE name=?", table).Scan(&sql); err != nil {
			t.Fatal(err)
		}
		sql = strings.ReplaceAll(sql, ",'audio_attributes_v2'", "")
		sql = strings.Replace(sql, table, table+"_legacy", 1)
		if _, err = d.conn.Exec(sql + "; INSERT INTO " + table + "_legacy SELECT * FROM " + table + "; DROP TABLE " + table + "; ALTER TABLE " + table + "_legacy RENAME TO " + table); err != nil {
			t.Fatal(err)
		}
	}
	d.externalSchemaReady = false
	if err = d.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	if err = d.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	cache, err := d.GetExternalAnalysis(referenceID, "audio_features")
	if err != nil || cache == nil || cache.Observation.AccountContext != o.AccountContext || *cache.Observation.BPM != *o.BPM {
		t.Fatalf("legacy cache lost: %+v %v", cache, err)
	}
	if status, err := d.GetExternalAnalysisStatus(referenceID, "audio_analysis"); err != nil || status == nil || status.Code != spotifyanalysis.NotFound {
		t.Fatalf("legacy status lost: %+v %v", status, err)
	}
	o.SourceEndpoint = spotifyanalysis.AttributesV2
	o.BPM = scalarPtr(70.4827)
	if err = d.PutExternalAnalysis(o, "extension222", o.RetrievedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = d.PutExternalAnalysisStatus(referenceID, spotifyanalysis.AttributesV2, status); err != nil {
		t.Fatal(err)
	}
}

func TestV2ScalarPriorityAndManualOverride(t *testing.T) {
	at := time.Now()
	legacy := SpotifyScalarField{Key: "tempo_bpm", Metric: "tempo", Units: "bpm", Value: json.RawMessage("137.98"), Endpoint: "audio_features", RetrievedAt: at, AdapterRevision: "legacy"}
	v2 := legacy
	v2.Endpoint = spotifyanalysis.AttributesV2
	v2.Value = json.RawMessage("70.4827")
	v2.RetrievedAt = at.Add(-time.Hour)
	for _, fields := range [][]SpotifyScalarField{{legacy, v2}, {v2, legacy}} {
		got := SelectProviderScalarFields(fields)
		if len(got) != 1 || got[0].Endpoint != spotifyanalysis.AttributesV2 {
			t.Fatalf("legacy overwrote V2: %+v", got)
		}
	}
	manual := legacy
	manual.Value = json.RawMessage("71")
	got := ResolveEffectiveScalar("tempo_bpm", "source", []ScalarCandidate{{SpotifyScalarField: v2, Source: "spotify_private", SourceFingerprint: "source"}, {SpotifyScalarField: manual, Source: "manual", SourceFingerprint: "source", Locked: true}})
	if got.Selected == nil || got.Selected.Source != "manual" {
		t.Fatalf("manual override lost: %+v", got)
	}
	v2.Stale = true
	gotFields := SelectProviderScalarFields([]SpotifyScalarField{v2, legacy})
	if gotFields[0].Endpoint != legacy.Endpoint {
		t.Fatal("stale V2 suppressed fresh fallback")
	}
}

func TestV2DownloadedScalarsSurviveCacheRetirement(t *testing.T) {
	d, path := evidenceFixture(t)
	o := referenceObservation()
	o.AccountContext = "v2-import-owner"
	o.RetrievedAt = time.Now()
	o.SourceEndpoint = spotifyanalysis.AttributesV2
	o.BPM = scalarPtr(70.4827)
	tonic, mode, camelot := 3, 1, "5B"
	o.Key = &tonic
	o.Mode = &mode
	o.Camelot = &camelot
	if err := d.ActivateSpotifyMetadataContext(o.AccountContext); err != nil {
		t.Fatal(err)
	}
	if err := d.PutExternalAnalysis(o, "extension222", o.RetrievedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "v2-import-job", referenceID)
	fingerprint := scanEvidence(t, d, path, "v2-import-song")
	if err := d.RetireSpotifyMetadataContext(o.AccountContext); err != nil {
		t.Fatal(err)
	}
	fields, _, recording, _, err := d.GetDownloadedSpotifyScalarCandidates(t.Context(), "v2-import-song", fingerprint)
	if err != nil || recording != referenceID {
		t.Fatalf("durable import: %s %v", recording, err)
	}
	found := false
	for _, f := range SelectProviderScalarFields(fields) {
		if f.Key == "tempo_bpm" {
			found = f.Endpoint == spotifyanalysis.AttributesV2 && f.DurableImport && string(f.Value) == "70.4827"
		}
	}
	if !found {
		t.Fatalf("V2 import lost after retirement: %+v", fields)
	}
}
