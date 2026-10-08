package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func agePrivateFixtures(t *testing.T, d *DB, at time.Time) {
	t.Helper()
	for _, table := range []string{"spotify_entity_snapshots", "spotify_audio_artifacts", "spotify_audio_observations", "external_track_analysis"} {
		if _, err := d.conn.Exec("UPDATE "+table+" SET retrieved_at=?,expires_at=?", at.UnixMilli(), at.Add(time.Hour).UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
}
func privateSweep(t *testing.T, d *DB, at time.Time, limit int) PrivateCacheRetentionResult {
	t.Helper()
	got, err := d.MaintainSpotifyPrivateCache(t.Context(), at, limit)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func TestPrivateCacheRetentionGraceBoundsAndReservation(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	collectionPages(t, d)
	at := time.Now().Add(-24 * time.Hour)
	agePrivateFixtures(t, d, at)
	if got := privateSweep(t, d, at.Add(2*time.Hour-time.Millisecond), 128); got.Collected != 0 {
		t.Fatal("grace shortened", got)
	}
	if got := privateSweep(t, d, at.Add(2*time.Hour), 2); got.Collected != 2 {
		t.Fatal("batch bound", got)
	}
	if retentionCount(t, d, "spotify_entity_snapshots") != 3 {
		t.Fatal("wrong number collected")
	}
	if _, err := d.conn.Exec(`UPDATE settings SET value='' WHERE key='spotify_metadata_active_context'`); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, time.Now(), 128); got.Collected != 0 {
		t.Fatal("reserved cache lost", got)
	}
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, time.Now(), 128); got.Collected != 3 {
		t.Fatal(got)
	}
	if retentionCount(t, d, "spotify_entity_relations") != 0 {
		t.Fatal("relations orphaned")
	}
}
func TestPrivateCacheRetentionQueueCollectionsAndResumePins(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	collectionPages(t, d)
	putCollectionFixture(t, d, "playlist", collectionID, "wrong-revision", "v2", `{}`)
	putCollectionFixture(t, d, "track", "AAAAAAAAAAAAAAAAAAAAAA", "unrelated", "", `{}`)
	queueLineage(t, d, "pin", collectionOrigins())
	agePrivateFixtures(t, d, time.Now().Add(-24*time.Hour))
	if got := privateSweep(t, d, time.Now(), 128); got.Collected != 2 {
		t.Fatal("exact active origin pins", got)
	}
	if retentionCount(t, d, "spotify_entity_snapshots") != 5 {
		t.Fatal("active collection lost")
	}
	if _, err := d.conn.Exec(`UPDATE spotify_downloads SET status='completed'`); err != nil {
		t.Fatal(err)
	}
	token, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID)
	if err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, time.Now(), 128); got.Collected != 2 {
		t.Fatal("incomplete resume protection", got)
	}
	if retentionCount(t, d, "spotify_playlist_traversals") != 1 {
		t.Fatal("generation removed")
	}
	if _, err := d.conn.Exec(`UPDATE spotify_playlist_traversals SET complete=1`); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, time.Now(), 128); got.Collected != 3 {
		t.Fatal(got)
	}
	next, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID)
	if err != nil || next.Generation <= token.Generation {
		t.Fatal("generation reset", next, err)
	}
	if err := d.PutSpotifyPlaylistSnapshots(token, nil, true); !errors.Is(err, ErrSpotifyTraversalSuperseded) {
		t.Fatal("old request became eligible", err)
	}
}
func TestPrivateCacheRetentionAudioStatusAndRollback(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	o := detailedObservation(t, `{"track":{"tempo":120,"duration":10},"sections":[{"start":0,"duration":10}]}`)
	o.AccountContext = "owner"
	if err := d.PutExternalAnalysis(o, "fixture", o.RetrievedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-24 * time.Hour)
	agePrivateFixtures(t, d, at)
	if _, err := d.conn.Exec(`UPDATE spotify_audio_field_attempts SET checked_at=?`, at.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	// A newer failure remains distinct from collected old last-good payload.
	if _, err := d.conn.Exec(`INSERT INTO external_track_analysis_status(provider,external_id,endpoint,schema_version,code,checked_at,retry_at,account_context) VALUES('spotify',?,'audio_analysis',1,'not_found',?,?,'owner')`, referenceID, time.Now().UnixMilli(), time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	queueLineage(t, d, "audio", nil)
	if got := privateSweep(t, d, time.Now(), 128); got.Collected != 0 {
		t.Fatal("active audio lost", got)
	}
	if _, err := d.conn.Exec(`UPDATE spotify_downloads SET status='completed'; CREATE TRIGGER fail_private_collection BEFORE DELETE ON spotify_audio_observations BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	before, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := d.MaintainSpotifyPrivateCache(t.Context(), time.Now(), 128); err == nil || got.Collected != 0 {
		t.Fatal("failed sweep reported success", got, err)
	}
	after, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "owner")
	if err != nil || before.Rows != after.Rows {
		t.Fatal("partial collection committed", before, after, err)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER fail_private_collection`); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "durable", "11dFghVXANMlKmJXsNCbNl")
	if got := privateSweep(t, d, time.Now(), 128); got.Collected == 0 {
		t.Fatal(got)
	}
	for _, table := range []string{"spotify_audio_artifacts", "spotify_audio_observations", "external_track_analysis"} {
		if retentionCount(t, d, table) != 0 {
			t.Fatal("payload/pair retained", table)
		}
	}
	if retentionCount(t, d, "spotify_audio_field_attempts") != 11 {
		t.Fatal("paired attempts not collected or orphan attempts removed")
	}
	if retentionCount(t, d, "external_track_analysis_status") != 1 || retentionCount(t, d, "spotify_download_evidence") != 1 {
		t.Fatal("new attempt or durable evidence lost")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.MaintainSpotifyPrivateCache(ctx, time.Now(), 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, limit := range []int{0, 129} {
		if _, err := d.MaintainSpotifyPrivateCache(t.Context(), time.Now(), limit); err == nil {
			t.Fatal("unbounded sweep")
		}
	}
}

func TestPrivateCacheRetentionExactGraphAndStatusPairs(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	album := "AAAAAAAAAAAAAAAAAAAAAA"
	artist := "BBBBBBBBBBBBBBBBBBBBBB"
	sibling := "CCCCCCCCCCCCCCCCCCCCCC"
	putCollectionFixture(t, d, "track", referenceID, "track", "", `{}`, SpotifyEntityRelation{Kind: "album", Position: 0, ChildType: "album", ChildID: album})
	putCollectionFixture(t, d, "album", album, "album", "", `{}`, SpotifyEntityRelation{Kind: "artists", Position: 0, ChildType: "artist", ChildID: artist}, SpotifyEntityRelation{Kind: "tracks", Position: 1, ChildType: "track", ChildID: sibling})
	putCollectionFixture(t, d, "artist", artist, "artist", "", `{}`, SpotifyEntityRelation{Kind: "albums", Position: 0, ChildType: "album", ChildID: album})
	putCollectionFixture(t, d, "track", sibling, "track", "", `{}`)
	agePrivateFixtures(t, d, time.Now().Add(-24*time.Hour))
	queueLineage(t, d, "pin", nil)
	if got := privateSweep(t, d, time.Now(), 128); got.Collected != 1 {
		t.Fatal("graph cycle or sibling pin", got)
	}
	if retentionCount(t, d, "spotify_entity_snapshots") != 3 {
		t.Fatal("reachable graph removed")
	}
	// Confirmed context isolation also applies to expired rows, not just reads.
	if _, err := d.conn.Exec(`INSERT INTO spotify_entity_snapshots(entity_type,spotify_id,resource,context_key,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,capture_revision) SELECT entity_type,spotify_id,resource,'other',schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,capture_revision FROM spotify_entity_snapshots`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`UPDATE spotify_downloads SET status='completed'`); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-24 * time.Hour)
	for i, key := range []SpotifySnapshotKey{{EntityType: "track", SpotifyID: referenceID, Resource: "track", ContextKey: "owner"}, {EntityType: "album", SpotifyID: album, Resource: "album", ContextKey: "owner"}} {
		checked := at.Add(-time.Minute)
		if i == 1 {
			checked = time.Now()
		}
		if err := d.PutSpotifyMetadataResourceStatus(key, SpotifyMetadataResourceStatus{State: "failed", Reason: "fixture", CheckedAt: checked, RetryAt: checked.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := privateSweep(t, d, time.Now(), 128); got.Collected != 3 {
		t.Fatal(got)
	}
	if retentionCount(t, d, "spotify_metadata_resource_status") != 1 {
		t.Fatal("new attempt lost or old paired status retained")
	}
	other, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "other")
	if err != nil || other.Rows != 3 {
		t.Fatal("other account changed", other, err)
	}
}

func TestPrivateCacheRetentionKeepsCompletedCollectionImports(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	collectionPages(t, d)
	id := queueLineage(t, d, "completed", collectionOrigins())
	completeLineage(t, d, path, id)
	before := readCollections(t, d, path)
	if before.CollectionStatus.State != "available" || len(before.Snapshots) != 5 {
		t.Fatal("fixture promotion failed", before)
	}
	agePrivateFixtures(t, d, time.Now().Add(-24*time.Hour))
	if got := privateSweep(t, d, time.Now(), 128); got.Collected != 5 {
		t.Fatal(got)
	}
	after := readCollections(t, d, path)
	if after.CollectionStatus.State != "available" || len(after.Snapshots) != 5 || len(after.Relations) != 5 {
		t.Fatal("durable capture lost", after)
	}
}
