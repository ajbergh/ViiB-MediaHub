package podcast

import (
    "bufio"
    "context"
    "errors"
    "io"
    "net/http"
    "strings"
    "time"
)

// FullMedia contains only validated audio metadata. A 200-only transfer is
// written directly to the caller's private, temporary storage; no publisher
// URL, redirect or signed query is returned to the browser.
type FullMedia struct{
    Size int64
    MIME string
}

// FetchFullMedia supports byte-range-incompatible publishers for *offline*
// downloads only. It must be called after a first-byte request returned
// ErrPublisherNoRange, and never processes arbitrary URLs without SSRF guards.
// The caller supplies its own small maximum; this function does not buffer a
// whole episode in memory and never accepts 206 as a complete file.
func (f *Fetcher) FetchFullMedia(ctx context.Context,raw,declared string,dst io.Writer,maxBytes int64)(FullMedia,error){
    var result FullMedia
    if maxBytes<=0||maxBytes>MaxFullMediaBytes||dst==nil{return result,ErrInvalidMedia}
    u,err:=ValidateMediaURL(raw,true)
    if err!=nil{return result,err}
    tr:=f.transport()
    defer tr.CloseIdleConnections()
    client:=&http.Client{
        Transport:tr,
        Timeout:45*time.Minute,
        CheckRedirect:func(req *http.Request,via []*http.Request)error{
            if len(via)>4{return ErrBlockedURL}
            if _,err:=ValidateMediaURL(req.URL.String(),true);err!=nil{return ErrBlockedURL}
            if len(via)>0&&via[len(via)-1].URL.Scheme=="https"&&req.URL.Scheme!="https"{return ErrBlockedURL}
            req.Header.Del("Authorization")
            req.Header.Del("Cookie")
            req.Header.Del("Range")
            req.Header.Del("If-Range")
            req.Header.Set("Accept-Encoding","identity")
            return nil
        },
    }
    request,err:=http.NewRequestWithContext(ctx,http.MethodGet,u.String(),nil)
    if err!=nil{return result,ErrBlockedURL}
    request.Header.Set("Accept","audio/*, application/ogg;q=0.9")
    request.Header.Set("Accept-Encoding","identity")
    request.Header.Set("User-Agent","ViiB-MediaHub-Podcast/1.0")
    response,err:=client.Do(request)
    if err!=nil{
        if errors.Is(err,ErrBlockedURL){return result,ErrBlockedURL}
        return result,ErrMediaUnavailable
    }
    defer response.Body.Close()
    if response.StatusCode!=http.StatusOK{return result,ErrMediaUnavailable}
    if encoding:=response.Header.Get("Content-Encoding");encoding!=""&&!strings.EqualFold(encoding,"identity"){
        return result,ErrInvalidMedia
    }
    if response.ContentLength>maxBytes{return result,ErrMediaTooLarge}
    mediaType:=SupportedPodcastAudioMIME(response.Header.Get("Content-Type"))
    if mediaType==""&&(response.Header.Get("Content-Type")==""||
        strings.EqualFold(response.Header.Get("Content-Type"),"application/octet-stream")){
        mediaType=SupportedPodcastAudioMIME(declared)
    }
    if mediaType==""{return result,ErrInvalidMedia}
    reader:=bufio.NewReaderSize(response.Body,32<<10)
    sample,err:=reader.Peek(512)
    if err!=nil&&!errors.Is(err,io.EOF){return result,ErrMediaUnavailable}
    if len(sample)==0{return result,ErrInvalidMedia}
    sniff:=http.DetectContentType(sample)
    if strings.Contains(sniff,"html")||strings.Contains(sniff,"xml")||strings.Contains(sniff,"svg"){
        return result,ErrInvalidMedia
    }
    n,err:=io.Copy(dst,io.LimitReader(reader,maxBytes))
    if err!=nil{return result,err}
    if n==maxBytes{
        var extra [1]byte
        more,readErr:=reader.Read(extra[:])
        if more>0{return result,ErrMediaTooLarge}
        if readErr!=nil&&!errors.Is(readErr,io.EOF){return result,ErrMediaUnavailable}
    }
    if n<=0||(response.ContentLength>=0&&response.ContentLength!=n){return result,ErrInvalidMedia}
    return FullMedia{Size:n,MIME:mediaType},nil
}
