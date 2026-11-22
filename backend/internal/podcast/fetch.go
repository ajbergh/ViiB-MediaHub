package podcast

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "io"
    "net"
    "net/http"
    "net/netip"
    "net/url"
    "strconv"
    "strings"
    "time"
)

var (
    ErrBlockedURL = errors.New("blocked podcast URL")
    ErrFeedUnavailable = errors.New("podcast feed unavailable")
    deniedIPv4 = []netip.Prefix{
        netip.MustParsePrefix("0.0.0.0/8"),
        netip.MustParsePrefix("10.0.0.0/8"),
        netip.MustParsePrefix("100.64.0.0/10"),
        netip.MustParsePrefix("127.0.0.0/8"),
        netip.MustParsePrefix("169.254.0.0/16"),
        netip.MustParsePrefix("172.16.0.0/12"),
        netip.MustParsePrefix("192.0.0.0/24"),
        netip.MustParsePrefix("192.0.2.0/24"),
        netip.MustParsePrefix("192.88.99.0/24"),
        netip.MustParsePrefix("192.168.0.0/16"),
        netip.MustParsePrefix("198.18.0.0/15"),
        netip.MustParsePrefix("198.51.100.0/24"),
        netip.MustParsePrefix("203.0.113.0/24"),
        netip.MustParsePrefix("224.0.0.0/4"),
        netip.MustParsePrefix("240.0.0.0/4"),
    }
    deniedIPv6 = []netip.Prefix{
        netip.MustParsePrefix("::/128"),
        netip.MustParsePrefix("::1/128"),
        netip.MustParsePrefix("64:ff9b::/96"), // well-known NAT64 prefix
        netip.MustParsePrefix("64:ff9b:1::/48"),
        netip.MustParsePrefix("100::/64"),
        netip.MustParsePrefix("2001::/32"), // Teredo
        netip.MustParsePrefix("2001:db8::/32"),
        netip.MustParsePrefix("fc00::/7"),
        netip.MustParsePrefix("fe80::/10"),
        netip.MustParsePrefix("ff00::/8"),
    }
)

// IsPublicIP blocks loopback, LAN, carrier-grade NAT, benchmarking,
// documentation, metadata, transition and IPv4-mapped addresses.
// This is a denylist layered onto netip.IsGlobalUnicast; caller must also
// pin DNS to the validated address at connection time.
func IsPublicIP(ip netip.Addr) bool {
    if !ip.IsValid() || !ip.IsGlobalUnicast() { return false }
    if ip.Is4In6() { ip = ip.Unmap() }
    deny := deniedIPv6
    if ip.Is4() { deny = deniedIPv4 }
    for _, prefix := range deny {
        if prefix.Contains(ip) { return false }
    }
    return true
}

// ValidatePublicURL performs URL-shape validation only; Fetcher separately
// checks resolved addresses on every network connection.
func ValidatePublicURL(raw string, allowHTTP bool) (*url.URL, error) {
    u, err := url.Parse(strings.TrimSpace(raw))
    if err != nil || u == nil || u.Hostname() == "" || u.User != nil {
        return nil, ErrBlockedURL
    }
    if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
        return nil, ErrBlockedURL
    }
    if u.Opaque != "" || u.Fragment != "" { return nil, ErrBlockedURL }
    if strings.ContainsAny(u.Hostname(), "% \t\r\n") { return nil, ErrBlockedURL }
    if u.Port() != "" {
        port, err := strconv.Atoi(u.Port())
        if err != nil || !((u.Scheme == "https" && port == 443) || (u.Scheme == "http" && port == 80)) {
            return nil, ErrBlockedURL
        }
    }
    // Authenticated/tokenized publisher feeds are outside the initial public
    // RSS feature. They need encrypted backend storage and redacted export.
    for key := range u.Query() {
        switch strings.ToLower(key) {
        case "token", "key", "apikey", "api_key", "auth", "authorization", "signature", "sig", "password", "pass", "secret", "access_token":
            return nil, ErrBlockedURL
        }
    }
    if literal, err := netip.ParseAddr(u.Hostname()); err == nil && !IsPublicIP(literal) {
        return nil, ErrBlockedURL
    }
    return u, nil
}

type FetchResult struct {
    Feed Feed
    ETag string
    LastModified string
    NotModified bool
}

