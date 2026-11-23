package api

import (
    "io"
    "net/http"
    "strings"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

// previewPodcastOPMLImport parses supplied XML without contacting any
// publisher. Only safe public HTTPS URLs are returned to the frontend.
func (a *API) previewPodcastOPMLImport(w http.ResponseWriter,r *http.Request) {
    contentType:=strings.ToLower(r.Header.Get("Content-Type"))
    if !strings.HasPrefix(contentType,"application/xml") &&
       !strings.HasPrefix(contentType,"text/xml") {
        respondV2Error(w,r,http.StatusBadRequest,"invalid_opml","Expected an OPML XML file",false,nil)
        return
    }
    data,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,podcast.MaxOPMLImportBytes+1))
    if err!=nil{
        respondV2Error(w,r,http.StatusBadRequest,"invalid_opml","OPML document exceeds the import limit",false,nil)
        return
    }
    preview,err:=podcast.ParseOPMLImport(data)
    if err!=nil{
        respondV2Error(w,r,http.StatusUnprocessableEntity,"invalid_opml","Unsupported or oversized OPML 2.0 document",false,nil)
        return
    }
    subscribed,err:=a.db.ListSubscribedPodcasts(r.Context())
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_library_unavailable","Unable to compare subscriptions",true,nil)
        return
    }
    known:=make(map[string]bool,len(subscribed))
    for _,feed:=range subscribed{known[feed.URL]=true}
    for i:=range preview.Feeds {
        preview.Feeds[i].AlreadyFollowed=known[preview.Feeds[i].URL]
    }
    w.Header().Set("Cache-Control","no-store")
    respondV2JSON(w,http.StatusOK,preview)
}
