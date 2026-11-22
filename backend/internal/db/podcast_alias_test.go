package db

import (
    "context"
    "path/filepath"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastRedirectAliasReusesFeedAndProgress(t *testing.T) {
    database,err:=New(filepath.Join(t.TempDir(),"aliases.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    requested:="https://original.example.org/podcast.xml"
    finalOne:="https://feeds.example.org/show.xml"
    finalTwo:="https://cdn.example.org/new/show.xml"
    first:=podcast.FetchResult{Feed:podcast.Feed{
        URL:finalOne, Title:"Feed Original",
        Episodes:[]podcast.Episode{{GUID:"stable-guid",Title:"Episode"}},
    },ETag:"\"original\""}
    firstID,err:=database.SavePodcastSubscriptionFromFetch(ctx,requested,first)
    if err!=nil{t.Fatal(err)}
    var episodeID string
    if err=database.conn.QueryRowContext(ctx,`
SELECT id FROM podcast_episodes WHERE feed_id=? AND guid='stable-guid'
`,firstID).Scan(&episodeID);err!=nil{t.Fatal(err)}
    if _,err=database.conn.ExecContext(ctx,`
INSERT INTO podcast_episode_state(episode_id,position_ms) VALUES (?,123000)
`,episodeID);err!=nil{t.Fatal(err)}
    // Publisher changes redirect target but original URL remains valid.
    second:=podcast.FetchResult{Feed:podcast.Feed{
        URL:finalTwo, Title:"Feed Relocated",
        Episodes:[]podcast.Episode{{GUID:"stable-guid",Title:"Edited episode"}},
    },ETag:"\"second\""}
    secondID,err:=database.SavePodcastSubscriptionFromFetch(ctx,requested,second)
    if err!=nil{t.Fatal(err)}
    if firstID!=secondID{t.Fatalf("redirect changed feed ID: %q vs %q",firstID,secondID)}
    // Following the new direct canonical URL must also yield the same ID.
    thirdID,err:=database.SavePodcastSubscriptionFromFetch(ctx,finalTwo,second)
    if err!=nil||thirdID!=firstID{t.Fatalf("direct canonical duplicated feed: %q %v",thirdID,err)}
    // Following a different source URL resolving to finalTwo also deduplicates.
    fourthID,err:=database.SavePodcastSubscriptionFromFetch(ctx,"https://alternate.example.org/rss",second)
    if err!=nil||fourthID!=firstID{t.Fatalf("same destination duplicated feed: %q %v",fourthID,err)}
    followed,err:=database.ListSubscribedPodcasts(ctx)
    if err!=nil||len(followed)!=1||followed[0].Title!="Feed Relocated"||followed[0].EpisodeCount!=1{
        t.Fatalf("duplicate or missing show: %#v %v",followed,err)
    }
    var position int64
    if err=database.conn.QueryRowContext(ctx,`SELECT position_ms FROM podcast_episode_state WHERE episode_id=?`,episodeID).Scan(&position);err!=nil{t.Fatal(err)}
    if position!=123000{t.Fatalf("position erased during redirect: %d",position)}
    for _,url:=range []string{requested,finalOne,"https://alternate.example.org/rss"}{
        var mapped string
        if err=database.conn.QueryRowContext(ctx,`SELECT feed_id FROM podcast_feed_aliases WHERE original_url=?`,url).Scan(&mapped);err!=nil||mapped!=firstID{
            t.Fatalf("alias for %s missing, id=%q err=%v",url,mapped,err)
        }
    }
}

func TestPodcastRedirectAliasesRejectPrivateFeeds(t *testing.T) {
    database,err:=New(filepath.Join(t.TempDir(),"aliases.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    result:=podcast.FetchResult{Feed:podcast.Feed{URL:"https://cdn.example.org/feed",Title:"Show"}}
    for _,url:=range []string{"http://127.0.0.1/rss","https://example.org/feed?token=secret","https://user:pass@example.org/rss"} {
        if _,err:=database.SavePodcastSubscriptionFromFetch(context.Background(),url,result);err==nil{
            t.Fatalf("unsafe alias persisted: %s",url)
        }
    }
    var count int
    if err:=database.conn.QueryRow(`SELECT COUNT(*) FROM podcast_feed_aliases`).Scan(&count);err!=nil{t.Fatal(err)}
    if count!=0{t.Fatalf("unsafe alias count=%d",count)}
}
