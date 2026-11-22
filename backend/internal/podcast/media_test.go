package podcast

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestPodcastMediaURLAndRangeGuards(t *testing.T) {
    for _,raw:=range []string{
        "http://publisher.example/audio.mp3",
        "https://127.0.0.1/audio.mp3",
        "https://localhost/audio.mp3",
        "https://user:pass@publisher.example/audio.mp3",
        "https://publisher.example/audio.mp3?token=secret",
        "https://publisher.example:8080/audio.mp3",
        "file:///tmp/audio.mp3",
    }{
        if _,err:=ValidateMediaURL(raw,false);!errors.Is(err,ErrBlockedURL){
            t.Errorf("unsafe media URL accepted: %s: %v",raw,err)
        }
    }
    if _,err:=ValidateMediaURL("https://publisher.example/media.mp3",false);err!=nil{t.Fatal(err)}
    for _,header:=range []string{
        "bytes=-500","bytes=2-1","bytes=0-1,2-4","items=0-2","bytes=abc-",
        "bytes=18446744073709551615-","bytes=17179869184-",
    }{
        if _,_,err:=parseMediaRange(header);!errors.Is(err,ErrInvalidMediaRange){
            t.Errorf("unsafe Range accepted: %q: %v",header,err)
        }
    }
    start,end,err:=parseMediaRange("bytes=128-")
    if err!=nil||start!=128||end!=128+MaxMediaSegmentBytes-1{
        t.Fatalf("open ended range must clamp: %d-%d %v",start,end,err)
    }
    start,end,err=parseMediaRange("bytes=0-10")
    if err!=nil||start!=0||end!=10{t.Fatalf("closed range: %d-%d %v",start,end,err)}
    if SupportedPodcastAudioMIME("text/html")!=""||SupportedPodcastAudioMIME("image/svg+xml")!=""{
        t.Fatal("non-audio content was accepted")
    }
    if got:=SupportedPodcastAudioMIME("audio/mpeg; charset=binary");got!="audio/mpeg"{
        t.Fatalf("supported publisher MIME not normalized: %q",got)
    }
}

func TestPodcastMediaSegment206AndBoundedOpenRange(t *testing.T) {
    source:=bytes.Repeat([]byte("a"),int(MaxMediaSegmentBytes)+50)
    server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        if r.URL.Path=="/redirect"{
            http.Redirect(w,r,"http://public.example/media",http.StatusFound)
            return
        }
        if r.URL.Path!="/media"{http.NotFound(w,r);return}
        if r.Header.Get("Accept-Encoding")!="identity"{t.Errorf("compression was not disabled")}
        if r.Header.Get("Range")!="bytes=0-2097151"&&r.Header.Get("Range")!="bytes=2097152-4194303"{
            t.Errorf("unexpected upstream Range: %q",r.Header.Get("Range"))
        }
        var first,last int
        _,err:=fmt.Sscanf(r.Header.Get("Range"),"bytes=%d-%d",&first,&last)
        if err!=nil{t.Errorf("malformed upstream range: %v",err)}
        if last>=len(source){last=len(source)-1}
        w.Header().Set("Content-Type","audio/mpeg")
        w.Header().Set("Content-Range",fmt.Sprintf("bytes %d-%d/%d",first,last,len(source)))
        w.WriteHeader(http.StatusPartialContent)
        _,_=w.Write(source[first:last+1])
    }))
    defer server.Close()
    fetcher:=testFetcher(t,server)
    first,err:=fetcher.FetchMediaSegment(context.Background(),"http://public.example/redirect","")
    if err!=nil||first.Start!=0||first.End!=MaxMediaSegmentBytes-1||
        first.Total!=int64(len(source))||len(first.Data)!=int(MaxMediaSegmentBytes){
        t.Fatalf("first segment unexpected: %+v err=%v",first,err)
    }
    last,err:=fetcher.FetchMediaSegment(context.Background(),"http://public.example/media","bytes=2097152-")
    if err!=nil||last.Start!=MaxMediaSegmentBytes||last.Total!=int64(len(source))||
        !bytes.Equal(last.Data,source[MaxMediaSegmentBytes:]){
        t.Fatalf("last segment unexpected: %+v err=%v",last,err)
    }
}

func TestPodcastMediaSegmentRejectsNonRangeWrongTypeAndBlockedRedirect(t *testing.T){
    source:=bytes.Repeat([]byte("z"),64)
    scenarios:=[]struct{path string;handler func(http.ResponseWriter,*http.Request);want error}{
        {"/full",func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","audio/mpeg");w.WriteHeader(200);_,_=w.Write(source)},ErrMediaUnavailable},
        {"/html",func(w http.ResponseWriter,r *http.Request){
            w.Header().Set("Content-Type","text/html")
            w.Header().Set("Content-Range","bytes 0-7/64")
            w.WriteHeader(206);_,_=w.Write(source[:8])
        },ErrInvalidMedia},
        {"/lying",func(w http.ResponseWriter,r *http.Request){
            w.Header().Set("Content-Type","audio/mpeg")
            w.Header().Set("Content-Range","bytes 5-7/64")
            w.WriteHeader(206);_,_=w.Write(source[:3])
        },ErrInvalidMedia},
        {"/blocked",func(w http.ResponseWriter,r *http.Request){
            http.Redirect(w,r,"http://127.0.0.1/private",http.StatusFound)
        },ErrBlockedURL},
        {"/large",func(w http.ResponseWriter,r *http.Request){
            w.Header().Set("Content-Type","audio/mpeg")
            w.Header().Set("Content-Range","bytes 0-2097151/2097152")
            w.WriteHeader(206)
            _,_=w.Write([]byte(strings.Repeat("x",int(MaxMediaSegmentBytes)+1)))
        },ErrInvalidMedia},
    }
    server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        for _,scenario:=range scenarios {
            if r.URL.Path==scenario.path{scenario.handler(w,r);return}
        }
        http.NotFound(w,r)
    }))
    defer server.Close()
    f:=testFetcher(t,server)
    for _,scenario:=range scenarios {
        _,err:=f.FetchMediaSegment(context.Background(),"http://public.example"+scenario.path,"bytes=0-7")
        if !errors.Is(err,scenario.want){t.Errorf("%s: expected %v, got %v",scenario.path,scenario.want,err)}
    }
}
