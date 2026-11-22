package api

import (
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

func TestPodcastOPMLPreviewReportsPublicAndAlreadyFollowed(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"preview.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    _,err=database.SavePodcastFeedSnapshot(context.Background(),podcast.Feed{
        URL:"https://example.org/rss",Title:"Existing",
    },true)
    if err!=nil{t.Fatal(err)}
    requestXML:=`<opml version="2.0"><body>
<outline title="Already subscribed" xmlUrl="https://example.org/rss"/>
<outline title="A new podcast" xmlUrl="https://second.example.org/rss.xml"/>
<outline text="Private" xmlUrl="https://private.example.org/feed?token=secret"/>
<outline title="Duplicate" xmlUrl="https://example.org/rss"/>
</body></opml>`
    req:=httptest.NewRequest(http.MethodPost,"/import/opml/preview",strings.NewReader(requestXML))
    req.Header.Set("Content-Type","application/xml")
    response:=httptest.NewRecorder()
    (&API{db:database}).PodcastRoutes().ServeHTTP(response,req)
    if response.Code!=200{t.Fatalf("preview %d: %s",response.Code,response.Body.String())}
    if response.Header().Get("Cache-Control")!="no-store"{t.Fatal("OPML preview cacheable")}
    if strings.Contains(response.Body.String(),"secret"){t.Fatal("private URL leaked to preview")}
    var preview podcast.OPMLImportPreview
    if err=json.Unmarshal(response.Body.Bytes(),&preview);err!=nil{t.Fatal(err)}
    if len(preview.Feeds)!=2||!preview.Feeds[0].AlreadyFollowed||
        preview.Feeds[1].AlreadyFollowed||preview.InvalidCount!=1||preview.DuplicateCount!=1 {
        t.Fatalf("unexpected preview: %+v",preview)
    }
}

func TestPodcastOPMLPreviewRejectsDTDAndMalformedContentType(t *testing.T){
    database,err:=db.New(filepath.Join(t.TempDir(),"preview-invalid.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    cases:=[]struct{mime,body string;status int}{
        {"application/xml",`<!DOCTYPE opml [<!ENTITY x SYSTEM "file:///etc/passwd">]><opml version="2.0"><body/></opml>`,422},
        {"text/plain",`<opml version="2.0"><body/></opml>`,400},
        {"application/xml",`<opml version="2.0"><body>`,422},
    }
    for _,c:=range cases{
        req:=httptest.NewRequest(http.MethodPost,"/import/opml/preview",strings.NewReader(c.body))
        req.Header.Set("Content-Type",c.mime)
        response:=httptest.NewRecorder()
        (&API{db:database}).PodcastRoutes().ServeHTTP(response,req)
        if response.Code!=c.status{t.Fatalf("mime %q: got %d want %d: %s",c.mime,response.Code,c.status,response.Body.String())}
        if strings.Contains(response.Body.String(),"passwd"){t.Fatal("input was reflected in error")}
    }
}
