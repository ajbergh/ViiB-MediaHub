package spotify

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

type streamAssetKey struct {
	track, quality string
	generation     uint64
}
type streamAsset struct {
	newReader   func() (io.ReadSeekCloser, error)
	close       func()
	size        int64
	contentType string
}
type streamAssetEntry struct {
	key       streamAssetKey
	ready     chan struct{}
	asset     *streamAsset
	err       error
	users     int // includes requests waiting for preparation
	reusable  bool
	idleAt    time.Time
	timer     *time.Timer
	closeOnce sync.Once
}

func (e *streamAssetEntry) close() {
	e.closeOnce.Do(func() {
		if e.asset != nil && e.asset.close != nil {
			e.asset.close()
		}
	})
}

type streamAssetPool struct {
	mu                      sync.Mutex
	entries                 map[streamAssetKey]*streamAssetEntry
	closed                  bool
	maxEntries              int
	maxBytes, maxAssetBytes int64
	idleTTL                 time.Duration
}

func newStreamAssetPool() *streamAssetPool {
	return &streamAssetPool{entries: map[streamAssetKey]*streamAssetEntry{}, maxEntries: 3, maxBytes: 32 << 20, maxAssetBytes: 16 << 20, idleTTL: 30 * time.Second}
}

// A request owns a session lease separately. Idle entries hold only a child
// asset context; session reset can close it without waiting for this pool.
func (p *streamAssetPool) acquire(ctx context.Context, key streamAssetKey, load func() (*streamAsset, error)) (*streamAsset, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, nil, fmt.Errorf("stream asset pool closed")
	}
	var retired []*streamAssetEntry
	for oldKey, e := range p.entries {
		if oldKey.generation != key.generation {
			delete(p.entries, oldKey)
			e.reusable = false
			if e.timer != nil {
				e.timer.Stop()
			}
			if e.users == 0 && e.asset != nil {
				retired = append(retired, e)
			}
		}
	}
	e, exists := p.entries[key]
	if exists {
		e.users++
		if e.timer != nil {
			e.timer.Stop()
			e.timer = nil
		}
	} else {
		e = &streamAssetEntry{key: key, ready: make(chan struct{}), users: 1, reusable: true}
		p.entries[key] = e
	}
	p.mu.Unlock()
	for _, old := range retired {
		old.close()
	}
	if !exists {
		a, err := load()
		p.mu.Lock()
		e.asset, e.err = a, err
		if p.closed || !e.reusable || err != nil || a == nil {
			e.reusable = false
			if p.entries[key] == e {
				delete(p.entries, key)
			}
			if err == nil {
				e.err = fmt.Errorf("stream asset retired during preparation")
			}
		} else if a.size <= 0 || a.size > p.maxAssetBytes {
			e.reusable = false
			delete(p.entries, key)
		} else {
			// Evict idle entries only. Active readers keep their own lifetime.
			for p.overBudgetLocked() {
				var oldest *streamAssetEntry
				for _, candidate := range p.entries {
					if candidate != e && candidate.users == 0 && candidate.asset != nil && (oldest == nil || candidate.idleAt.Before(oldest.idleAt)) {
						oldest = candidate
					}
				}
				if oldest == nil {
					e.reusable = false
					delete(p.entries, key)
					break
				}
				delete(p.entries, oldest.key)
				oldest.reusable = false
				if oldest.timer != nil {
					oldest.timer.Stop()
				}
				retired = append(retired, oldest)
			}
		}
		close(e.ready)
		p.mu.Unlock()
		for _, old := range retired {
			old.close()
		}
	}
	select {
	case <-ctx.Done():
		p.release(e)
		return nil, nil, ctx.Err()
	case <-e.ready:
	}
	p.mu.Lock()
	a, err := e.asset, e.err
	closed := p.closed
	p.mu.Unlock()
	if err != nil || closed {
		p.release(e)
		if err == nil {
			err = fmt.Errorf("stream asset pool closed")
		}
		return nil, nil, err
	}
	var once sync.Once
	return a, func() { once.Do(func() { p.release(e) }) }, nil
}
func (p *streamAssetPool) overBudgetLocked() bool {
	var count int
	var bytes int64
	for _, e := range p.entries {
		if e.asset != nil {
			count++
			bytes += e.asset.size
		}
	}
	return count > p.maxEntries || bytes > p.maxBytes
}
func (p *streamAssetPool) release(e *streamAssetEntry) {
	p.mu.Lock()
	e.users--
	shouldClose := e.users == 0 && !e.reusable && e.asset != nil
	if e.users == 0 && e.reusable && e.asset != nil {
		e.idleAt = time.Now()
		e.timer = time.AfterFunc(p.idleTTL, func() { p.expire(e) })
	}
	p.mu.Unlock()
	if shouldClose {
		e.close()
	}
}
func (p *streamAssetPool) expire(e *streamAssetEntry) {
	p.mu.Lock()
	shouldClose := e.users == 0 && p.entries[e.key] == e && time.Since(e.idleAt) >= p.idleTTL
	if shouldClose {
		delete(p.entries, e.key)
		e.reusable = false
	}
	p.mu.Unlock()
	if shouldClose {
		e.close()
	}
}
func (p *streamAssetPool) discard(key streamAssetKey, a *streamAsset) {
	p.mu.Lock()
	e := p.entries[key]
	if e != nil && e.asset == a {
		delete(p.entries, key)
		e.reusable = false
		if e.timer != nil {
			e.timer.Stop()
		}
	}
	p.mu.Unlock()
}
func (p *streamAssetPool) close() {
	p.mu.Lock()
	p.closed = true
	entries := p.entries
	p.entries = map[streamAssetKey]*streamAssetEntry{}
	for _, e := range entries {
		e.reusable = false
		if e.timer != nil {
			e.timer.Stop()
		}
	}
	p.mu.Unlock()
	for _, e := range entries {
		// A pending loader observes closed on publication and releases its asset.
		select {
		case <-e.ready:
			e.close()
		default:
		}
	}
}
