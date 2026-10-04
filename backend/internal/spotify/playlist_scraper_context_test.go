package spotify

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type scraperTransport func(*http.Request) (*http.Response, error)

func (f scraperTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPlaylistScrapeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ScrapePlaylistContext(ctx, "37i9dQZF1DXcBWIGoYBM5M"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled request was not rejected", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	client := &http.Client{Transport: scraperTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "open.spotify.com" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("unexpected credential/origin")
		}
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	done := make(chan error, 1)
	go func() { _, err := scrapePlaylist(ctx, "37i9dQZF1DXcBWIGoYBM5M", client); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("scrape did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation cause lost", err)
		}
	case <-time.After(time.Second):
		t.Fatal("scrape continued after cancellation")
	}
}
