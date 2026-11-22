// podcast_routes.go exposes the first, intentionally limited public-RSS
// subscription API. Podcast playback, feed scheduling and downloads remain
// separate roadmap milestones.
package api

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
    "github.com/go-chi/chi/v5"
)

const maxPodcastRequestBytes int64 = 4096

type podcastFeedRequest struct {
    URL string `json:"url"`
}

// PodcastRoutes mounts after the standard router-wide Host/Origin/CSRF
// boundary. Each handler checks its own request size and safe response fields.
func (a *API) PodcastRoutes() chi.Router {
    r := chi.NewRouter()
    r.Get("/feeds", a.listPodcastFeeds)
    r.Post("/feeds/preview", a.previewPodcastFeed)
    r.Post("/feeds", a.subscribePodcastFeed)
    r.Get("/feeds/{id}/artwork", a.getPodcastArtwork)
    r.Get("/feeds/{id}/episodes", a.listPodcastEpisodes)
    r.Get("/feeds/{id}/refresh", a.getPodcastRefresh)
    r.Post("/feeds/{id}/refresh", a.schedulePodcastRefresh)
    r.Delete("/feeds/{id}", a.unsubscribePodcastFeed)
    return r
}

func decodePodcastFeedRequest(w http.ResponseWriter, r *http.Request) (podcastFeedRequest, error) {
    var request podcastFeedRequest
    if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
        return request, errors.New("JSON content type required")
    }
    decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodcastRequestBytes))
    decoder.DisallowUnknownFields()
    if err := decoder.Decode(&request); err != nil { return request, err }
    var trailing any
    if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
        return request, errors.New("unexpected JSON content")
    }
    request.URL = strings.TrimSpace(request.URL)
    if len(request.URL) == 0 || len(request.URL) > 2048 {
        return request, errors.New("invalid URL length")
    }
    if _, err := podcast.ValidatePublicURL(request.URL, false); err != nil {
        return request, err
    }
    return request, nil
}

func (a *API) podcastFetch(ctx context.Context, url string) (podcast.FetchResult, error) {
    ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
    defer cancel()
    // Public HTTPS only; private feeds and legacy HTTP are gated until
    // credential encryption and policy decisions in POD-00C are complete.
    fetcher:=a.podcastFetcher
    if fetcher==nil {fetcher=podcast.NewFetcher(false)}
    return fetcher.Fetch(ctx,url,"","")
}

func (a *API) previewPodcastFeed(w http.ResponseWriter, r *http.Request) {
    request, err := decodePodcastFeedRequest(w,r)
    if err != nil {
        respondV2Error(w,r,http.StatusBadRequest,"invalid_feed_url","Provide a public HTTPS podcast RSS feed URL",false,nil)
        return
    }
    result, err := a.podcastFetch(r.Context(),request.URL)
    if err != nil {
        podcastFetchError(w,r,err)
        return
    }
    count := len(result.Feed.Episodes)
    if count > 10 { result.Feed.Episodes = result.Feed.Episodes[:10] }
    respondV2JSON(w,http.StatusOK,map[string]any{"feed":result.Feed,"episodeCount":count})
}

func podcastFetchError(w http.ResponseWriter,r *http.Request,err error) {
    switch {
    case errors.Is(err,podcast.ErrBlockedURL):
        respondV2Error(w,r,http.StatusBadRequest,"blocked_remote_target","The feed URL is not permitted",false,nil)
    case errors.Is(err,podcast.ErrInvalidFeed):
        respondV2Error(w,r,http.StatusUnprocessableEntity,"feed_invalid","The feed is not a supported RSS 2.0 podcast document",false,nil)
    default:
        respondV2Error(w,r,http.StatusBadGateway,"feed_unavailable","The publisher feed could not be retrieved",true,nil)
    }
}

func (a *API) subscribePodcastFeed(w http.ResponseWriter,r *http.Request) {
    request,err:=decodePodcastFeedRequest(w,r)
    if err!=nil {
        respondV2Error(w,r,http.StatusBadRequest,"invalid_feed_url","Provide a public HTTPS podcast RSS feed URL",false,nil)
        return
    }
    fetched,err:=a.podcastFetch(r.Context(),request.URL)
    if err!=nil{podcastFetchError(w,r,err);return}
    feedID,err:=a.db.SavePodcastSubscriptionFromFetch(r.Context(),request.URL,fetched)
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_save_failed","Unable to save the podcast subscription",true,nil)
        return
    }
    a.wakePodcastRefresher()
    respondV2JSON(w,http.StatusCreated,map[string]any{
        "id":feedID,"title":fetched.Feed.Title,"episodeCount":len(fetched.Feed.Episodes),
    })
}

func (a *API) listPodcastFeeds(w http.ResponseWriter,r *http.Request) {
    feeds,err:=a.db.ListSubscribedPodcasts(r.Context())
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_library_unavailable","Unable to list subscribed podcasts",true,nil)
        return
    }
    respondV2JSON(w,http.StatusOK,map[string]any{"feeds":feeds})
}

