package refresh

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

const idA = "5r9W9MJLvHk83fcZSPQ8SE"
const idB = "11dFghVXANMlKmJXsNCbNl"

type fixtureProvider struct {
	calls atomic.Int32
	fn    func(context.Context, string, string) (analysis.Observation, error)
}

func (p *fixtureProvider) Fetch(ctx context.Context, id string) (analysis.Observation, error) {
	p.calls.Add(1)
	return p.fn(ctx, id, Detailed)
}
func (p *fixtureProvider) FetchFeatures(ctx context.Context, id string) (analysis.Observation, error) {
	p.calls.Add(1)
	return p.fn(ctx, id, Features)
}

type fixtureClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *fixtureClock) now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.at }
func (c *fixtureClock) advance(d time.Duration) { c.mu.Lock(); c.at = c.at.Add(d); c.mu.Unlock() }
func fixture(t *testing.T, fn func(context.Context, string, string) (analysis.Observation, error)) (*Service, *db.DB, *fixtureProvider, *fixtureClock) {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	clock := &fixtureClock{at: time.Unix(1700000000, 0).UTC()}
	provider := &fixtureProvider{fn: fn}
	service, err := New(database, provider, Options{Enabled: true, AdapterRevision: "fixture-v1", Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	return service, database, provider, clock
}
func observation(id, endpoint string, at time.Time) analysis.Observation {
	bpm := 108.022
	return analysis.Observation{TrackID: id, SourceEndpoint: endpoint, Source: "spotify_internal", RetrievedAt: at, BPM: &bpm}
}
func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !condition() {
		select {
		case <-timer.C:
			t.Fatal("timed out waiting for test barrier")
		case <-ticker.C:
		}
	}
}
func flightCount(s *Service) int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.flights) }
func TestDisabledAndFreshCacheDoNotFetch(t *testing.T) {
	disabled, err := New(nil, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer disabled.Close()
	if _, err = disabled.Refresh(context.Background(), idA, Features); err == nil {
		t.Fatal("disabled request accepted")
	}
	if disabled.Status().Configured {
		t.Fatal("disabled service configured")
	}
	var clock *fixtureClock
	service, database, provider, c := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		return observation(id, endpoint, clock.now()), nil
	})
	clock = c
	if provider.calls.Load() != 0 {
		t.Fatal("construction performed network work")
	}
	o := observation(idA, Features, c.now())
	if err = database.PutExternalAnalysis(o, "fixture", c.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	result, err := service.Refresh(context.Background(), idA, Features)
	if err != nil || result.Cache == nil || !result.FromCache || provider.calls.Load() != 0 {
		t.Fatalf("cache hit %+v %v", result, err)
	}
	if _, err = service.Refresh(context.Background(), "invalid", Features); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if _, err = service.Refresh(context.Background(), idA, "other"); !errors.Is(err, ErrInvalidEndpoint) {
		t.Fatal("invalid endpoint accepted")
	}
}
func TestCoalescingAndIndependentWaiterCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var clock *fixtureClock
	service, _, provider, c := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		close(started)
		select {
		case <-release:
			return observation(id, endpoint, clock.now()), nil
		case <-ctx.Done():
			return analysis.Observation{}, ctx.Err()
		}
	})
	clock = c
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := service.Refresh(ctx, idA, Features); first <- err }()
	<-started
	const others = 12
	results := make(chan error, others)
	for i := 0; i < others; i++ {
		go func() {
			result, err := service.Refresh(context.Background(), idA, Features)
			if err == nil && result.Cache == nil {
				err = errors.New("missing shared cache")
			}
			results <- err
		}()
	}
	waitFor(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		return service.flights[key{idA, Features}].waiters == others+1
	})
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first waiter %v", err)
	}
	close(release)
	for i := 0; i < others; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("calls=%d", provider.calls.Load())
	}
}
func TestLastWaiterCancellationPreventsPersistence(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	service, database, _, _ := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return observation(id, endpoint, time.Unix(1700000000, 0)), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := service.Refresh(ctx, idA, Features); done <- err }()
	<-started
	cancel()
	<-canceled
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return flightCount(service) == 0 })
	cache, err := database.GetExternalAnalysis(idA, Features)
	if err != nil || cache != nil {
		t.Fatal("canceled operation persisted data")
	}
	status, err := database.GetExternalAnalysisStatus(idA, Features)
	if err != nil || status != nil {
		t.Fatal("cancellation persisted provider failure")
	}
}
func TestGlobalSerializationAndNoEndpointFallback(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{}, 2)
	var clock *fixtureClock
	service, _, provider, c := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		started <- endpoint
		select {
		case <-release:
		case <-ctx.Done():
			return analysis.Observation{}, ctx.Err()
		}
		if endpoint == Detailed {
			return analysis.Observation{}, &analysis.Error{Code: analysis.NotFound}
		}
		return observation(id, endpoint, clock.now()), nil
	})
	clock = c
	done := make(chan Result, 2)
	go func() { result, _ := service.Refresh(context.Background(), idA, Detailed); done <- result }()
	if endpoint := <-started; endpoint != Detailed {
		t.Fatal(endpoint)
	}
	go func() { result, _ := service.Refresh(context.Background(), idB, Features); done <- result }()
	waitFor(t, func() bool { return flightCount(service) == 2 })
	if provider.calls.Load() != 1 {
		t.Fatal("parallel provider request")
	}
	release <- struct{}{}
	if endpoint := <-started; endpoint != Features {
		t.Fatal(endpoint)
	}
	release <- struct{}{}
	first, second := <-done, <-done
	if first.Failure == nil && second.Failure == nil {
		t.Fatal("missing detailed failure")
	}
	if provider.calls.Load() != 2 {
		t.Fatal("unexpected fallback call")
	}
}
func TestCooldownSurvivesRestartAndBlocksQueuedOtherRecording(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service, database, provider, clock := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		close(started)
		<-release
		return analysis.Observation{}, &analysis.Error{Code: analysis.RateLimited, RetryAfter: 2 * time.Hour}
	})
	results := make(chan Result, 2)
	go func() { result, _ := service.Refresh(context.Background(), idA, Features); results <- result }()
	<-started
	go func() { result, _ := service.Refresh(context.Background(), idB, Detailed); results <- result }()
	waitFor(t, func() bool { return flightCount(service) == 2 })
	close(release)
	for i := 0; i < 2; i++ {
		result := <-results
		if result.Failure == nil || result.Failure.Code != analysis.RateLimited ||
			!result.Failure.RetryAt.Equal(clock.now().Add(2*time.Hour)) {
			t.Fatalf("cooldown %+v", result)
		}
	}
	if provider.calls.Load() != 1 {
		t.Fatal("queued request ignored provider cooldown")
	}
	service.Close()
	nextProvider := &fixtureProvider{fn: func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		return observation(id, endpoint, clock.now()), nil
	}}
	next, err := New(database, nextProvider, Options{Enabled: true, AdapterRevision: "next", Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	result, err := next.Refresh(context.Background(), idB, Features)
	if err != nil || result.Failure == nil || result.Failure.Code != analysis.RateLimited || nextProvider.calls.Load() != 0 {
		t.Fatal("restart bypassed global cooldown")
	}
	clock.advance(2*time.Hour + time.Second)
	result, err = next.Refresh(context.Background(), idB, Features)
	if err != nil || result.Cache == nil || result.Failure != nil || nextProvider.calls.Load() != 1 {
		t.Fatal("expired cooldown blocked fetch")
	}
}
func TestDisconnectFencesLateResponseAndClearsProviderOnce(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service, database, _, clock := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		close(started)
		<-release
		return observation(id, endpoint, time.Unix(1700000000, 0)), nil // intentionally ignores cancellation
	})
	var disconnects atomic.Int32
	service.opts.Disconnect = func() { disconnects.Add(1) }
	done := make(chan error, 1)
	go func() { _, err := service.Refresh(context.Background(), idA, Features); done <- err }()
	<-started
	service.Disconnect()
	service.Disconnect()
	close(release)
	err := <-done
	var typed *analysis.Error
	if !errors.As(err, &typed) || typed.Code != analysis.Disabled {
		t.Fatalf("late result: %v", err)
	}
	if disconnects.Load() != 1 {
		t.Fatal("provider disconnect count")
	}
	cache, err := database.GetExternalAnalysis(idA, Features)
	if err != nil || cache != nil {
		t.Fatal("disconnected service wrote late data")
	}
	if service.Status().Connected || service.Status().Configured {
		t.Fatal("disconnected status")
	}
	_ = clock
}
func TestFailurePreservesGoodCacheAndNotFoundTTL(t *testing.T) {
	service, database, provider, clock := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		return analysis.Observation{}, &analysis.Error{Code: analysis.NotFound}
	})
	old := observation(idA, Features, clock.now().Add(-2*time.Hour))
	if err := database.PutExternalAnalysis(old, "previous", clock.now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	result, err := service.Refresh(context.Background(), idA, Features)
	if err != nil || result.Cache == nil || result.Failure == nil || result.Failure.Code != analysis.NotFound {
		t.Fatalf("failure result %+v %v", result, err)
	}
	if !result.Failure.RetryAt.Equal(clock.now().Add(time.Hour)) {
		t.Fatal("not_found TTL")
	}
	result, err = service.Refresh(context.Background(), idA, Features)
	if err != nil || !result.FromCache || provider.calls.Load() != 1 {
		t.Fatal("negative cache bypassed")
	}
}
func TestMalformedAndArbitraryProviderResultsAreSafe(t *testing.T) {
	for _, kind := range []string{"identity", "error"} {
		t.Run(kind, func(t *testing.T) {
			service, database, _, clock := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
				if kind == "error" {
					return analysis.Observation{}, errors.New("secret-bearing upstream details")
				}
				return observation(idB, endpoint, time.Unix(1700000000, 0)), nil
			})
			result, err := service.Refresh(context.Background(), idA, Features)
			expected := analysis.ProviderChanged
			if kind == "error" {
				expected = analysis.TemporarilyUnavailable
			}
			if err != nil || result.Failure == nil || result.Failure.Code != expected {
				t.Fatalf("safe result %+v %v", result, err)
			}
			if cache, err := database.GetExternalAnalysis(idA, Features); err != nil || cache != nil {
				t.Fatal("invalid observation persisted")
			}
			_ = clock
		})
	}
}

