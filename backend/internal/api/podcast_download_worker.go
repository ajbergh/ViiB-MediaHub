package api

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "database/sql"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"
 
    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

var safePodcastEpisodeID = regexp.MustCompile(`^[0-9a-f]{40}package api

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "database/sql"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"
 
    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

)
var safePodcastTempName = regexp.MustCompile(`^\.part-[0-9]{7,20}package api

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "database/sql"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"
 
    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

)

const podcastAbandonedPartGrace = 4*time.Hour

type podcastFileVerification struct {
    size int64
    modified time.Time
    digest string
}

const podcastDownloadChunkBytes = podcast.MaxMediaSegmentBytes
const podcastDownloadTick = 3*time.Second

// Private, fixed-location storage: never construct paths from publisher names
// or use a URL/client-supplied filesystem path.
func (a *API) podcastStorageDir()(string,error) {
    if a.dataDir=="" { return "",errors.New("missing podcast data directory") }
    parent:=filepath.Join(a.dataDir,"podcasts")
    dir:=filepath.Join(parent,"episodes")
    for _,path:=range []string{parent,dir}{
        if err:=os.MkdirAll(path,0700);err!=nil{return "",err}
        st,err:=os.Lstat(path)
        if err!=nil||!st.IsDir()||st.Mode()&os.ModeSymlink!=0{
            return "",errors.New("unsafe podcast storage directory")
        }
    }
    return dir,nil
}

func (a *API) startPodcastDownloader() {
    a.podcastDownloadMu.Lock()
    defer a.podcastDownloadMu.Unlock()
    if a.podcastDownloadClosed||a.podcastDownloadOn||a.db==nil||a.dataDir==""{return}
    var random [16]byte
    if _,err:=rand.Read(random[:]);err!=nil{return}
    a.podcastDownloadOwner=hex.EncodeToString(random[:])
    ctx,cancel:=context.WithCancel(context.Background())
    a.podcastDownloadCancel=cancel
    a.podcastDownloadWake=make(chan struct{},1)
    a.podcastDownloadOn=true
    a.podcastDownloadWG.Add(1)
    go func(){
        defer a.podcastDownloadWG.Done()
        if dir,err:=a.podcastStorageDir();err==nil {
            a.cleanupPodcastAbandonedParts(dir,time.Now())
        }
        ticker:=time.NewTicker(podcastDownloadTick)
        defer ticker.Stop()
        // Recover expired leased transfers without waiting for a UI request.
        a.processPodcastDownload(ctx)
        for {
            select {
            case <-ctx.Done():return
            case <-ticker.C:
            case <-a.podcastDownloadWake:
            }
            a.processPodcastDownload(ctx)
        }
    }()
}

func (a *API) wakePodcastDownloader(){
    a.podcastDownloadMu.Lock()
    wake:=a.podcastDownloadWake
    running:=a.podcastDownloadOn&&!a.podcastDownloadClosed
    a.podcastDownloadMu.Unlock()
    if running {select{case wake<-struct{}{}:default:}}
}

func (a *API) closePodcastDownloader(){
    a.podcastDownloadMu.Lock()
    a.podcastDownloadClosed=true
    cancel:=a.podcastDownloadCancel
    a.podcastDownloadMu.Unlock()
    if cancel!=nil{cancel()}
    a.podcastDownloadWG.Wait()
}

// Bounded one-at-a-time downloader. The existing HTTP fetcher does strict
// public-IP DNS pinning, redirect checks, MIME sniffing and Content-Range
// validation on *every* 2 MiB chunk. 200-only publishers remain unsupported
// for offline downloads until a separate bounded-streaming implementation.
func (a *API) processPodcastDownload(ctx context.Context){
    if a.db==nil || a.dataDir=="" {return}
    owner:=a.podcastDownloadOwner
    if owner==""{owner="manual-podcast-test-worker"} // only used for injected tests
    id,err:=a.db.ClaimPodcastDownload(ctx,owner,time.Now())
    if err!=nil||id==""{return}
    workCtx,cancel:=context.WithTimeout(ctx,45*time.Minute)
    defer cancel()
    if err:=a.transferPodcastDownload(workCtx,id,owner);err!=nil{
        code:="download_unavailable"
        switch {
        case errors.Is(err,context.Canceled),errors.Is(err,context.DeadlineExceeded):
            code="download_interrupted"
        case errors.Is(err,podcast.ErrPublisherNoRange):
            code="download_range_required"
        case errors.Is(err,podcast.ErrBlockedURL),errors.Is(err,podcast.ErrInvalidMedia),errors.Is(err,podcast.ErrInvalidMediaRange):
            code="download_unsupported"
        case errors.Is(err,errPodcastDownloadTooLarge):
            code="download_too_large"
        case errors.Is(err,db.ErrPodcastOfflineQuotaExceeded):
            code="download_quota_exceeded"
        case errors.Is(err,os.ErrPermission),errors.Is(err,io.ErrShortWrite):
            code="download_disk_unavailable"
        }
        // Cancellation revoked the lease, so this UPDATE is a no-op.
        _=a.db.FailPodcastDownload(context.Background(),id,owner,code)
    }
}

