package db

import (
	"path/filepath"
	"testing"
)

func TestSpotifyDownloadConditionalTransitions(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	download := &SpotifyDownload{
		ID: "download-1", SpotifyID: "4iV5W9uYEdYUVa79Axb7Rh",
		SpotifyURI: "spotify:track:4iV5W9uYEdYUVa79Axb7Rh", Type: "track",
		Title: "Track", Status: "queued", AddedAt: 1,
	}
	if err := database.AddDownload(download); err != nil {
		t.Fatalf("add download: %v", err)
	}
	claimed, err := database.MarkDownloadStarted(download.ID)
	if err != nil || !claimed {
		t.Fatalf("claim download: claimed=%v err=%v", claimed, err)
	}
	if claimedAgain, err := database.MarkDownloadStarted(download.ID); err != nil || claimedAgain {
		t.Fatalf("second claim should fail conditionally: claimed=%v err=%v", claimedAgain, err)
	}
	if err := database.ResetDownloadForForceRestart(download.ID); err != nil {
		t.Fatalf("force reset: %v", err)
	}
	if changed, err := database.MarkDownloadFailed(download.ID, "stale worker"); err != nil || changed {
		t.Fatalf("stale failure overwrote queued state: changed=%v err=%v", changed, err)
	}
}

func TestSpotifyDownloadBatchDeduplicatesActiveEntries(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	first := &SpotifyDownload{ID: "one", SpotifyID: "4iV5W9uYEdYUVa79Axb7Rh", SpotifyURI: "spotify:track:4iV5W9uYEdYUVa79Axb7Rh", Type: "track", Title: "Track", Status: "queued", AddedAt: 1, Metadata: "{}"}
	second := *first
	second.ID = "two"
	ids, err := database.AddDownloads([]*SpotifyDownload{first, &second})
	if err != nil {
		t.Fatalf("add batch: %v", err)
	}
	if len(ids) != 2 || ids[0] != "one" || ids[1] != "one" {
		t.Fatalf("unexpected deduplicated IDs: %v", ids)
	}
	queued, err := database.GetQueuedDownloads(10)
	if err != nil {
		t.Fatalf("get queued downloads: %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("queued rows = %d, want 1", len(queued))
	}
}

func TestCountActiveDownloadsIncludesQueuedDownloadingAndConverting(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	statuses := []string{"queued", "downloading", "converting", "completed", "failed"}
	for i, status := range statuses {
		download := &SpotifyDownload{
			ID:         status,
			SpotifyID:  "spotify-" + status,
			SpotifyURI: "spotify:track:" + status,
			Type:       "track",
			Title:      status,
			Status:     status,
			AddedAt:    int64(i + 1),
		}
		if err := database.AddDownload(download); err != nil {
			t.Fatalf("add %s download: %v", status, err)
		}
	}

	count, err := database.CountActiveDownloads()
	if err != nil {
		t.Fatalf("count active downloads: %v", err)
	}
	if count != 3 {
		t.Fatalf("active downloads = %d, want 3", count)
	}
}

func TestSpotifyDownloadConversionTransitions(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	download := &SpotifyDownload{
		ID: "conversion", SpotifyID: "spotify-conversion",
		SpotifyURI: "spotify:track:conversion", Type: "track",
		Title: "Track", Status: "queued", AddedAt: 1,
	}
	if err := database.AddDownload(download); err != nil {
		t.Fatalf("add download: %v", err)
	}
	if changed, err := database.MarkDownloadStarted(download.ID); err != nil || !changed {
		t.Fatalf("mark started: changed=%v err=%v", changed, err)
	}
	if changed, err := database.MarkDownloadConverting(download.ID, "track.ogg"); err != nil || !changed {
		t.Fatalf("mark converting: changed=%v err=%v", changed, err)
	}
	got, err := database.GetDownload(download.ID)
	if err != nil {
		t.Fatalf("get converting download: %v", err)
	}
	if got.Status != "converting" || got.Progress != 100 || got.FilePath != "track.ogg" {
		t.Fatalf("converting download = %#v", got)
	}
	if changed, err := database.MarkDownloadCompleted(download.ID, "track.mp3"); err != nil || !changed {
		t.Fatalf("mark completed: changed=%v err=%v", changed, err)
	}
}
