package podcast

import (
    "bytes"
    "context"
    "errors"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestPodcastFullMedia200OnlyStreamingAndLimits(t *testing.T){
    payload:=[]byte("ID3!podcast-audio")
    server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        if r.Header.Get("Range")!=""||r.Header.Get("If-Range")!=""{
            t.Errorf("full-file request still includes range headers: %+v",r.Header)
        }
        if r.Header.Get("Accept-Encoding")!="identity"{
            t.Errorf("full-file response permitted compressed transfer: %q",r.Header.Get("Accept-Encoding"))
        }
        switch r.URL.Path {
        case "/valid":
            w.Header().Set("Content-Type","audio/mpeg")
            _,_=w.Write(payload)
        case "/unknown":
            w.Header().Set("Content-Type","application/octet-stream")
            w.(http.Flusher).Flush()
            _,_=w.Write(payload)
        case "/too-big":
            w.Header().Set("Content-Type","audio/mpeg")
            _,_=w.Write(bytes.Repeat([]byte("x"),65))
        case "/wrong-type":
            w.Header().Set("Content-Type","text/html")
            _,_=w.Write([]byte("<html>please sign in</html>"))
        case "/disguised-html":
            w.Header().Set("Content-Type","audio/mpeg")
            _,_=w.Write([]byte("<html>not actually audio</html>"))
        case "/redirect-private":
            http.Redirect(w,r,"http://127.0.0.1/private",http.StatusFound)
        case "/partial":
            w.Header().Set("Content-Type","audio/mpeg")
            w.Header().Set("Content-Range","bytes 0-3/4")
            w.WriteHeader(http.StatusPartialContent)
            _,_=w.Write([]byte("ID3!"))
        case "/encoded":
            w.Header().Set("Content-Type","audio/mpeg")
            w.Header().Set("Content-Encoding","gzip")
            _,_=w.Write([]byte("bad"))
        default:
            http.NotFound(w,r)
        }
    }))
    defer server.Close()
    f:=testFetcher(t,server)
    for _,tc:=range []struct{path,declared string;limit int64;want error}{
        {"/valid","",64,nil},
        {"/unknown","audio/mpeg",64,nil},
        {"/too-big","",32,ErrMediaTooLarge},
        {"/wrong-type","audio/mpeg",64,ErrInvalidMedia},
        {"/disguised-html","",64,ErrInvalidMedia},
        {"/redirect-private","audio/mpeg",64,ErrBlockedURL},
        {"/partial","audio/mpeg",64,ErrMediaUnavailable},
        {"/encoded","audio/mpeg",64,ErrInvalidMedia},
    }{
        t.Run(tc.path,func(t *testing.T){
            var out bytes.Buffer
            result,err:=f.FetchFullMedia(context.Background(),
                "http://public.example"+tc.path,tc.declared,&out,tc.limit)
            if tc.want!=nil{
                if !errors.Is(err,tc.want){t.Fatalf("expected %v, got %v",tc.want,err)}
                return
            }
            if err!=nil||result.Size!=int64(len(payload))||result.MIME!="audio/mpeg"||
                !bytes.Equal(out.Bytes(),payload){
                t.Fatalf("unexpected full audio: result=%+v err=%v bytes=%q",result,err,out.Bytes())
            }
        })
    }
}

func TestPodcastFullMediaInvalidInputsFailClosed(t *testing.T){
    f:=NewFetcher(false)
    var out bytes.Buffer
    for _,url:=range []string{
        "file:///private/episode.mp3",
        "https://localhost:443/media",
        "http://127.0.0.1/private",
    }{
        if _,err:=f.FetchFullMedia(context.Background(),url,"audio/mpeg",&out,32);
            !errors.Is(err,ErrBlockedURL){t.Fatalf("accepted blocked URL %q: %v",url,err)}
    }
    if _,err:=f.FetchFullMedia(context.Background(),"http://public.example/media","audio/mpeg",&out,0);
        !errors.Is(err,ErrInvalidMedia){t.Fatalf("accepted zero maximum: %v",err)}
}
