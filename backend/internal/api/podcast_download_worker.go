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
    "sync"
    "sync/atomic"
    "time"
 
    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

var safePodcastEpisodeID = regexp.MustCompile(`^[0-9a-f]{40}$`)
var safePodcastTempName = regexp.MustCompile(`^\.part-[0-9]{7,20}$`)

const podcastAbandonedPartGrace = 4*time.Hour

type podcastFileVerification struct {
    size int64
    modified time.Time
    digest string
}

const podcastDownloadChunkBytes = podcast.MaxMediaSegmentBytes
const podcastDownloadTick = 3*time.Second
// A 200-only publisher may take minutes and has no 2 MiB request boundary.
// This heartbeat keeps its 90s SQLite lease current and notices cancellation.
const podcastFullDownloadHeartbeat = 2*time.Second

type podcastByteCountingWriter struct {
    destination io.Writer
    bytes *atomic.Int64
}
func (w podcastByteCountingWriter)Write(p []byte)(int,error){
    n,err:=w.destination.Write(p)
    w.bytes.Add(int64(n))
    return n,err
}

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
    a.cleanupUnsubscribedPodcastFiles(ctx)
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
        case errors.Is(err,errPodcastDownloadTooLarge),errors.Is(err,podcast.ErrMediaTooLarge):
            code="download_too_large"
        case errors.Is(err,errPodcastPublisherChanged):
            code="download_publisher_changed"
        case errors.Is(err,db.ErrPodcastOfflineQuotaExceeded):
            code="download_quota_exceeded"
        case errors.Is(err,errPodcastDiskSpaceLow):
            code="download_disk_space_low"
        case errors.Is(err,os.ErrPermission),errors.Is(err,io.ErrShortWrite):
            code="download_disk_unavailable"
        }
        // Cancellation revoked the lease, so this UPDATE is a no-op.
        _=a.db.FailPodcastDownload(context.Background(),id,owner,code)
    }
}

// Reclaim a bounded batch of completed files from unfollowed feeds without
// touching saved listening positions or accepting user-defined disk paths.
// Successful deletion clears the database record and its quota reservation;
// busy files (notably Windows players) are retried on the next worker pass.
func (a *API) cleanupUnsubscribedPodcastFiles(ctx context.Context){
    if a.db==nil||a.dataDir==""{return}
    files,err:=a.db.ListUnsubscribedPodcastCompletedFiles(ctx,32)
    if err!=nil||len(files)==0{return}
    dir,err:=a.podcastStorageDir()
    if err!=nil{return}
    a.podcastDownloadFileMu.Lock()
    defer a.podcastDownloadFileMu.Unlock()
    for _,file:=range files{
        if ctx.Err()!=nil{return}
        if !safePodcastEpisodeID.MatchString(file.EpisodeID)||
            file.Filename!=file.EpisodeID+".audio"{continue}
        path:=filepath.Join(dir,file.Filename)
        info,err:=os.Lstat(path)
        if err==nil{
            if !info.Mode().IsRegular(){continue}
            if err=os.Remove(path);err!=nil{continue}
        }else if !errors.Is(err,os.ErrNotExist){continue}
        if err:=a.db.DeleteUnsubscribedPodcastCompletedFile(ctx,file.EpisodeID);err==nil{
            a.clearPodcastOfflineVerification(file.EpisodeID)
        }
    }
}

var errPodcastPublisherChanged=errors.New("publisher audio changed during ranged transfer")
var errPodcastDownloadTooLarge=errors.New("podcast audio exceeds offline size limit")
var errPodcastDiskSpaceLow=errors.New("insufficient podcast destination disk space")

// Keep room for the OS and other applications; this is independent of
// the 2 GiB podcast-owned reservation quota.
const podcastDiskFreeReserve uint64 = 128 << 20

func (a *API) requirePodcastDiskSpace(dir string,remaining int64)error{
    if remaining<0||remaining>db.MaxPodcastDownloadBytes {
        return errPodcastDiskSpaceLow
    }
    probe:=a.podcastDiskFreeBytes
    if probe==nil {probe=podcastAvailableDiskBytes}
    available,err:=probe(dir)
    if err!=nil{return errPodcastDiskSpaceLow}
    if available<uint64(remaining)+podcastDiskFreeReserve{return errPodcastDiskSpaceLow}
    return nil
}

// Abandoned transfers never become playable. Sweep only old numeric os.CreateTemp
// .part-* regular files inside the fixed private podcast directory.
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

