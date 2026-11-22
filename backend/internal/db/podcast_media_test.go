package db

import (
    "context"
    "database/sql"
    "errors"
    "path/filepath"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastMediaSourceAccessControl(t *testing.T){
    database,err:=New(filepath.Join(t.TempDir(),"media-source.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    feedID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/rss",Title:"Show",
        Episodes:[]podcast.Episode{{GUID:"a",Title:"Example",
            EnclosureURL:"https://cdn.example.org/audio.mp3",EnclosureType:"audio/mpeg"}},
    },true)
    if err!=nil{t.Fatal(err)}
    eps,err:=database.ListPodcastEpisodes(ctx,feedID,1)
    if err!=nil||len(eps)!=1{t.Fatalf("episodes %+v %v",eps,err)}
    raw,declared,err:=database.PodcastMediaSource(ctx,eps[0].ID)
    if err!=nil||raw!="https://cdn.example.org/audio.mp3"||declared!="audio/mpeg"{
        t.Fatalf("resolved media metadata: %q %q %v",raw,declared,err)
    }
    if err=database.UnsubscribePodcast(ctx,feedID);err!=nil{t.Fatal(err)}
    _,_,err=database.PodcastMediaSource(ctx,eps[0].ID)
    if !errors.Is(err,sql.ErrNoRows){t.Fatalf("unfollowed episode accessible: %v",err)}
}
