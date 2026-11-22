package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/podcast"
	"github.com/go-chi/chi/v5"
)

const (
	maxPodcastBookmarkRequestBytes int64 = 1024
	maxPodcastBookmarkPositionMs   int64 = 30 * 24 * 60 * 60 * 1000
)

var (
	podcastChaptersFetchers   sync.Map // map[*API]func(context.Context, string) ([]podcast.Chapter, error)
	podcastTranscriptFetchers sync.Map // map[*API]func(context.Context, string, string) ([]podcast.TranscriptCue, string, error)
)

func setPodcastChaptersFetcher(a *API, fn func(context.Context, string) ([]podcast.Chapter, error)) func() {
	if fn == nil {
		podcastChaptersFetchers.Delete(a)
		return func() {}
	}
	podcastChaptersFetchers.Store(a, fn)
	return func() { podcastChaptersFetchers.Delete(a) }
}

func setPodcastTranscriptFetcher(a *API, fn func(context.Context, string, string) ([]podcast.TranscriptCue, string, error)) func() {
	if fn == nil {
		podcastTranscriptFetchers.Delete(a)
		return func() {}
	}
	podcastTranscriptFetchers.Store(a, fn)
	return func() { podcastTranscriptFetchers.Delete(a) }
}

type podcastBookmarkRequest struct {
	PositionMs *int64 `json:"positionMs"`
	Note       string `json:"note"`
}

func (a *API) getPodcastEpisodeDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := a.db.GetSubscribedEpisodeDetail(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		respondV2Error(w, r, http.StatusNotFound, "podcast_episode_not_found",
			"Podcast episode not found", false, nil)
		return
	}
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_library_unavailable",
			"Unable to read podcast episode details", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"episode": detail})
}

