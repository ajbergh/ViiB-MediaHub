package api

import (
    "context"
    "encoding/xml"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastExportOnlyFollowedFeedsAndIsValidOPML(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"opml.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    _,err=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/podcast.xml",Title:"Science & Friends <Live>",
        Episodes:[]podcast.Episode{{GUID:"one",Title:"Do not export episode metadata"}},
    },true)
    if err!=nil{t.Fatal(err)}
    _,err=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://another.example.org/rss",Title:"Not followed",
    },false)
    if err!=nil{t.Fatal(err)}
    handler:=(&API{db:database}).PodcastRoutes()
    request:=httptest.NewRequest(http.MethodGet,"/export/opml",nil)
    response:=httptest.NewRecorder()
    handler.ServeHTTP(response,request)
    if response.Code!=http.StatusOK{t.Fatalf("HTTP %d: %s",response.Code,response.Body.String())}
    if response.Header().Get("Content-Disposition")!=`attachment; filename="viib-podcasts.opml"` ||
        !strings.HasPrefix(response.Header().Get("Content-Type"),"application/xml") ||
        response.Header().Get("Cache-Control")!="no-store" {
        t.Fatalf("unsafe OPML export headers: %#v",response.Header())
    }
    text:=response.Body.String()
    if strings.Contains(text,"Not followed")||strings.Contains(text,"Do not export episode metadata"){
        t.Fatal("exported unrelated or listening metadata")
    }
    var parsed struct{
        Version string `xml:"version,attr"`
        Body struct{
            Outlines []struct{
                Text string `xml:"text,attr"`
                XMLURL string `xml:"xmlUrl,attr"`
            } `xml:"outline"`
        } `xml:"body"`
    }
    if err=xml.Unmarshal(response.Body.Bytes(),&parsed);err!=nil{t.Fatal(err)}
    if parsed.Version!="2.0"||len(parsed.Body.Outlines)!=1||
       parsed.Body.Outlines[0].Text!="Science & Friends <Live>" ||
       parsed.Body.Outlines[0].XMLURL!="https://example.org/podcast.xml" {
        t.Fatalf("OPML invalid: %+v",parsed)
    }
}
