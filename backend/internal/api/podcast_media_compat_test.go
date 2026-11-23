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

func TestPodcastMediaNoRangeFallbackAndPrivateURLRedaction(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"signed-cdn.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    source:="https://cdn.example.org/pod.mp3?X-Amz-Signature=secret-value&Expires=3600"
    feedID,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://public.example/rss",Title:"Subscribed",
        Episodes:[]podcast.Episode{{
            GUID:"ep",Title:"Podcast chapter",EnclosureURL:source,EnclosureType:"audio/mpeg",
        }},
    },true)
    if err!=nil{t.Fatal(err)}
    episodes,err:=database.ListPodcastEpisodes(ctx,feedID,5)
    if err!=nil||len(episodes)!=1{t.Fatalf("episodes: %+v %v",episodes,err)}
    if !episodes[0].HasAudio{t.Fatal("audio metadata flag missing")}
    id:=episodes[0].ID
    getCalls,streamCalls:=0,0
    a:=&API{db:database,
        podcastMediaFetch:func(_ context.Context,url,rangeValue string)(podcast.MediaSegment,error){
            getCalls++
            if url!=source{t.Fatal("publisher URL not resolved from backend database")}
            return podcast.MediaSegment{},podcast.ErrPublisherNoRange
        },
        podcastMediaStream:func(_ context.Context,url,declared string,w http.ResponseWriter)error{
            streamCalls++
            if url!=source||declared!="audio/mpeg"{t.Fatal("wrong stream source or MIME")}
            w.Header().Set("Content-Type","audio/mpeg")
            w.WriteHeader(200)
            _,_=w.Write([]byte("media bytes"))
            return nil
        },
    }
    router:=a.PodcastRoutes()
    episodeResponse:=httptest.NewRecorder()
    router.ServeHTTP(episodeResponse,httptest.NewRequest("GET","/feeds/"+feedID+"/episodes",nil))
    if episodeResponse.Code!=200||
        bytes.Contains(episodeResponse.Body.Bytes(),[]byte("secret-value"))||
        bytes.Contains(episodeResponse.Body.Bytes(),[]byte("enclosureUrl"))||
        !bytes.Contains(episodeResponse.Body.Bytes(),[]byte(`"hasAudio":true`)){
        t.Fatalf("private enclosure leaked or availability missing: %s",episodeResponse.Body.String())
    }
    audioResponse:=httptest.NewRecorder()
    router.ServeHTTP(audioResponse,httptest.NewRequest("GET","/episodes/"+id+"/media",nil))
    if audioResponse.Code!=200||audioResponse.Body.String()!="media bytes"||
        getCalls!=1||streamCalls!=1{
        t.Fatalf("full audio stream fallback failed: status %d source calls %d stream calls %d",
            audioResponse.Code,getCalls,streamCalls)
    }
    seekReq:=httptest.NewRequest("GET","/episodes/"+id+"/media",nil)
    seekReq.Header.Set("Range","bytes=200-")
    denied:=httptest.NewRecorder()
    router.ServeHTTP(denied,seekReq)
    if denied.Code!=http.StatusRequestedRangeNotSatisfiable||streamCalls!=1{
        t.Fatalf("non-seekable publisher should reject nonzero seek: %d",denied.Code)
    }
    a.podcastMediaFetch=func(context.Context,string,string)(podcast.MediaSegment,error){
        return podcast.MediaSegment{},errors.New("do not print upstream URLs or tokens")
    }
    errorResponse:=httptest.NewRecorder()
    router.ServeHTTP(errorResponse,httptest.NewRequest("GET","/episodes/"+id+"/media",nil))
    if errorResponse.Code!=502||
        bytes.Contains(errorResponse.Body.Bytes(),[]byte("upstream URLs")){
        t.Fatalf("backend error exposed publisher details: %s",errorResponse.Body.String())
    }
}
