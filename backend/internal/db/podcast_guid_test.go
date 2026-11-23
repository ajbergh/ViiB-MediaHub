package db

import (
    "context"
    "path/filepath"
    "testing"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastGuidChangePreservesPlaybackState(t *testing.T){
    database,err:=New(filepath.Join(t.TempDir(),"guid.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    published:=time.Date(2026,11,15,18,0,0,0,time.UTC)
    episode:=podcast.Episode{GUID:"original",Title:"Identical episode",EnclosureURL:"https://cdn.example.org/episode.mp3",DurationSeconds:1234,PublishedAt:published}
    feed:=podcast.Feed{URL:"https://example.org/podcast",Title:"Show",Episodes:[]podcast.Episode{episode}}
    id,err:=database.SavePodcastFeedSnapshot(ctx,feed,true)
    if err!=nil{t.Fatal(err)}
    var originalID string
    if err=database.conn.QueryRowContext(ctx,"SELECT id FROM podcast_episodes WHERE feed_id=?",id).Scan(&originalID);err!=nil{t.Fatal(err)}
    if _,err=database.conn.ExecContext(ctx,"INSERT INTO podcast_episode_state(episode_id,position_ms) VALUES (?,?)",originalID,123000);err!=nil{t.Fatal(err)}
    feed.Episodes[0].GUID="new-guid"
    if _,err=database.SavePodcastFeedSnapshot(ctx,feed,true);err!=nil{t.Fatal(err)}
    if err=database.SchedulePodcastRefresh(ctx,id);err!=nil{t.Fatal(err)}
    target,err:=database.ClaimDuePodcastRefresh(ctx,time.Now())
    if err!=nil||target==nil{t.Fatalf("refresh target: %+v %v",target,err)}
    feed.Episodes[0].Description="New publisher notes"
    if err=database.FinishPodcastRefresh(ctx,*target,podcast.FetchResult{Feed:feed},time.Now());err!=nil{t.Fatal(err)}
    var count int
    if err=database.conn.QueryRowContext(ctx,"SELECT COUNT(*) FROM podcast_episodes WHERE feed_id=?",id).Scan(&count);err!=nil{t.Fatal(err)}
    if count!=1{t.Fatalf("GUID churn duplicated episode: %d",count)}
    var actualID,guid string
    if err=database.conn.QueryRowContext(ctx,"SELECT id,guid FROM podcast_episodes WHERE feed_id=?",id).Scan(&actualID,&guid);err!=nil{t.Fatal(err)}
    if actualID!=originalID||guid!="new-guid"{t.Fatalf("migrated id=%s old=%s guid=%s",actualID,originalID,guid)}
    var position int64
    if err=database.conn.QueryRowContext(ctx,"SELECT position_ms FROM podcast_episode_state WHERE episode_id=?",originalID).Scan(&position);err!=nil{t.Fatal(err)}
    if position!=123000{t.Fatalf("position lost: %d",position)}
}

func TestPodcastGuidMatchingDoesNotMergeAmbiguousEpisodes(t *testing.T){
    database,err:=New(filepath.Join(t.TempDir(),"ambiguous.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    published:=time.Date(2026,11,20,12,0,0,0,time.UTC)
    makeEpisode:=func(guid,title string)podcast.Episode{
        return podcast.Episode{GUID:guid,Title:title,PublishedAt:published,DurationSeconds:120,EnclosureURL:"https://cdn.example.org/reused.mp3"}
    }
    feed:=podcast.Feed{URL:"https://example.org/rss",Title:"Ambiguous",Episodes:[]podcast.Episode{makeEpisode("one","Repeated"),makeEpisode("two","Repeated")}}
    id,err:=database.SavePodcastFeedSnapshot(ctx,feed,true)
    if err!=nil{t.Fatal(err)}
    feed.Episodes=[]podcast.Episode{makeEpisode("three","Repeated")}
    if _,err=database.SavePodcastFeedSnapshot(ctx,feed,true);err!=nil{t.Fatal(err)}
    var count int
    if err=database.conn.QueryRowContext(ctx,"SELECT COUNT(*) FROM podcast_episodes WHERE feed_id=?",id).Scan(&count);err!=nil{t.Fatal(err)}
    if count!=3{t.Fatalf("ambiguous identity merged: %d",count)}
    feed.Episodes=[]podcast.Episode{makeEpisode("four","A different broadcast")}
    if _,err=database.SavePodcastFeedSnapshot(ctx,feed,true);err!=nil{t.Fatal(err)}
    if err=database.conn.QueryRowContext(ctx,"SELECT COUNT(*) FROM podcast_episodes WHERE feed_id=?",id).Scan(&count);err!=nil{t.Fatal(err)}
    if count!=4{t.Fatalf("new broadcast merged: %d",count)}
}