func (a *API) listPodcastEpisodes(w http.ResponseWriter,r *http.Request) {
    id:=chi.URLParam(r,"id")
    subscribed,err:=a.db.PodcastSubscribed(r.Context(),id)
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_library_unavailable","Unable to read the subscription",true,nil)
        return
    }
    if !subscribed{
        respondV2Error(w,r,http.StatusNotFound,"podcast_not_found","Podcast subscription not found",false,nil)
        return
    }
    limit:=50
    offset:=0
    if raw:=r.URL.Query().Get("limit");raw!=""{
        parsed,parseErr:=strconv.Atoi(raw)
        if parseErr!=nil || parsed<1 || parsed>100 {
            respondV2Error(w,r,http.StatusBadRequest,"invalid_pagination","Episode limit must be between 1 and 100",false,nil)
            return
        }
        limit=parsed
    }
    if raw:=r.URL.Query().Get("offset");raw!=""{
        parsed,parseErr:=strconv.Atoi(raw)
        if parseErr!=nil || parsed<0 || parsed>100000 {
            respondV2Error(w,r,http.StatusBadRequest,"invalid_pagination","Episode offset must be between 0 and 100000",false,nil)
            return
        }
        offset=parsed
    }
    episodes,hasMore,err:=a.db.ListPodcastEpisodesPage(r.Context(),id,limit,offset)
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_library_unavailable","Unable to list episodes",true,nil)
        return
    }
    var nextOffset any
    if hasMore {nextOffset=offset+len(episodes)}
    respondV2JSON(w,http.StatusOK,map[string]any{"episodes":episodes,"nextOffset":nextOffset})
}

func (a *API) unsubscribePodcastFeed(w http.ResponseWriter,r *http.Request) {
    id:=chi.URLParam(r,"id")
    err:=a.db.UnsubscribePodcast(r.Context(),id)
    if errors.Is(err,sql.ErrNoRows){
        respondV2Error(w,r,http.StatusNotFound,"podcast_not_found","Podcast subscription not found",false,nil)
        return
    }
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_unsubscribe_failed","Unable to remove subscription",true,nil)
        return
    }
    // Historical episode positions/media metadata remain in SQLite.
    w.WriteHeader(http.StatusNoContent)
}


// GET refresh returns only public status metadata, never publisher credentials
// or raw network errors.
func (a *API) getPodcastRefresh(w http.ResponseWriter,r *http.Request){
    status,err:=a.db.PodcastRefreshStatus(r.Context(),chi.URLParam(r,"id"))
    if errors.Is(err,sql.ErrNoRows){
        respondV2Error(w,r,http.StatusNotFound,"podcast_not_found","Podcast subscription not found",false,nil)
        return
    }
    if err!=nil {
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_refresh_unavailable","Unable to read refresh status",true,nil)
        return
    }
    respondV2JSON(w,http.StatusOK,status)
}

func (a *API) schedulePodcastRefresh(w http.ResponseWriter,r *http.Request){
    enabled,err:=a.db.GetSetting("podcasts_preview_enabled")
    if err!=nil || enabled!="true"{
        respondV2Error(w,r,http.StatusForbidden,"podcast_preview_disabled","Enable Podcast Preview in Settings to refresh feeds",false,nil)
        return
    }
    if err=a.db.SchedulePodcastRefresh(r.Context(),chi.URLParam(r,"id"));err!=nil{
        if errors.Is(err,sql.ErrNoRows) {
            respondV2Error(w,r,http.StatusNotFound,"podcast_not_found","Podcast subscription not found",false,nil)
            return
        }
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_refresh_unavailable","Unable to schedule feed refresh",true,nil)
        return
    }
    // A durable due timestamp is written before signaling. Restart recovers
    // queued work if this process exits immediately after returning 202.
    a.wakePodcastRefresher()
    respondV2JSON(w,http.StatusAccepted,map[string]any{"status":"queued"})
}


// Artwork is served by subscription ID only; callers cannot supply arbitrary
// proxy URLs. Production fetcher validates all redirects and public addresses.
func (a *API) getPodcastArtwork(w http.ResponseWriter,r *http.Request) {
    id:=chi.URLParam(r,"id")
    source,err:=a.db.PodcastArtworkURL(r.Context(),id)
    if errors.Is(err,sql.ErrNoRows) {
        respondV2Error(w,r,http.StatusNotFound,"podcast_not_found","Podcast subscription not found",false,nil)
        return
    }
    if err!=nil {
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_artwork_unavailable","Unable to read podcast artwork",true,nil)
        return
    }
    if source=="" {
        respondV2Error(w,r,http.StatusNotFound,"podcast_artwork_missing","This podcast has no supported public artwork",false,nil)
        return
    }
    ctx,cancel:=context.WithTimeout(r.Context(),12*time.Second)
    defer cancel()
    fetch:=a.podcastArtworkFetch
    if fetch==nil {fetch=podcast.NewFetcher(false).FetchArtwork}
    artwork,err:=fetch(ctx,source)
    if err!=nil{
        // Never leak the upstream URL, response or DNS details.
        if errors.Is(err,podcast.ErrBlockedURL) || errors.Is(err,podcast.ErrInvalidFeed){
            respondV2Error(w,r,http.StatusUnprocessableEntity,"podcast_artwork_invalid","Publisher artwork is not supported",false,nil)
        }else{
            respondV2Error(w,r,http.StatusBadGateway,"podcast_artwork_unavailable","Publisher artwork could not be retrieved",true,nil)
        }
        return
    }
    if len(artwork.Data)==0 || len(artwork.Data)>int(podcast.MaxArtworkBytes) || !podcastRasterType(artwork.MIME){
        respondV2Error(w,r,http.StatusUnprocessableEntity,"podcast_artwork_invalid","Publisher artwork is not supported",false,nil)
        return
    }
    w.Header().Set("Content-Type",artwork.MIME)
    w.Header().Set("X-Content-Type-Options","nosniff")
    w.Header().Set("Cache-Control","private, max-age=900")
    w.Header().Set("Content-Security-Policy","default-src 'none'; sandbox")
    w.Header().Set("Content-Length",strconv.Itoa(len(artwork.Data)))
    w.WriteHeader(http.StatusOK)
    _,_=w.Write(artwork.Data)
}

func podcastRasterType(t string) bool {
    switch t{
    case "image/png","image/jpeg","image/gif","image/webp":return true
    default:return false
    }
}
