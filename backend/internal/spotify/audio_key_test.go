package spotify

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsAudioKeyRejected(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("failed to decrypt aes cipher: crypto/aes: invalid key size 0"), true},
		{errors.New("failed retrieving aes key with code 1"), true},
		{fmt.Errorf("pin failed: %w", ErrAudioKeyRejected), true},
		{errors.New("crypto/aes: invalid key size 15"), false},
		{errors.New("connection reset by peer"), false},
		{nil, false},
	}

	for _, test := range tests {
		if got := IsAudioKeyRejected(test.err); got != test.want {
			t.Errorf("IsAudioKeyRejected(%v) = %v, want %v", test.err, got, test.want)
		}
	}
}

func TestAudioKeyRequestsAreSerializedAndCancelable(t *testing.T) {
	manager := NewSessionManager("token", t.TempDir())
	releaseFirst, err := manager.acquireAudioKeyRequest(context.Background())
	if err != nil {
		t.Fatalf("acquire first request: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.acquireAudioKeyRequest(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked request error = %v, want context canceled", err)
	}

	releaseFirst()
	releaseSecond, err := manager.acquireAudioKeyRequest(context.Background())
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	releaseSecond()
}

func TestResetSessionIfGenerationCoalescesRecovery(t *testing.T) {
	manager := NewSessionManager("token", t.TempDir())
	manager.mu.Lock()
	manager.initialized = true
	manager.generation = 7
	manager.mu.Unlock()

	if manager.ResetSessionIfGeneration(6) {
		t.Fatal("stale generation reset the current session")
	}
	if !manager.IsInitialized() {
		t.Fatal("stale generation changed session state")
	}

	if !manager.ResetSessionIfGeneration(7) {
		t.Fatal("current generation was not reset")
	}
	if manager.IsInitialized() {
		t.Fatal("current generation remained initialized after reset")
	}
	if manager.ResetSessionIfGeneration(7) {
		t.Fatal("same recovery reset an already-cleared session twice")
	}
}