func TestPendingDistinctRequestsAreBounded(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service, _, provider, _ := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		close(started)
		<-release
		return analysis.Observation{}, ctx.Err()
	})
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{}, maxPending)
	go func() { service.Refresh(ctx, idA, Features); done <- struct{}{} }()
	<-started
	for i := 1; i < maxPending; i++ {
		id := fmt.Sprintf("%022d", i)
		go func() { service.Refresh(ctx, id, Features); done <- struct{}{} }()
	}
	waitFor(t, func() bool { return flightCount(service) == maxPending })
	_, err := service.Refresh(context.Background(), idB, Detailed)
	var typed *analysis.Error
	if !errors.As(err, &typed) || typed.Code != analysis.TemporarilyUnavailable {
		t.Fatalf("pending bound %v", err)
	}
	if provider.calls.Load() != 1 {
		t.Fatal("queue exceeded provider concurrency")
	}
	cancel()
	for i := 0; i < maxPending; i++ {
		<-done
	}
	service.Disconnect()
}

func TestProviderSlotIsSharedAcrossSessionLifetimes(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	previous, _, _, _ := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		close(started)
		<-release
		return observation(id, endpoint, time.Unix(1700000000, 0)), nil
	})
	var clock *fixtureClock
	next, _, provider, c := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		return observation(id, endpoint, clock.now()), nil
	})
	clock = c
	oldDone := make(chan error, 1)
	go func() { _, err := previous.Refresh(context.Background(), idA, Features); oldDone <- err }()
	<-started
	previous.Disconnect()
	nextDone := make(chan error, 1)
	go func() { _, err := next.Refresh(context.Background(), idB, Features); nextDone <- err }()
	waitFor(t, func() bool { return flightCount(next) == 1 })
	if provider.calls.Load() != 0 {
		t.Fatal("new session overlapped retiring provider")
	}
	close(release)
	if err := <-oldDone; err == nil {
		t.Fatal("old session result accepted")
	}
	if err := <-nextDone; err != nil {
		t.Fatal(err)
	}
	if provider.calls.Load() != 1 {
		t.Fatal("new session not fetched")
	}
}