// Full SHA-256 on first read in this process, with size/mtime/digest-bound cache.
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
    a.podcastIntegrityCache[id]=podcastFileVerification{size:size,modified:modified,digest:want}
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
    // Reclaim a safe known-ID final file left by a crashed old lease.
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
    var requestedIfRange string
    if fetch==nil{
        fetcher:=podcast.NewFetcher(false)
        fetch=func(ctx context.Context,raw,header,declared string)(podcast.MediaSegment,error){
            return fetcher.FetchMediaSegmentWithIfRange(ctx,raw,header,declared,requestedIfRange)
        }
    }
    hash:=sha256.New()
    var received,total int64
    var mime,firstETag,firstLastModified string
    for {
        if err:=ctx.Err();err!=nil{return err}
        header:=fmt.Sprintf("bytes=%d-%d",received,received+podcastDownloadChunkBytes-1)
        seg,err:=fetch(ctx,raw,header,declared)
        if err!=nil{
            if received==0&&errors.Is(err,podcast.ErrPublisherNoRange){
                fullFetch:=a.podcastDownloadFullFetch
                if fullFetch==nil&&a.podcastDownloadFetch==nil{
                    fullFetch=podcast.NewFetcher(false).FetchFullMedia
                }
                if fullFetch==nil{return err} // injected range test: never make an unexpected real fetch
                // Unknown-length 200 publishers reserve the full per-episode
                // ceiling first, then atomically reconcile actual verified size.
                // A failed/blocked transfer releases its reservation.
                if err:=a.requirePodcastDiskSpace(dir,db.MaxPodcastDownloadBytes);err!=nil{return err}
                if err:=a.db.UpdatePodcastDownloadBytes(ctx,id,owner,0,db.MaxPodcastDownloadBytes);err!=nil{return err}
                sum:=sha256.New()
                var streamed atomic.Int64
                // The streaming 200 response is one long network request.
                // Keep the durable lease alive while reading, and revoke the
                // HTTP request promptly when Cancel/Unfollow clears the lease.
                streamCtx,stopStream:=context.WithCancel(ctx)
                leaseFailures:=make(chan error,1)
                var heartbeat sync.WaitGroup
                heartbeat.Add(1)
                go func(){
                    defer heartbeat.Done()
                    ticker:=time.NewTicker(podcastFullDownloadHeartbeat)
                    defer ticker.Stop()
                    for {
                        select {
                        case <-streamCtx.Done():return
                        case <-ticker.C:
                            if leaseErr:=a.db.UpdatePodcastDownloadBytes(streamCtx,id,owner,
                                streamed.Load(),db.MaxPodcastDownloadBytes);leaseErr!=nil{
                                leaseFailures<-leaseErr
                                stopStream()
                                return
                            }
                        }
                    }
                }()
                full,fullErr:=fullFetch(streamCtx,raw,declared,
                    podcastByteCountingWriter{destination:io.MultiWriter(out,sum),bytes:&streamed},
                    db.MaxPodcastDownloadBytes)
                stopStream()
                heartbeat.Wait()
                select{
                case leaseErr:=<-leaseFailures:return leaseErr
                default:
                }
                if fullErr!=nil{return fullErr}
                stat,statErr:=out.Stat()
                if statErr!=nil||!stat.Mode().IsRegular()||full.Size<=0||
                    full.Size>db.MaxPodcastDownloadBytes||stat.Size()!=full.Size||
                    streamed.Load()!=full.Size||
                    podcast.SupportedPodcastAudioMIME(full.MIME)==""{
                    return podcast.ErrInvalidMedia
                }
                if err:=a.db.UpdatePodcastDownloadBytes(ctx,id,owner,full.Size,full.Size);err!=nil{return err}
                return a.publishPodcastDownloadedAudio(ctx,id,owner,out,tempPath,dest,finalName,
                    full.MIME,hex.EncodeToString(sum.Sum(nil)),full.Size)
            }
            if received>0&&requestedIfRange!=""&&errors.Is(err,podcast.ErrPublisherNoRange){
                return errPodcastPublisherChanged
            }
            return err
        }
        // Stable validators avoid mixing two versions of an enclosure.
        // Origins with no validators retain the initial 206-only behavior;
        // they are not eligible for future persisted partial resume.
        if received==0{
            firstETag=podcast.StrongMediaETag(seg.ETag)
            firstLastModified=seg.LastModified
            requestedIfRange=firstETag
        }else if firstETag!=""&&podcast.StrongMediaETag(seg.ETag)!=firstETag ||
            firstLastModified!=""&&seg.LastModified!=firstLastModified {
            return errPodcastPublisherChanged
        }
        if seg.Start!=received||seg.Total<=0||seg.Total>db.MaxPodcastDownloadBytes||
            seg.End!=received+int64(len(seg.Data))-1||len(seg.Data)==0||
            int64(len(seg.Data))>podcastDownloadChunkBytes{
            return errPodcastDownloadTooLarge
        }
        if total==0 {total=seg.Total}else if total!=seg.Total {return podcast.ErrInvalidMedia}
        typ:=podcast.SupportedPodcastAudioMIME(seg.MIME)
        if typ=="" || mime!="" && mime!=typ{return podcast.ErrInvalidMedia}
        mime=typ
        // Probe the actual destination volume, not just the SQLite quota.
        // Reserving all remaining bytes prevents starting a transfer that
        // would consume the OS safety margin; recheck after each publisher
        // segment because other applications may use the same disk.
        if err:=a.requirePodcastDiskSpace(dir,total-received);err!=nil{return err}
        if n,err:=out.Write(seg.Data);err!=nil{return err}else if n!=len(seg.Data){return io.ErrShortWrite}
        if _,err:=hash.Write(seg.Data);err!=nil{return err}
        received+=int64(len(seg.Data))
        // Cancel and worker lease checks happen before every next chunk.
        if err:=a.db.UpdatePodcastDownloadBytes(ctx,id,owner,received,total);err!=nil{return err}
        if received==total{break}
        if received>total{return podcast.ErrInvalidMedia}
    }
    return a.publishPodcastDownloadedAudio(ctx,id,owner,out,tempPath,dest,finalName,
        mime,hex.EncodeToString(hash.Sum(nil)),received)
}

func (a *API) publishPodcastDownloadedAudio(ctx context.Context,id,owner string,out *os.File,
    tempPath,dest,finalName,mime,digest string,size int64)error{
    if err:=out.Sync();err!=nil{return err}
    if err:=out.Close();err!=nil{return err}
    // Never overwrite a previously published file or a symlink.
    if _,err:=os.Lstat(dest);err==nil{return errors.New("podcast destination already exists")}else if !errors.Is(err,os.ErrNotExist){return err}
    if err:=os.Rename(tempPath,dest);err!=nil{return err}
    if err:=a.db.CompletePodcastDownload(ctx,id,owner,finalName,mime,digest,size);err!=nil{
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

