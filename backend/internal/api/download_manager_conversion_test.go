// Tests download conversion failures and successful output handling.
package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/audio"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify"
)

func TestConvertDownloadedOggIfEnabled(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	manager := NewDownloadManager(database, t.TempDir())
	download := &db.SpotifyDownload{
		ID:     "download-id",
		Title:  "Track title",
		Artist: "Track artist",
		Album:  "Track album",
	}
	bpm := 109.724
	metadata := &spotify.DownloadMetadata{
		BPM: &bpm, InitialKey: "Am",
		AlbumArtist: "Album artist",
		TrackNumber: 4,
		DiscNumber:  2,
		ReleaseDate: "2025-06-07",
		Genre:       "Electronic",
	}

	called := false
	manager.oggToMP3Converter = func(context.Context, string, audio.MP3Metadata, audio.ActivityCallback) (string, error) {
		called = true
		return "", errors.New("converter must remain disabled")
	}
	got, err := manager.convertDownloadedOggIfEnabled(context.Background(), download, metadata, "track.ogg")
	if err != nil {
		t.Fatalf("disabled conversion returned an error: %v", err)
	}
	if got != "track.ogg" {
		t.Fatalf("disabled conversion path = %q, want track.ogg", got)
	}
	if called {
		t.Fatal("converter was called while setting was disabled")
	}

	if err := database.SetSetting("spotify_auto_convert_ogg_to_mp3", "true"); err != nil {
		t.Fatalf("enable conversion: %v", err)
	}
	manager.oggToMP3Converter = func(_ context.Context, path string, tags audio.MP3Metadata, activity audio.ActivityCallback) (string, error) {
		called = true
		if path != "track.ogg" {
			t.Errorf("converter path = %q, want track.ogg", path)
		}
		want := (audio.MP3Metadata{
			BPM: &bpm, InitialKey: "Am",
			Title:       "Track title",
			Artist:      "Track artist",
			Album:       "Track album",
			AlbumArtist: "Album artist",
			TrackNumber: 4,
			DiscNumber:  2,
			Date:        "2025-06-07",
			Genre:       "Electronic",
		})
		if tags != want {
			t.Errorf("converter metadata = %#v, want %#v", tags, want)
		}
		activity()
		return "track.mp3", nil
	}
	called = false
	got, err = manager.convertDownloadedOggIfEnabled(context.Background(), download, metadata, "track.ogg")
	if err != nil {
		t.Fatalf("enabled conversion returned an error: %v", err)
	}
	if got != "track.mp3" {
		t.Fatalf("enabled conversion path = %q, want track.mp3", got)
	}
	if !called {
		t.Fatal("converter was not called while setting was enabled")
	}
}

func TestConvertDownloadedOggIfEnabledPreservesPlaylistTags(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if err := database.SetSetting("spotify_auto_convert_ogg_to_mp3", "true"); err != nil {
		t.Fatalf("enable conversion: %v", err)
	}

	manager := NewDownloadManager(database, t.TempDir())
	download := &db.SpotifyDownload{ID: "download-id", Title: "Title", Artist: "Artist", Album: "Original album"}
	metadata := &spotify.DownloadMetadata{PlaylistName: "Road trip", PlaylistOrder: 12}
	manager.oggToMP3Converter = func(_ context.Context, _ string, tags audio.MP3Metadata, _ audio.ActivityCallback) (string, error) {
		if tags.Album != "Road trip" || tags.AlbumArtist != "Road trip" || tags.TrackNumber != 12 || tags.DiscNumber != 1 {
			t.Fatalf("playlist tags were not preserved: %#v", tags)
		}
		return "track.mp3", nil
	}

	if _, err := manager.convertDownloadedOggIfEnabled(context.Background(), download, metadata, "track.ogg"); err != nil {
		t.Fatalf("convert playlist download: %v", err)
	}
}

func TestConvertDownloadedOggIfEnabledReturnsConverterError(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if err := database.SetSetting("spotify_auto_convert_ogg_to_mp3", "true"); err != nil {
		t.Fatalf("enable conversion: %v", err)
	}

	manager := NewDownloadManager(database, t.TempDir())
	converterErr := errors.New("encode failed")
	manager.oggToMP3Converter = func(context.Context, string, audio.MP3Metadata, audio.ActivityCallback) (string, error) {
		return "", converterErr
	}
	_, err = manager.convertDownloadedOggIfEnabled(context.Background(), &db.SpotifyDownload{ID: "download-id"}, nil, "track.ogg")
	if !errors.Is(err, converterErr) {
		t.Fatalf("converter error = %v", err)
	}
}

