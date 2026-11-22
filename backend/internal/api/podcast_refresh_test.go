package api

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "sync"
    "testing"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
    "github.com/go-chi/chi/v5"
)

type testPodcastFetcher struct {
    mu sync.Mutex
    calls int
    etag string
    result podcast.FetchResult
    err error
}
func (f *testPodcastFetcher) Fetch(_ context.Context,_ string,etag,_ string)(podcast.FetchResult,error){
    f.mu.Lock()
    defer f.mu.Unlock()
    f.calls++
    f.etag=etag
    return f.result,f.err
}

func TestPodcastRefreshWorkerAnd304(t *testing.T) {
    database,err:=db.New(filepath.Join(t.TempDir(),"viib.db"))
    if err!=nil {t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    if err=database.SetSetting("podcasts_preview_enabled","true");err!=nil {t.Fatal(err)}
    id,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/feed",Title:"First",
        Episodes:[]podcast.Episode{{GUID:"one",Title:"Initial"}},
    },true)
    if err!=nil {t.Fatal(err)}
    fake:=&testPodcastFetcher{result:podcast.FetchResult{
        Feed:podcast.Feed{URL:"https://new.example.org/feed",Title:"Updated",
            Episodes:[]podcast.Episode{{GUID:"one",Title:"Revised"},{GUID:"two",Title:"New"}}},
        ETag:"\"edition-two\"",
    }}
    a:=&API{db:database,podcastFetcher:fake}
    a.processPodcastRefresh(ctx)
    feeds,err:=database.ListSubscribedPodcasts(ctx)
    if err!=nil || len(feeds)!=1 || feeds[0].ID!=id || feeds[0].Title!="Updated" || feeds[0].EpisodeCount!=2 {
        t.Fatalf("unexpected results: %#v err=%v",feeds,err)
    }
    if err=database.SchedulePodcastRefresh(ctx,id);err!=nil{t.Fatal(err)}
    fake.mu.Lock()
    fake.result=podcast.FetchResult{NotModified:true}
    fake.mu.Unlock()
    a.processPodcastRefresh(ctx)
    fake.mu.Lock()
    count,etag:=fake.calls,fake.etag
    fake.mu.Unlock()
    if count!=2 || etag!="\"edition-two\"" {
        t.Fatalf("conditional request metadata: calls=%d etag=%q",count,etag)
    }
    status,err:=database.PodcastRefreshStatus(ctx,id)
    if err!=nil || status.Failures!=0 || status.LastSuccessAt==0 {
        t.Fatalf("status after 304: %+v err=%v",status,err)
    }
}

func TestPodcastRefreshEndpointQueuesOnlyWhenEnabled(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"viib.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    id,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{URL:"https://example.org/feed",Title:"Example"},true)
    if err!=nil{t.Fatal(err)}
    a:=&API{db:database}
    urlCtx:=chi.NewRouteContext()
    urlCtx.URLParams.Add("id",id)
    req:=httptest.NewRequest(http.MethodPost,"/feeds/"+id+"/refresh",nil)
    req=req.WithContext(context.WithValue(req.Context(),chi.RouteCtxKey,urlCtx))
    response:=httptest.NewRecorder()
    a.schedulePodcastRefresh(response,req)
    if response.Code!=http.StatusForbidden{t.Fatalf("disabled preview queued refresh: %d",response.Code)}
    if err=database.SetSetting("podcasts_preview_enabled","true");err!=nil{t.Fatal(err)}
    response=httptest.NewRecorder()
    a.schedulePodcastRefresh(response,req)
    if response.Code!=http.StatusAccepted{t.Fatalf("refresh not accepted: %d %s",response.Code,response.Body.String())}
    var body struct{Status string `json:"status"`}
    if err=json.Unmarshal(response.Body.Bytes(),&body);err!=nil||body.Status!="queued"{
        t.Fatalf("unexpected accepted response: %+v err=%v",body,err)
    }
    claimed,err:=database.ClaimDuePodcastRefresh(ctx,time.Now())
    if err!=nil||claimed==nil||claimed.ID!=id{t.Fatalf("durable work not queued: %+v err=%v",claimed,err)}
}
