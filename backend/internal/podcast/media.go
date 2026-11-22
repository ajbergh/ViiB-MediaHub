package podcast

import (
    "context"
    "errors"
    "fmt"
    "io"
    "mime"
    "net/http"
    "net/netip"
    "net/url"
    "regexp"
    "strconv"
    "strings"
    "time"
)

// Every remote request and local HTTP response is capped; the browser can
// request subsequent ranges without buffering an entire publisher episode.
const MaxMediaSegmentBytes int64 = 2 << 20
const MaxMediaTotalBytes int64 = 16 << 30

var (
    ErrInvalidMedia = errors.New("unsupported podcast media")
    ErrMediaUnavailable = errors.New("podcast media unavailable")
    ErrInvalidMediaRange = errors.New("invalid podcast media range")
    ErrPublisherNoRange = errors.New("podcast publisher ignored Range")
)

var (
    requestedRangePattern = regexp.MustCompile(`^bytes=([0-9]+)-([0-9]*)$`)
    contentRangePattern = regexp.MustCompile(`^bytes ([0-9]+)-([0-9]+)/([0-9]+)$`)
)

type MediaSegment struct {
    Data []byte
    MIME string
    Start int64
    End int64
    Total int64
    // Strong validator metadata may be used internally by the downloader;
    // never serialize publisher response headers to the renderer.
    ETag string
    LastModified string
}

// StrongMediaETag accepts only bounded, well-formed strong HTTP entity tags.
// Weak tags cannot safely identify a specific representation for If-Range.
func StrongMediaETag(value string)string{
    v:=strings.TrimSpace(value)
    if len(v)<2||len(v)>256||v[0]!='"'||v[len(v)-1]!='"'{return ""}
    for i:=1;i<len(v)-1;i++{
        if v[i]<0x21||v[i]>0x7e||v[i]=='"'{return ""}
    }
    return v
}

func stableMediaLastModified(value string)string{
    stamp,err:=http.ParseTime(strings.TrimSpace(value))
    if err!=nil{return ""}
    return stamp.UTC().Format(http.TimeFormat)
}

// ValidateMediaURL applies the NETWORK policy to publisher enclosures,
// not the much narrower RSS subscription/OPML export URL policy.
// Signed CDN queries are common, must stay backend-only, and MUST never
// appear in rendered episode JSON, logs, error strings or redirect responses.
// Every dial is separately DNS-pinned and checked against the public-IP rules.
func ValidateMediaURL(raw string, allowHTTP bool) (*url.URL,error) {
    if raw==""||len(raw)>8192 || strings.ContainsAny(raw,"\r\n\x00"){return nil,ErrBlockedURL}
    parsed,err:=url.Parse(strings.TrimSpace(raw))
    if err!=nil||parsed==nil||parsed.Hostname()==""||parsed.User!=nil||
        parsed.Opaque!=""||parsed.Fragment!=""{return nil,ErrBlockedURL}
    if parsed.Scheme!="https" && !(allowHTTP && parsed.Scheme=="http"){return nil,ErrBlockedURL}
    if parsed.Port()!="" {
        port,err:=strconv.Atoi(parsed.Port())
        if err!=nil || !(parsed.Scheme=="https"&&port==443 || parsed.Scheme=="http"&&port==80){
            return nil,ErrBlockedURL
        }
    }
    hostname:=strings.ToLower(strings.TrimSuffix(parsed.Hostname(),"."))
    if hostname=="localhost"||strings.HasSuffix(hostname,".localhost")||
        strings.HasSuffix(hostname,".local")||strings.HasSuffix(hostname,".internal")||
        strings.HasSuffix(hostname,".home.arpa")||strings.HasSuffix(hostname,".lan")||
        strings.ContainsAny(hostname,"% \t\r\n") {return nil,ErrBlockedURL}
    // Reject all non-public literal IPv4/IPv6 addresses before connecting;
    // resolver addresses are checked independently in the dial callback.
    if literal,err:=netip.ParseAddr(hostname);err==nil&&!IsPublicIP(literal){
        return nil,ErrBlockedURL
    }
    return parsed,nil
}

