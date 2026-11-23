// podcast_triage_queue.go serves episode triage views, cross-show search,
// durable queue management, and per-show download retention cleanup handlers.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
)

const (
	maxPodcastTriageRequestBytes = 1024
	maxPodcastQueueReplaceBytes  = 65536
	maxPodcastSearchQueryLen     = 250
	maxPodcastEpisodeProgressMs  = int64(2592000000)
)

type podcastEpisodeStatePatchRequest struct {
	ExpectedRevision *int64 `json:"expectedRevision"`
	Starred          *bool  `json:"starred"`
	Archived         *bool  `json:"archived"`
	IsPlayed         *bool  `json:"isPlayed"`
	PositionMs       *int64 `json:"positionMs"`
	ListenedMs       *int64 `json:"listenedMs"`
}

type podcastQueueAddRequest struct {
	EpisodeID string `json:"episodeId"`
	Position  string `json:"position"`
}

type podcastQueueReplaceRequest struct {
	EpisodeIDs []string `json:"episodeIds"`
}

type podcastRetentionRequest struct {
	KeepLatestCount *int `json:"keepLatestCount"`
}

func decodeStrictPodcastJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return errors.New("JSON content type required")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

func parsePodcastPagination(r *http.Request) (int, int, error) {
	limit := 50
	offset := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return 0, 0, errors.New("Episode limit must be between 1 and 100")
		}
		limit = parsed
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 || parsed > 100000 {
			return 0, 0, errors.New("Episode offset must be between 0 and 100000")
		}
		offset = parsed
	}
	return limit, offset, nil
}

func (a *API) patchPodcastEpisodeState(w http.ResponseWriter, r *http.Request) {
	var input podcastEpisodeStatePatchRequest
	if err := decodeStrictPodcastJSON(w, r, maxPodcastTriageRequestBytes, &input); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_state",
			"Invalid podcast episode state update", false, nil)
		return
	}
	if input.Starred == nil && input.Archived == nil && input.IsPlayed == nil &&
		input.PositionMs == nil && input.ListenedMs == nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_state",
			"Provide at least one episode state field to update", false, nil)
		return
	}
	if (input.ExpectedRevision != nil && *input.ExpectedRevision < 0) ||
		(input.PositionMs != nil && (*input.PositionMs < 0 || *input.PositionMs > maxPodcastEpisodeProgressMs)) ||
		(input.ListenedMs != nil && (*input.ListenedMs < 0 || *input.ListenedMs > maxPodcastEpisodeProgressMs)) {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_state",
			"Episode state fields are out of range", false, nil)
		return
	}

	updated, err := a.db.PatchPodcastEpisodeState(
		r.Context(),
		chi.URLParam(r, "id"),
		input.ExpectedRevision,
		input.Starred,
		input.Archived,
		input.IsPlayed,
		input.PositionMs,
		input.ListenedMs,
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		respondV2Error(w, r, http.StatusNotFound, "podcast_episode_not_found",
			"Podcast episode not found", false, nil)
	case errors.Is(err, db.ErrPodcastProgressConflict):
		respondV2Error(w, r, http.StatusConflict, "podcast_progress_conflict",
			"Episode state changed in another session. Reload before saving.", true, nil)
	case errors.Is(err, db.ErrInvalidPodcastEpisodePatch):
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_state",
			"Invalid podcast episode state update", false, nil)
	case err != nil:
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_state_unavailable",
			"Unable to save podcast episode state", true, nil)
	default:
		w.Header().Set("Cache-Control", "no-store")
		respondV2JSON(w, http.StatusOK, map[string]any{"state": updated})
	}
}

