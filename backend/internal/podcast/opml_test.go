package podcast

import (
    "bytes"
    "encoding/xml"
    "errors"
    "strings"
    "testing"
)

func TestPodcastExportOPMLRoundTripAndEscaping(t *testing.T){
    doc,err:=ExportOPML([]OPMLSubscription{
        {Title:"News & <Science> \"Daily\"",URL:"https://example.org/rss?feed=rss"},
        {Title:"Second Show",URL:"https://another.example.org/podcasts.xml"},
    })
    if err!=nil{t.Fatal(err)}
    if !bytes.HasPrefix(doc,[]byte(xml.Header)) || !bytes.Contains(doc,[]byte(`version="2.0"`)){
        t.Fatalf("not OPML 2.0: %s",doc)
    }
    if bytes.Contains(doc,[]byte("<Science>")){t.Fatal("unescaped XML text")}
    var parsed struct{
        Version string `xml:"version,attr"`
        Body struct{
            Outlines []struct{
                Text string `xml:"text,attr"`
                XMLURL string `xml:"xmlUrl,attr"`
            } `xml:"outline"`
        } `xml:"body"`
    }
    if err=xml.Unmarshal(doc,&parsed);err!=nil{t.Fatal(err)}
    if parsed.Version!="2.0"||len(parsed.Body.Outlines)!=2 ||
        parsed.Body.Outlines[0].Text!="News & <Science> "Daily""||
        parsed.Body.Outlines[0].XMLURL!="https://example.org/rss?feed=rss" {
        t.Fatalf("OPML round trip mismatch: %+v",parsed)
    }
    empty,err:=ExportOPML(nil)
    if err!=nil || !bytes.Contains(empty,[]byte("<body>")){t.Fatalf("empty OPML invalid: %s %v",empty,err)}
}

func TestPodcastExportOPMLRejectsPrivateAndOversize(t *testing.T){
    for _,raw:=range []string{
        "http://example.org/rss",
        "https://127.0.0.1/private",
        "https://user:password@example.org/rss",
        "https://example.org/rss?token=secret",
    }{
        result,err:=ExportOPML([]OPMLSubscription{{Title:"Secret",URL:raw}})
        if !errors.Is(err,ErrInvalidOPMLExport)||result!=nil{
            t.Fatalf("unsafe feed exported: %q, err=%v",raw,err)
        }
        if err!=nil&&strings.Contains(err.Error(),"secret"){t.Fatal("feed secret leaked into error")}
    }
    tooMany:=make([]OPMLSubscription,MaxOPMLSubscriptions+1)
    if _,err:=ExportOPML(tooMany);!errors.Is(err,ErrInvalidOPMLExport){t.Fatal("too many subscriptions accepted")}
    huge:=[]OPMLSubscription{{Title:strings.Repeat("A",MaxOPMLBytes),URL:"https://example.org/rss"}}
    if _,err:=ExportOPML(huge);!errors.Is(err,ErrInvalidOPMLExport){t.Fatal("oversized OPML accepted")}
}
