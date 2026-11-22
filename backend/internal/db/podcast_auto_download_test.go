package db

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "path/filepath"
    "testing"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastAutoDownloadExplicitOptInOnlyNewEpisodes(t *testing.T){
    d,err:=New(filepath.Join(t.TempDir(),"auto-download.db"))
    if err!=nil{t.Fatal(err)}
    defer d.Close()
    ctx:=context.Background()
    feed:=podcast.Feed{URL:"https://example.org/rss",Title:"Example",
        Episodes:[]podcast.Episode{
            {GUID:"old",Title:"Old",EnclosureURL:"https://media.example.org/old.mp3",EnclosureType:"audio/mpeg"},
        }}
    id,err:=d.SavePodcastFeedSnapshot(ctx,feed,true)
    if err!=nil{t.Fatal(err)}
    policy,err:=d.GetPodcastAutoDownloadPolicy(ctx,id)
    if err!=nil||policy.Enabled||policy.NewestCount!=1{
        t.Fatalf("must be off by default: %+v %v",policy,err)
    }
    if _,err=d.UpdatePodcastAutoDownloadPolicy(ctx,id,PodcastAutoDownloadPolicy{
        Enabled:true,NewestCount:4,
    });err==nil{t.Fatal("accepted unbounded auto policy")}
    if _,err=d.UpdatePodcastAutoDownloadPolicy(ctx,"missing",PodcastAutoDownloadPolicy{
        Enabled:true,NewestCount:2,
    });!errors.Is(err,sql.ErrNoRows){t.Fatalf("unfollowed policy accepted: %v",err)}
    if _,err=d.UpdatePodcastAutoDownloadPolicy(ctx,id,PodcastAutoDownloadPolicy{
        Enabled:true,NewestCount:2,
    });err!=nil{t.Fatal(err)}

    original:=feed.Episodes[0]
    feed.Episodes=[]podcast.Episode{original}
    for i:=1;i<=4;i++{
        feed.Episodes=append(feed.Episodes,podcast.Episode{
            GUID:fmt.Sprintf("new-%d",i),Title:fmt.Sprintf("Episode %d",i),
            EnclosureURL:fmt.Sprintf("https://media.example.org/%d.mp3",i),
            EnclosureType:"audio/mpeg",
            PublishedAt:time.Now().Add(time.Duration(i)*time.Minute),
        })
    }
    // Force all new rows to be strictly after policy arming.
    now:=time.Now().Add(2*time.Second)
    target:=PodcastRefreshTarget{ID:id,URL:feed.URL}
    if err:=d.FinishPodcastRefresh(ctx,target,podcast.FetchResult{Feed:feed},now);err!=nil{t.Fatal(err)}
    rows,err:=d.ListPodcastDownloads(ctx)
    if err!=nil||len(rows)!=2{t.Fatalf("wanted max 2 newly discovered, got %+v %v",rows,err)}
    for _,row:=range rows{
        if row.EpisodeTitle=="Old"||row.Status!="queued"{
            t.Fatalf("historical/non-queued download: %+v",row)
        }
    }
    // A 304 and a repeat 200 snapshot cannot enqueue the old catalog,
    // and existing queued work must not be duplicated.
    if err:=d.FinishPodcastRefresh(ctx,target,podcast.FetchResult{NotModified:true},now.Add(time.Second));err!=nil{t.Fatal(err)}
    if err:=d.FinishPodcastRefresh(ctx,target,podcast.FetchResult{Feed:feed},now.Add(2*time.Second));err!=nil{t.Fatal(err)}
    rows,err=d.ListPodcastDownloads(ctx)
    if err!=nil||len(rows)!=2{t.Fatalf("duplicate refresh queued extra: %+v %v",rows,err)}
    // Turning the policy off is a non-destructive preference change.
    if _,err:=d.UpdatePodcastAutoDownloadPolicy(ctx,id,PodcastAutoDownloadPolicy{
        Enabled:false,NewestCount:1,
    });err!=nil{t.Fatal(err)}
    feed.Episodes=append(feed.Episodes,podcast.Episode{
        GUID:"new-disabled",Title:"No download",EnclosureURL:"https://media.example.org/no.mp3",
        EnclosureType:"audio/mpeg",
    })
    if err:=d.FinishPodcastRefresh(ctx,target,podcast.FetchResult{Feed:feed},now.Add(3*time.Second));err!=nil{t.Fatal(err)}
    rows,err=d.ListPodcastDownloads(ctx)
    if err!=nil||len(rows)!=2{t.Fatalf("policy disabled changed jobs: %+v %v",rows,err)}
    if err:=d.UnsubscribePodcast(ctx,id);err!=nil{t.Fatal(err)}
    if _,err:=d.GetPodcastAutoDownloadPolicy(ctx,id);!errors.Is(err,sql.ErrNoRows){
        t.Fatalf("unfollow retained policy: %v",err)
    }
}

func TestPodcastAutoDownloadValidatesEnclosuresAndCapsQueue(t *testing.T){
    d,err:=New(filepath.Join(t.TempDir(),"auto-caps.db"))
    if err!=nil{t.Fatal(err)}
    defer d.Close()
    ctx:=context.Background()
    feed:=podcast.Feed{URL:"https://example.org/caps",Title:"Caps"}
    id,err:=d.SavePodcastFeedSnapshot(ctx,feed,true)
    if err!=nil{t.Fatal(err)}
    if _,err:=d.UpdatePodcastAutoDownloadPolicy(ctx,id,PodcastAutoDownloadPolicy{
        Enabled:true,NewestCount:3,
    });err!=nil{t.Fatal(err)}
    for i:=0;i<18;i++{
        url:=fmt.Sprintf("https://media.example.org/%d.mp3",i)
        if i==17{url="http://127.0.0.1/internal.mp3"}
        feed.Episodes=append(feed.Episodes,podcast.Episode{
            GUID:fmt.Sprintf("episode-%d",i),Title:fmt.Sprintf("E%d",i),
            EnclosureURL:url,EnclosureType:"audio/mpeg",
            PublishedAt:time.Now().Add(time.Duration(i)*time.Minute),
        })
    }
    target:=PodcastRefreshTarget{ID:id,URL:feed.URL}
    now:=time.Now().Add(2*time.Second)
    if err:=d.FinishPodcastRefresh(ctx,target,podcast.FetchResult{Feed:feed},now);err!=nil{t.Fatal(err)}
    rows,err:=d.ListPodcastDownloads(ctx)
    if err!=nil||len(rows)!=3{t.Fatalf("per-show max 3 not enforced: %+v %v",rows,err)}
    for _,row:=range rows{
        if row.EpisodeTitle=="E17"{t.Fatal("private enclosure auto queued")}
    }
    // Even after new episodes arrive, existing per-show pending work blocks
    // further automatic queue growth. No queued work is silently deleted.
    feed.Episodes=append(feed.Episodes,podcast.Episode{
        GUID:"latest",Title:"Latest",EnclosureURL:"https://media.example.org/latest.mp3",
        EnclosureType:"audio/mpeg",
    })
    if err:=d.FinishPodcastRefresh(ctx,target,podcast.FetchResult{Feed:feed},now.Add(time.Second));err!=nil{t.Fatal(err)}
    rows,err=d.ListPodcastDownloads(ctx)
    if err!=nil||len(rows)!=3{t.Fatalf("per-show pending cap exceeded: %+v %v",rows,err)}
}
