package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/go-chi/chi/v5"
)

// V2Routes exposes additive, scalable APIs while the legacy /api routes remain
// available for compatibility.
func (a *API) V2Routes() chi.Router {
	r := chi.NewRouter()
	if err := a.db.EnsureLibrarySyncSchema(); err != nil {
		logger.API("Failed to initialize library synchronization schema: %v", err)
	}

	r.Get("/library/snapshot", a.getLibrarySnapshotV2)
	r.Get("/library/changes", a.getLibraryChangesV2)
	r.Get("/library/revision", a.getLibraryRevisionV2)
	r.Get("/library/events", a.libraryRevisionEventsV2)
	r.Get("/library/stats", a.getLibrarySyncStatsV2)
	r.Get("/search", a.searchLibraryV2)
	return r
}

func parseBoundedInt(raw string, fallback, maximum int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func transformSongForAPI(song *db.Song) {
	song.FilePath = "/api/audio/" + song.ID
	if song.CoverPath != "" {
		song.CoverPath = "/api/cover/" + song.ID
	}
}

func transformSongsForAPI(songs []db.Song) {
	for i := range songs {
		transformSongForAPI(&songs[i])
	}
}

func (a *API) getLibrarySnapshotV2(w http.ResponseWriter, r *http.Request) {
	if err := a.db.EnsureLibrarySyncSchema(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	page, err := a.db.ListSongsPage(r.URL.Query().Get("cursor"), parseBoundedInt(r.URL.Query().Get("limit"), 500, 2000))
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	transformSongsForAPI(page.Songs)
	respondJSON(w, page)
}

func (a *API) getLibraryChangesV2(w http.ResponseWriter, r *http.Request) {
	if err := a.db.EnsureLibrarySyncSchema(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	since, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil || since < 0 {
		respondError(w, http.StatusBadRequest, "since must be a non-negative revision")
		return
	}
	page, err := a.db.GetLibraryChanges(since, parseBoundedInt(r.URL.Query().Get("limit"), 500, 2000))
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	transformSongsForAPI(page.Songs)
	if !page.HasMore {
		_ = a.db.PruneLibraryChanges(100000)
	}
	respondJSON(w, page)
}

func (a *API) getLibraryRevisionV2(w http.ResponseWriter, r *http.Request) {
	if err := a.db.EnsureLibrarySyncSchema(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	revision, err := a.db.LibraryRevision()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]int64{"revision": revision})
}

func (a *API) getLibrarySyncStatsV2(w http.ResponseWriter, r *http.Request) {
	if err := a.db.EnsureLibrarySyncSchema(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	revision, retained, err := a.db.LibrarySyncStats()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]int64{"revision": revision, "retainedChanges": retained})
}

func (a *API) searchLibraryV2(w http.ResponseWriter, r *http.Request) {
	if err := a.db.EnsureLibrarySyncSchema(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 250 {
		respondError(w, http.StatusBadRequest, "search query is too long")
		return
	}
	result, err := a.db.SearchLibrary(query, parseBoundedInt(r.URL.Query().Get("limit"), 50, 200))
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	transformSongsForAPI(result.Tracks)
	for i := range result.Albums {
		if result.Albums[i].CoverPath != "" && len(result.Tracks) > 0 {
			// Cover serving is song-ID based. Prefer a matching track's API cover.
			for _, song := range result.Tracks {
				artist := song.AlbumArtist
				if artist == "" {
					artist = song.Artist
				}
				if song.Album == result.Albums[i].Name && artist == result.Albums[i].Artist && song.CoverPath != "" {
					result.Albums[i].CoverPath = song.CoverPath
					break
				}
			}
		}
	}
	respondJSON(w, result)
}

func parseLastEventRevision(r *http.Request) int64 {
	candidates := []string{r.Header.Get("Last-Event-ID"), r.URL.Query().Get("since")}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if revision, err := strconv.ParseInt(candidate, 10, 64); err == nil && revision >= 0 {
			return revision
		}
	}
	return 0
}

func writeRevisionEvent(w http.ResponseWriter, revision int64) error {
	payload, err := json.Marshal(map[string]int64{"revision": revision})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: library_revision\ndata: %s\n\n", revision, payload)
	return err
}

// libraryRevisionEventsV2 provides replay-safe revision notifications. Clients
// reconnect with Last-Event-ID and fetch the durable delta log separately.
func (a *API) libraryRevisionEventsV2(w http.ResponseWriter, r *http.Request) {
	if err := a.db.EnsureLibrarySyncSchema(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	lastSent := parseLastEventRevision(r)
	if current, err := a.db.LibraryRevision(); err == nil && current > lastSent {
		if err := writeRevisionEvent(w, current); err != nil {
			return
		}
		lastSent = current
		flusher.Flush()
	}

	revisionTicker := time.NewTicker(500 * time.Millisecond)
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer revisionTicker.Stop()
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-revisionTicker.C:
			current, err := a.db.LibraryRevision()
			if err != nil || current <= lastSent {
				continue
			}
			if err := writeRevisionEvent(w, current); err != nil {
				return
			}
			lastSent = current
			flusher.Flush()
		case <-heartbeatTicker.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
