package spotify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type poolTestReader struct{ *bytes.Reader }

func (*poolTestReader) Close() error { return nil }
func poolTestAsset(size int64, closes *atomic.Int32) *streamAsset {
	return &streamAsset{size: size, contentType: "audio/ogg", newReader: func() (io.ReadSeekCloser, error) { return &poolTestReader{bytes.NewReader([]byte("0123456789"))}, nil }, close: func() { closes.Add(1) }}
}
func TestStreamAssetReuseHasIndependentReaders(t *testing.T) {
	pool := newStreamAssetPool()
	defer pool.close()
	var loads, closes atomic.Int32
	load := func() (*streamAsset, error) { loads.Add(1); return poolTestAsset(10, &closes), nil }
	key := streamAssetKey{"track", "high", 1}
	first, releaseFirst, err := pool.acquire(context.Background(), key, load)
	if err != nil {
		t.Fatal(err)
	}
	second, releaseSecond, err := pool.acquire(context.Background(), key, load)
	if err != nil {
		t.Fatal(err)
	}
	r1, _ := first.newReader()
	r2, _ := second.newReader()
	if _, err := r1.Seek(5, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	a, b := make([]byte, 1), make([]byte, 1)
	r1.Read(a)
	r2.Read(b)
	if string(a) != "5" || string(b) != "0" || loads.Load() != 1 {
		t.Fatal("read positions or shared preparation incorrect")
	}
	r1.Close()
	releaseFirst()
	releaseFirst()
	if closes.Load() != 0 {
		t.Fatal("closing one reader closed another's asset")
	}
	r2.Close()
	releaseSecond()
	third, releaseThird, err := pool.acquire(context.Background(), key, load)
	if err != nil {
		t.Fatal(err)
	}
	if third != first || loads.Load() != 1 {
		t.Fatal("sequential range repeated preparation")
	}
	releaseThird()
	pool.close()
	if closes.Load() != 1 {
		t.Fatal("cached asset not closed exactly once")
	}
}
func TestStreamAssetPreparationCoalescesAndWaiterCanCancel(t *testing.T) {
	pool := newStreamAssetPool()
	defer pool.close()
	var loads, closes atomic.Int32
	entered, ready := make(chan struct{}), make(chan struct{})
	load := func() (*streamAsset, error) {
		loads.Add(1)
		close(entered)
		<-ready
		return poolTestAsset(10, &closes), nil
	}
	key := streamAssetKey{"track", "high", 1}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, release, err := pool.acquire(context.Background(), key, load)
		if err != nil {
			t.Error(err)
		} else {
			release()
		}
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, release, err := pool.acquire(ctx, key, load)
		if release != nil {
			release()
		}
		result <- err
	}()
	cancel()
	if err := <-result; err != context.Canceled {
		t.Fatalf("waiter error=%v", err)
	}
	close(ready)
	wg.Wait()
	_, release, err := pool.acquire(context.Background(), key, load)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if loads.Load() != 1 {
		t.Fatal("duplicate preparation")
	}
}
func TestStreamAssetGenerationQualityAndRetirement(t *testing.T) {
	pool := newStreamAssetPool()
	var loads, closes atomic.Int32
	load := func() (*streamAsset, error) { loads.Add(1); return poolTestAsset(10, &closes), nil }
	_, r1, _ := pool.acquire(context.Background(), streamAssetKey{"track", "high", 1}, load)
	_, r2, _ := pool.acquire(context.Background(), streamAssetKey{"track", "low", 1}, load)
	r2()
	_, r3, err := pool.acquire(context.Background(), streamAssetKey{"track", "high", 2}, load)
	if err != nil {
		t.Fatal(err)
	}
	if closes.Load() != 1 {
		t.Fatal("idle prior generation not invalidated")
	}
	r1()
	if closes.Load() != 2 {
		t.Fatal("old active generation not closed after readers drained")
	}
	r3()
	pool.close()
	if closes.Load() != 3 || loads.Load() != 3 {
		t.Fatal("quality/generation ownership incorrect")
	}
	if _, _, err := pool.acquire(context.Background(), streamAssetKey{"track", "high", 2}, load); err == nil {
		t.Fatal("retired pool reopened")
	}
}
func TestStreamAssetBoundsAndIdleExpiry(t *testing.T) {
	pool := newStreamAssetPool()
	pool.maxEntries = 1
	pool.maxBytes = 10
	pool.maxAssetBytes = 10
	pool.idleTTL = 25 * time.Millisecond
	defer pool.close()
	var closes atomic.Int32
	load := func() (*streamAsset, error) { return poolTestAsset(10, &closes), nil }
	_, release, _ := pool.acquire(context.Background(), streamAssetKey{"one", "high", 1}, load)
	release()
	_, release, _ = pool.acquire(context.Background(), streamAssetKey{"two", "high", 1}, load)
	if closes.Load() != 1 {
		t.Fatal("idle entry not evicted at budget")
	}
	release()
	deadline := time.After(time.Second)
	for closes.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("idle asset not expired")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	_, release, _ = pool.acquire(context.Background(), streamAssetKey{"large", "high", 1}, func() (*streamAsset, error) { return poolTestAsset(11, &closes), nil })
	release()
	if closes.Load() != 3 {
		t.Fatal("oversized asset retained idle")
	}
}
func TestStreamAssetClosingDuringPreparationReleasesAsset(t *testing.T) {
	pool := newStreamAssetPool()
	var closes atomic.Int32
	entered, ready := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, release, err := pool.acquire(context.Background(), streamAssetKey{"one", "high", 1}, func() (*streamAsset, error) { close(entered); <-ready; return poolTestAsset(10, &closes), nil })
		if release != nil {
			release()
		}
		done <- err
	}()
	<-entered
	pool.close()
	close(ready)
	if err := <-done; err == nil {
		t.Fatal("late prepared asset escaped retirement")
	}
	if closes.Load() != 1 {
		t.Fatal("late prepared asset leaked")
	}
}
func TestStreamAssetFailureAndDiscardCanBeRetried(t *testing.T) {
	pool := newStreamAssetPool()
	defer pool.close()
	key := streamAssetKey{"one", "high", 1}
	if _, _, err := pool.acquire(context.Background(), key, func() (*streamAsset, error) { return nil, fmt.Errorf("fixture failure") }); err == nil {
		t.Fatal("failed preparation accepted")
	}
	var closes atomic.Int32
	a, release, err := pool.acquire(context.Background(), key, func() (*streamAsset, error) { return poolTestAsset(10, &closes), nil })
	if err != nil {
		t.Fatal(err)
	}
	pool.discard(key, a)
	release()
	if closes.Load() != 1 {
		t.Fatal("failed asset remained cached")
	}
	_, release, err = pool.acquire(context.Background(), key, func() (*streamAsset, error) { return poolTestAsset(10, &closes), nil })
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestStreamAssetByteBudgetDoesNotEvictActiveReaders(t *testing.T) {
	pool := newStreamAssetPool()
	pool.maxEntries = 3
	pool.maxBytes = 15
	defer pool.close()
	var closes atomic.Int32
	load := func() (*streamAsset, error) { return poolTestAsset(10, &closes), nil }
	first, releaseFirst, err := pool.acquire(context.Background(), streamAssetKey{"one", "high", 1}, load)
	if err != nil {
		t.Fatal(err)
	}
	_, releaseSecond, err := pool.acquire(context.Background(), streamAssetKey{"two", "high", 1}, load)
	if err != nil {
		t.Fatal(err)
	}
	if closes.Load() != 0 {
		t.Fatal("budget eviction interrupted active readers")
	}
	releaseSecond()
	if closes.Load() != 1 {
		t.Fatal("new asset beyond byte budget retained idle")
	}
	same, releaseSame, err := pool.acquire(context.Background(), streamAssetKey{"one", "high", 1}, func() (*streamAsset, error) { t.Fatal("active cached asset prepared twice"); return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if same != first {
		t.Fatal("active asset was replaced")
	}
	releaseSame()
	releaseFirst()
	pool.close()
	if closes.Load() != 2 {
		t.Fatal("asset lifetime leaked")
	}
}
