package db

import (
    "context"
    "fmt"
    "path/filepath"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastEpisodesPaginationCoversAll(t *testing.T) {
    database,err:=New(filepath.Join(t.TempDir(),"pages.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    feed:=podcast.Feed{URL:"https://example.org/rss",Title:"Paged"}
    for i:=0;i<7;i++{
        feed.Episodes=append(feed.Episodes,podcast.Episode{
            GUID:fmt.Sprintf("episode-%d",i),Title:fmt.Sprintf("Episode %d",i),
        })
    }
    ctx:=context.Background()
    id,err:=database.SavePodcastFeedSnapshot(ctx,feed,true)
    if err!=nil{t.Fatal(err)}
    seen:=map[string]bool{}
    offset:=0
    for page:=0;page<5;page++{
        episodes,more,err:=database.ListPodcastEpisodesPage(ctx,id,3,offset)
        if err!=nil{t.Fatal(err)}
        if len(episodes)>3{t.Fatalf("unbounded page: %d",len(episodes))}
        for _,e:=range episodes{
            if seen[e.ID]{t.Fatalf("duplicated episode ID %q",e.ID)}
            seen[e.ID]=true
        }
        offset+=len(episodes)
        if !more{break}
    }
    if len(seen)!=7{t.Fatalf("pagination lost episodes: %d",len(seen))}
    empty,more,err:=database.ListPodcastEpisodesPage(ctx,id,3,100)
    if err!=nil||more||len(empty)!=0{t.Fatalf("unexpected end page: %#v more=%v err=%v",empty,more,err)}
}