func TestOggConversionDoesNotOccupyDownloadSlot(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if err := database.SetSetting("spotify_auto_convert_ogg_to_mp3", "true"); err != nil {
		t.Fatalf("enable conversion: %v", err)
	}

	manager := NewDownloadManager(database, t.TempDir())
	defer manager.cancel()
	conversionStarted := make(chan struct{})
	releaseConversion := make(chan struct{})
	manager.oggToMP3Converter = func(ctx context.Context, _ string, _ audio.MP3Metadata, _ audio.ActivityCallback) (string, error) {
		close(conversionStarted)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-releaseConversion:
			path := filepath.Join(t.TempDir(), "first.mp3")
			if err := os.WriteFile(path, []byte("converted fixture"), 0600); err != nil {
				return "", err
			}
			return path, nil
		}
	}

	first := &db.SpotifyDownload{
		ID: "first", SpotifyID: "5r9W9MJLvHk83fcZSPQ8SE", SpotifyURI: "spotify:track:first",
		Type: "track", Title: "First", Status: "queued", AddedAt: 1,
	}
	second := &db.SpotifyDownload{
		ID: "second", SpotifyID: "spotify-second", SpotifyURI: "spotify:track:second",
		Type: "track", Title: "Second", Status: "queued", AddedAt: 2,
	}
	for _, download := range []*db.SpotifyDownload{first, second} {
		if err := database.AddDownload(download); err != nil {
			t.Fatalf("add %s: %v", download.ID, err)
		}
	}
	if changed, err := database.MarkDownloadStarted(first.ID); err != nil || !changed {
		t.Fatalf("mark first started: changed=%v err=%v", changed, err)
	}

	if err := manager.startOggConversion(context.Background(), first, nil, "first.ogg"); err != nil {
		t.Fatalf("start conversion: %v", err)
	}
	select {
	case <-conversionStarted:
	case <-time.After(time.Second):
		t.Fatal("conversion did not start")
	}

	converting, err := database.GetDownload(first.ID)
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	if converting.Status != "converting" {
		t.Fatalf("first status = %q, want converting", converting.Status)
	}
	if manager.GetActiveDownloadCount() != 0 {
		t.Fatalf("conversion consumed a download slot: active=%d", manager.GetActiveDownloadCount())
	}

	manager.dispatchDownloads()
	select {
	case job := <-manager.workChan:
		if job.download.ID != second.ID {
			t.Fatalf("dispatched %q, want %q", job.download.ID, second.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("next queued download was not dispatched while conversion was active")
	}

	close(releaseConversion)
	manager.conversionWg.Wait()
	completed, err := database.GetDownload(first.ID)
	if err != nil {
		t.Fatalf("get completed first: %v", err)
	}
	if completed.Status != "completed" || filepath.Base(completed.FilePath) != "first.mp3" {
		t.Fatalf("completed first = %#v", completed)
	}
}

func TestOggConversionsRunInParallelUpToConfiguredLimit(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	manager := NewDownloadManager(database, t.TempDir())
	defer manager.cancel()
	if err := manager.SetMaxConversionWorkers(2); err != nil {
		t.Fatalf("set conversion workers: %v", err)
	}
	if got := manager.GetMaxConversionWorkers(); got != 2 {
		t.Fatalf("conversion workers = %d, want 2", got)
	}

	started := make(chan string, 3)
	release := make(chan struct{}, 3)
	manager.oggToMP3Converter = func(ctx context.Context, path string, _ audio.MP3Metadata, _ audio.ActivityCallback) (string, error) {
		started <- path
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-release:
			final := filepath.Join(t.TempDir(), filepath.Base(path)+".mp3")
			if err := os.WriteFile(final, []byte("converted fixture"), 0600); err != nil {
				return "", err
			}
			return final, nil
		}
	}

	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("conversion-%d", i)
		download := &db.SpotifyDownload{
			ID: id, SpotifyID: fmt.Sprintf("%022d", i), SpotifyURI: "spotify:track:" + id,
			Type: "track", Title: id, Status: "queued", AddedAt: int64(i),
		}
		if err := database.AddDownload(download); err != nil {
			t.Fatalf("add %s: %v", id, err)
		}
		if changed, err := database.MarkDownloadStarted(id); err != nil || !changed {
			t.Fatalf("mark %s started: changed=%v err=%v", id, changed, err)
		}
		if err := manager.startOggConversion(context.Background(), download, nil, id+".ogg"); err != nil {
			t.Fatalf("start %s conversion: %v", id, err)
		}
	}

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("two conversions did not begin in parallel")
		}
	}
	select {
	case path := <-started:
		t.Fatalf("third conversion %q exceeded configured worker limit", path)
	case <-time.After(150 * time.Millisecond):
	}
	if manager.GetActiveDownloadCount() != 0 {
		t.Fatalf("parallel conversions consumed download slots: active=%d", manager.GetActiveDownloadCount())
	}

	release <- struct{}{}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("third conversion did not begin after a worker became available")
	}
	release <- struct{}{}
	release <- struct{}{}

	done := make(chan struct{})
	go func() {
		manager.conversionWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("conversions did not finish")
	}

	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("conversion-%d", i)
		download, err := database.GetDownload(id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if download.Status != "completed" {
			t.Fatalf("%s status = %q, want completed", id, download.Status)
		}
	}
}