func (a *API) getPodcastEpisodeChapters(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	source, _, err := a.db.PodcastEpisodeChaptersSource(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		respondV2Error(w, r, http.StatusNotFound, "podcast_episode_not_found",
			"Podcast episode not found", false, nil)
		return
	}
	if errors.Is(err, podcast.ErrBlockedURL) {
		respondV2Error(w, r, http.StatusUnprocessableEntity, "podcast_chapters_unsupported",
			"Episode chapters URL is not supported", false, nil)
		return
	}
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_chapters_unavailable",
			"Unable to read episode chapters metadata", true, nil)
		return
	}
	if source == "" {
		w.Header().Set("Cache-Control", "private, max-age=300")
		respondV2JSON(w, http.StatusOK, map[string]any{
			"chapters": []podcast.Chapter{},
			"source":   "none",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	fetch := podcast.NewFetcher(false).FetchChapters
	if override, ok := podcastChaptersFetchers.Load(a); ok {
		fetch = override.(func(context.Context, string) ([]podcast.Chapter, error))
	}
	chapters, err := fetch(ctx, source)
	if err != nil {
		if errors.Is(err, podcast.ErrBlockedURL) || errors.Is(err, podcast.ErrInvalidChapters) {
			respondV2Error(w, r, http.StatusUnprocessableEntity, "podcast_chapters_invalid",
				"Publisher chapters are not supported", false, nil)
			return
		}
		respondV2Error(w, r, http.StatusBadGateway, "podcast_chapters_unavailable",
			"Publisher chapters could not be retrieved", true, nil)
		return
	}
	if chapters == nil {
		chapters = []podcast.Chapter{}
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	respondV2JSON(w, http.StatusOK, map[string]any{
		"chapters": chapters,
		"source":   "podcast2",
	})
}

func (a *API) getPodcastEpisodeTranscript(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	source, declaredType, err := a.db.PodcastEpisodeTranscriptSource(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		respondV2Error(w, r, http.StatusNotFound, "podcast_episode_not_found",
			"Podcast episode not found", false, nil)
		return
	}
	if errors.Is(err, podcast.ErrBlockedURL) {
		respondV2Error(w, r, http.StatusUnprocessableEntity, "podcast_transcript_unsupported",
			"Episode transcript URL is not supported", false, nil)
		return
	}
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_transcript_unavailable",
			"Unable to read episode transcript metadata", true, nil)
		return
	}
	if source == "" {
		w.Header().Set("Cache-Control", "private, max-age=300")
		respondV2JSON(w, http.StatusOK, map[string]any{
			"cues":   []podcast.TranscriptCue{},
			"format": "none",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	fetch := podcast.NewFetcher(false).FetchTranscript
	if override, ok := podcastTranscriptFetchers.Load(a); ok {
		fetch = override.(func(context.Context, string, string) ([]podcast.TranscriptCue, string, error))
	}
	cues, format, err := fetch(ctx, source, declaredType)
	if err != nil {
		if errors.Is(err, podcast.ErrBlockedURL) || errors.Is(err, podcast.ErrInvalidTranscript) {
			respondV2Error(w, r, http.StatusUnprocessableEntity, "podcast_transcript_invalid",
				"Publisher transcript is not supported", false, nil)
			return
		}
		respondV2Error(w, r, http.StatusBadGateway, "podcast_transcript_unavailable",
			"Publisher transcript could not be retrieved", true, nil)
		return
	}
	if cues == nil {
		cues = []podcast.TranscriptCue{}
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	respondV2JSON(w, http.StatusOK, map[string]any{
		"cues":   cues,
		"format": format,
	})
}

func (a *API) listPodcastEpisodeBookmarks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bookmarks, err := a.db.ListEpisodeBookmarks(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		respondV2Error(w, r, http.StatusNotFound, "podcast_episode_not_found",
			"Podcast episode not found", false, nil)
		return
	}
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_bookmarks_unavailable",
			"Unable to read episode bookmarks", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"bookmarks": bookmarks})
}

func (a *API) createPodcastEpisodeBookmark(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_bookmark",
			"Expected a JSON bookmark payload", false, nil)
		return
	}
	var input podcastBookmarkRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodcastBookmarkRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_bookmark",
			"Invalid podcast bookmark payload", false, nil)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_bookmark",
			"Only one bookmark payload is allowed", false, nil)
		return
	}
	if input.PositionMs == nil || *input.PositionMs < 0 || *input.PositionMs > maxPodcastBookmarkPositionMs ||
		!utf8.ValidString(input.Note) || strings.ContainsRune(input.Note, '\x00') ||
		utf8.RuneCountInString(strings.TrimSpace(input.Note)) > db.MaxPodcastBookmarkNoteRunes {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_bookmark",
			"Bookmark position and note must be within valid bounds", false, nil)
		return
	}

	bookmark, err := a.db.CreateEpisodeBookmark(r.Context(), chi.URLParam(r, "id"), *input.PositionMs, input.Note)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		respondV2Error(w, r, http.StatusNotFound, "podcast_episode_not_found",
			"Podcast episode not found", false, nil)
	case errors.Is(err, db.ErrInvalidBookmark):
		respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_bookmark",
			"Bookmark position and note must be within valid bounds", false, nil)
	case err != nil:
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_bookmarks_unavailable",
			"Unable to save episode bookmark", true, nil)
	default:
		w.Header().Set("Cache-Control", "no-store")
		respondV2JSON(w, http.StatusCreated, map[string]any{"bookmark": bookmark})
	}
}

func (a *API) deletePodcastEpisodeBookmark(w http.ResponseWriter, r *http.Request) {
	episodeID := chi.URLParam(r, "id")
	bookmarkID := chi.URLParam(r, "bookmarkId")
	if bookmarkID == "" {
		bookmarkID = chi.URLParam(r, "bookmarkID")
	}
	err := a.db.DeleteEpisodeBookmark(r.Context(), episodeID, bookmarkID)
	if errors.Is(err, sql.ErrNoRows) {
		respondV2Error(w, r, http.StatusNotFound, "podcast_bookmark_not_found",
			"Podcast bookmark not found", false, nil)
		return
	}
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "podcast_bookmarks_unavailable",
			"Unable to delete episode bookmark", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
