package api

import (
    "database/sql"
    "errors"
    "net/http"
    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
    "github.com/go-chi/chi/v5"
)

func (a *API) listPodcastDownloads(w http.ResponseWriter,r *http.Request){
    rows,err:=a.db.ListPodcastDownloads(r.Context())
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to list podcast downloads",true,nil)
        return
    }
    w.Header().Set("Cache-Control","no-store")
    respondV2JSON(w,http.StatusOK,map[string]any{"downloads":rows})
}

func (a *API) getPodcastDownload(w http.ResponseWriter,r *http.Request){
    id:=chi.URLParam(r,"id")
    record,err:=a.db.GetPodcastDownload(r.Context(),id)
    if errors.Is(err,sql.ErrNoRows){
        respondV2Error(w,r,http.StatusNotFound,"podcast_download_not_found",
            "Podcast download not found",false,nil);return
    }
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to read podcast download",true,nil);return
    }
    w.Header().Set("Cache-Control","no-store")
    respondV2JSON(w,http.StatusOK,map[string]any{"download":record})
}

func (a *API) queuePodcastDownload(w http.ResponseWriter,r *http.Request){
    id:=chi.URLParam(r,"id")
    if !safePodcastEpisodeID.MatchString(id){
        respondV2Error(w,r,http.StatusNotFound,"podcast_episode_not_found","Episode not found",false,nil);return
    }
    // Reuse the subscription-scoped media policy before enqueuing network work.
    source,_,err:=a.db.PodcastMediaSource(r.Context(),id)
    if errors.Is(err,sql.ErrNoRows){
        respondV2Error(w,r,http.StatusNotFound,"podcast_episode_not_found","Episode not found",false,nil);return
    }
    if errors.Is(err,podcast.ErrBlockedURL)||errors.Is(err,podcast.ErrInvalidMedia)||source==""{
        respondV2Error(w,r,http.StatusUnprocessableEntity,"podcast_download_unsupported",
            "This episode has no supported public audio",false,nil);return
    }
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to check episode audio",true,nil);return
    }
    row,err:=a.db.EnqueuePodcastDownload(r.Context(),id)
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to queue podcast audio",true,nil);return
    }
    a.wakePodcastDownloader()
    w.Header().Set("Cache-Control","no-store")
    respondV2JSON(w,http.StatusAccepted,map[string]any{"download":row})
}

func (a *API) cancelPodcastDownload(w http.ResponseWriter,r *http.Request){
    id:=chi.URLParam(r,"id")
    row,err:=a.db.CancelPodcastDownload(r.Context(),id)
    if errors.Is(err,sql.ErrNoRows){
        respondV2Error(w,r,http.StatusNotFound,"podcast_download_not_found",
            "Podcast download not found",false,nil);return
    }
    if errors.Is(err,db.ErrPodcastDownloadNotCancelable){
        respondV2Error(w,r,http.StatusConflict,"podcast_download_not_cancelable",
            "Completed downloads cannot be canceled",false,nil);return
    }
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to cancel podcast download",true,nil);return
    }
    w.Header().Set("Cache-Control","no-store")
    respondV2JSON(w,http.StatusOK,map[string]any{"download":row})
}