var errPodcastDownloadTooLarge=errors.New("podcast audio exceeds offline size limit")

// Abandoned temporary transfers never become playable. Remove only old,
 // numeric os.CreateTemp-generated .part-* regular files inside ViiB's own
 // fixed podcast directory. Recent files and foreign/symlink names are kept.
func (a *API) cleanupPodcastAbandonedParts(dir string,now time.Time){
    handle,err:=os.Open(dir)
    if err!=nil{return}
    defer handle.Close()
    scanned:=0
    for scanned<4096 {
        batch,err:=handle.ReadDir(64)
        if err!=nil&&err!=io.EOF{return}
        for _,entry:=range batch {
            scanned++
            name:=entry.Name()
            if !safePodcastTempName.MatchString(name){continue}
            path:=filepath.Join(dir,name)
            stat,statErr:=os.Lstat(path)
            if statErr!=nil||!stat.Mode().IsRegular()||now.Sub(stat.ModTime())<podcastAbandonedPartGrace{continue}
            _=os.Remove(path)
        }
        if err==io.EOF||len(batch)==0{return}
    }
}

// First local read in each process verifies the full SHA-256 persisted at
// download publication. Subsequent bounded HTTP Range requests reuse the
// result only while size/modtime/digest remain stable. Never trust a file
// merely because its name and length match SQLite.
func (a *API) verifyPodcastOfflineFile(id string,f *os.File,size int64,modified time.Time,want string) error{
    if size<=0||size>db.MaxPodcastDownloadBytes||len(want)!=64 {
        return errors.New("invalid podcast offline integrity metadata")
    }
    a.podcastIntegrityMu.Lock()
    defer a.podcastIntegrityMu.Unlock()
    if cache,ok:=a.podcastIntegrityCache[id];ok &&
        cache.size==size&&cache.modified.Equal(modified)&&strings.EqualFold(cache.digest,want){
        _,err:=f.Seek(0,io.SeekStart)
        return err
    }
    if _,err:=f.Seek(0,io.SeekStart);err!=nil{return err}
    hasher:=sha256.New()
    n,err:=io.CopyN(hasher,f,size)
    if err!=nil||n!=size{return errors.New("podcast offline integrity read failed")}
    if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)),want) {
        return errors.New("podcast offline checksum mismatch")
    }
    if _,err:=f.Seek(0,io.SeekStart);err!=nil{return err}
    if a.podcastIntegrityCache==nil||len(a.podcastIntegrityCache)>=128{
        a.podcastIntegrityCache=make(map[string]podcastFileVerification)
    }
    a.podcastIntegrityCache[id]=podcastFileVerification{
        size:size,modified:modified,digest:want,
    }
    return nil
}

func (a *API) clearPodcastOfflineVerification(id string){
    a.podcastIntegrityMu.Lock()
    delete(a.podcastIntegrityCache,id)
    a.podcastIntegrityMu.Unlock()
}

func (a *API) transferPodcastDownload(ctx context.Context,id,owner string) error {
    if !safePodcastEpisodeID.MatchString(id){return podcast.ErrInvalidMedia}
    raw,declared,err:=a.db.PodcastMediaSource(ctx,id)
    if err!=nil{return err}
    dir,err:=a.podcastStorageDir()
    if err!=nil{return err}
    // If an earlier worker crashed after rename but before publication, its
    // known-ID final file is orphaned. This job owns the new lease: remove
    // only that regular file, under the same lock as offline ServeContent.
    finalName:=id+".audio"
    dest:=filepath.Join(dir,finalName)
    a.podcastDownloadFileMu.Lock()
    orphan,orphanErr:=os.Lstat(dest)
    if orphanErr==nil {
        if !orphan.Mode().IsRegular(){
            a.podcastDownloadFileMu.Unlock()
            return podcast.ErrInvalidMedia
        }
        orphanErr=os.Remove(dest)
    } else if errors.Is(orphanErr,os.ErrNotExist){orphanErr=nil}
    a.podcastDownloadFileMu.Unlock()
    if orphanErr!=nil{return orphanErr}
    a.clearPodcastOfflineVerification(id)
    out,err:=os.CreateTemp(dir,".part-*")
    if err!=nil{return err}
    tempPath:=out.Name()
    defer func(){_ = out.Close();_ = os.Remove(tempPath)}()
    if err:=out.Chmod(0600);err!=nil{return err}
    fetch:=a.podcastDownloadFetch
    if fetch==nil{
        fetcher:=podcast.NewFetcher(false)
        fetch=fetcher.FetchMediaSegmentWithHint
    }
    hash:=sha256.New()
    var received,total int64
    var mime string
    for {
        if err:=ctx.Err();err!=nil{return err}
        header:=fmt.Sprintf("bytes=%d-%d",received,received+podcastDownloadChunkBytes-1)
        seg,err:=fetch(ctx,raw,header,declared)
        if err!=nil{return err}
        if seg.Start!=received||seg.Total<=0||seg.Total>db.MaxPodcastDownloadBytes||
            seg.End!=received+int64(len(seg.Data))-1||len(seg.Data)==0||
            int64(len(seg.Data))>podcastDownloadChunkBytes{
            return errPodcastDownloadTooLarge
        }
        if total==0 {total=seg.Total}else if total!=seg.Total {return podcast.ErrInvalidMedia}
        typ:=podcast.SupportedPodcastAudioMIME(seg.MIME)
        if typ=="" || mime!="" && mime!=typ{return podcast.ErrInvalidMedia}
        mime=typ
        if n,err:=out.Write(seg.Data);err!=nil{return err}else if n!=len(seg.Data){return io.ErrShortWrite}
        if _,err:=hash.Write(seg.Data);err!=nil{return err}
        received+=int64(len(seg.Data))
        // Cancel and worker lease checks happen before every next chunk.
        if err:=a.db.UpdatePodcastDownloadBytes(ctx,id,owner,received,total);err!=nil{return err}
        if received==total{break}
        if received>total{return podcast.ErrInvalidMedia}
    }
    if err:=out.Sync();err!=nil{return err}
    if err:=out.Close();err!=nil{return err}
    // Never publish over a newly created file or a symlink.
    if _,err:=os.Lstat(dest);err==nil{return errors.New("podcast destination already exists")}else if !errors.Is(err,os.ErrNotExist){return err}
    if err:=os.Rename(tempPath,dest);err!=nil{return err}
    tempPath="" // renamed: remove only on a failed database publication
    if err:=a.db.CompletePodcastDownload(ctx,id,owner,finalName,mime,hex.EncodeToString(hash.Sum(nil)),received);err!=nil{
        _=os.Remove(dest)
        return err
    }
    a.clearPodcastOfflineVerification(id)
    return nil
}

