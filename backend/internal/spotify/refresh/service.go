// Package refresh coordinates explicit, optional Spotify reference requests.
// Construction performs no remote work; no credentials are read from settings.
package refresh

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

const (
	Features   = "audio_features"
	Detailed   = "audio_analysis"
	maxPending = 32
)

var ErrInvalidEndpoint = errors.New("invalid Spotify reference endpoint")
var ErrStorage = errors.New("spotify reference storage unavailable")

// Shared across session lifetimes, including a retiring provider whose HTTP
// operation has not yet acknowledged cancellation.
var providerGate = make(chan struct{}, 1)

type Provider interface {
	Fetch(context.Context, string) (analysis.Observation, error)
	FetchFeatures(context.Context, string) (analysis.Observation, error)
}
type Store interface {
	GetExternalAnalysis(string, string) (*db.ExternalAnalysisCache, error)
	GetExternalAnalysisStatus(string, string) (*db.ExternalAnalysisStatus, error)
	GetExternalAnalysisCooldown() (time.Time, error)
	PutExternalAnalysis(analysis.Observation, string, time.Time) error
	PutExternalAnalysisStatus(string, string, db.ExternalAnalysisStatus) error
}
type Options struct {
	Enabled         bool
	AdapterRevision string
	SuccessTTL      time.Duration
	UnavailableTTL  time.Duration
	Now             func() time.Time
	// Disconnect clears the owning in-memory token/session provider, if present.
	Disconnect func()
}
type Result struct {
	Cache     *db.ExternalAnalysisCache  `json:"cache"`
	Failure   *db.ExternalAnalysisStatus `json:"failure"`
	FromCache bool                       `json:"fromCache"`
}
type Status struct {
	State      string     `json:"state"`
	Configured bool       `json:"configured"`
	Connected  bool       `json:"connected"`
	CacheOnly  bool       `json:"cacheOnly"`
	RetryAt    *time.Time `json:"retryAt,omitempty"`
}
type key struct{ id, endpoint string }
type flight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	result  Result
	err     error
}
type Service struct {
	workers    sync.WaitGroup
	mu         sync.Mutex
	store      Store
	provider   Provider
	opts       Options
	now        func() time.Time
	root       context.Context
	cancel     context.CancelFunc
	flights    map[key]*flight
	closed     bool
	generation uint64
	cooldown   time.Time
	connected  bool
	state      string
}

// New is inert even when enabled. Each service represents one session lifetime;
// reconnect requires a new instance and an explicit account-cache policy.
func New(store Store, provider Provider, opts Options) (*Service, error) {
	if opts.Enabled && (store == nil || provider == nil || opts.AdapterRevision == "" || len(opts.AdapterRevision) > 256) {
		return nil, errors.New("enabled Spotify reference service requires store, provider and revision")
	}
	if opts.SuccessTTL == 0 {
		opts.SuccessTTL = 7 * 24 * time.Hour
	}
	if opts.UnavailableTTL == 0 {
		opts.UnavailableTTL = time.Hour
	}
	if opts.SuccessTTL <= 0 || opts.UnavailableTTL <= 0 {
		return nil, errors.New("reference cache TTL must be positive")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	root, cancel := context.WithCancel(context.Background())
	return &Service{store: store, provider: provider, opts: opts, now: now, root: root, cancel: cancel,
		flights: make(map[key]*flight), generation: 1, state: "ready"}, nil
}
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := Status{State: s.state, Configured: s.opts.Enabled && !s.closed, Connected: s.connected, CacheOnly: !s.opts.Enabled || s.closed}
	if !s.opts.Enabled || s.closed {
		status.State = "disabled"
		return status
	}
	if s.cooldown.After(s.now()) {
		at := s.cooldown
		status.State = string(analysis.RateLimited)
		status.RetryAt = &at
	}
	return status
}

// Disconnect fences writes and cancels queued/active requests before returning.
// The mutex is held through persistence, so generation changes cannot race a
// database commit. Existing completed cache entries are purged by the owner.
func (s *Service) Disconnect() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.generation++
	s.connected = false
	s.cancel()
	callback := s.opts.Disconnect
	s.opts.Disconnect = nil
	s.mu.Unlock()
	if callback != nil {
		callback()
	}
}

// Close also drains provider operations; Disconnect only fences and cancels.
func (s *Service) Close() { s.Disconnect(); s.workers.Wait() }

