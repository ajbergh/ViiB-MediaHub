package api

import (
    "context"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastProgressAPIReadWriteAndConflict(t *testing.T) {
    database,err:=db.New(filepath.Join(t.TempDir(),"progress.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    feedID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/show",Title:"Show",
        Episodes:[]podcast.Episode{{GUID:"a",Title:"An episode"}},
    },true)
    if err!=nil{t.Fatal(err)}
    episodes,err:=database.ListPodcastEpisodes(ctx,feedID,5)
    if err!=nil||len(episodes)!=1{t.Fatalf("episodes: %+v %v",episodes,err)}
    id:=episodes[0].ID
    router:=(&API{db:database}).PodcastRoutes()
    send:=func(method,payload string)*httptest.ResponseRecorder{
        t.Helper()
        req:=httptest.NewRequest(method,"/episodes/"+id+"/progress",strings.NewReader(payload))
        if payload!=""{req.Header.Set("Content-Type","application/json")}
        rec:=httptest.NewRecorder()
        router.ServeHTTP(rec,req)
        return rec
    }
    initial:=send(http.MethodGet,"")
    if initial.Code!=200||!strings.Contains(initial.Body.String(),`"revision":0`){
        t.Fatalf("initial: %d %s",initial.Code,initial.Body.String())
    }
    saved:=send(http.MethodPut,`{"positionMs":43000,"listenedMs":43000,"expectedRevision":0,"isPlayed":true}`)
    if saved.Code!=200||!strings.Contains(saved.Body.String(),`"revision":1`)||
        !strings.Contains(saved.Body.String(),`"isPlayed":true`){
        t.Fatalf("saved: %d %s",saved.Code,saved.Body.String())
    }
    stale:=send(http.MethodPut,`{"positionMs":1000,"listenedMs":1000,"expectedRevision":0,"isPlayed":false}`)
    if stale.Code!=http.StatusConflict||!strings.Contains(stale.Body.String(),"podcast_progress_conflict"){
        t.Fatalf("stale write should conflict: %d %s",stale.Code,stale.Body.String())
    }
    episodes,err=database.ListPodcastEpisodes(ctx,feedID,5)
    if err!=nil||!episodes[0].IsPlayed||episodes[0].PositionMs!=43000||
        episodes[0].ProgressRevision!=1 {
        t.Fatalf("episode page did not join durable progress: %+v %v",episodes,err)
    }
    invalid:=send(http.MethodPut,`{"positionMs":1,"listenedMs":1,"expectedRevision":1}`)
    if invalid.Code!=400{t.Fatalf("incomplete progress accepted: %d %s",invalid.Code,invalid.Body.String())}
}

func TestPodcastProgressAPIUnknownEpisode(t *testing.T) {
    database,err:=db.New(filepath.Join(t.TempDir(),"empty-progress.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    router:=(&API{db:database}).PodcastRoutes()
    for _,method:=range []string{http.MethodGet,http.MethodPut}{
        req:=httptest.NewRequest(method,"/episodes/unknown/progress",
            strings.NewReader(`{"positionMs":0,"listenedMs":0,"expectedRevision":0,"isPlayed":false}`))
        req.Header.Set("Content-Type","application/json")
        rec:=httptest.NewRecorder()
        router.ServeHTTP(rec,req)
        if rec.Code!=http.StatusNotFound{
            t.Fatalf("%s unknown episode should be 404, got %d: %s",method,rec.Code,rec.Body.String())
        }
    }
}
