// podcast_events.go provides an in-process SSE broadcaster for podcast
// download and refresh state snapshots without mutating the API struct.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const (
	podcastEventsFallbackPoll      = 5 * time.Second
	podcastEventsHeartbeatInterval = 15 * time.Second
)

type podcastEventBroker struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

var podcastEventBrokers sync.Map // map[*API]*podcastEventBroker

func (a *API) podcastBroker() *podcastEventBroker {
	if a == nil {
		return nil
	}
	if existing, ok := podcastEventBrokers.Load(a); ok {
		return existing.(*podcastEventBroker)
	}
	created := &podcastEventBroker{
		subs: make(map[chan struct{}]struct{}),
	}
	actual, _ := podcastEventBrokers.LoadOrStore(a, created)
	return actual.(*podcastEventBroker)
}

func (a *API) subscribePodcastEvents() (chan struct{}, func()) {
	broker := a.podcastBroker()
	if broker == nil {
		return nil, func() {}
	}
	ch := make(chan struct{}, 1)
	broker.mu.Lock()
	if broker.subs == nil {
		broker.subs = make(map[chan struct{}]struct{})
	}
	broker.subs[ch] = struct{}{}
	broker.mu.Unlock()
	return ch, func() {
		broker.mu.Lock()
		delete(broker.subs, ch)
		broker.mu.Unlock()
	}
}

func (a *API) notifyPodcastEvent() {
	broker := a.podcastBroker()
	if broker == nil {
		return
	}
	broker.mu.Lock()
	for ch := range broker.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	broker.mu.Unlock()
}

func (a *API) podcastEventSubscriberCount() int {
	broker := a.podcastBroker()
	if broker == nil {
		return 0
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	return len(broker.subs)
}

func (a *API) podcastEventsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondV2Error(w, r, http.StatusInternalServerError, "streaming_unsupported", "Streaming is not supported", false, nil)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := a.subscribePodcastEvents()
	defer unsub()

	lastPayload := ""
	sendSnapshot := func() bool {
		if a.db == nil {
			return true
		}
		ctx := r.Context()
		downloads, err := a.db.ListPodcastDownloads(ctx)
		if err != nil {
			return true
		}
		usedBytes, err := a.db.PodcastOfflineStorage(ctx)
		if err != nil {
			return true
		}
		refreshCount, err := a.db.CountActivePodcastRefreshes(ctx)
		if err != nil {
			return true
		}
		payload, err := json.Marshal(map[string]any{
			"downloads": downloads,
			"storage": map[string]int64{
				"usedBytes": usedBytes,
				"maxBytes":  db.MaxPodcastOfflineLibraryBytes,
			},
			"refreshCount": refreshCount,
		})
		if err != nil || string(payload) == lastPayload {
			return true
		}
		lastPayload = string(payload)
		if _, err := fmt.Fprintf(w, "event: podcasts\ndata: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !sendSnapshot() {
		return
	}

	fallbackTicker := time.NewTicker(podcastEventsFallbackPoll)
	heartbeat := time.NewTicker(podcastEventsHeartbeatInterval)
	defer fallbackTicker.Stop()
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if !sendSnapshot() {
				return
			}
		case <-fallbackTicker.C:
			if !sendSnapshot() {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