type Fetcher struct {
    AllowHTTP bool
    // These seams allow test-only deterministic fake DNS and local HTTP
    // fixtures without weakening production's public-IP policy.
    lookup func(context.Context, string) ([]netip.Addr, error)
    dial func(context.Context, string, string) (net.Conn, error)
}

func NewFetcher(allowHTTP bool) *Fetcher {
    return &Fetcher{
        AllowHTTP: allowHTTP,
        lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
            return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
        },
        dial: (&net.Dialer{Timeout: 5*time.Second, KeepAlive: -1}).DialContext,
    }
}

func (f *Fetcher) transport() *http.Transport {
    return &http.Transport{
        Proxy: nil, // environment proxies must not bypass dial-time DNS checks
        DisableCompression: true,
        DisableKeepAlives: true,
        ForceAttemptHTTP2: false,
        ResponseHeaderTimeout: 10*time.Second,
        TLSHandshakeTimeout: 8*time.Second,
        DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
            hostname, port, err := net.SplitHostPort(address)
            if err != nil { return nil, ErrBlockedURL }
            addresses, err := f.lookup(ctx, hostname)
            if err != nil || len(addresses) == 0 {
                return nil, ErrFeedUnavailable
            }
            // Reject mixed public/private DNS sets rather than selecting
            // another IP and potentially hiding DNS rebinding.
            for _, ip := range addresses {
                if !IsPublicIP(ip) { return nil, ErrBlockedURL }
            }
            // Dial exactly the IP that passed validation, not the hostname.
            return f.dial(ctx, network, net.JoinHostPort(addresses[0].String(), port))
        },
    }
}

// Fetch reads one public RSS feed with conditional GET, bounded redirect
// chains, DNS pinning and strict byte limits. It never returns upstream URL
// or request headers in errors, as those can contain private feed tokens.
func (f *Fetcher) Fetch(ctx context.Context, raw, etag, modified string) (FetchResult, error) {
    var result FetchResult
    u, err := ValidatePublicURL(raw, f.AllowHTTP)
    if err != nil { return result, err }
    tr := f.transport()
    defer tr.CloseIdleConnections()
    redirects := 0
    client := &http.Client{
        Transport: tr,
        Timeout: 20*time.Second,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            redirects++
            if redirects > 4 { return ErrBlockedURL }
            if _, err := ValidatePublicURL(req.URL.String(), f.AllowHTTP); err != nil {
                return ErrBlockedURL
            }
            if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
                return ErrBlockedURL
            }
            req.Header.Del("Authorization")
            req.Header.Del("Cookie")
            return nil
        },
    }
    request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
    if err != nil { return result, ErrBlockedURL }
    request.Header.Set("User-Agent", "ViiB-MediaHub-Podcast/1.0 (+https://github.com/ajbergh/ViiB-MediaHub)")
    request.Header.Set("Accept", "application/rss+xml, application/xml, text/xml;q=0.9")
    request.Header.Set("Accept-Encoding", "identity")
    if etag != "" { request.Header.Set("If-None-Match", etag) }
    if modified != "" { request.Header.Set("If-Modified-Since", modified) }
    response, err := client.Do(request)
    if err != nil {
        if errors.Is(err, ErrBlockedURL) { return result, ErrBlockedURL }
        return result, ErrFeedUnavailable
    }
    defer response.Body.Close()
    if response.StatusCode == http.StatusNotModified {
        result.NotModified = true
        result.ETag = response.Header.Get("ETag")
        result.LastModified = response.Header.Get("Last-Modified")
        return result, nil
    }
    if response.StatusCode != http.StatusOK {
        return result, fmt.Errorf("%w: HTTP %d", ErrFeedUnavailable, response.StatusCode)
    }
    if encoding := response.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
        return result, fmt.Errorf("%w: compressed feed not supported", ErrInvalidFeed)
    }
    if response.ContentLength > MaxFeedBytes { return result, ErrInvalidFeed }
    body, err := io.ReadAll(io.LimitReader(response.Body, MaxFeedBytes+1))
    if err != nil || int64(len(body)) > MaxFeedBytes { return result, ErrInvalidFeed }
    parsed, err := ParseRSS(bytes.NewReader(body), response.Request.URL.String())
    if err != nil { return result, err }
    result.Feed = parsed
    result.ETag = response.Header.Get("ETag")
    result.LastModified = response.Header.Get("Last-Modified")
    return result, nil
}
