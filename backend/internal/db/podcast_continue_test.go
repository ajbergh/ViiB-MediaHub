package db

import (
    "context"
    "path/filepath"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastContinueListeningOnlyFollowedIncomplete(t *testing.T) {
    database,err:=New(filepath.Join(t.TempDir(),"continue.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    feedID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/rss",Title:"Following",
        Episodes:[]podcast.Episode{
            {GUID:"one",Title:"In progress"},{GUID:"two",Title:"Completed"},
        },
    },true)
    if err!=nil{t.Fatal(err)}
    episodes,err:=database.ListPodcastEpisodes(ctx,feedID,10)
    if err!=nil||len(episodes)!=2{t.Fatalf("episodes: %+v %v",episodes,err)}
    for _,ep:=range episodes{
        played:=ep.Title=="Completed"
        _,err=database.SetPodcastProgress(ctx,ep.ID,30000,30000,0,played)
        if err!=nil{t.Fatal(err)}
    }
    active,err:=database.ListContinueListening(ctx,10)
    if err!=nil||len(active)!=1||active[0].Title!="In progress"||
        active[0].FeedTitle!="Following"||active[0].PositionMs!=30000 {
        t.Fatalf("continued episodes: %+v %v",active,err)
    }
    if err:=database.UnsubscribePodcast(ctx,feedID);err!=nil{t.Fatal(err)}
    active,err=database.ListContinueListening(ctx,10)
    if err!=nil||len(active)!=0{t.Fatalf("unfollowed episode leaked to Continue Listening: %+v %v",active,err)}
}
