// artwork.go retrieves only bounded raster artwork from validated public
// publisher hosts. No untrusted image URL is handed directly to the UI.
package podcast

import (
    "context"
    "errors"
    "io"
    "net/http"
    "net/url"
    "strconv"
    "strings"
    "time"
)

const MaxArtworkBytes int64 = 2 << 20

type Artwork struct {
    Data []byte
    MIME string
}

// ValidateArtworkURL reuses the RSS connector's address/scheme/port policy
// while permitting a small allowlist of public CDN resize parameters. Tokens,
// signatures, private URLs and unknown parameters are deliberately refused.
func ValidateArtworkURL(raw string, allowHTTP bool) (*url.URL,error) {
    if len(raw)==0 || len(raw)>2048 {return nil,ErrBlockedURL}
    parsed,err:=url.Parse(strings.TrimSpace(raw))
    if err!=nil || parsed==nil{return nil,ErrBlockedURL}
    query:=parsed.RawQuery
    parsed.RawQuery=""
    normalized,err:=ValidatePublicURL(parsed.String(),allowHTTP)
    if err!=nil{return nil,err}
    params,err:=url.ParseQuery(query)
    if err!=nil{return nil,ErrBlockedURL}
    for key,values:=range params{
        if len(values)!=1{return nil,ErrBlockedURL}
        value:=strings.ToLower(strings.TrimSpace(values[0]))
        switch strings.ToLower(key) {
        case "w","h","width","height":
            n,err:=strconv.Atoi(value)
            if err!=nil||n<1||n>4096{return nil,ErrBlockedURL}
        case "q","quality":
            n,err:=strconv.Atoi(value)
            if err!=nil||n<1||n>100{return nil,ErrBlockedURL}
        case "format":
            if value!="jpg"&&value!="jpeg"&&value!="png"&&value!="webp"&&value!="gif" {return nil,ErrBlockedURL}
        case "fit":
            if value!="crop"&&value!="cover"&&value!="contain"&&value!="max" {return nil,ErrBlockedURL}
        case "size":
            parts:=strings.Split(value,"x")
            if len(parts)!=2{return nil,ErrBlockedURL}
            for _,s:=range parts{
                n,err:=strconv.Atoi(s)
                if err!=nil||n<1||n>4096{return nil,ErrBlockedURL}
            }
        default:
            return nil,ErrBlockedURL
        }
    }
    normalized.RawQuery=query
    return normalized,nil
}

func SafeArtworkURL(raw string) string {
    if strings.TrimSpace(raw)==""{return ""}
    u,err:=ValidateArtworkURL(raw,false)
    if err!=nil{return ""}
    return u.String()
}

// FetchArtwork rejects non-raster content (notably script-bearing SVG/HTML),
// applies public-only DNS checks at dial, validates redirects and caps bytes.
// Production callers use HTTPS only; HTTP is a test-only fixture option.
func (f *Fetcher) FetchArtwork(ctx context.Context,raw string)(Artwork,error){
    var artwork Artwork
    u,err:=ValidateArtworkURL(raw,f.AllowHTTP)
    if err!=nil{return artwork,err}
    tr:=f.transport()
    defer tr.CloseIdleConnections()
    client:=&http.Client{
        Transport:tr,
        Timeout:15*time.Second,
        CheckRedirect:func(req *http.Request,via []*http.Request)error{
            if len(via)>4{return ErrBlockedURL}
            if _,err:=ValidateArtworkURL(req.URL.String(),f.AllowHTTP);err!=nil{return ErrBlockedURL}
            if len(via)>0 && via[len(via)-1].URL.Scheme=="https" && req.URL.Scheme!="https"{return ErrBlockedURL}
            req.Header.Del("Authorization")
            req.Header.Del("Cookie")
            return nil
        },
    }
    request,err:=http.NewRequestWithContext(ctx,http.MethodGet,u.String(),nil)
    if err!=nil{return artwork,ErrBlockedURL}
    request.Header.Set("Accept","image/png,image/jpeg,image/webp,image/gif")
    request.Header.Set("Accept-Encoding","identity")
    request.Header.Set("User-Agent","ViiB-MediaHub-Podcast/1.0")
    response,err:=client.Do(request)
    if err!=nil{
        if errors.Is(err,ErrBlockedURL){return artwork,ErrBlockedURL}
        return artwork,ErrFeedUnavailable
    }
    defer response.Body.Close()
    if response.StatusCode!=http.StatusOK{return artwork,ErrFeedUnavailable}
    if response.ContentLength>MaxArtworkBytes{return artwork,ErrInvalidFeed}
    if encoding:=response.Header.Get("Content-Encoding");encoding!="" && !strings.EqualFold(encoding,"identity"){return artwork,ErrInvalidFeed}
    data,err:=io.ReadAll(io.LimitReader(response.Body,MaxArtworkBytes+1))
    if err!=nil||len(data)==0||int64(len(data))>MaxArtworkBytes{return artwork,ErrInvalidFeed}
    sniffed:=http.DetectContentType(data)
    switch sniffed {
    case "image/png","image/jpeg","image/gif","image/webp":
        return Artwork{Data:data,MIME:sniffed},nil
    default:
        return artwork,ErrInvalidFeed
    }
}
