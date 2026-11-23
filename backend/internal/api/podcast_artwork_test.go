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

func TestPodcastArtworkRecoversMissingURLFromRSSAndPodcastIndex(t *testing.T) {
    t.Setenv("VIIB_PODCAST_INDEX_API_KEY", "pi-key")
    t.Setenv("VIIB_PODCAST_INDEX_API_SECRET", "pi-secret")
    t.Setenv("PODCAST_INDEX_API_KEY", "")
    t.Setenv("PODCAST_INDEX_API_SECRET", "")

    database, err := db.New(filepath.Join(t.TempDir(), "artwork-recover.db"))
    if err != nil {
        t.Fatal(err)
    }
    defer database.Close()
    ctx := context.Background()

    // 1. Subscribed feed with missing imageUrl recovers from RSS re-fetch (e.g. ?aid=rss_feed).
    rssShowID, err := database.SavePodcastFeedSnapshot(ctx, podcast.Feed{
        URL: "https://feeds.simplecast.com/freakonomics", Title: "Freakonomics Radio", ImageURL: "",
    }, true)
    if err != nil {
        t.Fatal(err)
    }
    img := []byte{137, 80, 78, 71, 13, 10, 26, 10}
    const simplecastArt = "https://image.simplecastcdn.com/images/freak/3000x3000/image.jpg?aid=rss_feed"
    a := &API{
        db: database,
        podcastFetcher: testFeedFetcher(func(_ context.Context, _ string) (podcast.FetchResult, error) {
            return podcast.FetchResult{
                Feed: podcast.Feed{
                    URL:      "https://feeds.simplecast.com/freakonomics",
                    Title:    "Freakonomics Radio",
                    ImageURL: simplecastArt,
                },
            }, nil
        }),
        podcastArtworkFetch: func(_ context.Context, raw string) (podcast.Artwork, error) {
            if raw != simplecastArt {
                t.Fatalf("unexpected recovered artwork URL: %q", raw)
            }
            return podcast.Artwork{Data: img, MIME: "image/png"}, nil
        },
    }
    rec := httptest.NewRecorder()
    a.PodcastRoutes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/feeds/"+rssShowID+"/artwork", nil))
    if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), img) {
        t.Fatalf("expected RSS artwork recovery 200, got %d", rec.Code)
    }
    persisted, err := database.PodcastArtworkURL(ctx, rssShowID)
    if err != nil || persisted != simplecastArt {
        t.Fatalf("expected recovered URL persisted, got %q (%v)", persisted, err)
    }

    // 2. When RSS still has no artwork, fall back to Podcast Index LookupByFeedURL.
    dirShowID, err := database.SavePodcastFeedSnapshot(ctx, podcast.Feed{
        URL: "https://example.org/no-art-in-rss.xml", Title: "No Art In RSS", ImageURL: "",
    }, true)
    if err != nil {
        t.Fatal(err)
    }
    const piArt = "https://cdn.podcastindex.org/covers/recovered.png"
    b := &API{
        db: database,
        podcastFetcher: testFeedFetcher(func(_ context.Context, _ string) (podcast.FetchResult, error) {
            return podcast.FetchResult{
                Feed: podcast.Feed{URL: "https://example.org/no-art-in-rss.xml", Title: "No Art In RSS"},
            }, nil
        }),
        podcastArtworkFetch: func(_ context.Context, raw string) (podcast.Artwork, error) {
            if raw != piArt {
                t.Fatalf("unexpected Podcast Index artwork URL: %q", raw)
            }
            return podcast.Artwork{Data: img, MIME: "image/png"}, nil
        },
    }
    b.setPodcastDirectoryClientForTest(&fakeLookupDirectoryClient{
        show: podcast.DirectoryShow{
            ID: 101, Title: "No Art In RSS", FeedURL: "https://example.org/no-art-in-rss.xml", ArtworkURL: piArt,
        },
        found: true,
    })
    defer b.setPodcastDirectoryClientForTest(nil)

    rec2 := httptest.NewRecorder()
    b.PodcastRoutes().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/feeds/"+dirShowID+"/artwork", nil))
    if rec2.Code != http.StatusOK || !bytes.Equal(rec2.Body.Bytes(), img) {
        t.Fatalf("expected Podcast Index fallback 200, got %d", rec2.Code)
    }
    persisted2, err := database.PodcastArtworkURL(ctx, dirShowID)
    if err != nil || persisted2 != piArt {
        t.Fatalf("expected Podcast Index URL persisted, got %q (%v)", persisted2, err)
    }
}

type testFeedFetcher func(context.Context, string) (podcast.FetchResult, error)

func (f testFeedFetcher) Fetch(ctx context.Context, raw, _, _ string) (podcast.FetchResult, error) {
    return f(ctx, raw)
}

type fakeLookupDirectoryClient struct {
    show  podcast.DirectoryShow
    found bool
}

func (f *fakeLookupDirectoryClient) Search(context.Context, podcast.DirectoryCredentials, string, int) ([]podcast.DirectoryShow, error) {
    return nil, nil
}
func (f *fakeLookupDirectoryClient) Trending(context.Context, podcast.DirectoryCredentials, string, string, int) ([]podcast.DirectoryShow, error) {
    return nil, nil
}
func (f *fakeLookupDirectoryClient) LookupByFeedURL(context.Context, podcast.DirectoryCredentials, string) (podcast.DirectoryShow, bool, error) {
    return f.show, f.found, nil
}