func (a *API) listPodcastEpisodesByView(w http.ResponseWriter, r *http.Request) {
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	if view == "" {
		view = "inbox"
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > maxPodcastSearchQueryLen {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_search_query",
			"Search query is too long", false, nil)
		return
	}
	limit, offset, err := parsePodcastPagination(r)
	if err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_pagination", err.Error(), false, nil)
		return
	}
	episodes, hasMore, err := a.db.ListPodcastEpisodesByView(r.Context(), view, query, limit, offset)
	if errors.Is(err, db.ErrInvalidPodcastView) {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_view",
			"Supported views are inbox, starred, in_progress, played, archived, and all", false, nil)
		return
	}
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_library_unavailable",
			"Unable to list podcast episodes", true, nil)
		return
	}
	var nextOffset any
	if hasMore {
		nextOffset = offset + len(episodes)
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{
		"episodes":   episodes,
		"nextOffset": nextOffset,
	})
}

func (a *API) searchPodcastEpisodes(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > maxPodcastSearchQueryLen {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_search_query",
			"Search query is too long", false, nil)
		return
	}
	limit, offset, err := parsePodcastPagination(r)
	if err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_pagination", err.Error(), false, nil)
		return
	}
	episodes, hasMore, err := a.db.SearchPodcastEpisodes(r.Context(), query, limit, offset)
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_library_unavailable",
			"Unable to search podcast episodes", true, nil)
		return
	}
	var nextOffset any
	if hasMore {
		nextOffset = offset + len(episodes)
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{
		"episodes":   episodes,
		"nextOffset": nextOffset,
	})
}

func (a *API) getPodcastQueue(w http.ResponseWriter, r *http.Request) {
	items, err := a.db.ListPodcastQueue(r.Context())
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_queue_unavailable",
			"Unable to read podcast queue", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"queue": items})
}

func (a *API) addPodcastQueue(w http.ResponseWriter, r *http.Request) {
	var input podcastQueueAddRequest
	if err := decodeStrictPodcastJSON(w, r, maxPodcastTriageRequestBytes, &input); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_queue",
			"Invalid podcast queue request", false, nil)
		return
	}
	input.EpisodeID = strings.TrimSpace(input.EpisodeID)
	input.Position = strings.TrimSpace(input.Position)
	if input.Position == "" {
		input.Position = "end"
	}
	if input.EpisodeID == "" || (input.Position != "next" && input.Position != "end") {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_queue",
			"Provide an episodeId and position (next or end)", false, nil)
		return
	}

	err := a.db.AddToPodcastQueue(r.Context(), input.EpisodeID, input.Position)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		respondV2Error(w, r, http.StatusNotFound, "podcast_episode_not_found",
			"Podcast episode not found", false, nil)
		return
	case errors.Is(err, db.ErrPodcastEpisodeNoAudio):
		respondV2Error(w, r, http.StatusUnprocessableEntity, "podcast_episode_no_audio",
			"This episode has no audio enclosure", false, nil)
		return
	case errors.Is(err, db.ErrInvalidPodcastQueue):
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_queue",
			"Invalid podcast queue request", false, nil)
		return
	case err != nil:
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_queue_unavailable",
			"Unable to update podcast queue", true, nil)
		return
	}

	items, err := a.db.ListPodcastQueue(r.Context())
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_queue_unavailable",
			"Unable to read podcast queue", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"queue": items})
}

func (a *API) replacePodcastQueue(w http.ResponseWriter, r *http.Request) {
	var input podcastQueueReplaceRequest
	if err := decodeStrictPodcastJSON(w, r, maxPodcastQueueReplaceBytes, &input); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_queue",
			"Invalid podcast queue payload", false, nil)
		return
	}
	if input.EpisodeIDs == nil || len(input.EpisodeIDs) > db.MaxPodcastQueueSize {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_queue",
			"Provide a valid episodeIds list", false, nil)
		return
	}

	err := a.db.ReplacePodcastQueue(r.Context(), input.EpisodeIDs)
	switch {
	case errors.Is(err, db.ErrInvalidPodcastQueue):
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_queue",
			"Invalid podcast queue payload", false, nil)
		return
	case errors.Is(err, sql.ErrNoRows):
		respondV2Error(w, r, http.StatusNotFound, "podcast_episode_not_found",
			"Podcast episode not found", false, nil)
		return
	case errors.Is(err, db.ErrPodcastEpisodeNoAudio):
		respondV2Error(w, r, http.StatusUnprocessableEntity, "podcast_episode_no_audio",
			"Queued episodes must have an audio enclosure", false, nil)
		return
	case err != nil:
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_queue_unavailable",
			"Unable to replace podcast queue", true, nil)
		return
	}

	items, err := a.db.ListPodcastQueue(r.Context())
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_queue_unavailable",
			"Unable to read podcast queue", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"queue": items})
}

