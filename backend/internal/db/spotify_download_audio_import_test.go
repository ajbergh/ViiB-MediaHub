package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func TestDownloadAudioImportSurvivesPrivateRetirement(t *testing.T) {
	d, path := evidenceFixture(t)
	const owner = "download-owner"
	if err := d.ActivateSpotifyMetadataContext(owner); err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	featureObservation := spotifyanalysis.Observation{TrackID: referenceID, AccountContext: owner, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: time.Now(), Energy: &zero}
	if err := d.PutExternalAnalysis(featureObservation, "fixture", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	o, err := spotifyanalysis.ValidateDomainPayload(referenceID, "audio_analysis", []byte(`{"track":{},"beats":[{"start":0,"duration":1,"confidence":0}]}`))
	if err != nil {
		t.Fatal(err)
	}
	o.AccountContext = owner
	o.RetrievedAt = time.Now()
	if err = d.PutExternalAnalysis(o, "fixture", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	snapshot := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "track", ContextKey: owner}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{"title":"retained","popularity":0}`), RetrievedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	snapshot.Relations = []SpotifyEntityRelation{{Kind: "artists", Position: 0, ChildType: "artist", ChildID: "AAAAAAAAAAAAAAAAAAAAAA", Metadata: []byte(`{"role":"primary"}`)}}
	snapshot.Relations = append(snapshot.Relations, SpotifyEntityRelation{Kind: "artists", Position: 1, ChildType: "artist", ChildID: "AAAAAAAAAAAAAAAAAAAAAA"}, SpotifyEntityRelation{Kind: "artists", Position: 2, ChildType: "artist", Unavailable: true})
	related := snapshot
	related.EntityType = "artist"
	related.SpotifyID = "AAAAAAAAAAAAAAAAAAAAAA"
	related.Relations = nil
	if err = d.PutSpotifyEntitySnapshot(related); err != nil {
		t.Fatal(err)
	}
	unrelated := related
	unrelated.SpotifyID = "BBBBBBBBBBBBBBBBBBBBBB"
	if err = d.PutSpotifyEntitySnapshot(unrelated); err != nil {
		t.Fatal(err)
	}
	if err = d.PutSpotifyEntitySnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "job", referenceID)
	if err = d.ActivateSpotifyMetadataContext(owner); err != nil {
		t.Fatal(err)
	}
	if _, err = d.MarkDownloadCompletedWithEvidence(t.Context(), "job", path); err != nil {
		t.Fatal(err)
	}
	if _, err = d.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	if err = d.RetireSpotifyMetadataContext(owner); err != nil {
		t.Fatal(err)
	}
	if err = d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var count int
	if err = d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_audio_imports WHERE file_path=? AND spotify_id=?`, path, referenceID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	var catalog []byte
	if err = d.conn.QueryRow(`SELECT payload FROM spotify_download_catalog_imports WHERE recording_id=?`, referenceID).Scan(&catalog); err != nil || len(catalog) == 0 {
		t.Fatalf("catalog not retained: %s %v", catalog, err)
	}
	var relatedCount, unrelatedCount, relationCount int
	if err = d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_catalog_imports WHERE entity_type='artist' AND spotify_id='AAAAAAAAAAAAAAAAAAAAAA'`).Scan(&relatedCount); err != nil {
		t.Fatal(err)
	}
	if err = d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_catalog_imports WHERE spotify_id='BBBBBBBBBBBBBBBBBBBBBB'`).Scan(&unrelatedCount); err != nil {
		t.Fatal(err)
	}
	if err = d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_catalog_relations WHERE relation_kind='artists' AND position=0 AND child_id='AAAAAAAAAAAAAAAAAAAAAA'`).Scan(&relationCount); err != nil {
		t.Fatal(err)
	}
	if relatedCount != 1 || unrelatedCount != 0 || relationCount != 1 {
		t.Fatalf("catalog graph scope %d %d %d", relatedCount, unrelatedCount, relationCount)
	}
	if err := d.SetSetting("spotify_metadata_active_context", owner); err != nil {
		t.Fatal(err)
	}
	fingerprint := scanEvidence(t, d, path, "imported")
	var bound string
	if err = d.conn.QueryRow(`SELECT source_fingerprint FROM spotify_download_import_bindings WHERE song_id='imported'`).Scan(&bound); err != nil {
		t.Fatal(err)
	}
	if err = d.SetSetting("spotify_metadata_active_context", ""); err != nil {
		t.Fatal(err)
	}
	if bound != fingerprint {
		t.Fatalf("wrong source binding: %s", bound)
	}
	catalogRead, err := d.GetDownloadedSpotifyCatalog(t.Context(), "imported", fingerprint)
	if err != nil || catalogRead == nil || len(catalogRead.Snapshots) != 2 || len(catalogRead.Relations) != 3 || catalogRead.Provenance != "spotify_download_import" || catalogRead.Relations[0].ParentID != referenceID {
		t.Fatalf("durable catalog read after reopen/retirement/cleanup: %+v %v", catalogRead, err)
	}
	if catalogRead.Relations[1].Position != 1 || catalogRead.Relations[1].ChildID != catalogRead.Relations[0].ChildID || catalogRead.Relations[2].Position != 2 || !catalogRead.Relations[2].Unavailable {
		t.Fatalf("ordered duplicates/placeholders lost: %+v", catalogRead.Relations)
	}
	if _, err = d.conn.Exec(`UPDATE spotify_download_catalog_relations SET metadata_json='{"token":"secret"}' WHERE position=1`); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "imported", fingerprint); err == nil || got != nil {
		t.Fatal("unsanitized retained relation admitted")
	}
	if _, err = d.conn.Exec(`UPDATE spotify_download_catalog_relations SET metadata_json='{}' WHERE position=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = d.conn.Exec(`UPDATE spotify_download_catalog_imports SET expires_at=retrieved_at+1`); err != nil {
		t.Fatal(err)
	}
	staleCatalog, err := d.GetDownloadedSpotifyCatalog(t.Context(), "imported", fingerprint)
	if err != nil || staleCatalog == nil || !staleCatalog.Snapshots[0].Stale {
		t.Fatalf("last-good catalog lost: %+v %v", staleCatalog, err)
	}
	var catalogHash string
	if err = d.conn.QueryRow(`SELECT payload_hash FROM spotify_download_catalog_imports WHERE entity_type='track'`).Scan(&catalogHash); err != nil {
		t.Fatal(err)
	}
	if _, err = d.conn.Exec(`UPDATE spotify_download_catalog_imports SET payload_hash='corrupt' WHERE entity_type='track'`); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "imported", fingerprint); err == nil || got != nil {
		t.Fatal("corrupt catalog admitted")
	}
	if _, err = d.conn.Exec(`UPDATE spotify_download_catalog_imports SET payload_hash=? WHERE entity_type='track'`, catalogHash); err != nil {
		t.Fatal(err)
	}
	importedFields, importedAttempts, importedRecording, err := d.GetDownloadedSpotifyScalarCandidates(t.Context(), "imported", fingerprint)
	if err != nil || importedRecording != referenceID || len(importedFields) == 0 || len(importedAttempts) == 0 {
		t.Fatalf("durable scalar import: fields=%d attempts=%d recording=%s err=%v", len(importedFields), len(importedAttempts), importedRecording, err)
	}
	for _, field := range importedFields {
		if !field.DurableImport {
			t.Fatalf("fresh download scalar provenance = %+v", field)
		}
	}
	artifact, err := d.GetDownloadedSpotifyAudioArtifact(t.Context(), "imported", fingerprint, "audio_analysis", "beats")
	if err != nil || artifact == nil {
		t.Fatalf("import read: %v %v", artifact, err)
	}
	var originalHash string
	if err = d.conn.QueryRow(`SELECT payload_hash FROM spotify_download_audio_imports WHERE artifact_kind='beats'`).Scan(&originalHash); err != nil {
		t.Fatal(err)
	}
	if _, err = d.conn.Exec(`UPDATE spotify_download_audio_imports SET payload_hash='corrupted' WHERE artifact_kind='beats'`); err != nil {
		t.Fatal(err)
	}
	if corrupted, readErr := d.GetDownloadedSpotifyAudioArtifact(t.Context(), "imported", fingerprint, "audio_analysis", "beats"); readErr == nil || corrupted != nil {
		t.Fatal("corrupted payload admitted")
	}
	if _, err = d.conn.Exec(`UPDATE spotify_download_audio_imports SET payload_hash=? WHERE artifact_kind='beats'`, originalHash); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "imported", "changed"); err != nil || got != nil {
		t.Fatal("changed source admitted catalog", err)
	}
	if artifact, err = d.GetDownloadedSpotifyAudioArtifact(t.Context(), "imported", "changed", "audio_analysis", "beats"); err != nil || artifact != nil {
		t.Fatal("changed source admitted", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), original...)
	changed[0] ^= 1
	if err = os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if artifact, err = d.GetDownloadedSpotifyAudioArtifact(t.Context(), "imported", fingerprint, "audio_analysis", "beats"); err != nil || artifact != nil {
		t.Fatal("same-size/mtime replacement admitted", err)
	}
	if got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "imported", fingerprint); err != nil || got != nil {
		t.Fatal("replacement admitted catalog", err)
	}
	if err = os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err = d.DeleteSpotifyRecording("imported"); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "imported", fingerprint); err != nil || got != nil {
		t.Fatal("unlinked catalog admitted", err)
	}
	if artifact, err = d.GetDownloadedSpotifyAudioArtifact(t.Context(), "imported", fingerprint, "audio_analysis", "beats"); err != nil || artifact != nil {
		t.Fatal("removed link admitted", err)
	}
	if fields, attempts, _, err := d.GetDownloadedSpotifyScalarCandidates(t.Context(), "imported", fingerprint); err != nil || len(fields) != 0 || len(attempts) != 0 {
		t.Fatalf("removed link admitted durable scalars: %d %d %v", len(fields), len(attempts), err)
	}
	var status string
	if err = d.conn.QueryRow(`SELECT state FROM spotify_download_import_status WHERE file_path=?`, path).Scan(&status); err != nil || status != "available" {
		t.Fatalf("import status %s %v", status, err)
	}
	if count != 2 {
		t.Fatalf("expected durable domain and beats, got %d", count)
	}
}

