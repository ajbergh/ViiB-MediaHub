// v2_jobs_pressure.go carries the DJ playback signal that reduces background
// analysis pressure. Playback lives in the frontend audio graph, so the backend
// cannot observe it and has to be told.
package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// analysisForegroundPriority is the priority at or above which a job is treated
// as an explicit "analyze this now" request and ignores playback pressure. A DJ
// who asks for a track to be analyzed during a set means it.
const analysisForegroundPriority = 50

// defaultPressureTTL bounds how long one report suppresses analysis. The
// frontend renews it while playback continues, so a closed or crashed UI cannot
// pause the library queue forever.
const defaultPressureTTL = 30 * time.Second

// maxPressureTTL keeps a caller from parking analysis indefinitely with a
// single request.
const maxPressureTTL = 10 * time.Minute

// playbackPressure is a TTL-guarded flag rather than a plain boolean precisely
// so that a missing "playback stopped" message cannot strand the queue.
type playbackPressure struct {
	mu    sync.Mutex
	until time.Time
}

func (p *playbackPressure) report(ttl time.Duration) {
	if ttl <= 0 {
		ttl = defaultPressureTTL
	}
	if ttl > maxPressureTTL {
		ttl = maxPressureTTL
	}
	p.mu.Lock()
	p.until = time.Now().Add(ttl)
	p.mu.Unlock()
}

func (p *playbackPressure) clear() {
	p.mu.Lock()
	p.until = time.Time{}
	p.mu.Unlock()
}

func (p *playbackPressure) active() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Now().Before(p.until)
}

// remaining reports how long the current report still suppresses analysis.
func (p *playbackPressure) remaining() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	remaining := time.Until(p.until)
	if remaining < 0 {
		return 0
	}
	return remaining
}

type analysisPressureRequest struct {
	Active     bool `json:"active"`
	TTLSeconds int  `json:"ttlSeconds,omitempty"`
}

// analysisPressureV2 records or clears the playback signal. It is a hint, not a
// lock: analysis correctness never depends on it.
func (a *API) analysisPressureV2(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJobRequestBytes)
	var request analysisPressureRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_request", "The analysis pressure request is not valid JSON", false, nil)
		return
	}
	if request.Active {
		a.analysisPressure.report(time.Duration(request.TTLSeconds) * time.Second)
	} else {
		a.analysisPressure.clear()
		// Deferred work should resume as soon as playback stops rather than
		// waiting out its backoff or the dispatcher's idle poll.
		_, _ = a.db.ClearJobBackoff()
		a.wakeJobScheduler()
	}
	respondV2JSON(w, http.StatusAccepted, map[string]any{
		"active":           a.analysisPressure.active(),
		"remainingSeconds": int(a.analysisPressure.remaining().Seconds()),
	})
}

// analysisThrottled reports whether a job of the given priority must yield to
// active playback.
func (a *API) analysisThrottled(priority int) bool {
	if priority >= analysisForegroundPriority {
		return false
	}
	return a.analysisPressure.active()
}
