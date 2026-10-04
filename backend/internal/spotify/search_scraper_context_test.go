package spotify

import (
	"context"
	"errors"
	"testing"
)

func TestCanceledPlaylistSearchDoesNotStartBrowser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := SearchPlaylistsContext(ctx, "fixture")
	if result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled search: %v", err)
	}
}
