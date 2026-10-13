package db

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSpotifyPrivateCacheUsageIsolationAndLifecycle(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	collectionPages(t, d)
	queueLineage(t, d, "pending", collectionOrigins())
	if _, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID); err != nil {
		t.Fatal(err)
	}
	o := detailedObservation(t, `{"track":{"tempo":120,"duration":10},"sections":[{"start":0,"duration":10}]}`)
	o.AccountContext = "owner"
	if err := d.PutExternalAnalysis(o, "fixture", o.RetrievedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	key := SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "waveform", ContextKey: "owner"}
	now := time.Now()
	if err := d.PutSpotifyMetadataResourceStatus(key, SpotifyMetadataResourceStatus{State: "failed", Reason: "fixture", CheckedAt: now, RetryAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`INSERT INTO external_track_analysis_status(provider,external_id,endpoint,schema_version,code,checked_at,retry_at,account_context) VALUES('spotify',?,'audio_analysis',1,'not_found',1,2,'owner')`, referenceID); err != nil {
		t.Fatal(err)
	}
	before, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Tables) != 10 || before.PayloadBytes <= 0 || before.DecodedPayloadBytes <= 0 {
		t.Fatal(before)
	}
	var rows, stored, decoded int64
	for _, table := range before.Tables {
		var count int64
		if err := d.conn.QueryRow("SELECT COUNT(*) FROM " + table.Table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if table.Rows != count || count == 0 {
			t.Fatalf("family missing or miscounted: %+v actual %d", table, count)
		}
		rows += table.Rows
		stored += table.PayloadBytes
		decoded += table.DecodedPayloadBytes
	}
	if rows != before.Rows || stored != before.PayloadBytes || decoded != before.DecodedPayloadBytes {
		t.Fatal("aggregate mismatch")
	}
	// Byte accounting must count UTF-8 bytes, and must not decompress artifacts.
	if _, err := d.conn.Exec(`UPDATE spotify_audio_observations SET value_json='"é"'; UPDATE spotify_audio_artifacts SET payload=x'0102',decoded_size=101`); err != nil {
		t.Fatal(err)
	}
	measured, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range measured.Tables {
		if table.Table == "spotify_audio_observations" && table.PayloadBytes != 4*table.Rows {
			t.Fatal("text character count used as byte count", table)
		}
		if table.Table == "spotify_audio_artifacts" && (table.PayloadBytes != 2*table.Rows || table.DecodedPayloadBytes != 101*table.Rows) {
			t.Fatal("artifact accounting", table)
		}
	}
	if _, err := d.conn.Exec(`INSERT INTO spotify_entity_snapshots(entity_type,spotify_id,resource,context_key,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,capture_revision) SELECT entity_type,spotify_id,resource,'other',schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,capture_revision FROM spotify_entity_snapshots`); err != nil {
		t.Fatal(err)
	}
	other, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "other")
	if err != nil || other.Rows != 5 || other.PayloadBytes <= 0 {
		t.Fatal("second context not counted", other, err)
	}
	for _, other := range []string{"absent", "owner' OR 1=1 --", ""} {
		got, err := d.GetSpotifyPrivateCacheUsage(t.Context(), other)
		if err != nil || got.Rows != 0 {
			t.Fatal("context leaked", other, got, err)
		}
	}
	// Final-file promotion and exact unlink choices are outside these diagnostics.
	finishEvidence(t, d, path, "completed", "11dFghVXANMlKmJXsNCbNl")
	after, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "owner")
	if err != nil || !reflect.DeepEqual(measured, after) {
		t.Fatal("durable evidence changed private accounting", after, err)
	}
	if err := d.RetireSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	retired, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range retired.Tables {
		if strings.HasPrefix(table.Table, "spotify_entity_") || table.Table == "spotify_audio_artifacts" || table.Table == "spotify_metadata_resource_status" || table.Table == "spotify_playlist_traversals" || table.Table == "spotify_download_lineage_staging" {
			if table.Rows != 0 {
				t.Fatal("domain retirement retained rows", table)
			}
		}
	}
	// Runtime retirement composes domain retirement with compatibility-cache purge.
	if err := d.PurgeExternalAnalysis(); err != nil {
		t.Fatal(err)
	}
	if retentionCount(t, d, "spotify_download_evidence") == 0 {
		t.Fatal("private purge removed durable evidence")
	}
	empty, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "owner")
	if err != nil || empty.Rows != 0 || empty.PayloadBytes != 0 || empty.DecodedPayloadBytes != 0 {
		t.Fatal(empty, err)
	}
}

func TestSpotifyPrivateCacheUsageLegacyAndCancellation(t *testing.T) {
	d, _ := evidenceFixture(t)
	if _, err := d.conn.Exec(`INSERT INTO external_track_analysis_status(provider,external_id,endpoint,schema_version,code,checked_at,retry_at,account_context) VALUES('spotify',?,'audio_analysis',1,'not_found',1,2,'')`, referenceID); err != nil {
		t.Fatal(err)
	}
	legacy, err := d.GetSpotifyPrivateCacheUsage(t.Context(), "")
	if err != nil || legacy.Rows != 1 {
		t.Fatal(legacy, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.GetSpotifyPrivateCacheUsage(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	if _, err := d.GetSpotifyPrivateCacheUsage(t.Context(), strings.Repeat("a", 257)); err == nil {
		t.Fatal("oversized context accepted")
	}
}
