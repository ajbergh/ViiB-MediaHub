package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func putOrphanStatusFixture(t *testing.T, d *DB, family, id, owner, state string, checked, retry time.Time) {
	t.Helper()
	var err error
	switch family {
	case "resource":
		_, err = d.conn.Exec(`INSERT INTO spotify_metadata_resource_status(entity_type,spotify_id,resource,context_key,state,reason,checked_at,retry_at) VALUES('track',?,'audio_features',?,?,'fixture',?,?)`, id, owner, state, checked.UnixMilli(), retry.UnixMilli())
	case "analysis":
		_, err = d.conn.Exec(`INSERT INTO external_track_analysis_status(provider,external_id,endpoint,schema_version,code,checked_at,retry_at,account_context) VALUES('spotify',?,'audio_features',1,?,?,?,?)`, id, state, checked.UnixMilli(), retry.UnixMilli(), owner)
	case "field":
		_, err = d.conn.Exec(`INSERT INTO spotify_audio_field_attempts(spotify_id,endpoint,context_key,field_key,state,reason,checked_at,adapter_revision) VALUES(?,'audio_features',?,'tempo_bpm',?,'fixture',?,'fixture')`, id, owner, state, checked.UnixMilli())
	default:
		t.Fatal("unknown fixture family")
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrivateStatusRetentionGraceCooldownAndCombinedLimit(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)
	old := now.Add(-PrivateCacheStatusOrphanGrace)
	for _, family := range []string{"resource", "analysis", "field"} {
		putOrphanStatusFixture(t, d, family, "old", "owner", "not_found", old, old)
		putOrphanStatusFixture(t, d, family, "fresh", "owner", "not_found", now, now)
		putOrphanStatusFixture(t, d, family, "other", "other", "not_found", old, old)
		putOrphanStatusFixture(t, d, family, "legacy", "", "not_found", old, old)
		for _, state := range []string{"pending", "running"} {
			putOrphanStatusFixture(t, d, family, state, "owner", state, old, old)
		}
	}
	for _, family := range []string{"resource", "analysis"} {
		putOrphanStatusFixture(t, d, family, "cooldown", "owner", "cooldown", old, now.Add(time.Hour))
		putOrphanStatusFixture(t, d, family, "retry-grace", "owner", "rate_limited", old, old.Add(time.Millisecond))
	}
	if got := privateSweep(t, d, now.Add(-time.Millisecond), 128); got.StatusCollected != 0 || got.Collected != 0 {
		t.Fatal("status grace shortened", got)
	}
	if got := privateSweep(t, d, now, 2); got.StatusCollected != 2 || got.Collected != 0 {
		t.Fatal("status batch bound", got)
	}
	if got := privateSweep(t, d, now, 128); got.StatusCollected != 1 {
		t.Fatal("wrong orphan status selection", got)
	}
	if got := privateSweep(t, d, now.Add(time.Millisecond), 128); got.StatusCollected != 2 {
		t.Fatal("retry grace boundary", got)
	}
	// Payload and status families share one limit rather than independent budgets.
	putCollectionFixture(t, d, "track", referenceID, "old-payload", "", `{}`)
	agePrivateFixtures(t, d, now.Add(-40*24*time.Hour))
	putOrphanStatusFixture(t, d, "resource", "remaining", "owner", "not_found", old, old)
	if got := privateSweep(t, d, now, 1); got.Collected+got.StatusCollected != 1 {
		t.Fatal("combined bound", got)
	}
	if got := privateSweep(t, d, now, 1); got.Collected+got.StatusCollected != 1 {
		t.Fatal("second bounded pass", got)
	}
}

func TestPrivateStatusRetentionPinsReservationAndRollback(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-40 * 24 * time.Hour)
	for _, family := range []string{"resource", "analysis", "field"} {
		putOrphanStatusFixture(t, d, family, referenceID, "owner", "not_found", old, old)
	}
	queueLineage(t, d, "active", nil)
	if got := privateSweep(t, d, now, 128); got.StatusCollected != 0 {
		t.Fatal("active download status lost", got)
	}
	// A missing child has no payload, but its failure still belongs to the active graph.
	const album = "AAAAAAAAAAAAAAAAAAAAAA"
	putCollectionFixture(t, d, "track", referenceID, "graph", "", `{}`, SpotifyEntityRelation{Kind: "album", Position: 0, ChildType: "album", ChildID: album})
	if err := d.PutSpotifyMetadataResourceStatus(SpotifySnapshotKey{EntityType: "album", SpotifyID: album, Resource: "detail", ContextKey: "owner"}, SpotifyMetadataResourceStatus{State: "failed", Reason: "fixture", CheckedAt: old, RetryAt: old}); err != nil {
		t.Fatal(err)
	}
	token, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.PutSpotifyMetadataResourceStatus(SpotifySnapshotKey{EntityType: "playlist", SpotifyID: collectionID, Resource: "page", ContextKey: "owner"}, SpotifyMetadataResourceStatus{State: "failed", Reason: "fixture", CheckedAt: old, RetryAt: old}); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, now, 128); got.StatusCollected != 0 {
		t.Fatal("graph or traversal status lost", got)
	}
	putOrphanStatusFixture(t, d, "resource", "orphan", "owner", "not_found", old, old)
	if _, err := d.ReserveSpotifyMetadataRuntime("retention-restart"); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, now, 128); got.StatusCollected != 0 {
		t.Fatal("pending owner data lost", got)
	}
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	putOrphanStatusFixture(t, d, "analysis", "rollback-orphan", "owner", "not_found", old, old)
	before, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_orphan_collection BEFORE DELETE ON spotify_metadata_resource_status BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if got, err := d.MaintainSpotifyPrivateCache(t.Context(), now, 128); err == nil || got.Collected != 0 || got.StatusCollected != 0 {
		t.Fatal("status rollback reported success", got, err)
	}
	after, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "owner")
	if err != nil || before.Rows != after.Rows {
		t.Fatal("earlier status delete escaped rollback", before, after, err)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER fail_orphan_collection`); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, now, 128); got.StatusCollected != 2 {
		t.Fatal("orphan not collected after rollback", got)
	}
	if _, err := d.conn.Exec(`UPDATE spotify_downloads SET status='completed'`); err != nil {
		t.Fatal(err)
	}
	if err := d.PutSpotifyPlaylistSnapshots(token, nil, true); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, now, 128); got.StatusCollected != 5 {
		t.Fatal("released work statuses retained", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.MaintainSpotifyPrivateCache(ctx, now, 1); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
}

func TestPrivateStatusRetentionKeepsPayloadEvidenceAndFuturePairedCooldown(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)
	old := now.Add(-40 * 24 * time.Hour)
	o := referenceObservation()
	o.AccountContext = "owner"
	o.RetrievedAt = old
	if err := d.PutExternalAnalysis(o, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	putOrphanStatusFixture(t, d, "resource", referenceID, "owner", "cooldown", old, now.Add(time.Hour))
	putOrphanStatusFixture(t, d, "analysis", referenceID, "owner", "rate_limited", old, now.Add(time.Hour))
	privateSweep(t, d, now, 128)
	for _, table := range []string{"spotify_audio_field_attempts", "spotify_metadata_resource_status", "external_track_analysis_status"} {
		var n int
		query := "SELECT COUNT(*) FROM " + table
		if table == "spotify_audio_field_attempts" {
			query += " WHERE field_key='tempo_bpm'"
		}
		if err := d.conn.QueryRow(query).Scan(&n); err != nil || n != 1 {
			t.Fatalf("payload-associated evidence lost: %s %d %v", table, n, err)
		}
	}
	// Now expire the payloads without changing their original check time. Paired
	// deletion must preserve future shared cooldowns, even on equal timestamps.
	putCollectionFixture(t, d, "track", referenceID, "audio_features", "", `{}`)
	for _, table := range []string{"spotify_entity_snapshots", "spotify_audio_observations", "external_track_analysis"} {
		if _, err := d.conn.Exec("UPDATE "+table+" SET retrieved_at=?,expires_at=?", old.UnixMilli(), old.Add(time.Hour).UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	if got := privateSweep(t, d, now, 128); got.Collected == 0 {
		t.Fatal("fixture payloads not collected", got)
	}
	for _, table := range []string{"spotify_metadata_resource_status", "external_track_analysis_status"} {
		if retentionCount(t, d, table) != 1 {
			t.Fatal("payload collection shortened cooldown", table)
		}
	}
	if got := privateSweep(t, d, now.Add(time.Hour+PrivateCacheStatusOrphanGrace), 128); got.StatusCollected != 2 {
		t.Fatal("settled cooldown orphans not collected", got)
	}
}
