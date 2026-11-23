package podcast

import (
    "context"
    "encoding/base64"
    "errors"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func artworkPNG(t *testing.T)[]byte{
    t.Helper()
    data,err:=base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/6u0AAAAASUVORK5CYII=")
    if err!=nil{t.Fatal(err)}
    return data
}

func TestPodcastArtworkURLRejectsSecretsAndPrivateTargets(t *testing.T){
    invalid:=[]string{
        "http://images.example.com/img.png",
        "https://127.0.0.1/a.png",
        "https://user:pass@images.example.com/a.png",
        "https://images.example.com/img.png?token=secret",
        "https://images.example.com/img.png?width=secret",
        "https://images.example.com/img.png?size=evil",
        "https://images.example.com/img.png?width=0",
        "https://images.example.com/img.png?height=500000",
        "https://images.example.com/img.png?format=svg",
        "https://images.example.com/img.png?width=100&width=200",
        "file:///tmp/cover.png",
    }
    for _,s:=range invalid{
        if _,err:=ValidateArtworkURL(s,false);!errors.Is(err,ErrBlockedURL){t.Errorf("accepted unsafe URL %q: %v",s,err)}
        if safe:=SafeArtworkURL(s);safe!="" {t.Errorf("unsafe URL persisted: %s",safe)}
    }
    for _,safe:=range []string{
        "https://images.example.com/cover.png?width=300&format=webp&quality=85",
        "https://image.simplecastcdn.com/images/2be48404/3000x3000/image.jpg?aid=rss_feed",
        "https://cdn.example.org/art.jpg?auto=compress,format&fm=webp&dpr=2&crop=faces,center&v=20261122",
    }{
        if _,err:=ValidateArtworkURL(safe,false);err!=nil{t.Fatalf("safe CDN artwork %q rejected: %v",safe,err)}
        if got:=SafeArtworkURL(safe);got!=safe{t.Errorf("safe URL changed: %q -> %q",safe,got)}
    }
}

func TestPodcastArtworkFetchSniffingLimitsAndBlockedRedirects(t *testing.T){
    png:=artworkPNG(t)
    server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        switch r.URL.Path{
        case "/valid":
            w.Header().Set("Content-Type","application/octet-stream")
            _,_=w.Write(png)
        case "/html":
            w.Header().Set("Content-Type","image/png")
            _,_=w.Write([]byte("<html><script>bad()</script></html>"))
        case "/svg":
            w.Header().Set("Content-Type","image/svg+xml")
            _,_=w.Write([]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"))
        case "/huge":
            w.Header().Set("Content-Type","image/png")
            _,_=w.Write([]byte(strings.Repeat("x",int(MaxArtworkBytes)+1)))
        case "/redirect":
            http.Redirect(w,r,"http://127.0.0.1/admin",http.StatusFound)
        default:
            http.NotFound(w,r)
        }
    }))
    defer server.Close()
    f:=testFetcher(t,server)
    artwork,err:=f.FetchArtwork(context.Background(),"http://public.example/valid")
    if err!=nil||artwork.MIME!="image/png"||len(artwork.Data)!=len(png){t.Fatalf("valid png: %+v %v",artwork,err)}
    for _,path:=range []string{"/html","/svg","/huge"}{
        _,err:=f.FetchArtwork(context.Background(),"http://public.example"+path)
        if !errors.Is(err,ErrInvalidFeed){t.Errorf("accepted invalid artwork %s: %v",path,err)}
    }
    _,err=f.FetchArtwork(context.Background(),"http://public.example/redirect")
    if !errors.Is(err,ErrBlockedURL){t.Fatalf("private redirect followed: %v",err)}
    _,err=f.FetchArtwork(context.Background(),"http://127.0.0.1/valid")
    if !errors.Is(err,ErrBlockedURL){t.Fatalf("private direct target accepted: %v",err)}
}
