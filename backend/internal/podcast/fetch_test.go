package podcast

import (
    "context"
    "errors"
    "net"
    "net/http"
    "net/http/httptest"
    "net/netip"
    "os"
    "strings"
    "testing"
)

func TestPublicAddressDenylist(t *testing.T) {
    for _, value := range []string{
        "127.0.0.1","10.1.2.3","192.168.2.8","172.16.0.1",
        "169.254.169.254","100.64.0.1","198.18.0.1","203.0.113.3",
        "0.0.0.1","224.1.1.1","::1","fc00::1","fe80::1",
        "::ffff:127.0.0.1","64:ff9b::a9fe:a9fe","2001:db8::1",
    } {
        if IsPublicIP(netip.MustParseAddr(value)) {
            t.Errorf("private or special-use IP was accepted: %s", value)
        }
    }
    for _, value := range []string{"8.8.8.8","1.1.1.1","2001:4860:4860::8888"} {
        if !IsPublicIP(netip.MustParseAddr(value)) {
            t.Errorf("public IP was rejected: %s", value)
        }
    }
}
func TestPublicURLValidation(t *testing.T) {
    for _, value := range []string{
        "file:///etc/passwd", "http://example.org/feed", "https://127.0.0.1/feed",
        "https://user:pass@example.org/feed", "https://example.org:1234/rss",
        "https://example.org/feed?access_token=secret", "https://[::1]/feed",
    } {
        if _, err := ValidatePublicURL(value, false); !errors.Is(err, ErrBlockedURL) {
            t.Errorf("unsafe URL accepted: %q err=%v", value, err)
        }
    }
    if _, err := ValidatePublicURL("https://example.org/feed?feed=rss2", false); err != nil {
        t.Fatal(err)
    }
    if _, err := ValidatePublicURL("http://example.org/feed", true); err != nil {
        t.Fatal(err)
    }
}

// Supply public fake DNS and a local test-only dial, keeping production's
// default resolver/dial and SSRF checks unchanged.
func testFetcher(t *testing.T, server *httptest.Server) *Fetcher {
    t.Helper()
    f := NewFetcher(true)
    f.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
        if host == "public.example" { return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil }
        return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
    }
    f.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
        return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
    }
    return f
}

func TestFetchConditionalAndRedirectBlock(t *testing.T) {
    fixture, err := osReadFixture()
    if err != nil { t.Fatal(err) }
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/rss":
            if r.Header.Get("If-None-Match") == `"v1"` {
                w.WriteHeader(http.StatusNotModified)
                return
            }
            w.Header().Set("ETag", `"v1"`)
            w.Header().Set("Content-Type", "application/rss+xml")
            _, _ = w.Write([]byte(fixture))
        case "/redirect":
            http.Redirect(w,r,"http://127.0.0.1/private",http.StatusFound)
        default:
            http.NotFound(w,r)
        }
    }))
    defer server.Close()
    f:=testFetcher(t,server)
    got,err:=f.Fetch(context.Background(),"http://public.example/rss","","")
    if err!=nil || got.Feed.Title!="Example & Friends" || got.ETag!=`"v1"` {
        t.Fatalf("fetch failed: %#v err %v",got,err)
    }
    cached,err:=f.Fetch(context.Background(),"http://public.example/rss",got.ETag,"")
    if err!=nil || !cached.NotModified { t.Fatalf("304 handling: %#v err=%v",cached,err) }
    _,err=f.Fetch(context.Background(),"http://public.example/redirect","","")
    if !errors.Is(err,ErrBlockedURL){t.Fatalf("redirect SSRF not blocked: %v",err)}
}

func TestFetchDNSRebindingAndOversize(t *testing.T) {
    server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
        _,_=w.Write([]byte(strings.Repeat("x",int(MaxFeedBytes)+1)))
    }))
    defer server.Close()
    f:=testFetcher(t,server)
    _,err:=f.Fetch(context.Background(),"http://127.0.0.1/rss","","")
    if !errors.Is(err,ErrBlockedURL){t.Fatalf("local literal accepted: %v",err)}
    _,err=f.Fetch(context.Background(),"http://public.example/large","","")
    if !errors.Is(err,ErrInvalidFeed){t.Fatalf("oversize accepted: %v",err)}
    // Same dialer, but fake DNS now returns private IP at connection time.
    f.lookup=func(_ context.Context,_ string)([]netip.Addr,error){
        return []netip.Addr{netip.MustParseAddr("8.8.8.8"),netip.MustParseAddr("127.0.0.1")},nil
    }
    _,err=f.Fetch(context.Background(),"http://public.example/rss","","")
    if !errors.Is(err,ErrBlockedURL){t.Fatalf("mixed DNS records accepted: %v",err)}
}

func osReadFixture() (string,error) {
    data,err:=os.ReadFile("testdata/basic.xml")
    return string(data),err
}