func TestSetMaxConversionWorkersRejectsOutOfRangeValues(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "downloads.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	manager := NewDownloadManager(database, t.TempDir())

	for _, workers := range []int{MinConversionWorkers - 1, MaxConversionWorkers + 1} {
		if err := manager.SetMaxConversionWorkers(workers); err == nil {
			t.Fatalf("SetMaxConversionWorkers(%d) succeeded", workers)
		}
	}
}

func TestConversionDetachesWorkerButRetainsOriginalAccount(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	manager := NewDownloadManager(a.db, t.TempDir())
	defer manager.cancel()
	manager.tokenSource = a.spotifyAuth
	job, cancelJob := a.spotifyAuth.requestContext(context.Background())
	defer cancelJob()
	conversion, cancelConversion := manager.conversionContext(job)
	defer cancelConversion()
	cancelJob()
	if conversion.Err() != nil {
		t.Fatal("worker cleanup cancelled transferred conversion")
	}
	a.spotifyAuth.beginRetirement()
	select {
	case <-conversion.Done():
	case <-time.After(time.Second):
		t.Fatal("account retirement did not cancel conversion")
	}
	if !errors.Is(context.Cause(conversion), errSpotifyAccountChanged) {
		t.Fatalf("wrong cancellation cause: %v", context.Cause(conversion))
	}
}

func TestAccountRetirementRequeuesActiveConversion(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	manager := NewDownloadManager(a.db, t.TempDir())
	defer manager.cancel()
	manager.tokenSource = a.spotifyAuth
	job, cancelJob := a.spotifyAuth.requestContext(context.Background())
	defer cancelJob()
	started := make(chan struct{})
	manager.oggToMP3Converter = func(ctx context.Context, _ string, _ audio.MP3Metadata, _ audio.ActivityCallback) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}
	download := &db.SpotifyDownload{ID: "retire-conversion", SpotifyID: "5r9W9MJLvHk83fcZSPQ8SE", Type: "track", Title: "Fixture", Status: "queued", AddedAt: 1}
	if err := a.db.AddDownload(download); err != nil {
		t.Fatal(err)
	}
	if changed, err := a.db.MarkDownloadStarted(download.ID); err != nil || !changed {
		t.Fatalf("start: %v %v", changed, err)
	}
	if err := manager.startOggConversion(job, download, nil, "fixture.ogg"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("conversion did not start")
	}
	cancelJob()
	a.spotifyAuth.beginRetirement()
	done := make(chan struct{})
	go func() { manager.conversionWg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("conversion did not stop")
	}
	row, err := a.db.GetDownload(download.ID)
	if err != nil || row.Status != "queued" {
		t.Fatalf("retired conversion state: %#v %v", row, err)
	}
}