func TestDownloadedSpotifyScoreSummariesStaySourceBound(t *testing.T) {
	d, path := evidenceFixture(t)
	const owner = "score-import-owner"
	if err := d.ActivateSpotifyMetadataContext(owner); err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	bpm, meter := 125.0, 3
	observation := spotifyanalysis.Observation{TrackID: referenceID, AccountContext: owner, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: time.Now(), Energy: &zero, BPM: &bpm, TimeSignature: &meter}
	if err := d.PutExternalAnalysis(observation, "fixture", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := d.AddDownload(&SpotifyDownload{ID: "score-job", SpotifyID: referenceID, Type: "track", Title: "Score fixture", Status: "queued", FilePath: path, AddedAt: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.MarkDownloadStarted("score-job"); err != nil || !changed {
		t.Fatalf("start download: %v %v", changed, err)
	}
	if changed, err := d.MarkDownloadConverting("score-job", path); err != nil || !changed {
		t.Fatalf("begin completion: %v %v", changed, err)
	}
	if changed, err := d.MarkDownloadCompletedWithEvidence(t.Context(), "score-job", path); err != nil || !changed {
		t.Fatalf("complete download: %v %v", changed, err)
	}
	fingerprint := scanEvidence(t, d, path, "score-imported")
	if _, _, recording, err := d.GetDownloadedSpotifyScalarCandidates(t.Context(), "score-imported", fingerprint); err != nil || recording != referenceID {
		t.Fatal(err)
	}
	if _, err := d.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	if err := d.RetireSpotifyMetadataContext(owner); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	d, err = New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	batch, batchErr := d.GetDownloadedSpotifyScalarCandidateBatch(map[string]string{"score-imported": fingerprint}, time.Now())
	if batchErr != nil || len(batch["score-imported"]) != 3 {
		t.Fatalf("durable generic batch: %+v %v", batch, batchErr)
	}
	for _, field := range batch["score-imported"] {
		if !field.DurableImport {
			t.Fatal("lost durable provenance")
		}
	}
	summaries, err := d.GetDownloadedSpotifyScoreSummaries(map[string]string{"score-imported": fingerprint}, time.Now())
	if err != nil || len(summaries["score-imported"]) != 1 || summaries["score-imported"]["spotify_energy_score"].Value != 0 {
		t.Fatalf("durable score summary = %+v, err=%v", summaries, err)
	}
	if _, err := d.ReserveSpotifyMetadataRuntime("score-reader"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("score-reader", "oauth", "score-account", "score-reader-owner"); err != nil {
		t.Fatal(err)
	}
	privateValue := .8
	if err := d.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: referenceID, AccountContext: "score-reader-owner", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: observation.RetrievedAt.Add(time.Minute), Energy: &privateValue}, "fixture", observation.RetrievedAt.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	fence := SpotifyMetadataReadFence{Epoch: "score-reader", ContextKey: "score-reader-owner"}
	if got, err := d.GetSpotifyScoreSummariesForRuntime(fence, map[string]string{"score-imported": fingerprint}, observation.RetrievedAt.Add(5*time.Minute)); err != nil || got["score-imported"]["spotify_energy_score"].Value != 0 {
		t.Fatalf("newer expired private score displaced fresh durable zero: %+v, err=%v", got, err)
	}
	privateValue = .4
	if err := d.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: referenceID, AccountContext: "score-reader-owner", Source: "spotify_internal", SourceEndpoint: "audio_analysis", RetrievedAt: observation.RetrievedAt, Energy: &privateValue}, "fixture", observation.RetrievedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetSpotifyScoreSummariesForRuntime(fence, map[string]string{"score-imported": fingerprint}, observation.RetrievedAt.Add(5*time.Minute)); err != nil || got["score-imported"]["spotify_energy_score"].Value != .4 || got["score-imported"]["spotify_energy_score"].Endpoint != "audio_analysis" {
		t.Fatalf("equal-time detailed endpoint did not win: %+v, err=%v", got, err)
	}
	if got, err := d.GetDownloadedSpotifyScoreSummaries(map[string]string{"score-imported": fingerprint}, observation.RetrievedAt.Add(2*time.Hour)); err != nil || !got["score-imported"]["spotify_energy_score"].Stale {
		t.Fatalf("expired score must remain explicitly stale: %+v, err=%v", got, err)
	}
	for _, raw := range []string{"null", " null ", "1.1", "-0.1", `"invalid"`, "{}"} {
		if _, err := d.conn.Exec(`UPDATE spotify_download_scalar_imports SET value_json=? WHERE field_key='spotify_energy_score'`, raw); err != nil {
			t.Fatal(err)
		}
		if got, err := d.GetDownloadedSpotifyScoreSummaries(map[string]string{"score-imported": fingerprint}, time.Now()); err != nil || len(got) != 0 {
			t.Fatalf("invalid durable score %s admitted: %+v, err=%v", raw, got, err)
		}
	}
	if _, err := d.conn.Exec(`UPDATE spotify_download_scalar_imports SET value_json='0' WHERE field_key='spotify_energy_score'`); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), original...)
	changed[0] ^= 1
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetDownloadedSpotifyScoreSummaries(map[string]string{"score-imported": fingerprint}, time.Now()); err != nil || len(got) != 0 {
		t.Fatalf("same-size/mtime replacement admitted: %+v, err=%v", got, err)
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if stale, err := d.GetDownloadedSpotifyScoreSummaries(map[string]string{"score-imported": "old-fingerprint"}, time.Now()); err != nil || len(stale) != 0 {
		t.Fatalf("old source score summary = %+v, err=%v", stale, err)
	}
	if err := d.DeleteSpotifyRecording("score-imported"); err != nil {
		t.Fatal(err)
	}
	if unlinked, err := d.GetDownloadedSpotifyScoreSummaries(map[string]string{"score-imported": fingerprint}, time.Now()); err != nil || len(unlinked) != 0 {
		t.Fatalf("unlinked score summary = %+v, err=%v", unlinked, err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("score-imported", fingerprint); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.ConfirmSpotifyRecording("score-imported", referenceID, fingerprint, true); err != nil || !ok {
		t.Fatalf("explicit source confirmation should permit intentional relink: %v %v", ok, err)
	}
	if relinked, err := d.GetDownloadedSpotifyScoreSummaries(map[string]string{"score-imported": fingerprint}, time.Now()); err != nil || len(relinked["score-imported"]) != 1 || relinked["score-imported"]["spotify_energy_score"].Value != 0 {
		t.Fatalf("confirmed durable score summary = %+v, err=%v", relinked, err)
	}
}

func TestDownloadAudioOversizedStatusDoesNotFailCompletion(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.SetSetting("spotify_metadata_active_context", "owner"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"domain", "beats", "bars", "sections", "segments"} {
		_, err := d.conn.Exec(`INSERT INTO spotify_audio_artifacts(spotify_id,resource,artifact_kind,context_key,schema_version,adapter_revision,encoding,payload,payload_hash,decoded_size,retrieved_at,expires_at) VALUES(?,'audio_analysis',?,'owner',1,'fixture','gzip-json',zeroblob(8388608),'fixture',1,?,?)`, referenceID, kind, time.Now().UnixMilli(), time.Now().Add(time.Hour).UnixMilli())
		if err != nil {
			t.Fatal(err)
		}
	}
	finishEvidence(t, d, path, "oversized", referenceID)
	var state string
	var count int
	if err := d.conn.QueryRow(`SELECT state FROM spotify_download_import_status WHERE file_path=?`, path).Scan(&state); err != nil || state != "oversized" {
		t.Fatalf("oversized status %s %v", state, err)
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_audio_imports`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("oversized imported %d %v", count, err)
	}
}
