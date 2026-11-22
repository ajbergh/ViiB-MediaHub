package api

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "net/http"
    "strconv"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
    "github.com/go-chi/chi/v5"
)

// getPodcastEpisodeMedia fetches short byte ranges only from a subscribed
// episode's validated public HTTPS enclosure. No publisher URL is accepted
// in an HTTP query/body or returned to the browser.
func (a *API) getPodcastEpisodeMedia(w http.ResponseWriter,r *http.Request) {
    id:=chi.URLParam(r,"id")
    if len(id)==0||len(id)>128{
        respondV2Error(w,r,http.StatusNotFound,"podcast_episode_not_found",
            "Podcast episode not found",false,nil)
        return
    }
    source,_,err:=a.db.PodcastMediaSource(r.Context(),id)
    if errors.Is(err,sql.ErrNoRows){
        respondV2Error(w,r,http.StatusNotFound,"podcast_episode_not_found",
            "Podcast episode not found",false,nil)
        return
    }
    if errors.Is(err,podcast.ErrBlockedURL)||errors.Is(err,podcast.ErrInvalidMedia){
        respondV2Error(w,r,http.StatusUnprocessableEntity,"podcast_media_unsupported",
            "This episode has no supported public audio enclosure",false,nil)
        return
    }
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_media_unavailable",
            "Unable to load podcast episode metadata",true,nil)
        return
    }
    if source==""{
        respondV2Error(w,r,http.StatusUnprocessableEntity,"podcast_media_unsupported",
            "This episode has no supported public audio enclosure",false,nil)
        return
    }
    ctx,cancel:=context.WithTimeout(r.Context(),28*time.Second)
    defer cancel()
    fetch:=a.podcastMediaFetch
    if fetch==nil{fetch=podcast.NewFetcher(false).FetchMediaSegment}
    segment,err:=fetch(ctx,source,r.Header.Get("Range"))
    switch{
    case errors.Is(err,podcast.ErrInvalidMediaRange):
        respondV2Error(w,r,http.StatusRequestedRangeNotSatisfiable,"podcast_range_invalid",
            "A single valid audio byte range is required",false,nil)
        return
    case errors.Is(err,podcast.ErrBlockedURL)||errors.Is(err,podcast.ErrInvalidMedia):
        respondV2Error(w,r,http.StatusUnprocessableEntity,"podcast_media_unsupported",
            "Publisher audio enclosure is not supported",false,nil)
        return
    case err!=nil:
        respondV2Error(w,r,http.StatusBadGateway,"podcast_media_unavailable",
            "Publisher audio range could not be retrieved",true,nil)
        return
    }
    expected:=segment.End-segment.Start+1
    if segment.Start<0||segment.End<segment.Start||segment.Total<=segment.End||
        segment.Total>podcast.MaxMediaTotalBytes||expected<=0||
        expected>podcast.MaxMediaSegmentBytes||int64(len(segment.Data))!=expected||
        podcast.SupportedPodcastAudioMIME(segment.MIME)==""{
        respondV2Error(w,r,http.StatusBadGateway,"podcast_media_unavailable",
            "Publisher audio range was invalid",true,nil)
        return
    }
    w.Header().Set("Content-Type",segment.MIME)
    w.Header().Set("Accept-Ranges","bytes")
    w.Header().Set("Content-Range",fmt.Sprintf("bytes %d-%d/%d",segment.Start,segment.End,segment.Total))
    w.Header().Set("Content-Length",strconv.Itoa(len(segment.Data)))
    w.Header().Set("Cache-Control","private, no-store")
    w.Header().Set("X-Content-Type-Options","nosniff")
    w.WriteHeader(http.StatusPartialContent)
    _,_=w.Write(segment.Data)
}
