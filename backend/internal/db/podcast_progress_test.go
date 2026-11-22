package db

import (
    "context"
    "database/sql"
    "errors"
    "path/filepath"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastProgressCASAndPersistence(t *testing.T){
    filename:=filepath.Join(t.TempDir(),"progress.db")
    database,err:=New(filename)
    if err!=nil{t.Fatal(err)}
    ctx:=context.Background()
    id,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/show",Title:"Show",
        Episodes:[]podcast.Episode{{GUID:"a",Title:"Episode"}},
    },true)
    if err!=nil{t.Fatal(err)}
    episodes,err:=database.ListPodcastEpisodes(ctx,id,1)
    if err!=nil||len(episodes)!=1{t.Fatalf("episodes: %+v %v",episodes,err)}
    episodeID:=episodes[0].ID
    initial,err:=database.GetPodcastProgress(ctx,episodeID)
    if err!=nil||initial.Revision!=0||initial.PositionMs!=0{t.Fatalf("empty state: %+v %v",initial,err)}
    if _,err=database.SetPodcastProgress(ctx,episodeID,5000,1000,5,false);!errors.Is(err,ErrPodcastProgressConflict){
        t.Fatalf("first progress skipped revision zero: %v",err)
    }
    next,err:=database.SetPodcastProgress(ctx,episodeID,60000,60000,0,false)
    if err!=nil||next.PositionMs!=60000||next.Revision!=1||next.ListenedMs!=60000||next.IsPlayed{
        t.Fatalf("first progress: %+v %v",next,err)
    }
    _,err=database.SetPodcastProgress(ctx,episodeID,1000,1000,0,false)
    if !errors.Is(err,ErrPodcastProgressConflict){t.Fatalf("stale write accepted: %v",err)}
    after,err:=database.GetPodcastProgress(ctx,episodeID)
    if err!=nil||after.PositionMs!=60000{t.Fatalf("stale write corrupted position: %+v %v",after,err)}
    // Backward seeking is legal; cumulative listening time is monotonic.
    seek,err:=database.SetPodcastProgress(ctx,episodeID,30000,40000,1,false)
    if err!=nil||seek.Revision!=2||seek.PositionMs!=30000||seek.ListenedMs!=60000{
        t.Fatalf("seek or listening total failed: %+v %v",seek,err)
    }
    played,err:=database.SetPodcastProgress(ctx,episodeID,120000,90000,2,true)
    if err!=nil||played.Revision!=3||!played.IsPlayed||played.CompletedAt==0{
        t.Fatalf("completion: %+v %v",played,err)
    }
    if err=database.Close();err!=nil{t.Fatal(err)}
    reopened,err:=New(filename)
    if err!=nil{t.Fatal(err)}
    defer reopened.Close()
    persisted,err:=reopened.GetPodcastProgress(ctx,episodeID)
    if err!=nil||persisted.PositionMs!=120000||persisted.Revision!=3||!persisted.IsPlayed{
        t.Fatalf("restart lost progress: %+v %v",persisted,err)
    }
    undone,err:=reopened.SetPodcastProgress(ctx,episodeID,120000,90000,3,false)
    if err!=nil||undone.IsPlayed||undone.CompletedAt!=0{t.Fatalf("mark unplayed failed: %+v %v",undone,err)}
}
func TestPodcastProgressRejectsUnknownAndInvalid(t *testing.T){
    database,err:=New(filepath.Join(t.TempDir(),"empty.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    if _,err=database.GetPodcastProgress(ctx,"missing");!errors.Is(err,sql.ErrNoRows){t.Fatalf("unknown state: %v",err)}
    if _,err=database.SetPodcastProgress(ctx,"missing",1000,1000,0,false);!errors.Is(err,sql.ErrNoRows){
        t.Fatalf("unknown episode progress: %v",err)
    }
    if _,err=database.SetPodcastProgress(ctx,"missing",-1,0,0,false);err==nil{t.Fatal("negative position accepted")}
    if _,err=database.SetPodcastProgress(ctx,"missing",0,0,-1,false);err==nil{t.Fatal("negative revision accepted")}
}
