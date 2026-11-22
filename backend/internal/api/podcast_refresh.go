// podcast_refresh.go runs one bounded feed refresher, independent of the
// library analysis scheduler and Spotify transfer workers.
package api

import (
    "context"
    "database/sql"
    "errors"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

type podcastFeedFetcher interface {
    Fetch(context.Context,string,string,string)(podcast.FetchResult,error)
}

func (a *API) startPodcastRefresher() {
    a.podcastRefreshMu.Lock()
    defer a.podcastRefreshMu.Unlock()
    if a.podcastRefreshClosed || a.podcastRefreshOn || a.db==nil {return}
    ctx,cancel:=context.WithCancel(context.Background())
    a.podcastRefreshCancel=cancel
    a.podcastRefreshWake=make(chan struct{},1)
    a.podcastRefreshOn=true
    a.podcastRefreshWG.Add(1)
    go func() {
        defer a.podcastRefreshWG.Done()
        ticker:=time.NewTicker(10*time.Second)
        defer ticker.Stop()
        for{
            select{
            case <-ctx.Done():return
            case <-ticker.C:
            case <-a.podcastRefreshWake:
            }
            // At most one remote request per tick/wake, limiting the network
            // pressure even after importing a large feed collection.
            a.processPodcastRefresh(ctx)
        }
    }()
}

func (a *API) wakePodcastRefresher(){
    a.podcastRefreshMu.Lock()
    wake:=a.podcastRefreshWake
    running:=a.podcastRefreshOn && !a.podcastRefreshClosed
    a.podcastRefreshMu.Unlock()
    if running {
        select {case wake<-struct{}{}:default:}
    }
}

func (a *API) closePodcastRefresher(){
    a.podcastRefreshMu.Lock()
    a.podcastRefreshClosed=true
    cancel:=a.podcastRefreshCancel
    a.podcastRefreshMu.Unlock()
    if cancel!=nil{cancel()}
    a.podcastRefreshWG.Wait()
}

func (a *API) processPodcastRefresh(ctx context.Context){
    // Preview is opt-in: disable unsolicited feed traffic when the user has
    // not enabled the experimental podcast UI.
    enabled,err:=a.db.GetSetting("podcasts_preview_enabled")
    if err!=nil || enabled!="true" {return}
    ctx,cancel:=context.WithTimeout(ctx,25*time.Second)
    defer cancel()
    claimed,err:=a.db.ClaimDuePodcastRefresh(ctx,time.Now())
    if err!=nil||claimed==nil{return}
    fetcher:=a.podcastFetcher
    if fetcher==nil{fetcher=podcast.NewFetcher(false)}
    result,fetchErr:=fetcher.Fetch(ctx,claimed.URL,claimed.ETag,claimed.LastModified)
    if ctx.Err()!=nil{return} // expired lease recovers unfinished work
    if fetchErr!=nil{
        code:="feed_unavailable"
        if errors.Is(fetchErr,podcast.ErrBlockedURL){code="blocked_remote_target"}
        if errors.Is(fetchErr,podcast.ErrInvalidFeed){code="feed_invalid"}
        _=a.db.FailPodcastRefresh(ctx,claimed.ID,code,time.Now())
        return
    }
    if err=a.db.FinishPodcastRefresh(ctx,*claimed,result,time.Now());err!=nil{
        if errors.Is(err,sql.ErrNoRows){return} // unfollowed while fetching
        _=a.db.FailPodcastRefresh(ctx,claimed.ID,"feed_unavailable",time.Now())
        return
    }
    // No extra publisher traffic is initiated here; the existing bounded
    // download queue checks for new opt-in jobs after successful refresh.
    if !result.NotModified{a.wakePodcastDownloader()}
}

