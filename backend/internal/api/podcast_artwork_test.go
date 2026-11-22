package api

import (
    "bytes"
    "context"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastArtworkProxyScopesFeedAndForcesRasterHeaders(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"artwork.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    subscribed,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/rss",Title:"Artwork",
        ImageURL:"https://images.example.org/show.png?width=300",
    },true)
    if err!=nil{t.Fatal(err)}
    other,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/other",Title:"Not followed",
        ImageURL:"https://images.example.org/other.png",
    },false)
    if err!=nil{t.Fatal(err)}
    image:=[]byte{137,80,78,71,13,10,26,10}
    called:=0
    a:=&API{db:database,podcastArtworkFetch:func(_ context.Context,raw string)(podcast.Artwork,error){
        called++
        if raw!="https://images.example.org/show.png?width=300"{t.Fatalf("unexpected publisher URL: %q",raw)}
        return podcast.Artwork{Data:image,MIME:"image/png"},nil
    }}
    router:=a.PodcastRoutes()
    for _,id:=range []string{"missing",other}{
        response:=httptest.NewRecorder()
        router.ServeHTTP(response,httptest.NewRequest(http.MethodGet,"/feeds/"+id+"/artwork",nil))
        if response.Code!=http.StatusNotFound{t.Fatalf("unknown/unfollowed artwork HTTP %d",response.Code)}
    }
    if called!=0{t.Fatalf("unsubscribed image triggered remote work: %d",called)}
    response:=httptest.NewRecorder()
    router.ServeHTTP(response,httptest.NewRequest(http.MethodGet,"/feeds/"+subscribed+"/artwork",nil))
    if response.Code!=http.StatusOK || !bytes.Equal(response.Body.Bytes(),image){
        t.Fatalf("raster image not served: status=%d bytes=%v",response.Code,response.Body.Bytes())
    }
    if response.Header().Get("Content-Type")!="image/png" || response.Header().Get("X-Content-Type-Options")!="nosniff" {
        t.Fatalf("unsafe artwork headers: %#v",response.Header())
    }
    if called!=1{t.Fatalf("unexpected fetch count: %d",called)}
    if err=database.UnsubscribePodcast(ctx,subscribed);err!=nil{t.Fatal(err)}
    response=httptest.NewRecorder()
    router.ServeHTTP(response,httptest.NewRequest(http.MethodGet,"/feeds/"+subscribed+"/artwork",nil))
    if response.Code!=http.StatusNotFound{t.Fatalf("unfollowed artwork still available: %d",response.Code)}
}
func TestPodcastArtworkProxyMissingOrUnsafeContent(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"artwork.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    unsafeID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/private-art",Title:"Unsafe",ImageURL:"https://images.example.org/a.svg?token=secret",
    },true)
    if err!=nil{t.Fatal(err)}
    if url,err:=database.PodcastArtworkURL(ctx,unsafeID);err!=nil||url!=""{t.Fatalf("unsafe artwork was persisted: %q %v",url,err)}
    a:=&API{db:database}
    r:=httptest.NewRecorder()
    a.PodcastRoutes().ServeHTTP(r,httptest.NewRequest(http.MethodGet,"/feeds/"+unsafeID+"/artwork",nil))
    if r.Code!=http.StatusNotFound{t.Fatalf("missing image status %d",r.Code)}
    safeID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/good-art",Title:"Safe",ImageURL:"https://images.example.org/show.png",
    },true)
    if err!=nil{t.Fatal(err)}
    a.podcastArtworkFetch=func(_ context.Context,_ string)(podcast.Artwork,error){
        return podcast.Artwork{Data:[]byte("<svg></svg>"),MIME:"image/svg+xml"},nil
    }
    r=httptest.NewRecorder()
    a.PodcastRoutes().ServeHTTP(r,httptest.NewRequest(http.MethodGet,"/feeds/"+safeID+"/artwork",nil))
    if r.Code!=http.StatusUnprocessableEntity{t.Fatalf("SVG proxy response not blocked: %d",r.Code)}
}
