package db

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastDiagnosticsFreshDB(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "fresh-podcast-diag.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	diag, err := d.PodcastDiagnostics(context.Background())
	if err != nil {
		t.Fatalf("PodcastDiagnostics on fresh DB: %v", err)
	}
	if diag.SubscribedCount != 0 || diag.EpisodeCount != 0 || diag.QueueCount != 0 ||
		diag.BookmarkCount != 0 || diag.DownloadBytesUsed != 0 {
		t.Fatalf("expected zeroed counts, got %+v", diag)
	}
	if diag.DownloadsByStatus == nil || diag.FeedErrorsByCode == nil || diag.DownloadErrorsByCode == nil {
		t.Fatalf("expected non-nil maps, got %+v", diag)
	}
	active, err := d.CountActivePodcastRefreshes(context.Background())
	if err != nil || active != 0 {
		t.Fatalf("CountActivePodcastRefreshes = %d, %v", active, err)
	}
}

func TestPodcastDiagnosticsAggregatesAndRedacts(t *testing.T) {
	ctx := context.Background()
	d, err := New(filepath.Join(t.TempDir(), "podcast-diag.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	secretFeedURL := "https://publisher.example.org/private-rss.xml"
	secretShowTitle := "Sensitive Show Title"
	secretEp1Title := "Sensitive Episode One"
	secretEp2Title := "Sensitive Episode Two"
	secretEnclosure := "https://cdn.example.org/audio/ep1.mp3?token=secret-enclosure-token"

	feedID1, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:   secretFeedURL,
		Title: secretShowTitle,
		Episodes: []podcast.Episode{
			{GUID: "ep-1", Title: secretEp1Title, EnclosureURL: secretEnclosure, EnclosureType: "audio/mpeg"},
			{GUID: "ep-2", Title: secretEp2Title, EnclosureURL: "https://cdn.example.org/audio/ep2.mp3", EnclosureType: "audio/mpeg"},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	feedID2, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:   "https://second.example.org/rss.xml",
		Title: "Second Show",
		Episodes: []podcast.Episode{
			{GUID: "ep-3", Title: "Episode Three", EnclosureURL: "https://cdn.example.org/audio/ep3.mp3", EnclosureType: "audio/mpeg"},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	// Unsubscribed feed still exists in podcast_feeds/podcast_episodes.
	unsubID, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:   "https://unsub.example.org/rss.xml",
		Title: "Unsubscribed Show",
		Episodes: []podcast.Episode{
			{GUID: "ep-4", Title: "Episode Four", EnclosureURL: "https://cdn.example.org/audio/ep4.mp3", EnclosureType: "audio/mpeg"},
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	eps1, err := d.ListPodcastEpisodes(ctx, feedID1, 10)
	if err != nil || len(eps1) != 2 {
		t.Fatalf("ListPodcastEpisodes feed1: %v %+v", err, eps1)
	}
	eps2, err := d.ListPodcastEpisodes(ctx, feedID2, 10)
	if err != nil || len(eps2) != 1 {
		t.Fatalf("ListPodcastEpisodes feed2: %v %+v", err, eps2)
	}

	// Queue + bookmark rows.
	if _, err := d.conn.ExecContext(ctx, `INSERT INTO podcast_queue(episode_id, sort_key, added_at) VALUES (?, 1.0, ?)`, eps1[0].ID, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.ExecContext(ctx, `INSERT INTO podcast_bookmarks(id, episode_id, position_ms, note, created_at) VALUES ('bm-1', ?, 12000, 'Secret Bookmark Note', ?)`, eps1[0].ID, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}

	// Download states: 1 completed (1024 bytes), 1 failed (download_too_large).
	if _, err := d.EnqueuePodcastDownload(ctx, eps1[0].ID); err != nil {
		t.Fatal(err)
	}
	claimed, err := d.ClaimPodcastDownload(ctx, "diag-worker", time.Now())
	if err != nil || claimed != eps1[0].ID {
		t.Fatalf("claim: %q %v", claimed, err)
	}
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := d.CompletePodcastDownload(ctx, eps1[0].ID, "diag-worker", eps1[0].ID+".audio", "audio/mpeg", digest, 1024); err != nil {
		t.Fatal(err)
	}
	if _, err := d.EnqueuePodcastDownload(ctx, eps1[1].ID); err != nil {
		t.Fatal(err)
	}
	claimed2, err := d.ClaimPodcastDownload(ctx, "diag-worker", time.Now())
	if err != nil || claimed2 != eps1[1].ID {
		t.Fatalf("claim2: %q %v", claimed2, err)
	}
	if err := d.FailPodcastDownload(ctx, eps1[1].ID, "diag-worker", "download_too_large"); err != nil {
		t.Fatal(err)
	}

	// Feed errors: one standard error code, one non-allowlisted raw string that must be sanitized.
	if err := d.FailPodcastRefresh(ctx, feedID1, "feed_unavailable", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.ExecContext(ctx, `UPDATE podcast_feeds SET last_error_code = ? WHERE id = ?`, "https://leak.example.org/secret?key=1", feedID2); err != nil {
		t.Fatal(err)
	}

	// Active refresh lease on subscribed feed1 and unsubscribed feed.
	futureLease := time.Now().Add(2 * time.Minute).UnixMilli()
	if _, err := d.conn.ExecContext(ctx, `UPDATE podcast_feeds SET refresh_lease_until = ? WHERE id IN (?, ?)`, futureLease, feedID1, unsubID); err != nil {
		t.Fatal(err)
	}

	active, err := d.CountActivePodcastRefreshes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("CountActivePodcastRefreshes = %d, want 1 (only subscribed feeds)", active)
	}

	diag, err := d.PodcastDiagnostics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if diag.SubscribedCount != 2 {
		t.Fatalf("SubscribedCount = %d, want 2", diag.SubscribedCount)
	}
	if diag.EpisodeCount != 4 {
		t.Fatalf("EpisodeCount = %d, want 4", diag.EpisodeCount)
	}
	if diag.QueueCount != 1 {
		t.Fatalf("QueueCount = %d, want 1", diag.QueueCount)
	}
	if diag.BookmarkCount != 1 {
		t.Fatalf("BookmarkCount = %d, want 1", diag.BookmarkCount)
	}
	if diag.DownloadBytesUsed != 1024 {
		t.Fatalf("DownloadBytesUsed = %d, want 1024", diag.DownloadBytesUsed)
	}
	if diag.DownloadsByStatus["completed"] != 1 || diag.DownloadsByStatus["failed"] != 1 {
		t.Fatalf("DownloadsByStatus = %+v", diag.DownloadsByStatus)
	}
	if diag.FeedErrorsByCode["feed_unavailable"] != 1 || diag.FeedErrorsByCode["other"] != 1 {
		t.Fatalf("FeedErrorsByCode = %+v", diag.FeedErrorsByCode)
	}
	if diag.DownloadErrorsByCode["download_too_large"] != 1 {
		t.Fatalf("DownloadErrorsByCode = %+v", diag.DownloadErrorsByCode)
	}

	encoded, err := json.Marshal(diag)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(encoded)
	for _, forbidden := range []string{
		secretFeedURL, secretShowTitle, secretEp1Title, secretEp2Title,
		secretEnclosure, "secret-enclosure-token", "Secret Bookmark Note",
		"leak.example.org", feedID1, feedID2, eps1[0].ID, eps1[1].ID, ".audio",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("diagnostics JSON leaked %q: %s", forbidden, raw)
		}
	}
}
