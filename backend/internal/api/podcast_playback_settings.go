package api

import (
    "database/sql"
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "strings"

    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/go-chi/chi/v5"
)

type podcastShowPlaybackRequest struct {
    DefaultSpeed     float64 `json:"defaultSpeed"`
    SkipIntroSeconds int     `json:"skipIntroSeconds"`
    SkipOutroSeconds int     `json:"skipOutroSeconds"`
}

type podcastGlobalPlaybackRequest struct {
    DefaultSpeed       float64 `json:"defaultSpeed"`
    SkipBackSeconds    int     `json:"skipBackSeconds"`
    SkipForwardSeconds int     `json:"skipForwardSeconds"`
}

func (a *API) getPodcastShowPlaybackSettings(w http.ResponseWriter, r *http.Request) {
    settings, err := a.db.GetPodcastShowPlaybackSettings(r.Context(), chi.URLParam(r, "id"))
    if errors.Is(err, sql.ErrNoRows) {
        respondV2Error(w, r, http.StatusNotFound, "podcast_not_found",
            "Podcast subscription not found", false, nil)
        return
    }
    if err != nil {
        respondV2Error(w, r, http.StatusInternalServerError, "podcast_library_unavailable",
            "Unable to read show playback settings", true, nil)
        return
    }
    w.Header().Set("Cache-Control", "no-store")
    respondV2JSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *API) putPodcastShowPlaybackSettings(w http.ResponseWriter, r *http.Request) {
    if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
        respondV2Error(w, r, http.StatusBadRequest, "invalid_playback_settings",
            "Provide JSON podcast playback settings", false, nil)
        return
    }
    var input podcastShowPlaybackRequest
    decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
    decoder.DisallowUnknownFields()
    if err := decoder.Decode(&input); err != nil {
        respondV2Error(w, r, http.StatusBadRequest, "invalid_playback_settings",
            "Invalid show playback settings", false, nil)
        return
    }
    var extra any
    if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) ||
        !db.IsAllowedPodcastPlaybackSpeed(input.DefaultSpeed, true) ||
        input.SkipIntroSeconds < 0 || input.SkipIntroSeconds > db.MaxPodcastSkipTrimSeconds ||
        input.SkipOutroSeconds < 0 || input.SkipOutroSeconds > db.MaxPodcastSkipTrimSeconds {
        respondV2Error(w, r, http.StatusBadRequest, "invalid_playback_settings",
            "Choose a valid playback speed and skip durations up to 600 seconds", false, nil)
        return
    }
    saved, err := a.db.UpdatePodcastShowPlaybackSettings(r.Context(), chi.URLParam(r, "id"),
        db.PodcastShowPlaybackSettings{
            DefaultSpeed:     input.DefaultSpeed,
            SkipIntroSeconds: input.SkipIntroSeconds,
            SkipOutroSeconds: input.SkipOutroSeconds,
        })
    if errors.Is(err, sql.ErrNoRows) {
        respondV2Error(w, r, http.StatusNotFound, "podcast_not_found",
            "Podcast subscription not found", false, nil)
        return
    }
    if err != nil {
        respondV2Error(w, r, http.StatusInternalServerError, "podcast_library_unavailable",
            "Unable to update show playback settings", true, nil)
        return
    }
    w.Header().Set("Cache-Control", "no-store")
    respondV2JSON(w, http.StatusOK, map[string]any{"settings": saved})
}

func (a *API) getPodcastGlobalSettings(w http.ResponseWriter, r *http.Request) {
    playback, err := a.db.GetPodcastGlobalPlaybackSettings(r.Context())
    if err != nil {
        respondV2Error(w, r, http.StatusInternalServerError, "podcast_library_unavailable",
            "Unable to read podcast settings", true, nil)
        return
    }
    w.Header().Set("Cache-Control", "no-store")
    respondV2JSON(w, http.StatusOK, map[string]any{"playback": playback})
}

func (a *API) putPodcastGlobalSettings(w http.ResponseWriter, r *http.Request) {
    if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
        respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_settings",
            "Provide JSON podcast settings", false, nil)
        return
    }
    var input podcastGlobalPlaybackRequest
    decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
    decoder.DisallowUnknownFields()
    if err := decoder.Decode(&input); err != nil {
        respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_settings",
            "Invalid podcast playback settings", false, nil)
        return
    }
    var extra any
    if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) ||
        !db.IsAllowedPodcastPlaybackSpeed(input.DefaultSpeed, false) ||
        input.SkipBackSeconds < db.MinPodcastStepSeconds || input.SkipBackSeconds > db.MaxPodcastStepSeconds ||
        input.SkipForwardSeconds < db.MinPodcastStepSeconds || input.SkipForwardSeconds > db.MaxPodcastStepSeconds {
        respondV2Error(w, r, http.StatusBadRequest, "invalid_podcast_settings",
            "Choose a supported playback speed and skip intervals between 5 and 120 seconds", false, nil)
        return
    }
    saved, err := a.db.UpdatePodcastGlobalPlaybackSettings(r.Context(),
        db.PodcastGlobalPlaybackSettings{
            DefaultSpeed:       input.DefaultSpeed,
            SkipBackSeconds:    input.SkipBackSeconds,
            SkipForwardSeconds: input.SkipForwardSeconds,
        })
    if err != nil {
        respondV2Error(w, r, http.StatusInternalServerError, "podcast_library_unavailable",
            "Unable to save podcast playback settings", true, nil)
        return
    }
    w.Header().Set("Cache-Control", "no-store")
    respondV2JSON(w, http.StatusOK, map[string]any{"playback": saved})
}
