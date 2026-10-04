package api

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func awaitLifecycle(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("lifecycle operation timed out")
	}
}
func TestEnrichmentShutdownCancelsDrainsAndRejectsAdmission(t *testing.T) {
	a := &API{}
	ctx, finish, ok := a.beginEnrichment(context.Background())
	if !ok {
		t.Fatal("initial admission rejected")
	}
	closed := make(chan struct{})
	go func() { a.closeEnrichment(); close(closed) }()
	awaitLifecycle(t, ctx.Done())
	select {
	case <-closed:
		t.Fatal("shutdown returned before worker cleanup")
	default:
	}
	if _, _, admitted := a.beginEnrichment(context.Background()); admitted {
		t.Fatal("admitted job during shutdown")
	}
	finish()
	finish()
	awaitLifecycle(t, closed)
	a.closeEnrichment()
}
func TestEnrichmentRequestCancellationDoesNotCancelDetachedJob(t *testing.T) {
	a := &API{}
	parent, cancel := context.WithCancel(context.Background())
	requested, finishRequest, _ := a.beginEnrichment(parent)
	detached, finishDetached, _ := a.beginEnrichment(context.Background())
	cancel()
	awaitLifecycle(t, requested.Done())
	if detached.Err() != nil {
		t.Fatal("request cancellation stopped detached job")
	}
	finishRequest()
	closed := make(chan struct{})
	go func() { a.closeEnrichment(); close(closed) }()
	awaitLifecycle(t, detached.Done())
	finishDetached()
	awaitLifecycle(t, closed)
}

// Exercise the real provider and worker handler, rather than only the job tracker.
func TestUnifiedEnrichmentCancelsProviderAndDrainsOnShutdown(t *testing.T) {
	entered := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		entered <- struct{}{}
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	defer upstream.Close()
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for key, value := range map[string]string{"llm_provider": "ollama", "llm_model": "fixture", "llm_base_url": upstream.URL} {
		if err := database.SetSetting(key, value); err != nil {
			t.Fatal(err)
		}
	}
	song := db.Song{ID: "shutdown-song", FilePath: "/fixture/shutdown.flac", Title: "Shutdown", Artist: "Fixture", Album: "Fixture"}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	a := &API{db: database}
	returned := make(chan struct{})
	recorder := httptest.NewRecorder()
	go func() {
		a.enrichAllMetadataStream(recorder, httptest.NewRequest("GET", "/enrich", nil))
		close(returned)
	}()
	awaitLifecycle(t, entered)
	a.Close()
	awaitLifecycle(t, cancelled)
	awaitLifecycle(t, returned)
	a.Close()
	songs, err := database.GetAllSongs()
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 1 || songs[0].Mood != "" {
		t.Fatalf("cancelled enrichment changed songs: %#v", songs)
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("handler did not reach initial SSE event")
	}
}

func TestMoodEnrichmentContinuesAfterDisconnectUntilAPIShutdown(t *testing.T) {
	entered := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		entered <- struct{}{}
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	defer upstream.Close()
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for key, value := range map[string]string{"llm_provider": "ollama", "llm_model": "fixture", "llm_base_url": upstream.URL} {
		if err := database.SetSetting(key, value); err != nil {
			t.Fatal(err)
		}
	}
	song := db.Song{ID: "mood-shutdown", FilePath: "/fixture/mood.flac", Title: "Mood", Artist: "Fixture"}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	a := &API{db: database}
	parent, disconnect := context.WithCancel(context.Background())
	defer disconnect()
	returned := make(chan struct{})
	go func() {
		a.enrichMoodStream(httptest.NewRecorder(), httptest.NewRequest("GET", "/mood", nil).WithContext(parent))
		close(returned)
	}()
	awaitLifecycle(t, entered)
	disconnect()
	awaitLifecycle(t, returned)
	select {
	case <-cancelled:
		t.Fatal("client disconnect cancelled background mood provider")
	default:
	}
	a.Close()
	awaitLifecycle(t, cancelled)
	songs, err := database.GetAllSongs()
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 1 || songs[0].Mood != "" {
		t.Fatal("cancelled mood analysis committed results")
	}
}
