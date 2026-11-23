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

func TestPodcastContinueListeningAPIBounded(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"continue-api.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    feedID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/rss",Title:"Testing Continue",
        Episodes:[]podcast.Episode{{GUID:"one",Title:"Episode one"}},
    },true)
    if err!=nil{t.Fatal(err)}
    eps,err:=database.ListPodcastEpisodes(ctx,feedID,2)
    if err!=nil||len(eps)!=1{t.Fatalf("episodes: %+v %v",eps,err)}
    if _,err=database.SetPodcastProgress(ctx,eps[0].ID,43000,43000,0,false);err!=nil{t.Fatal(err)}
    router:=(&API{db:database}).PodcastRoutes()
    for _,tc:=range []struct{path string;status int}{
        {"/episodes/continue?limit=1",http.StatusOK},
        {"/episodes/continue?limit=0",http.StatusBadRequest},
        {"/episodes/continue?limit=1000",http.StatusBadRequest},
        {"/episodes/continue?limit=abc",http.StatusBadRequest},
    }{
        req:=httptest.NewRequest(http.MethodGet,tc.path,nil)
        rec:=httptest.NewRecorder()
        router.ServeHTTP(rec,req)
        if rec.Code!=tc.status{t.Fatalf("%s -> %d: %s",tc.path,rec.Code,rec.Body.String())}
        if tc.status==http.StatusOK && (!strings.Contains(rec.Body.String(),"Episode one")||
            !strings.Contains(rec.Body.String(),`"feedTitle":"Testing Continue"`)){
            t.Fatalf("missing episode metadata: %s",rec.Body.String())
        }
    }
}
