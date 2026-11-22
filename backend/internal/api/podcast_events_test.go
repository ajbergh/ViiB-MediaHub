package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/podcast"
	"github.com/go-chi/chi/v5"
)

type podcastEventPayload struct {
	Downloads []db.PodcastDownloadRecord `json:"downloads"`
	Storage   struct {
		UsedBytes int64 `json:"usedBytes"`
		MaxBytes  int64 `json:"maxBytes"`
	} `json:"storage"`
	RefreshCount int64 `json:"refreshCount"`
}

func TestPodcastEventsSSEStreamNotifyDedupAndCleanup(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "podcast-events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	feedID, err := database.SavePodcastFeedSnapshot(context.Background(), podcast.Feed{
		URL:   "https://publisher.example.org/rss.xml",
		Title: "Events Show",
		Episodes: []podcast.Episode{
			{
				GUID:          "ep-1",
				Title:         "Episode One",
				EnclosureURL:  "https://cdn.example.org/ep1.mp3?token=secret-enclosure-token",
				EnclosureType: "audio/mpeg",
			},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := database.ListPodcastEpisodes(context.Background(), feedID, 10)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("ListPodcastEpisodes: %v %+v", err, episodes)
	}
	epID := episodes[0].ID

	api := &API{db: database}
	router := chi.NewRouter()
	router.Get("/v2/podcasts/events", api.podcastEventsSSE)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// Record baseline goroutines after test server setup.
	runtime.GC()
	baselineGoroutines := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/v2/podcasts/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache, no-transform" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q", got)
	}

	type sseFrame struct {
		event string
		data  string
	}
	frames := make(chan sseFrame, 16)
	scannerDone := make(chan struct{})
	go func() {
		defer close(scannerDone)
		scanner := bufio.NewScanner(resp.Body)
		var curEvent string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				curEvent = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				frames <- sseFrame{event: curEvent, data: strings.TrimPrefix(line, "data: ")}
			}
		}
	}()

	// 1. Initial snapshot arrives immediately.
	var initial sseFrame
	select {
	case initial = <-frames:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial podcast SSE snapshot")
	}
	if initial.event != "podcasts" {
		t.Fatalf("initial event = %q, want podcasts", initial.event)
	}
	var payload1 podcastEventPayload
	if err := json.Unmarshal([]byte(initial.data), &payload1); err != nil {
		t.Fatalf("unmarshal initial payload: %v (%s)", err, initial.data)
	}
	if len(payload1.Downloads) != 0 || payload1.Storage.UsedBytes != 0 ||
		payload1.Storage.MaxBytes != db.MaxPodcastOfflineLibraryBytes || payload1.RefreshCount != 0 {
		t.Fatalf("unexpected initial snapshot: %+v", payload1)
	}
	if got := api.podcastEventSubscriberCount(); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}

	// 2. Notification without state change is deduplicated.
	api.notifyPodcastEvent()
	select {
	case extra := <-frames:
		t.Fatalf("unexpected duplicate SSE frame: %+v", extra)
	case <-time.After(100 * time.Millisecond):
	}

	// 3. Mutate download state and notify -> updated snapshot streams promptly.
	if _, err := database.EnqueuePodcastDownload(context.Background(), epID); err != nil {
		t.Fatal(err)
	}
	api.notifyPodcastEvent()

	var updated sseFrame
	select {
	case updated = <-frames:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notified podcast SSE snapshot")
	}
	if updated.event != "podcasts" {
		t.Fatalf("updated event = %q, want podcasts", updated.event)
	}
	var payload2 podcastEventPayload
	if err := json.Unmarshal([]byte(updated.data), &payload2); err != nil {
		t.Fatalf("unmarshal updated payload: %v", err)
	}
	if len(payload2.Downloads) != 1 || payload2.Downloads[0].EpisodeID != epID || payload2.Downloads[0].Status != "queued" {
		t.Fatalf("unexpected updated downloads: %+v", payload2.Downloads)
	}
	for _, forbidden := range []string{"publisher.example.org", "cdn.example.org", "secret-enclosure-token", ".audio", ".part-"} {
		if strings.Contains(updated.data, forbidden) {
			t.Fatalf("SSE payload leaked %q: %s", forbidden, updated.data)
		}
	}

	// 4. Cancel request context and verify clean unsubscribe without goroutine leaks.
	cancel()
	<-scannerDone

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if api.podcastEventSubscriberCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := api.podcastEventSubscriberCount(); got != 0 {
		t.Fatalf("subscriber still registered after context cancel: count=%d", got)
	}

	// Wait briefly for server handler goroutine to unwind.
	var finalGoroutines int
	for i := 0; i < 40; i++ {
		runtime.Gosched()
		finalGoroutines = runtime.NumGoroutine()
		if finalGoroutines <= baselineGoroutines+1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if finalGoroutines > baselineGoroutines+1 {
		t.Fatalf("potential goroutine leak: baseline=%d final=%d", baselineGoroutines, finalGoroutines)
	}
}
