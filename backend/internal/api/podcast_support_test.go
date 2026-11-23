package api

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestSupportBundleIncludesRedactedPodcastDiagnostics(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	database, err := db.New(filepath.Join(dataDir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	const (
		secretFeedURL     = "https://private-publisher.example.org/secret-feed.xml"
		secretShowTitle   = "Ultra Private Podcast Show"
		secretEpTitle     = "Confidential Episode Title"
		secretEnclosure   = "https://cdn.private-publisher.example.org/ep1.mp3?auth=secret-cdn-sig"
		secretBookmark    = "Confidential bookmark note"
		secretPIKey       = "pi-api-key-secret-value"
		secretPISecret    = "pi-api-secret-value"
	)

	if err := database.SetSetting("podcast_index_credentials", `{"apiKey":"`+secretPIKey+`","apiSecret":"`+secretPISecret+`"}`); err != nil {
		t.Fatal(err)
	}

	feedID, err := database.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:   secretFeedURL,
		Title: secretShowTitle,
		Episodes: []podcast.Episode{
			{
				GUID:          "secret-ep-guid",
				Title:         secretEpTitle,
				EnclosureURL:  secretEnclosure,
				EnclosureType: "audio/mpeg",
			},
			{
				GUID:          "secret-ep-guid-2",
				Title:         "Second Secret Episode",
				EnclosureURL:  "https://cdn.private-publisher.example.org/ep2.mp3",
				EnclosureType: "audio/mpeg",
			},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := database.ListPodcastEpisodes(ctx, feedID, 10)
	if err != nil || len(episodes) != 2 {
		t.Fatalf("ListPodcastEpisodes: %v %+v", err, episodes)
	}

	if err := database.AddToPodcastQueue(ctx, episodes[0].ID, "end"); err != nil {
		t.Fatal(err)
	}
	rawConn, err := sql.Open("sqlite", filepath.Join(dataDir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawConn.ExecContext(ctx, `INSERT INTO podcast_bookmarks(id, episode_id, position_ms, note, created_at) VALUES ('bm-secret', ?, 5000, ?, ?)`, episodes[0].ID, secretBookmark, time.Now().UnixMilli()); err != nil {
		_ = rawConn.Close()
		t.Fatal(err)
	}
	_ = rawConn.Close()

	if _, err := database.EnqueuePodcastDownload(ctx, episodes[0].ID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := database.ClaimPodcastDownload(ctx, "support-worker", time.Now()); err != nil || claimed != episodes[0].ID {
		t.Fatalf("claim 1: %q %v", claimed, err)
	}
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := database.CompletePodcastDownload(ctx, episodes[0].ID, "support-worker", episodes[0].ID+".audio", "audio/mpeg", digest, 2048); err != nil {
		t.Fatal(err)
	}
	if _, err := database.EnqueuePodcastDownload(ctx, episodes[1].ID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := database.ClaimPodcastDownload(ctx, "support-worker", time.Now()); err != nil || claimed != episodes[1].ID {
		t.Fatalf("claim 2: %q %v", claimed, err)
	}
	if err := database.FailPodcastDownload(ctx, episodes[1].ID, "support-worker", "download_too_large"); err != nil {
		t.Fatal(err)
	}
	if err := database.FailPodcastRefresh(ctx, feedID, "feed_unavailable", time.Now()); err != nil {
		t.Fatal(err)
	}

	a := &API{db: database, dataDir: dataDir}
	bundleBytes, _, err := a.CreateSupportBundle()
	if err != nil {
		t.Fatalf("CreateSupportBundle: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(bundleBytes), int64(len(bundleBytes)))
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string]string, len(reader.File))
	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		entries[f.Name] = string(b)
	}

	diagJSON, ok := entries["diagnostics.json"]
	if !ok {
		t.Fatal("missing diagnostics.json in support bundle")
	}

	var diag supportBundleDiagnostics
	if err := json.Unmarshal([]byte(diagJSON), &diag); err != nil {
		t.Fatalf("unmarshal diagnostics.json: %v", err)
	}
	if !diag.Integrations["podcast_index"] {
		t.Fatalf("expected integrationsConfigured.podcast_index == true, got %+v", diag.Integrations)
	}
	if diag.Podcast == nil {
		t.Fatal("expected non-nil Podcast diagnostics in support bundle")
	}
	if diag.Podcast.SubscribedCount != 1 || diag.Podcast.EpisodeCount != 2 ||
		diag.Podcast.QueueCount != 1 || diag.Podcast.BookmarkCount != 1 ||
		diag.Podcast.DownloadBytesUsed != 2048 {
		t.Fatalf("unexpected podcast diagnostic counters: %+v", diag.Podcast)
	}
	if diag.Podcast.DownloadsByStatus["completed"] != 1 || diag.Podcast.DownloadsByStatus["failed"] != 1 {
		t.Fatalf("unexpected DownloadsByStatus: %+v", diag.Podcast.DownloadsByStatus)
	}
	if diag.Podcast.FeedErrorsByCode["feed_unavailable"] != 1 {
		t.Fatalf("unexpected FeedErrorsByCode: %+v", diag.Podcast.FeedErrorsByCode)
	}
	if diag.Podcast.DownloadErrorsByCode["download_too_large"] != 1 {
		t.Fatalf("unexpected DownloadErrorsByCode: %+v", diag.Podcast.DownloadErrorsByCode)
	}

	for _, forbidden := range []string{
		secretFeedURL, secretShowTitle, secretEpTitle, "Second Secret Episode",
		secretEnclosure, "secret-cdn-sig", secretBookmark,
		secretPIKey, secretPISecret,
		feedID, episodes[0].ID, episodes[1].ID,
		".audio", ".part-", "podcasts/episodes", "podcasts\\episodes",
	} {
		for name, content := range entries {
			if strings.Contains(content, forbidden) {
				t.Fatalf("support bundle entry %s leaked forbidden string %q", name, forbidden)
			}
		}
	}
}

func TestSupportBundlePodcastDiagnosticsWithoutCredentials(t *testing.T) {
	t.Setenv("VIIB_PODCAST_INDEX_API_KEY", "")
	t.Setenv("VIIB_PODCAST_INDEX_API_SECRET", "")
	t.Setenv("PODCAST_INDEX_API_KEY", "")
	t.Setenv("PODCAST_INDEX_API_SECRET", "")
	dataDir := t.TempDir()
	database, err := db.New(filepath.Join(dataDir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	a := &API{db: database, dataDir: dataDir}
	bundleBytes, _, err := a.CreateSupportBundle()
	if err != nil {
		t.Fatalf("CreateSupportBundle: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(bundleBytes), int64(len(bundleBytes)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range reader.File {
		if f.Name != "diagnostics.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		var diag supportBundleDiagnostics
		if err := json.Unmarshal(b, &diag); err != nil {
			t.Fatal(err)
		}
		if diag.Integrations["podcast_index"] {
			t.Fatalf("expected podcast_index == false when unconfigured")
		}
		if diag.Podcast == nil || diag.Podcast.SubscribedCount != 0 {
			t.Fatalf("expected zeroed Podcast diagnostics on fresh DB, got %+v", diag.Podcast)
		}
		return
	}
	t.Fatal("diagnostics.json missing from support bundle")
}
