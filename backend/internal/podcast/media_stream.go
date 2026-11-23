package podcast

import (
    "bufio"
    "context"
    "errors"
    "io"
    "net/http"
    "strconv"
    "strings"
)

// MaxFullMediaBytes is a disk-free, bounded safeguard for publishers that
// return 200 OK to Range requests. Data is forwarded to the client as it
// arrives; ViiB does not hold an episode in Go memory.
const MaxFullMediaBytes int64 = 1 << 30 // 1 GiB

// StreamMedia is a compatibility fallback for valid public audio publishers
// that do not implement RFC 7233 byte ranges. It re-fetches WITHOUT Range
// and streams their 200 audio body to the existing music player as HTTP 200.
// Playback works; seeking beyond the browser's buffered data may not work.
// The caller must only invoke this for an initial byte-zero request.
func (f *Fetcher) StreamMedia(ctx context.Context,raw,declaredType string,w http.ResponseWriter) error {
    u,err:=ValidateMediaURL(raw,true)
    if err!=nil{return err}
    tr:=f.transport()
    defer tr.CloseIdleConnections()
    client:=&http.Client{
        Transport:tr,
        CheckRedirect:func(request *http.Request,via []*http.Request) error {
            if len(via)>4{return ErrBlockedURL}
            if _,err:=ValidateMediaURL(request.URL.String(),true);err!=nil{return ErrBlockedURL}
            if len(via)>0 && via[len(via)-1].URL.Scheme=="https" && request.URL.Scheme!="https"{
                return ErrBlockedURL
            }
            request.Header.Del("Authorization")
            request.Header.Del("Cookie")
            request.Header.Del("Range")
            request.Header.Set("Accept-Encoding","identity")
            return nil
        },
    }
    request,err:=http.NewRequestWithContext(ctx,http.MethodGet,u.String(),nil)
    if err!=nil{return ErrBlockedURL}
    request.Header.Set("Accept","audio/*,application/ogg;q=0.9")
    request.Header.Set("Accept-Encoding","identity")
    request.Header.Set("User-Agent","ViiB-MediaHub-Podcast/1.0")
    response,err:=client.Do(request)
    if err!=nil{
        if errors.Is(err,ErrBlockedURL){return ErrBlockedURL}
        return ErrMediaUnavailable
    }
    defer response.Body.Close()
    if response.StatusCode!=http.StatusOK{return ErrMediaUnavailable}
    if encoding:=response.Header.Get("Content-Encoding");encoding!="" &&
        !strings.EqualFold(encoding,"identity"){return ErrInvalidMedia}
    if response.ContentLength>MaxFullMediaBytes{return ErrInvalidMedia}
    reader:=bufio.NewReader(response.Body)
    peek,err:=reader.Peek(512)
    if err!=nil && !errors.Is(err,io.EOF){return ErrMediaUnavailable}
    if len(peek)==0{return ErrInvalidMedia}
    sniffed:=http.DetectContentType(peek)
    if strings.Contains(sniffed,"html")||
        strings.Contains(sniffed,"xml")||strings.Contains(sniffed,"svg"){
        return ErrInvalidMedia
    }
    audioType:=SupportedPodcastAudioMIME(response.Header.Get("Content-Type"))
    if audioType=="" && (response.Header.Get("Content-Type")=="" ||
        strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")),"application/octet-stream")){
        audioType=SupportedPodcastAudioMIME(declaredType)
    }
    if audioType==""{return ErrInvalidMedia}
    w.Header().Set("Content-Type",audioType)
    w.Header().Set("Cache-Control","private, no-store")
    w.Header().Set("X-Content-Type-Options","nosniff")
    w.Header().Set("Accept-Ranges","none")
    if response.ContentLength>=0{
        w.Header().Set("Content-Length",strconv.FormatInt(response.ContentLength,10))
    }
    w.WriteHeader(http.StatusOK)
    // Readers and writers remain streaming. When the upstream does not
    // advertise a length, stop at the same bound rather than accepting
    // unlimited network traffic or storing unbounded data on disk.
    _,_ = io.Copy(w,io.LimitReader(reader,MaxFullMediaBytes))
    // Once HTTP 200 headers are sent, adding JSON errors would corrupt audio.
    return nil
}
