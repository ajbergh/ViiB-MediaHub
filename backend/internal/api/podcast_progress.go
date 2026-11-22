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

type podcastProgressRequest struct {
    PositionMs *int64 `json:"positionMs"`
    ListenedMs *int64 `json:"listenedMs"`
    ExpectedRevision *int64 `json:"expectedRevision"`
    IsPlayed *bool `json:"isPlayed"`
}

func (a *API) getPodcastEpisodeProgress(w http.ResponseWriter,r *http.Request) {
    state,err:=a.db.GetPodcastProgress(r.Context(),chi.URLParam(r,"id"))
    if errors.Is(err,sql.ErrNoRows) {
        respondV2Error(w,r,http.StatusNotFound,"podcast_episode_not_found","Podcast episode not found",false,nil)
        return
    }
    if err!=nil {
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_progress_unavailable","Unable to read episode progress",true,nil)
        return
    }
    respondV2JSON(w,http.StatusOK,state)
}

func (a *API) updatePodcastEpisodeProgress(w http.ResponseWriter,r *http.Request) {
    if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")),"application/json") {
        respondV2Error(w,r,http.StatusBadRequest,"invalid_podcast_progress","Expected a JSON progress update",false,nil)
        return
    }
    var input podcastProgressRequest
    decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096))
    decoder.DisallowUnknownFields()
    if err:=decoder.Decode(&input);err!=nil {
        respondV2Error(w,r,http.StatusBadRequest,"invalid_podcast_progress","Invalid podcast progress update",false,nil)
        return
    }
    var extra any
    if err:=decoder.Decode(&extra);!errors.Is(err,io.EOF){
        respondV2Error(w,r,http.StatusBadRequest,"invalid_podcast_progress","Only one progress update is allowed",false,nil)
        return
    }
    if input.PositionMs==nil||input.ListenedMs==nil||input.ExpectedRevision==nil||input.IsPlayed==nil ||
       *input.PositionMs<0||*input.PositionMs>2592000000||*input.ListenedMs<0||*input.ListenedMs>2592000000||*input.ExpectedRevision<0 {
        respondV2Error(w,r,http.StatusBadRequest,"invalid_podcast_progress","Progress fields are missing or out of range",false,nil)
        return
    }
    next,err:=a.db.SetPodcastProgress(r.Context(),chi.URLParam(r,"id"),*input.PositionMs,*input.ListenedMs,*input.ExpectedRevision,*input.IsPlayed)
    switch {
    case errors.Is(err,sql.ErrNoRows):
        respondV2Error(w,r,http.StatusNotFound,"podcast_episode_not_found","Podcast episode not found",false,nil)
    case errors.Is(err,db.ErrPodcastProgressConflict):
        respondV2Error(w,r,http.StatusConflict,"podcast_progress_conflict","Episode progress changed in another session. Reload before saving.",true,nil)
    case err!=nil:
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_progress_unavailable","Unable to save episode progress",true,nil)
    default:
        respondV2JSON(w,http.StatusOK,next)
    }
}
