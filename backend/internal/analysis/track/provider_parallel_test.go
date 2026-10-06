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

func TestProviderLookupDoesNotBlockLocalDecode(t *testing.T) {
	d, ids := runnerCatalog(t, 1)
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
		select {
		case <-opened:
			return nil
		case <-ctx.Done():
			return nil
		}
	}}
	progress, err := Run(ctx, d, analysis.NewDefaultDecoderRegistry(), ids, opts)
	if err != nil || progress.Analyzed != 1 || ctx.Err() != nil {
		t.Fatalf("provider serialized decode: %+v %v", progress, err)
	}
}
