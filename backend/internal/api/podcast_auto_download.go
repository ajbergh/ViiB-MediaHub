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

type podcastAutoDownloadRequest struct {
    Enabled bool `json:"enabled"`
    NewestCount int `json:"newestCount"`
}

func (a *API) getPodcastAutoDownloadPolicy(w http.ResponseWriter,r *http.Request){
    policy,err:=a.db.GetPodcastAutoDownloadPolicy(r.Context(),chi.URLParam(r,"id"))
    if errors.Is(err,sql.ErrNoRows){
        respondV2Error(w,r,http.StatusNotFound,"podcast_not_found",
            "Podcast subscription not found",false,nil);return
    }
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_library_unavailable",
            "Unable to read podcast download preferences",true,nil);return
    }
    w.Header().Set("Cache-Control","no-store")
    respondV2JSON(w,http.StatusOK,map[string]any{"policy":policy})
}

func (a *API) putPodcastAutoDownloadPolicy(w http.ResponseWriter,r *http.Request){
    if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")),"application/json"){
        respondV2Error(w,r,http.StatusBadRequest,"invalid_auto_download_policy",
            "Provide JSON podcast download preferences",false,nil);return
    }
    var input podcastAutoDownloadRequest
    decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024))
    decoder.DisallowUnknownFields()
    if err:=decoder.Decode(&input);err!=nil{
        respondV2Error(w,r,http.StatusBadRequest,"invalid_auto_download_policy",
            "Invalid podcast download preferences",false,nil);return
    }
    var extra any
    if err:=decoder.Decode(&extra);!errors.Is(err,io.EOF) ||
        input.NewestCount<1||input.NewestCount>3{
        respondV2Error(w,r,http.StatusBadRequest,"invalid_auto_download_policy",
            "Choose 1 to 3 new episodes per refresh",false,nil);return
    }
    policy,err:=a.db.UpdatePodcastAutoDownloadPolicy(r.Context(),chi.URLParam(r,"id"),
        db.PodcastAutoDownloadPolicy{Enabled:input.Enabled,NewestCount:input.NewestCount})
    if errors.Is(err,sql.ErrNoRows){
        respondV2Error(w,r,http.StatusNotFound,"podcast_not_found",
            "Podcast subscription not found",false,nil);return
    }
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_library_unavailable",
            "Unable to update download preferences",true,nil);return
    }
    w.Header().Set("Cache-Control","no-store")
    respondV2JSON(w,http.StatusOK,map[string]any{"policy":policy})
}
