package api

import (
    "bytes"
    "context"
    "errors"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastMediaEndpointOnlyForFollowedEpisodeAndRangeHeaders(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"media-api.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    feedID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://publisher.example/rss",Title:"Followed show",
        Episodes:[]podcast.Episode{{
            GUID:"one",Title:"Audio episode",EnclosureURL:"https://audio.example.org/one.mp3",
            EnclosureType:"audio/mpeg",
        }},
    },true)
    if err!=nil{t.Fatal(err)}
    episodes,err:=database.ListPodcastEpisodes(ctx,feedID,10)
    if err!=nil||len(episodes)!=1{t.Fatalf("episodes: %+v %v",episodes,err)}
    id:=episodes[0].ID
    calls:=0
    handler:=(&API{db:database,podcastMediaFetch:func(_ context.Context,raw,rangeHeader string)(podcast.MediaSegment,error){
        calls++
        if raw!="https://audio.example.org/one.mp3"||rangeHeader!="bytes=10-13"{
            t.Fatalf("unexpected publisher source/range: %q %q",raw,rangeHeader)
        }
        return podcast.MediaSegment{Data:[]byte("ABCD"),MIME:"audio/mpeg",
            Start:10,End:13,Total:100},nil
    }}).PodcastRoutes()
    bad:=httptest.NewRecorder()
    handler.ServeHTTP(bad,httptest.NewRequest(http.MethodGet,"/episodes/unknown/media",nil))
    if bad.Code!=404||calls!=0{t.Fatalf("unknown episode proxied remotely: %d calls %d",bad.Code,calls)}
    request:=httptest.NewRequest(http.MethodGet,"/episodes/"+id+"/media",nil)
    request.Header.Set("Range","bytes=10-13")
    response:=httptest.NewRecorder()
    handler.ServeHTTP(response,request)
    if response.Code!=http.StatusPartialContent ||
        !bytes.Equal(response.Body.Bytes(),[]byte("ABCD")) ||
        response.Header().Get("Content-Range")!="bytes 10-13/100" ||
        response.Header().Get("Accept-Ranges")!="bytes" ||
        response.Header().Get("Content-Length")!="4" ||
        response.Header().Get("Content-Type")!="audio/mpeg" ||
        response.Header().Get("Cache-Control")!="private, no-store" ||
        calls!=1{
        t.Fatalf("media 206 contract failed: status=%d headers=%+v body=%q calls=%d",
            response.Code,response.Header(),response.Body.String(),calls)
    }
    if err=database.UnsubscribePodcast(ctx,feedID);err!=nil{t.Fatal(err)}
    unfollowed:=httptest.NewRecorder()
    handler.ServeHTTP(unfollowed,request)
    if unfollowed.Code!=404||calls!=1{
        t.Fatalf("unfollowed episode still proxied: %d calls %d",unfollowed.Code,calls)
    }
}

func TestPodcastMediaEndpointRejectsPublisherErrorsAndBadSource(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"media-errors.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    feedID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://publisher.example/rss",Title:"Show",
        Episodes:[]podcast.Episode{
            {GUID:"good",Title:"Good",EnclosureURL:"https://audio.example.org/a.mp3",EnclosureType:"audio/mpeg"},
            {GUID:"bad",Title:"Private",EnclosureURL:"http://127.0.0.1/private",EnclosureType:"audio/mpeg"},
            {GUID:"html",Title:"HTML",EnclosureURL:"https://audio.example.org/page",EnclosureType:"text/html"},
        },
    },true)
    if err!=nil{t.Fatal(err)}
    eps,err:=database.ListPodcastEpisodes(ctx,feedID,10)
    if err!=nil||len(eps)!=3{t.Fatalf("episodes: %+v %v",eps,err)}
    IDs:=map[string]string{}
    for _,ep:=range eps{IDs[ep.Title]=ep.ID}
    requests:=0
    fetchErr:=podcast.ErrInvalidMediaRange
    a:=&API{db:database,podcastMediaFetch:func(context.Context,string,string)(podcast.MediaSegment,error){
        requests++
        return podcast.MediaSegment{},fetchErr
    }}
    handler:=a.PodcastRoutes()
    request:=func(id string)*httptest.ResponseRecorder{
        rec:=httptest.NewRecorder()
        handler.ServeHTTP(rec,httptest.NewRequest(http.MethodGet,"/episodes/"+id+"/media",nil))
        return rec
    }
    for _,title:=range []string{"Private","HTML"}{
        rec:=request(IDs[title])
        if rec.Code!=422{t.Fatalf("%s accepted: %d %s",title,rec.Code,rec.Body.String())}
    }
    if requests!=0{t.Fatalf("unsafe source triggered upstream call %d",requests)}
    for _,scenario:=range []struct{err error;status int}{
        {podcast.ErrInvalidMediaRange,416},
        {podcast.ErrBlockedURL,422},
        {podcast.ErrInvalidMedia,422},
        {podcast.ErrMediaUnavailable,502},
        {errors.New("must not expose sensitive data"),502},
    }{
        fetchErr=scenario.err
        rec:=request(IDs["Good"])
        if rec.Code!=scenario.status{t.Fatalf("error %v -> HTTP %d body %s",scenario.err,rec.Code,rec.Body.String())}
        if bytes.Contains(rec.Body.Bytes(),[]byte("sensitive data")){t.Fatal("upstream error leaked in response")}
    }
    a.podcastMediaFetch=func(context.Context,string,string)(podcast.MediaSegment,error){
        return podcast.MediaSegment{Data:[]byte("<script>evil</script>"),MIME:"text/html",
            Start:0,End:20,Total:100},nil
    }
    rec:=request(IDs["Good"])
    if rec.Code!=502{t.Fatalf("untrusted media response accepted: %d",rec.Code)}
}