func TestDisabledAdapterDoesNotPersistFailure(t *testing.T) {
	service, database, _, _ := fixture(t, func(context.Context, string, string) (analysis.Observation, error) {
		return analysis.Observation{}, &analysis.Error{Code: analysis.Disabled}
	})
	_, err := service.Refresh(context.Background(), idA, Features)
	var typed *analysis.Error
	if !errors.As(err, &typed) || typed.Code != analysis.Disabled {
		t.Fatalf("disabled adapter: %v", err)
	}
	if status, err := database.GetExternalAnalysisStatus(idA, Features); err != nil || status != nil {
		t.Fatal("disabled adapter wrote failure cache")
	}
}

func TestCloseDrainsPreviouslyDisconnectedProvider(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	service, database, _, _ := fixture(t, func(ctx context.Context, id, endpoint string) (analysis.Observation, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release // simulate transport cleanup after acknowledging cancellation
		return observation(id, endpoint, time.Unix(1700000000, 0)), nil
	})
	result := make(chan error, 1)
	go func() { _, err := service.Refresh(context.Background(), idA, Features); result <- err }()
	<-started
	service.Disconnect()
	<-cancelled
	closed := make(chan struct{})
	go func() { service.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned before provider drained")
	default:
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not finish")
	}
	if err := <-result; err == nil {
		t.Fatal("accepted retired result")
	}
	cache, err := database.GetExternalAnalysis(idA, Features)
	if err != nil || cache != nil {
		t.Fatal("retired operation committed cache")
	}
	service.Close()
}
