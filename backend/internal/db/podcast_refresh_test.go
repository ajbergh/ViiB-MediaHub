package db

import (
    "context"
    "database/sql"
    "errors"
    "path/filepath"
    "testing"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastRefreshClaim304AndHistory(t *testing.T) {
    database,err:=New(filepath.Join(t.TempDir(),"podcasts.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    feed:=podcast.Feed{URL:"https://example.org/rss",Title:"Original",Episodes:[]podcast.Episode{
        {GUID:"keep-1",Title:"Keep One"},
        {GUID:"old-2",Title:"Back Catalog"},
    }}
    id,err:=database.SavePodcastFeedSnapshot(ctx,feed,true)
    if err!=nil{t.Fatal(err)}
    if err=database.SchedulePodcastRefresh(ctx,id);err!=nil{t.Fatal(err)}
    now:=time.Unix(1795000000,0)
    claimed,err:=database.ClaimDuePodcastRefresh(ctx,now)
    if err!=nil||claimed==nil||claimed.ID!=id{t.Fatalf("claim: %#v %v",claimed,err)}
    second,err:=database.ClaimDuePodcastRefresh(ctx,now)
    if err!=nil||second!=nil{t.Fatalf("duplicate claim: %#v %v",second,err)}
    if err=database.FinishPodcastRefresh(ctx,*claimed,podcast.FetchResult{NotModified:true,ETag:`"a"`},now);err!=nil{t.Fatal(err)}
    status,err:=database.PodcastRefreshStatus(ctx,id)
    if err!=nil||status.Refreshing||status.Failures!=0||status.LastSuccessAt!=now.UnixMilli(){t.Fatalf("304 status: %+v %v",status,err)}
    if status.NextRefreshAt<=now.UnixMilli(){t.Fatalf("next refresh not scheduled: %+v",status)}
    if err=database.SchedulePodcastRefresh(ctx,id);err!=nil{t.Fatal(err)}
    claimed,err=database.ClaimDuePodcastRefresh(ctx,now.Add(time.Second))
    if err!=nil||claimed==nil||claimed.ETag!=`"a"`{t.Fatalf("conditional headers: %#v %v",claimed,err)}
    var stateID string
    if err=database.conn.QueryRowContext(ctx,`SELECT id FROM podcast_episodes WHERE guid='keep-1'`).Scan(&stateID);err!=nil{t.Fatal(err)}
    if _,err=database.conn.ExecContext(ctx,`INSERT INTO podcast_episode_state(episode_id,position_ms) VALUES (?,72500)`,stateID);err!=nil{t.Fatal(err)}
    // Publisher truncates history and updates episode title: history survives.
    feed.URL="https://publisher.example/redirected-feed"
    feed.Title="New Name"
    feed.Episodes=[]podcast.Episode{{GUID:"keep-1",Title:"Renamed Episode"}}
    if err=database.FinishPodcastRefresh(ctx,*claimed,podcast.FetchResult{Feed:feed},now.Add(2*time.Second));err!=nil{t.Fatal(err)}
    var preservedPosition int64
    var episodeCount,unlistedCount int
    if err=database.conn.QueryRowContext(ctx,`SELECT position_ms FROM podcast_episode_state WHERE episode_id=?`,stateID).Scan(&preservedPosition);err!=nil{t.Fatal(err)}
    if err=database.conn.QueryRowContext(ctx,`SELECT COUNT(*),SUM(CASE WHEN listed_in_feed=0 THEN 1 ELSE 0 END) FROM podcast_episodes WHERE feed_id=?`,id).Scan(&episodeCount,&unlistedCount);err!=nil{t.Fatal(err)}
    if preservedPosition!=72500||episodeCount!=2||unlistedCount!=1{t.Fatalf("history: position %d episodes %d unlisted %d",preservedPosition,episodeCount,unlistedCount)}
    list,err:=database.ListSubscribedPodcasts(ctx)
    if err!=nil||len(list)!=1||list[0].ID!=id||list[0].Title!="New Name"{t.Fatalf("feed ID lost: %#v %v",list,err)}
    if list[0].URL!="https://publisher.example/redirected-feed" {t.Fatalf("canonical URL stale: %q",list[0].URL)}
    var oldAlias string
    if err=database.conn.QueryRowContext(ctx,`SELECT feed_id FROM podcast_feed_aliases WHERE original_url=?`,"https://example.org/rss").Scan(&oldAlias);err!=nil||oldAlias!=id{
        t.Fatalf("redirect alias not retained: %q %v",oldAlias,err)
    }
}
func TestPodcastRefreshRetryAndLeaseRecovery(t *testing.T){
    database,err:=New(filepath.Join(t.TempDir(),"podcasts.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    id,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{URL:"https://example.org/rss",Title:"Test"},true)
    if err!=nil{t.Fatal(err)}
    now:=time.Now()
    first,err:=database.ClaimDuePodcastRefresh(ctx,now)
    if err!=nil||first==nil{t.Fatalf("first claim: %v %v",first,err)}
    // Simulate process exit before completion: the lease expires.
    second,err:=database.ClaimDuePodcastRefresh(ctx,now.Add(podcastRefreshLease+time.Second))
    if err!=nil||second==nil{t.Fatalf("lease did not recover: %v %v",second,err)}
    if err=database.FailPodcastRefresh(ctx,id,"bad token https://private.example",now.Add(podcastRefreshLease+2*time.Second));err!=nil{t.Fatal(err)}
    status,err:=database.PodcastRefreshStatus(ctx,id)
    if err!=nil||status.LastErrorCode!="feed_unavailable"||status.Failures!=1{t.Fatalf("failure: %+v %v",status,err)}
    third,err:=database.ClaimDuePodcastRefresh(ctx,now.Add(podcastRefreshLease+3*time.Second))
    if err!=nil||third!=nil{t.Fatalf("backoff ignored: %#v %v",third,err)}
    if err=database.UnsubscribePodcast(ctx,id);err!=nil{t.Fatal(err)}
    err=database.SchedulePodcastRefresh(ctx,id)
    if !errors.Is(err,sql.ErrNoRows){t.Fatalf("unsubscribed refresh queued: %v",err)}
}


func TestPodcastInitialFetchDoesNotRedownload(t *testing.T) {
    database,err:=New(filepath.Join(t.TempDir(),"initial.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    id,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/feed",Title:"Example",
    },true)
    if err!=nil{t.Fatal(err)}
    if err=database.RecordInitialPodcastFetch(ctx,id,"\"v1\"","Sun, 22 Nov 2026 12:00:00 GMT");err!=nil{t.Fatal(err)}
    due,err:=database.ClaimDuePodcastRefresh(ctx,time.Now())
    if err!=nil||due!=nil{t.Fatalf("initial fetch should not immediately repeat: %+v %v",due,err)}
    if err=database.SchedulePodcastRefresh(ctx,id);err!=nil{t.Fatal(err)}
    due,err=database.ClaimDuePodcastRefresh(ctx,time.Now())
    if err!=nil||due==nil||due.ETag!="\"v1\""||due.LastModified==""{
        t.Fatalf("manual refresh lost validators: %+v %v",due,err)
    }
}


func TestPodcastRefreshRedirectCollisionDoesNotMergeShows(t *testing.T) {
    database,err:=New(filepath.Join(t.TempDir(),"conflicts.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    firstURL:="https://first.example.org/feed"
    secondURL:="https://second.example.org/feed"
    firstID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{URL:firstURL,Title:"First"},true)
    if err!=nil{t.Fatal(err)}
    secondID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{URL:secondURL,Title:"Second"},true)
    if err!=nil{t.Fatal(err)}
    // Only the first feed is due. SavePodcastFeedSnapshot deliberately leaves
    // unscheduled synthetic fixtures immediately eligible, unlike production
    // subscribe which records initial successful fetch validators and a
    // future normal refresh. Reflect that production behavior for feed two.
    if err=database.RecordInitialPodcastFetch(ctx,secondID,"","");err!=nil{t.Fatal(err)}
    if err=database.SchedulePodcastRefresh(ctx,firstID);err!=nil{t.Fatal(err)}
    target,err:=database.ClaimDuePodcastRefresh(ctx,time.Now())
    if err!=nil||target==nil||target.ID!=firstID{t.Fatalf("first target: %#v %v",target,err)}
    err=database.FinishPodcastRefresh(ctx,*target,podcast.FetchResult{Feed:podcast.Feed{
        URL:secondURL,Title:"Should Not Replace",
    }},time.Now())
    if err==nil{t.Fatal("redirect collision unexpectedly merged identities")}
    feeds,err:=database.ListSubscribedPodcasts(ctx)
    if err!=nil||len(feeds)!=2{t.Fatalf("lost subscriptions: %#v %v",feeds,err)}
    names:=map[string]string{}
    for _,feed:=range feeds {names[feed.ID]=feed.Title}
    if names[firstID]!="First"||names[secondID]!="Second"{
        t.Fatalf("redirect collision changed history: %#v",names)
    }
}