// SupportedPodcastAudioMIME blocks HTML, SVG and non-audio response bodies.
// A media URL from RSS is metadata, not authorization to fetch any type.
func SupportedPodcastAudioMIME(raw string) string {
    base,_,err:=mime.ParseMediaType(strings.TrimSpace(raw))
    if err!=nil{return ""}
    switch strings.ToLower(base){
    case "audio/mpeg","audio/mp3","audio/x-mp3","audio/mpeg3",
        "audio/mp4","audio/x-m4a","audio/aac","audio/x-aac",
        "audio/ogg","audio/opus","audio/wav","audio/x-wav",
        "audio/flac","audio/x-flac","audio/webm","application/ogg":
        return strings.ToLower(base)
    default:
        return ""
    }
}

func parseMediaRange(header string)(int64,int64,error){
    if header==""{return 0,MaxMediaSegmentBytes-1,nil}
    parts:=requestedRangePattern.FindStringSubmatch(strings.TrimSpace(header))
    if len(parts)!=3{return 0,0,ErrInvalidMediaRange}
    start,err:=strconv.ParseInt(parts[1],10,64)
    if err!=nil||start<0||start>=MaxMediaTotalBytes{return 0,0,ErrInvalidMediaRange}
    maxEnd:=start+MaxMediaSegmentBytes-1
    if maxEnd>=MaxMediaTotalBytes{maxEnd=MaxMediaTotalBytes-1}
    end:=maxEnd
    if parts[2]!=""{
        requestedEnd,parseErr:=strconv.ParseInt(parts[2],10,64)
        if parseErr!=nil||requestedEnd<start{return 0,0,ErrInvalidMediaRange}
        if requestedEnd<end{end=requestedEnd}
    }
    return start,end,nil
}

func parseUpstreamContentRange(header string,start,end int64)(int64,int64,error){
    values:=contentRangePattern.FindStringSubmatch(strings.TrimSpace(header))
    if len(values)!=4{return 0,0,ErrInvalidMedia}
    first,err1:=strconv.ParseInt(values[1],10,64)
    last,err2:=strconv.ParseInt(values[2],10,64)
    total,err3:=strconv.ParseInt(values[3],10,64)
    if err1!=nil||err2!=nil||err3!=nil||first!=start||last<first||
        last>end||total<=last||total>MaxMediaTotalBytes{
        return 0,0,ErrInvalidMedia
    }
    return last,total,nil
}

// FetchMediaSegment requires a valid 206 Content-Range from the publisher,
// even when the browser sends no Range. Never silently proxy a whole 200,
// redirect response, compressed payload or unbounded stream.
func (f *Fetcher) FetchMediaSegment(ctx context.Context,raw,rangeHeader string)(MediaSegment,error){
    return f.FetchMediaSegmentWithHint(ctx,raw,rangeHeader,"")
}

// FetchMediaSegmentWithHint permits a publisher's generic octet-stream MIME
// only when its RSS enclosure explicitly declared a supported audio type.
// The body is still sniffed, so obvious HTML/XML cannot masquerade as audio.
func (f *Fetcher) FetchMediaSegmentWithHint(ctx context.Context,raw,rangeHeader,declaredType string)(MediaSegment,error){
    return f.FetchMediaSegmentWithIfRange(ctx,raw,rangeHeader,declaredType,"")
}

