package api

import (
    "context"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastEpisodePaginationRejectsInvalidQueries(t *testing.T) {
    database,err:=db.New(filepath.Join(t.TempDir(),"pages.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    id,err:=database.SavePodcastFeedSnapshot(context.Background(),podcast.Feed{
        URL:"https://example.org/rss",Title:"Paged",
        Episodes:[]podcast.Episode{{GUID:"one",Title:"Episode One"}},
    },true)
    if err!=nil{t.Fatal(err)}
    handler:=(&API{db:database}).PodcastRoutes()
    for _,params:=range []string{"limit=0","limit=101","limit=nope","offset=-1","offset=nope","offset=100001"}{
        req:=httptest.NewRequest(http.MethodGet,"/feeds/"+id+"/episodes?"+params,nil)
        response:=httptest.NewRecorder()
        handler.ServeHTTP(response,req)
        if response.Code!=http.StatusBadRequest{t.Fatalf("%s: status %d %s",params,response.Code,response.Body.String())}
    }
    req:=httptest.NewRequest(http.MethodGet,"/feeds/"+id+"/episodes?offset=0&limit=1",nil)
    response:=httptest.NewRecorder()
    handler.ServeHTTP(response,req)
    if response.Code!=http.StatusOK{t.Fatalf("valid page failed: %d %s",response.Code,response.Body.String())}
}
