package podcast

import (
    "context"
    "errors"
    "fmt"
    "io"
    "mime"
    "net/http"
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
}

// ValidateMediaURL rejects URL credentials, private or secret-bearing query
// inputs and non-HTTPS origins. DNS is validated AGAIN at the actual dial,
// using the existing public-IP-pinned Feed Fetcher transport.
func ValidateMediaURL(raw string, allowHTTP bool) (*url.URL,error) {
    if raw==""||len(raw)>2048{return nil,ErrBlockedURL}
    return ValidatePublicURL(raw,allowHTTP)
}

// SupportedPodcastAudioMIME blocks HTML, SVG and non-audio response bodies.
// A media URL from RSS is metadata, not authorization to fetch any type.
func SupportedPodcastAudioMIME(raw string) string {
    base,_,err:=mime.ParseMediaType(strings.TrimSpace(raw))
    if err!=nil{return ""}
    switch strings.ToLower(base){
    case "audio/mpeg","audio/mp4","audio/x-m4a","audio/aac",
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
    var segment MediaSegment
    u,err:=ValidateMediaURL(raw,f.AllowHTTP)
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
            if _,err:=ValidateMediaURL(req.URL.String(),f.AllowHTTP);err!=nil{return ErrBlockedURL}
            if len(via)>0&&via[len(via)-1].URL.Scheme=="https"&&req.URL.Scheme!="https"{
                return ErrBlockedURL
            }
            req.Header.Del("Authorization")
            req.Header.Del("Cookie")
            req.Header.Set("Range",fmt.Sprintf("bytes=%d-%d",start,end))
            req.Header.Set("Accept-Encoding","identity")
            return nil
        },
    }
    request,err:=http.NewRequestWithContext(ctx,http.MethodGet,u.String(),nil)
    if err!=nil{return segment,ErrBlockedURL}
    request.Header.Set("Range",fmt.Sprintf("bytes=%d-%d",start,end))
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
    if response.StatusCode!=http.StatusPartialContent{
        return segment,ErrMediaUnavailable
    }
    if encoding:=response.Header.Get("Content-Encoding");encoding!=""&&!strings.EqualFold(encoding,"identity"){
        return segment,ErrInvalidMedia
    }
    audioType:=SupportedPodcastAudioMIME(response.Header.Get("Content-Type"))
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
    return MediaSegment{Data:data,MIME:audioType,Start:start,End:last,Total:total},nil
}
