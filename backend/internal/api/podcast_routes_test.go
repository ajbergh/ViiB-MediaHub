package api

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastRoutesListAndUnsubscribe(t *testing.T) {
    database,err:=db.New(filepath.Join(t.TempDir(),"viib.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    handler:=(&API{db:database}).PodcastRoutes()
    check:=func(method,path,payload string,want int) *httptest.ResponseRecorder {
        t.Helper()
        req:=httptest.NewRequest(method,path,strings.NewReader(payload))
        if payload!="" {req.Header.Set("Content-Type","application/json")}
        response:=httptest.NewRecorder()
        handler.ServeHTTP(response,req)
        if response.Code!=want{t.Fatalf("%s %s: status %d, body %s",method,path,response.Code,response.Body.String())}
        return response
    }
    response:=check(http.MethodGet,"/feeds","",http.StatusOK)
    var initial struct{Feeds []db.PodcastFeedRecord `json:"feeds"`}
    if err:=json.Unmarshal(response.Body.Bytes(),&initial);err!=nil||len(initial.Feeds)!=0{
        t.Fatalf("unexpected initial feeds %v %v",initial,err)
    }
    id,err:=database.SavePodcastFeedSnapshot(context.Background(),podcast.Feed{
        URL:"https://example.org/rss",Title:"Example Podcast",
        Episodes:[]podcast.Episode{{GUID:"episode-a",Title:"Episode A"}},
    },true)
    if err!=nil{t.Fatal(err)}
    response=check(http.MethodGet,"/feeds","",http.StatusOK)
    if !bytes.Contains(response.Body.Bytes(),[]byte("Example Podcast")){t.Fatalf("missing podcast in response: %s",response.Body.String())}
    response=check(http.MethodGet,"/feeds/"+id+"/episodes","",http.StatusOK)
    if !bytes.Contains(response.Body.Bytes(),[]byte("Episode A")){t.Fatalf("missing episode: %s",response.Body.String())}
    check(http.MethodDelete,"/feeds/"+id,"",http.StatusNoContent)
    check(http.MethodGet,"/feeds/"+id+"/episodes","",http.StatusNotFound)
    check(http.MethodDelete,"/feeds/"+id,"",http.StatusNotFound)
}

func TestPodcastRoutesRejectUntrustedInput(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"viib.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    handler:=(&API{db:database}).PodcastRoutes()
    for _,candidate:=range []string{
        `{"url":"https://127.0.0.1/private"}`,
        `{"url":"http://example.org/rss"}`,
        `{"url":"https://example.org/rss?token=supersecret"}`,
        `{"url":"file:///etc/passwd"}`,
        `{"url":"https://example.org/rss","extra":"x"}`,
    }{
        req:=httptest.NewRequest(http.MethodPost,"/feeds/preview",strings.NewReader(candidate))
        req.Header.Set("Content-Type","application/json")
        res:=httptest.NewRecorder()
        handler.ServeHTTP(res,req)
        if res.Code!=http.StatusBadRequest{
            t.Errorf("unsafe input accepted: status=%d body=%s",res.Code,res.Body.String())
        }
        if strings.Contains(res.Body.String(),"supersecret"){t.Fatal("secret reflected in error")}
    }
}


func TestPodcastPublicRSSPreviewAndFollowSuccess(t *testing.T) {
    database,err:=db.New(filepath.Join(t.TempDir(),"podcast-http.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    url:="https://publisher.example.org/podcast.xml"
    fake:=&testPodcastFetcher{result:podcast.FetchResult{Feed:podcast.Feed{
        URL:"https://cdn.publisher.example.org/podcast.xml",
        Title:"Working Podcast",Author:"Publisher",
        Episodes:[]podcast.Episode{{GUID:"e1",Title:"Episode One"}},
    }}}
    handler:=(&API{db:database,podcastFetcher:fake}).PodcastRoutes()
    post:=func(path string)*httptest.ResponseRecorder{
        req:=httptest.NewRequest(http.MethodPost,path,
            strings.NewReader("{\"url\":\""+url+"\"}"))
        req.Header.Set("Content-Type","application/json")
        response:=httptest.NewRecorder()
        handler.ServeHTTP(response,req)
        return response
    }
    preview:=post("/feeds/preview")
    if preview.Code!=http.StatusOK||!bytes.Contains(preview.Body.Bytes(),[]byte("Working Podcast")){
        t.Fatalf("RSS preview returned %d: %s",preview.Code,preview.Body.String())
    }
    followed:=post("/feeds")
    if followed.Code!=http.StatusCreated||!bytes.Contains(followed.Body.Bytes(),[]byte("Working Podcast")){
        t.Fatalf("RSS follow returned %d: %s",followed.Code,followed.Body.String())
    }
    list:=httptest.NewRecorder()
    handler.ServeHTTP(list,httptest.NewRequest(http.MethodGet,"/feeds",nil))
    if list.Code!=http.StatusOK||!bytes.Contains(list.Body.Bytes(),[]byte("Working Podcast")){
        t.Fatalf("RSS following missing: %d %s",list.Code,list.Body.String())
    }
    fake.mu.Lock()
    calls:=fake.calls
    fake.mu.Unlock()
    if calls!=2{t.Fatalf("preview+follow calls %d, expected two",calls)}
}
