package track

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"io"
	"sync"
	"testing"
	"time"
)

func TestProviderScalarsAreRetrievedBeforeLocalFallbackSelection(t *testing.T) {
	d, ids := runnerCatalog(t, 1)
	providerStarted := make(chan struct{})
	providerRelease := make(chan struct{})
	opened := make(chan struct{})
	var once sync.Once
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	opts := RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		s, e := analysis.ResolveLocalSource(d, id)
		open := s.Open
		s.OpenStream = func() (io.ReadCloser, error) { once.Do(func() { close(opened) }); return open() }
		return s, e
	}, SpotifyFeatures: func(ctx context.Context, source analysis.ResolvedSource) *spotifyanalysis.Observation {
		close(providerStarted)
		select {
		case <-providerRelease:
			return nil
		case <-ctx.Done():
			return nil
		}
	}}
	done := make(chan struct{})
	var progress RunProgress
	var runErr error
	go func() { progress, runErr = Run(ctx, d, analysis.NewDefaultDecoderRegistry(), ids, opts); close(done) }()
	select {
	case <-providerStarted:
	case <-ctx.Done():
		t.Fatal("provider lookup did not start")
	}
	select {
	case <-opened:
		t.Fatal("local decode began before Spotify field retrieval completed")
	default:
	}
	close(providerRelease)
	select {
	case <-opened:
	case <-ctx.Done():
		t.Fatal("local fallback did not start after provider retrieval failed")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("analysis did not finish")
	}
	if runErr != nil || progress.Analyzed != 1 || ctx.Err() != nil {
		t.Fatalf("fallback after provider failure: %+v %v", progress, runErr)
	}
}
