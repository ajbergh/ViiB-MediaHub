package podcast

import (
    "errors"
    "strings"
    "testing"
)

func TestPodcastOPMLImportNestedValidatedDeduplicated(t *testing.T){
    input:=`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>Subscribed</title></head><body>
<outline text="News"><outline type="rss" text="News &amp; Science" xmlUrl="https://public.example/rss.xml"/>
<outline text="duplicate" xmlUrl="https://public.example/rss.xml"/>
<outline type="rss" text="HTTP rejected" xmlUrl="http://public.example/rss"/>
<outline text="token secret" xmlUrl="https://public.example/rss?token=not-for-export"/>
<outline text="Second" xmlUrl="https://public.example/other.xml"/>
</outline></body></opml>`
    preview,err:=ParseOPMLImport([]byte(input))
    if err!=nil{t.Fatal(err)}
    if len(preview.Feeds)!=2||preview.DuplicateCount!=1||preview.InvalidCount!=2 {
        t.Fatalf("bad import preview %+v",preview)
    }
    if preview.Feeds[0].Title!="News & Science"||
        preview.Feeds[0].URL!="https://public.example/rss.xml"{
        t.Fatalf("unexpected extracted outline %+v",preview.Feeds[0])
    }
}

func TestPodcastOPMLImportRejectsEntitiesDepthAndOversize(t *testing.T){
    cases:=[]string{
        `<!DOCTYPE opml [ <!ENTITY password SYSTEM "file:///private"> ]><opml version="2.0"><body/></opml>`,
        `<opml version="1.0"><body/></opml>`,
        `<opml version="2.0"><body><outline></body></opml>`,
        `<feed><body/></feed>`,
    }
    nested:=`<opml version="2.0"><body>`+strings.Repeat("<outline>",MaxOPMLImportDepth+2)+
        strings.Repeat("</outline>",MaxOPMLImportDepth+2)+`</body></opml>`
    cases=append(cases,nested,strings.Repeat("a",MaxOPMLImportBytes+1))
    for _,input:=range cases{
        _,err:=ParseOPMLImport([]byte(input))
        if !errors.Is(err,ErrInvalidOPMLImport){t.Fatalf("input unexpectedly accepted: %v",err)}
    }
}

func TestPodcastOPMLImportEmptyBody(t *testing.T){
    preview,err:=ParseOPMLImport([]byte(`<opml version="2.0"><body></body></opml>`))
    if err!=nil||len(preview.Feeds)!=0{t.Fatalf("empty import: %+v %v",preview,err)}
}