// Offline transfers send If-Range only with a syntactically valid *strong*
// ETag. A changed representation should trigger upstream 200, rejected here
// instead of splicing two different publisher files into one local download.
func (f *Fetcher) FetchMediaSegmentWithIfRange(ctx context.Context,raw,rangeHeader,declaredType,ifRange string)(MediaSegment,error){
    var segment MediaSegment
    if ifRange!=""&&StrongMediaETag(ifRange)!=ifRange{return segment,ErrInvalidMedia}
    u,err:=ValidateMediaURL(raw,true)
    if err!=nil{return segment,err}
    start,end,err:=parseMediaRange(rangeHeader)
    if err!=nil{return segment,err}
    tr:=f.transport()
    defer tr.CloseIdleConnections()
    client:=&http.Client{
        Transport:tr,
        Timeout:30*time.Second,
        CheckRedirect:func(req *http.Request,via []*http.Request)error{
            if len(via)>4{return ErrBlockedURL}
            if _,err:=ValidateMediaURL(req.URL.String(),true);err!=nil{return ErrBlockedURL}
            if len(via)>0&&via[len(via)-1].URL.Scheme=="https"&&req.URL.Scheme!="https"{
                return ErrBlockedURL
            }
            req.Header.Del("Authorization")
            req.Header.Del("Cookie")
            req.Header.Set("Range",fmt.Sprintf("bytes=%d-%d",start,end))
            if ifRange!=""{req.Header.Set("If-Range",ifRange)}else{req.Header.Del("If-Range")}
            req.Header.Set("Accept-Encoding","identity")
            return nil
        },
    }
    request,err:=http.NewRequestWithContext(ctx,http.MethodGet,u.String(),nil)
    if err!=nil{return segment,ErrBlockedURL}
    request.Header.Set("Range",fmt.Sprintf("bytes=%d-%d",start,end))
    if ifRange!=""{request.Header.Set("If-Range",ifRange)}
    request.Header.Set("Accept","audio/*, application/ogg;q=0.9")
    request.Header.Set("Accept-Encoding","identity")
    request.Header.Set("User-Agent","ViiB-MediaHub-Podcast/1.0")
    response,err:=client.Do(request)
    if err!=nil{
        if errors.Is(err,ErrBlockedURL){return segment,ErrBlockedURL}
        return segment,ErrMediaUnavailable
    }
    defer response.Body.Close()
    if response.StatusCode==http.StatusRequestedRangeNotSatisfiable{
        return segment,ErrInvalidMediaRange
    }
    if response.StatusCode==http.StatusOK{
        return segment,ErrPublisherNoRange
    }
    if response.StatusCode!=http.StatusPartialContent{
        return segment,ErrMediaUnavailable
    }
    if encoding:=response.Header.Get("Content-Encoding");encoding!=""&&!strings.EqualFold(encoding,"identity"){
        return segment,ErrInvalidMedia
    }
    audioType:=SupportedPodcastAudioMIME(response.Header.Get("Content-Type"))
    if audioType=="" && (response.Header.Get("Content-Type")=="" ||
        strings.EqualFold(response.Header.Get("Content-Type"),"application/octet-stream")){
        audioType=SupportedPodcastAudioMIME(declaredType)
    }
    if audioType==""{return segment,ErrInvalidMedia}
    last,total,err:=parseUpstreamContentRange(response.Header.Get("Content-Range"),start,end)
    if err!=nil{return segment,err}
    length:=last-start+1
    if length<=0||length>MaxMediaSegmentBytes||
        (response.ContentLength>=0&&response.ContentLength!=length){
        return segment,ErrInvalidMedia
    }
    data,err:=io.ReadAll(io.LimitReader(response.Body,length+1))
    if err!=nil||int64(len(data))!=length{return segment,ErrInvalidMedia}
    if sniffed:=http.DetectContentType(data);strings.Contains(sniffed,"html")||
        strings.Contains(sniffed,"xml")||
        strings.Contains(sniffed,"svg"){return segment,ErrInvalidMedia}
    return MediaSegment{Data:data,MIME:audioType,Start:start,End:last,Total:total,
        ETag:StrongMediaETag(response.Header.Get("ETag")),
        LastModified:stableMediaLastModified(response.Header.Get("Last-Modified"))},nil
}