// servePodcastOfflineMedia returns false only when no completed offline
// record exists. Corrupt/missing files never fall through to a different
// user-controlled filesystem path. ServeContent honors RFC 7233 ranges.
func (a *API) servePodcastOfflineMedia(w http.ResponseWriter,r *http.Request,id string) bool {
    if a.db==nil{return false}
    a.podcastDownloadFileMu.RLock()
    defer a.podcastDownloadFileMu.RUnlock()
    file,err:=a.db.PodcastCompletedDownload(r.Context(),id)
    if errors.Is(err,sql.ErrNoRows){return false}
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Unable to check offline podcast audio",true,nil)
        return true
    }
    if !safePodcastEpisodeID.MatchString(id)||file.Filename!=id+".audio" ||
        file.Size<=0||file.Size>db.MaxPodcastDownloadBytes||
        podcast.SupportedPodcastAudioMIME(file.MIME)=="" {
        _=a.db.InvalidatePodcastDownload(r.Context(),id)
        respondV2Error(w,r,http.StatusGone,"podcast_download_missing",
            "Downloaded audio needs to be retried",false,nil)
        return true
    }
    dir,dirErr:=a.podcastStorageDir()
    if dirErr!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Offline podcast storage is unavailable",true,nil)
        return true
    }
    path:=filepath.Join(dir,file.Filename)
    stat,statErr:=os.Lstat(path)
    if statErr!=nil||!stat.Mode().IsRegular()||stat.Size()!=file.Size {
        _=a.db.InvalidatePodcastDownload(r.Context(),id)
        respondV2Error(w,r,http.StatusGone,"podcast_download_missing",
            "Downloaded audio needs to be retried",false,nil)
        return true
    }
    f,err:=os.Open(path)
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_download_unavailable",
            "Offline podcast storage is unavailable",true,nil)
        return true
    }
    defer f.Close()
    // Re-check the opened handle to reject swapped symlinks or changed-size
    // files between lstat and open, even if another local process writes here.
    opened,openErr:=f.Stat()
    if openErr!=nil||!opened.Mode().IsRegular()||opened.Size()!=file.Size{
        _=a.db.InvalidatePodcastDownload(r.Context(),id)
        a.clearPodcastOfflineVerification(id)
        respondV2Error(w,r,http.StatusGone,"podcast_download_missing",
            "Downloaded audio needs to be retried",false,nil)
        return true
    }
    if err:=a.verifyPodcastOfflineFile(id,f,file.Size,opened.ModTime(),file.SHA256);err!=nil{
        _=a.db.InvalidatePodcastDownload(r.Context(),id)
        a.clearPodcastOfflineVerification(id)
        respondV2Error(w,r,http.StatusGone,"podcast_download_corrupt",
            "Downloaded audio failed integrity checks; retry the download",false,nil)
        return true
    }
    w.Header().Set("Content-Type",file.MIME)
    w.Header().Set("Cache-Control","private, no-store")
    w.Header().Set("X-Content-Type-Options","nosniff")
    http.ServeContent(w,r,file.Filename,stat.ModTime(),f)
    return true
}

