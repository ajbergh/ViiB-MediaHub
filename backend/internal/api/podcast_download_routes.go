package api

import (
    "database/sql"
    "errors"
    "net/http"
    "os"
    "path/filepath"
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
    used,err:=a.db.PodcastOfflineStorage(r.Context())
    if err!=nil {
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to read podcast storage",true,nil)
        return
    }
    w.Header().Set("Cache-Control","no-store")
    respondV2JSON(w,http.StatusOK,map[string]any{"downloads":rows,
        "storage":map[string]int64{"usedBytes":used,"maxBytes":db.MaxPodcastOfflineLibraryBytes}})
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
    if !safePodcastEpisodeID.MatchString(id) {
        respondV2Error(w,r,http.StatusNotFound,"podcast_download_not_found",
            "Podcast download not found",false,nil);return
    }
    existing,err:=a.db.GetPodcastDownload(r.Context(),id)
    if errors.Is(err,sql.ErrNoRows) {
        respondV2Error(w,r,http.StatusNotFound,"podcast_download_not_found",
            "Podcast download not found",false,nil);return
    }
    if err!=nil {
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to read podcast download",true,nil);return
    }
    if existing.Status=="completed" {
        a.removeCompletedPodcastDownload(w,r,id)
        return
    }
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

// Removing a completed download deletes only its fixed-name offline copy.
// It does NOT delete listening progress or un-follow the publisher. The
// server-side read/write lock coordinates with local ServeContent requests.
// On Windows an in-use file may refuse unlink; return a retryable conflict
// without incorrectly clearing the completed database record.
func (a *API) removeCompletedPodcastDownload(w http.ResponseWriter,r *http.Request,id string){
    a.podcastDownloadFileMu.Lock()
    defer a.podcastDownloadFileMu.Unlock()
    file,err:=a.db.PodcastCompletedDownload(r.Context(),id)
    if errors.Is(err,sql.ErrNoRows){
        respondV2Error(w,r,http.StatusConflict,"podcast_download_not_completed",
            "The episode is no longer available offline",false,nil);return
    }
    if err!=nil {
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to read offline download metadata",true,nil);return
    }
    if !safePodcastEpisodeID.MatchString(id)||file.Filename!=id+".audio" {
        respondV2Error(w,r,http.StatusConflict,"podcast_download_path_invalid",
            "The offline file path requires repair",false,nil);return
    }
    dir,err:=a.podcastStorageDir()
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to access offline storage",true,nil);return
    }
    path:=filepath.Join(dir,file.Filename)
    info,err:=os.Lstat(path)
    if err!=nil && !errors.Is(err,os.ErrNotExist){
        respondV2Error(w,r,http.StatusConflict,"podcast_download_file_busy",
            "Unable to remove offline audio; close other players and retry",true,nil);return
    }
    if err==nil{
        if !info.Mode().IsRegular(){
            respondV2Error(w,r,http.StatusConflict,"podcast_download_path_invalid",
                "The offline file path requires repair",false,nil);return
        }
        if removeErr:=os.Remove(path);removeErr!=nil{
            respondV2Error(w,r,http.StatusConflict,"podcast_download_file_busy",
                "Unable to remove offline audio; close other players and retry",true,nil);return
        }
    }
    if err:=a.db.DeletePodcastCompletedDownload(r.Context(),id);err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Offline file was removed but its status needs repair",true,nil);return
    }
    w.Header().Set("Cache-Control","no-store")
    w.WriteHeader(http.StatusNoContent)
}
