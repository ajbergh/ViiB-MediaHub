package api

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify"
)

func TestIsRetriableDownloadError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("read: unexpected EOF"), true},
		{fmt.Errorf("dial: connection reset by peer"), true},
		{fmt.Errorf("request timeout"), true},
		{fmt.Errorf("permission denied"), false},
		{context.Canceled, false},
		{errors.New("crypto/aes: invalid key size 0"), true},
		{fmt.Errorf("pin: %w", spotify.ErrAudioKeyRejected), true},
		{errors.New("crypto/aes: invalid key size 15"), false},
	}
	for _, test := range tests {
		if got := isRetriableDownloadError(test.err); got != test.want {
			t.Errorf("isRetriableDownloadError(%q) = %v, want %v", test.err, got, test.want)
		}
	}
}

func TestDownloadStallTimeoutIsPhaseAware(t *testing.T) {
	now := time.Now()
	inactiveFor := StreamStallTimeout + time.Second

	streaming := downloadTracker{
		lastUpdateTime: now.Add(-inactiveFor),
		phase:          spotify.DownloadPhaseStreaming,
	}
	if !isDownloadStalled(streaming, now) {
		t.Fatal("inactive audio transfer should be considered stalled")
	}

	preparing := downloadTracker{
		lastUpdateTime: now.Add(-inactiveFor),
		phase:          spotify.DownloadPhasePreparing,
	}
	if isDownloadStalled(preparing, now) {
		t.Fatal("stream preparation should use the longer setup timeout")
	}

	preparing.lastUpdateTime = now.Add(-SetupStallTimeout - time.Second)
	if !isDownloadStalled(preparing, now) {
		t.Fatal("inactive stream preparation should eventually be considered stalled")
	}
}

func TestStalledDownloadAutomaticallyRestartsOnce(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	download := &db.SpotifyDownload{
		ID:         "stalled-download",
		SpotifyID:  "4iV5W9uYEdYUVa79Axb7Rh",
		SpotifyURI: "spotify:track:4iV5W9uYEdYUVa79Axb7Rh",
		Type:       "track",
		Title:      "Track",
		Status:     "queued",
		AddedAt:    1,
	}
	if err := database.AddDownload(download); err != nil {
		t.Fatalf("add download: %v", err)
	}
	if claimed, err := database.MarkDownloadStarted(download.ID); err != nil || !claimed {
		t.Fatalf("mark first attempt started: claimed=%v err=%v", claimed, err)
	}

	manager := NewDownloadManager(database, t.TempDir())
	firstCtx, firstCancel := context.WithCancel(context.Background())
	manager.activeDownloads[download.ID] = firstCancel
	manager.downloadProgress[download.ID] = downloadTracker{
		lastUpdateTime: time.Now().Add(-StreamStallTimeout - time.Second),
		phase:          spotify.DownloadPhaseStreaming,
		title:          download.Title,
	}
	manager.checkForStalledDownloads()

	select {
	case <-firstCtx.Done():
	default:
		t.Fatal("first stalled attempt was not cancelled")
	}
	if !manager.restartRequests[download.ID] || manager.stallRestarts[download.ID] != 1 {
		t.Fatalf("first stall was not scheduled for one restart: restart=%v attempts=%d",
			manager.restartRequests[download.ID], manager.stallRestarts[download.ID])
	}
	current, err := database.GetDownload(download.ID)
	if err != nil {
		t.Fatalf("read first stalled attempt: %v", err)
	}
	if current.Status != "downloading" {
		t.Fatalf("first stall should wait for worker cleanup before requeue: status=%q", current.Status)
	}

	// Simulate the worker's deferred restart cleanup and a second attempt.
	delete(manager.restartRequests, download.ID)
	delete(manager.activeDownloads, download.ID)
	if err := database.ResetDownloadForForceRestart(download.ID); err != nil {
		t.Fatalf("requeue first stalled attempt: %v", err)
	}
	if claimed, err := database.MarkDownloadStarted(download.ID); err != nil || !claimed {
		t.Fatalf("mark second attempt started: claimed=%v err=%v", claimed, err)
	}
	secondCtx, secondCancel := context.WithCancel(context.Background())
	manager.activeDownloads[download.ID] = secondCancel
	manager.downloadProgress[download.ID] = downloadTracker{
		lastUpdateTime: time.Now().Add(-StreamStallTimeout - time.Second),
		phase:          spotify.DownloadPhaseStreaming,
		title:          download.Title,
	}
	manager.checkForStalledDownloads()

	select {
	case <-secondCtx.Done():
	default:
		t.Fatal("repeated stalled attempt was not cancelled")
	}
	current, err = database.GetDownload(download.ID)
	if err != nil {
		t.Fatalf("read repeated stalled attempt: %v", err)
	}
	if current.Status != "failed" {
		t.Fatalf("repeated stall should fail: status=%q", current.Status)
	}
	event := <-manager.progressChan
	if event.Status != "failed" || !strings.Contains(event.Error, "audio transfer") {
		t.Fatalf("unexpected repeated-stall event: %#v", event)
	}
}

func TestQueueMutationsPublishCountRefreshEvent(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	manager := NewDownloadManager(database, t.TempDir())
	id, err := manager.QueueDownload(
		"4iV5W9uYEdYUVa79Axb7Rh",
		"spotify:track:4iV5W9uYEdYUVa79Axb7Rh",
		"track",
		"Track",
		"Artist",
		"Album",
		nil,
	)
	if err != nil {
		t.Fatalf("queue download: %v", err)
	}
	if event := <-manager.progressChan; event.Status != "queue_changed" {
		t.Fatalf("queue event status = %q, want queue_changed", event.Status)
	}

	count, err := manager.GetActiveQueueCount()
	if err != nil || count != 1 {
		t.Fatalf("active count after queue = %d, err=%v", count, err)
	}

	if err := manager.DeleteDownload(id); err != nil {
		t.Fatalf("delete download: %v", err)
	}
	if event := <-manager.progressChan; event.Status != "queue_changed" {
		t.Fatalf("delete event status = %q, want queue_changed", event.Status)
	}

	count, err = manager.GetActiveQueueCount()
	if err != nil || count != 0 {
		t.Fatalf("active count after delete = %d, err=%v", count, err)
	}
}