func (a *API) deletePodcastQueueItem(w http.ResponseWriter, r *http.Request) {
	episodeID := chi.URLParam(r, "episodeId")
	if strings.TrimSpace(episodeID) == "" {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_queue",
			"Provide an episode ID", false, nil)
		return
	}
	if err := a.db.RemoveFromPodcastQueue(r.Context(), episodeID); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_queue_unavailable",
			"Unable to remove episode from podcast queue", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getPodcastRetention(w http.ResponseWriter, r *http.Request) {
	policy, err := a.db.GetPodcastRetentionPolicy(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, sql.ErrNoRows) {
		respondV2Error(w, r, http.StatusNotFound, "podcast_not_found",
			"Podcast subscription not found", false, nil)
		return
	}
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_library_unavailable",
			"Unable to read podcast retention policy", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"policy": policy})
}

func (a *API) putPodcastRetention(w http.ResponseWriter, r *http.Request) {
	var input podcastRetentionRequest
	if err := decodeStrictPodcastJSON(w, r, maxPodcastTriageRequestBytes, &input); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_retention",
			"Invalid podcast retention policy", false, nil)
		return
	}
	if input.KeepLatestCount == nil || *input.KeepLatestCount < 0 ||
		*input.KeepLatestCount > db.MaxPodcastKeepLatestCount {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_retention",
			"keepLatestCount must be between 0 and 50", false, nil)
		return
	}

	feedID := chi.URLParam(r, "id")
	policy, err := a.db.SetPodcastRetentionPolicy(r.Context(), feedID, *input.KeepLatestCount)
	if errors.Is(err, sql.ErrNoRows) {
		respondV2Error(w, r, http.StatusNotFound, "podcast_not_found",
			"Podcast subscription not found", false, nil)
		return
	}
	if errors.Is(err, db.ErrInvalidPodcastRetention) {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_retention",
			"keepLatestCount must be between 0 and 50", false, nil)
		return
	}
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_library_unavailable",
			"Unable to update podcast retention policy", true, nil)
		return
	}

	a.enforcePodcastRetentionForFeed(r.Context(), feedID, "")

	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"policy": policy})
}

func (a *API) enforcePodcastRetentionForEpisode(ctx context.Context, episodeID string) {
	if a.db == nil || episodeID == "" {
		return
	}
	feedID, err := a.db.PodcastEpisodeFeedID(ctx, episodeID)
	if err != nil || feedID == "" {
		return
	}
	a.enforcePodcastRetentionForFeed(ctx, feedID, episodeID)
}

func (a *API) enforcePodcastRetentionForFeed(ctx context.Context, feedID, excludeEpisodeID string) {
	if a.db == nil || a.dataDir == "" || feedID == "" {
		return
	}
	expired, err := a.db.SelectExpiredCompletedDownloads(ctx, feedID, excludeEpisodeID)
	if err != nil || len(expired) == 0 {
		return
	}
	dir, err := a.podcastStorageDir()
	if err != nil {
		return
	}
	a.podcastDownloadFileMu.Lock()
	defer a.podcastDownloadFileMu.Unlock()
	for _, item := range expired {
		if ctx.Err() != nil {
			return
		}
		if !safePodcastEpisodeID.MatchString(item.EpisodeID) ||
			item.RelativePath != item.EpisodeID+".audio" {
			continue
		}
		path := filepath.Join(dir, item.RelativePath)
		info, statErr := os.Lstat(path)
		if statErr == nil {
			if !info.Mode().IsRegular() {
				continue
			}
			if removeErr := os.Remove(path); removeErr != nil {
				continue
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if err := a.db.DeleteCompletedDownloadRecord(ctx, item.EpisodeID); err == nil {
			a.clearPodcastOfflineVerification(item.EpisodeID)
		}
	}
}
