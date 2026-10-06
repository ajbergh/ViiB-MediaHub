package db

import (
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadAudioImportSurvivesPrivateRetirement(t *testing.T) {
	d, path := evidenceFixture(t)
	const owner = "download-owner"
	if err := d.SetSetting("spotify_metadata_active_context", owner); err != nil {
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
	fingerprint := scanEvidence(t, d, path, "imported")
	var bound string
	if err = d.conn.QueryRow(`SELECT source_fingerprint FROM spotify_download_import_bindings WHERE song_id='imported'`).Scan(&bound); err != nil {
		t.Fatal(err)
	}
	if bound != fingerprint {
		t.Fatalf("wrong source binding: %s", bound)
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
	if err = os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err = d.DeleteSpotifyRecording("imported"); err != nil {
		t.Fatal(err)
	}
	if artifact, err = d.GetDownloadedSpotifyAudioArtifact(t.Context(), "imported", fingerprint, "audio_analysis", "beats"); err != nil || artifact != nil {
		t.Fatal("removed link admitted", err)
	}
	var status string
	if err = d.conn.QueryRow(`SELECT state FROM spotify_download_import_status WHERE file_path=?`, path).Scan(&status); err != nil || status != "available" {
		t.Fatalf("import status %s %v", status, err)
	}
	if count != 2 {
		t.Fatalf("expected durable domain and beats, got %d", count)
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
