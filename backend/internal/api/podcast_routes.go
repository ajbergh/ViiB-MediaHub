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
    r.Get("/feeds/{id}/episodes", a.listPodcastEpisodes)
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
    return podcast.NewFetcher(false).Fetch(ctx, url, "", "")
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
    feedID,err:=a.db.SavePodcastFeedSnapshot(r.Context(),fetched.Feed,true)
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_save_failed","Unable to save the podcast subscription",true,nil)
        return
    }
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
    episodes,err:=a.db.ListPodcastEpisodes(r.Context(),id,100)
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_library_unavailable","Unable to list episodes",true,nil)
        return
    }
    respondV2JSON(w,http.StatusOK,map[string]any{"episodes":episodes})
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
