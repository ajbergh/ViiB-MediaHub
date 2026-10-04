package spotify

import (
	"testing"
	"time"
)

func TestClearCredentialsWaitsForMediaLeaseAndErasesBearer(t *testing.T) {
	manager := NewSessionManager("fixture-old-bearer", t.TempDir())
	manager.lastTokenUsed = "fixture-old-bearer"
	manager.initialized = true
	manager.useMu.RLock() // Equivalent to the lease held by AcquireSession.
	started, finished := make(chan struct{}), make(chan struct{})
	go func() { close(started); manager.ClearCredentials(); close(finished) }()
	<-started
	select {
	case <-finished:
		manager.useMu.RUnlock()
		t.Fatal("credentials retired while lease was active")
	case <-time.After(20 * time.Millisecond):
	}
	manager.useMu.RUnlock()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("retirement did not complete after release")
	}
	if manager.accessToken != "" || manager.lastTokenUsed != "" || manager.IsInitialized() {
		t.Fatal("old account credentials were retained")
	}
	manager.UpdateAccessToken("fixture-new-bearer")
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if manager.accessToken != "" {
		t.Fatal("close retained bearer")
	}
}