// Refresh honors a fresh cache and persisted cooldowns; it is not a force/bulk
// fetch API. Waiters share work but independently cancel. The last departing
// waiter cancels the operation. Queued distinct keys and operation time are bounded.
func (s *Service) Refresh(ctx context.Context, id, endpoint string) (Result, error) {
	if !db.ValidSpotifyRecordingID(id) {
		return Result{}, &analysis.Error{Code: analysis.InvalidTrackID}
	}
	if endpoint != Features && endpoint != Detailed {
		return Result{}, ErrInvalidEndpoint
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	s.mu.Lock()
	if s.closed || !s.opts.Enabled {
		s.mu.Unlock()
		return Result{}, &analysis.Error{Code: analysis.Disabled}
	}
	k := key{id, endpoint}
	if f := s.flights[k]; f != nil {
		f.waiters++
		s.mu.Unlock()
		return s.wait(ctx, k, f)
	}
	if len(s.flights) >= maxPending {
		s.mu.Unlock()
		return Result{}, &analysis.Error{Code: analysis.TemporarilyUnavailable, RetryAfter: time.Second}
	}
	workRoot := s.root
	if background, _ := ctx.Value(backgroundKey{}).(bool); background {
		workRoot = BackgroundContext(workRoot)
	}
	workCtx, cancel := context.WithTimeout(workRoot, 20*time.Second)
	f := &flight{done: make(chan struct{}), cancel: cancel, waiters: 1}
	s.flights[k] = f
	generation := s.generation
	s.workers.Add(1)
	s.mu.Unlock()
	go s.run(workCtx, k, f, generation)
	return s.wait(ctx, k, f)
}
func (s *Service) wait(ctx context.Context, k key, f *flight) (Result, error) {
	select {
	case <-f.done:
		return f.result, f.err
	case <-ctx.Done():
		s.mu.Lock()
		if s.flights[k] == f {
			f.waiters--
			if f.waiters == 0 {
				f.cancel()
			}
		}
		s.mu.Unlock()
		return Result{}, ctx.Err()
	}
}
func (s *Service) run(ctx context.Context, k key, f *flight, generation uint64) {
	defer s.workers.Done()
	result, err := s.execute(ctx, k, generation)
	f.cancel()
	s.mu.Lock()
	if s.closed || s.generation != generation {
		result = Result{}
		err = &analysis.Error{Code: analysis.Disabled}
	}
	f.result, f.err = result, err
	delete(s.flights, k)
	close(f.done)
	s.mu.Unlock()
}
func (s *Service) execute(ctx context.Context, k key, generation uint64) (Result, error) {
	// Admission precedes cooldown checks so queued work observes a prior 429.
	release, err := AcquireProviderSlot(ctx)
	if err != nil {
		return Result{}, err
	}
	defer release()
	s.mu.Lock()
	if s.closed || s.generation != generation || ctx.Err() != nil {
		s.mu.Unlock()
		return Result{}, context.Canceled
	}
	cache, err := s.store.GetExternalAnalysis(k.id, k.endpoint)
	if err != nil {
		s.mu.Unlock()
		return Result{}, ErrStorage
	}
	status, err := s.store.GetExternalAnalysisStatus(k.id, k.endpoint)
	if err != nil {
		s.mu.Unlock()
		return Result{}, ErrStorage
	}
	persistedCooldown, err := s.store.GetExternalAnalysisCooldown()
	if err != nil {
		s.mu.Unlock()
		return Result{}, ErrStorage
	}
	if persistedCooldown.After(s.cooldown) {
		s.cooldown = persistedCooldown
	}
	now := s.now()
	if status != nil && cache != nil && !status.CheckedAt.After(cache.Observation.RetrievedAt) {
		status = nil
	}
	result := Result{Cache: cache, Failure: status, FromCache: true}
	if cache != nil && now.Before(cache.ExpiresAt) {
		s.mu.Unlock()
		return result, nil
	}
	if now.Before(s.cooldown) {
		result.Failure = &db.ExternalAnalysisStatus{Code: analysis.RateLimited, CheckedAt: now, RetryAt: s.cooldown}
		s.mu.Unlock()
		return result, nil
	}
	if status != nil && now.Before(status.RetryAt) {
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()
	var observation analysis.Observation
	if k.endpoint == Features {
		observation, err = s.provider.FetchFeatures(ctx, k.id)
	} else {
		observation, err = s.provider.Fetch(ctx, k.id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.generation != generation {
		return Result{}, &analysis.Error{Code: analysis.Disabled}
	}
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	now = s.now()
	if err == nil && (observation.TrackID != k.id || observation.SourceEndpoint != k.endpoint ||
		observation.Source != "spotify_internal" || observation.RetrievedAt.IsZero() || observation.RetrievedAt.After(now.Add(time.Second)) ||
		analysis.ValidateObservation(observation) != nil) {
		err = &analysis.Error{Code: analysis.ProviderChanged}
	}
	if err == nil {
		if s.store.PutExternalAnalysis(observation, s.opts.AdapterRevision, observation.RetrievedAt.Add(s.opts.SuccessTTL)) != nil {
			return Result{}, ErrStorage
		}
		updated, readErr := s.store.GetExternalAnalysis(k.id, k.endpoint)
		if readErr != nil {
			return Result{}, ErrStorage
		}
		s.connected = true
		s.state = "available"
		return Result{Cache: updated}, nil
	}
	var providerError *analysis.Error
	if errors.As(err, &providerError) && providerError.Code == analysis.Disabled {
		return Result{}, &analysis.Error{Code: analysis.Disabled}
	}
	safe := safeFailure(err)
	delay := time.Minute
	if safe.Code == analysis.NotFound || safe.Code == analysis.AnalysisUnavailable {
		delay = s.opts.UnavailableTTL
	}
	if safe.RetryAfter > delay {
		delay = safe.RetryAfter
	}
	retryAt := now.Add(delay)
	if safe.Code == analysis.RateLimited && retryAt.After(s.cooldown) {
		s.cooldown = retryAt
	}
	failure := db.ExternalAnalysisStatus{Code: safe.Code, CheckedAt: now, RetryAt: retryAt}
	if s.store.PutExternalAnalysisStatus(k.id, k.endpoint, failure) != nil {
		return Result{}, ErrStorage
	}
	s.state = string(safe.Code)
	if safe.Code == analysis.AuthenticationRequired {
		s.connected = false
	}
	return Result{Cache: cache, Failure: &failure}, nil
}
func safeFailure(err error) *analysis.Error {
	var typed *analysis.Error
	if errors.As(err, &typed) {
		switch typed.Code {
		case analysis.AuthenticationRequired, analysis.AccessDenied, analysis.NotFound, analysis.AnalysisUnavailable,
			analysis.RateLimited, analysis.TemporarilyUnavailable, analysis.ProviderChanged:
			return &analysis.Error{Code: typed.Code, RetryAfter: typed.RetryAfter}
		}
	}
	return &analysis.Error{Code: analysis.TemporarilyUnavailable}
}
