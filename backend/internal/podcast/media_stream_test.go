package podcast

import (
    "bytes"
    "context"
    "errors"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestPodcastFullAudioFallbackStreams200WithoutBufferingEpisode(t *testing.T){
    data:=append([]byte{0xff,0xfb,0x90,0x64},bytes.Repeat([]byte{0,1,2,3},2048)...)
    requests:=0
    server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        requests++
        if r.Header.Get("Range")!=""{t.Errorf("complete fallback must not send Range")}
        if r.Header.Get("Accept-Encoding")!="identity"{t.Errorf("expected identity encoding")}
        w.Header().Set("Content-Type","application/octet-stream")
        w.Header().Set("Content-Length","8196")
        w.WriteHeader(http.StatusOK)
        _,_=w.Write(data)
    }))
    defer server.Close()
    f:=testFetcher(t,server)
    rec:=httptest.NewRecorder()
    source:="http://public.example/audio.mp3?signature=abc%2Fxyz&expires=123456"
    if err:=f.StreamMedia(context.Background(),source,"audio/mpeg",rec);err!=nil{
        t.Fatalf("full response rejected: %v",err)
    }
    if rec.Code!=200||!bytes.Equal(rec.Body.Bytes(),data)||
        rec.Header().Get("Content-Type")!="audio/mpeg"||
        rec.Header().Get("Cache-Control")!="private, no-store"||
        rec.Header().Get("Accept-Ranges")!="none"||
        requests!=1 {
        t.Fatalf("invalid full stream: %d, headers %+v, size %d, calls %d",
            rec.Code,rec.Header(),rec.Body.Len(),requests)
    }
}

func TestPodcastFullAudioFallbackRejectsHTMLAndOversizedResponses(t *testing.T){
    for _,tc:=range []struct{name string;mime string;data []byte;length string}{
        {"html masquerading as audio","audio/mpeg",[]byte("<html><script>alert(1)</script></html>"),""},
        {"overlarge audio","audio/mpeg",[]byte("ignored"),"1073741825"},
    }{
        t.Run(tc.name,func(t *testing.T){
            server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
                w.Header().Set("Content-Type",tc.mime)
                if tc.length!=""{w.Header().Set("Content-Length",tc.length)}
                w.WriteHeader(200)
                _,_=w.Write(tc.data)
            }))
            defer server.Close()
            f:=testFetcher(t,server)
            rec:=httptest.NewRecorder()
            err:=f.StreamMedia(context.Background(),"http://public.example/a.mp3","audio/mpeg",rec)
            if !errors.Is(err,ErrInvalidMedia)||rec.Body.Len()!=0{
                t.Fatalf("invalid response accepted: err %v size %d",err,rec.Body.Len())
            }
        })
    }
}
