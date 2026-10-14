package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

func TestMain(m *testing.M) {
	// Explicit real-model tests live in internal/semantic. Ordinary API tests
	// must never download weights into every temporary application data dir.
	managedEmbeddingProvider = func(ctx context.Context, _ string, _ *http.Client, _ func(float64)) (semantic.EmbeddingProvider, error) {
		return nil, fmt.Errorf("model acquisition disabled in API unit tests")
	}
	os.Exit(m.Run())
}

func TestSemanticPreparationShutdownAndAdmission(t *testing.T) {
	api := &API{dataDir: t.TempDir()}
	done := make(chan struct{})
	go func() { api.initSemanticService(); close(done) }()
	deadline := time.Now().Add(time.Second)
	for {
		api.semanticMu.RLock()
		active := api.semanticInitializing
		api.semanticMu.RUnlock()
		if active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initialization did not start")
		}
		time.Sleep(time.Millisecond)
	}
	// A duplicate request cannot create another initializer or model download.
	api.initSemanticService()
	api.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel preparation retry")
	}
	api.initSemanticService() // Closed APIs reject new background workers.
	api.semanticMu.RLock()
	defer api.semanticMu.RUnlock()
	if !api.semanticClosed || api.semanticInitializing {
		t.Fatal("preparation remained active after shutdown")
	}
}
