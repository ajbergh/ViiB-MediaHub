package api

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "database/sql"
 "encoding/json"
 "errors"
 "net/http"
 "net/http/httptest"
 "io"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/ajbergh/viib-mediahub/internal/db"
 "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func seedPodcastDownload(t *testing.T)(*db.DB,string){
 t.Helper()
 d,err:=db.New(filepath.Join(t.TempDir(),"podcast-transfer.db"))
 if err!=nil{t.Fatal(err)}
 feedID,err:=d.SavePodcastFeedSnapshot(context.Background(),podcast.Feed{
  URL:"https://publisher.example/rss",Title:"Offline show",
  Episodes:[]podcast.Episode{{GUID:"one",Title:"Offline episode",
    EnclosureURL:"https://cdn.example.org/signed.mp3?signature=keep-private",EnclosureType:"audio/mpeg"}},
 },true)
 if err!=nil{t.Fatal(err)}
 eps,err:=d.ListPodcastEpisodes(context.Background(),feedID,10)
 if err!=nil||len(eps)!=1{t.Fatalf("episodes: %+v %v",eps,err)}
 return d,eps[0].ID
}

func TestPodcastDownloadQueueTransferAndOfflineRange(t *testing.T) {
 d,id:=seedPodcastDownload(t);defer d.Close()
 dir:=t.TempDir()
 calls:=0
 a:=&API{db:d,dataDir:dir,podcastDownloadFetch:func(_ context.Context,raw,rangeHeader,mime string)(podcast.MediaSegment,error){
  calls++
  if !strings.Contains(raw,"signature=keep-private")||mime!="audio/mpeg"{t.Fatalf("missing publisher metadata")}
  if calls==1 {
   if rangeHeader!="bytes=0-2097151"{t.Fatalf("first range %q",rangeHeader)}
   return podcast.MediaSegment{Start:0,End:3,Total:8,MIME:"audio/mpeg",ETag:"\"episode-v1\"",Data:[]byte("ID3a")},nil
  }
  if calls==2 {
   if rangeHeader!="bytes=4-2097155"{t.Fatalf("second range %q",rangeHeader)}
   return podcast.MediaSegment{Start:4,End:7,Total:8,MIME:"audio/mpeg",ETag:"\"episode-v1\"",Data:[]byte("bcde")},nil
  }
  t.Fatal("too many publisher requests");return podcast.MediaSegment{},errors.New("too many")
 }}
 router:=a.PodcastRoutes()
 request:=func(method,path string)*httptest.ResponseRecorder {
  t.Helper();rec:=httptest.NewRecorder();router.ServeHTTP(rec,httptest.NewRequest(method,path,nil));return rec
 }
 queued:=request(http.MethodPost,"/episodes/"+id+"/download")
 if queued.Code!=202||!bytes.Contains(queued.Body.Bytes(),[]byte("queued")){t.Fatalf("queue %d: %s",queued.Code,queued.Body.String())}
 if bytes.Contains(queued.Body.Bytes(),[]byte("signature")){t.Fatal("source URL leaked")}
 duplicate:=request(http.MethodPost,"/episodes/"+id+"/download")
 if duplicate.Code!=202{t.Fatalf("idempotent queue: %d",duplicate.Code)}
 a.processPodcastDownload(context.Background())
 if calls!=2{t.Fatalf("expected two bounded range calls, got %d",calls)}
 status:=request(http.MethodGet,"/episodes/"+id+"/download")
 if status.Code!=200||!bytes.Contains(status.Body.Bytes(),[]byte("completed")){t.Fatalf("status %d: %s",status.Code,status.Body.String())}
 list:=request(http.MethodGet,"/downloads")
 if list.Code!=200||!bytes.Contains(list.Body.Bytes(),[]byte(id)){t.Fatalf("listing %d: %s",list.Code,list.Body.String())}
 if bytes.Contains(list.Body.Bytes(),[]byte("signature"))||bytes.Contains(list.Body.Bytes(),[]byte(".audio")) {
  t.Fatalf("internal path or URL leaked: %s",list.Body.String())
 }
 onlineRequests:=0
 a.podcastMediaFetch=func(context.Context,string,string)(podcast.MediaSegment,error){
  onlineRequests++;return podcast.MediaSegment{},errors.New("network disabled")
 }
 mediaReq:=httptest.NewRequest(http.MethodGet,"/episodes/"+id+"/media",nil)
 mediaReq.Header.Set("Range","bytes=2-5")
 media:=httptest.NewRecorder()
 router.ServeHTTP(media,mediaReq)
 if media.Code!=http.StatusPartialContent||media.Body.String()!="3abc"||
  media.Header().Get("Content-Range")!="bytes 2-5/8"||
  onlineRequests!=0||media.Header().Get("Content-Type")!="audio/mpeg"{
  t.Fatalf("offline Range %d headers=%+v body=%q upstream=%d",media.Code,media.Header(),media.Body.String(),onlineRequests)
 }
 file,err:=d.PodcastCompletedDownload(context.Background(),id)
 if err!=nil{t.Fatal(err)}
 onDisk:=filepath.Join(dir,"podcasts","episodes",file.Filename)
 stat,err:=os.Stat(onDisk)
 if err!=nil||stat.Size()!=8{t.Fatalf("downloaded file %v %+v",err,stat)}
 canceled:=request(http.MethodDelete,"/episodes/"+id+"/download")
 if canceled.Code!=204{t.Fatalf("completed file not deleted: %d",canceled.Code)}
}

func TestPodcastDownloadCancelAndRangeUnsupported(t *testing.T) {
 d,id:=seedPodcastDownload(t);defer d.Close()
 a:=&API{db:d,dataDir:t.TempDir()}
 handler:=a.PodcastRoutes()
 req:=func(method,path string)*httptest.ResponseRecorder{
  t.Helper();res:=httptest.NewRecorder();handler.ServeHTTP(res,httptest.NewRequest(method,path,nil));return res
 }
 if out:=req(http.MethodDelete,"/episodes/"+id+"/download");out.Code!=404{t.Fatalf("missing cancel: %d",out.Code)}
 if out:=req(http.MethodPost,"/episodes/"+id+"/download");out.Code!=202{t.Fatalf("queue: %d",out.Code)}
 if out:=req(http.MethodDelete,"/episodes/"+id+"/download");out.Code!=200{t.Fatalf("cancel: %d %s",out.Code,out.Body.String())}
 if out:=req(http.MethodPost,"/episodes/"+id+"/download");out.Code!=202{t.Fatalf("retry: %d",out.Code)}
 a.podcastDownloadFetch=func(context.Context,string,string,string)(podcast.MediaSegment,error){return podcast.MediaSegment{},podcast.ErrPublisherNoRange}
 a.processPodcastDownload(context.Background())
 row,err:=d.GetPodcastDownload(context.Background(),id)
 if err!=nil||row.Status!="failed"||row.ErrorCode!="download_range_required"{t.Fatalf("unsupported publisher: %+v %v",row,err)}
 if out:=req(http.MethodPost,"/episodes/"+id+"/download");out.Code!=202{t.Fatalf("retry failure: %d",out.Code)}
 a.podcastDownloadFetch=func(ctx context.Context,_,_,_ string)(podcast.MediaSegment,error){
  if _,err:=d.CancelPodcastDownload(ctx,id);err!=nil{t.Fatal(err)}
  return podcast.MediaSegment{Start:0,End:3,Total:4,MIME:"audio/mpeg",Data:[]byte("ID3x")},nil
 }
 a.processPodcastDownload(context.Background())
 row,err=d.GetPodcastDownload(context.Background(),id)
 if err!=nil||row.Status!="canceled"{t.Fatalf("in-flight cancel: %+v %v",row,err)}
 if _,err=os.Stat(filepath.Join(a.dataDir,"podcasts","episodes",id+".audio"));!errors.Is(err,os.ErrNotExist){t.Fatalf("canceled file published: %v",err)}
 missing:=req(http.MethodPost,"/episodes/not-found/download")
 if missing.Code!=404{t.Fatalf("unsafe ID admitted: %d",missing.Code)}
 if out:=req(http.MethodGet,"/downloads");out.Code!=200{t.Fatalf("list after cancellation: %d",out.Code)}
}

func TestPodcastDownloadStatusJSONIsBoundedAndRedacted(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 a:=&API{db:d,dataDir:t.TempDir()}
 response:=httptest.NewRecorder()
 a.PodcastRoutes().ServeHTTP(response,httptest.NewRequest(http.MethodPost,"/episodes/"+id+"/download",nil))
 if response.Code!=202{t.Fatalf("queue: %d",response.Code)}
 var payload struct{ Download db.PodcastDownloadRecord `json:"download"` }
 if err:=json.Unmarshal(response.Body.Bytes(),&payload);err!=nil{t.Fatal(err)}
 if payload.Download.ID!=id||payload.Download.Status!="queued"{t.Fatalf("download JSON %+v",payload)}
 if strings.Contains(response.Body.String(),"cdn.example")||strings.Contains(response.Body.String(),"destinationRelativePath"){t.Fatal("download endpoint leaked private source or path")}
}

func TestPodcastCompletedFileDeletePreservesHistoryAndRestoresOnlinePlayback(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 ctx:=context.Background()
 if _,err:=d.SetPodcastProgress(ctx,id,45000,30000,0,false);err!=nil{t.Fatal(err)}
 a:=&API{db:d,dataDir:t.TempDir(),podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
    return podcast.MediaSegment{Start:0,End:3,Total:4,MIME:"audio/mpeg",Data:[]byte("ID3!")},nil
 }}
 handler:=a.PodcastRoutes()
 request:=func(method,path string)*httptest.ResponseRecorder{
    t.Helper();rec:=httptest.NewRecorder()
    handler.ServeHTTP(rec,httptest.NewRequest(method,path,nil))
    return rec
 }
 if res:=request("POST","/episodes/"+id+"/download");res.Code!=202{t.Fatalf("queue: %d",res.Code)}
 a.processPodcastDownload(ctx)
 file,err:=d.PodcastCompletedDownload(ctx,id);if err!=nil{t.Fatal(err)}
 diskPath:=filepath.Join(a.dataDir,"podcasts","episodes",file.Filename)
 if _,err=os.Stat(diskPath);err!=nil{t.Fatal(err)}
 offline:=request("GET","/episodes/"+id+"/media")
 if offline.Code!=http.StatusOK||offline.Body.String()!="ID3!"{t.Fatalf("offline playback: %d %q",offline.Code,offline.Body.String())}
 removed:=request("DELETE","/episodes/"+id+"/download")
 if removed.Code!=204{t.Fatalf("delete: %d %s",removed.Code,removed.Body.String())}
 if _,err=os.Lstat(diskPath);!errors.Is(err,os.ErrNotExist){t.Fatalf("file remains: %v",err)}
 if _,err=d.GetPodcastDownload(ctx,id);!errors.Is(err,sql.ErrNoRows){
    t.Fatalf("completed metadata remained: %v",err)
 }
 progress,err:=d.GetPodcastProgress(ctx,id)
 if err!=nil||progress.PositionMs!=45000||progress.ListenedMs!=30000{
    t.Fatalf("deletion modified playback history: %+v %v",progress,err)
 }
 upstream:=0
 a.podcastMediaFetch=func(_ context.Context,_,_ string)(podcast.MediaSegment,error){
    upstream++
    return podcast.MediaSegment{Start:0,End:3,Total:4,MIME:"audio/mpeg",Data:[]byte("NET!")},nil
 }
 online:=request("GET","/episodes/"+id+"/media")
 if online.Code!=206||online.Body.String()!="NET!"||upstream!=1{
    t.Fatalf("online fallback: %d %q count %d",online.Code,online.Body.String(),upstream)
 }
 if deleted:=request("DELETE","/episodes/"+id+"/download");deleted.Code!=404{
    t.Fatalf("second removal should report absent record: %d",deleted.Code)
 }
}

func TestPodcastCompletedRemovalRefusesUnsafeLocalObject(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 a:=&API{db:d,dataDir:t.TempDir(),podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
    return podcast.MediaSegment{Start:0,End:3,Total:4,MIME:"audio/mpeg",Data:[]byte("ID3!")},nil
 }}
 router:=a.PodcastRoutes()
 req:=func(method string)*httptest.ResponseRecorder{
    rec:=httptest.NewRecorder()
    router.ServeHTTP(rec,httptest.NewRequest(method,"/episodes/"+id+"/download",nil))
    return rec
 }
 if res:=req("POST");res.Code!=202{t.Fatalf("queue: %d",res.Code)}
 a.processPodcastDownload(context.Background())
 path:=filepath.Join(a.dataDir,"podcasts","episodes",id+".audio")
 if err:=os.Remove(path);err!=nil{t.Fatal(err)}
 if err:=os.Mkdir(path,0700);err!=nil{t.Fatal(err)}
 removed:=req("DELETE")
 if removed.Code!=409{t.Fatalf("special file deleted: %d",removed.Code)}
 if info,err:=os.Stat(path);err!=nil||!info.IsDir(){t.Fatalf("unexpected directory removal: %v",err)}
}

func TestPodcastOfflineIntegrityRejectsCorruptionAndRecoversOnRetry(t *testing.T){
    d,id:=seedPodcastDownload(t);defer d.Close()
    ctx:=context.Background()
    pathRoot:=t.TempDir()
    makeAPI:=func()*API{
        return &API{db:d,dataDir:pathRoot,podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
            return podcast.MediaSegment{Start:0,End:3,Total:4,MIME:"audio/mpeg",Data:[]byte("ID3!")},nil
        }}
    }
    a:=makeAPI()
    router:=a.PodcastRoutes()
    req:=func(handler http.Handler,method,path string)*httptest.ResponseRecorder{
        t.Helper();res:=httptest.NewRecorder()
        handler.ServeHTTP(res,httptest.NewRequest(method,path,nil))
        return res
    }
    if out:=req(router,"POST","/episodes/"+id+"/download");out.Code!=202{t.Fatalf("queue: %d",out.Code)}
    a.processPodcastDownload(ctx)
    if good:=req(router,"GET","/episodes/"+id+"/media");good.Code!=200||good.Body.String()!="ID3!"{
        t.Fatalf("first verified playback %d %q",good.Code,good.Body.String())
    }
    diskPath:=filepath.Join(pathRoot,"podcasts","episodes",id+".audio")
    if err:=os.WriteFile(diskPath,[]byte("BAD!"),0600);err!=nil{t.Fatal(err)}
    // A new desktop/API process must perform the checksum check again even
    // when a cached previous process already verified this file.
    reopened:=makeAPI()
    corrupt:=req(reopened.PodcastRoutes(),"GET","/episodes/"+id+"/media")
    if corrupt.Code!=http.StatusGone||!strings.Contains(corrupt.Body.String(),"podcast_download_corrupt"){
        t.Fatalf("corrupted same-size file still served: %d %s",corrupt.Code,corrupt.Body.String())
    }
    row,err:=d.GetPodcastDownload(ctx,id)
    if err!=nil||row.Status!="failed"{t.Fatalf("corruption status: %+v %v",row,err)}
    if again:=req(reopened.PodcastRoutes(),"POST","/episodes/"+id+"/download");again.Code!=202{
        t.Fatalf("retry after corruption: %d",again.Code)
    }
    // Retried claim must remove only its known orphan final file, not write
    // over an unsafe path or make the corrupt media downloadable again.
    reopened.processPodcastDownload(ctx)
    restored:=req(reopened.PodcastRoutes(),"GET","/episodes/"+id+"/media")
    if restored.Code!=200||restored.Body.String()!="ID3!"{
        t.Fatalf("safe retry not restored: %d %q",restored.Code,restored.Body.String())
    }
}

func TestPodcastStaleTempCleanupKeepsRecentAndForeignFiles(t *testing.T){
    a:=&API{dataDir:t.TempDir()}
    dir,err:=a.podcastStorageDir();if err!=nil{t.Fatal(err)}
    now:=time.Now()
    files:=[]struct{name string;old bool;removed bool}{
        {".part-123456789",true,true},
        {".part-987654321",false,false},
        {".part-private-not-owned",true,false},
        {"unrelated-file.audio",true,false},
    }
    for _,tc:=range files{
        path:=filepath.Join(dir,tc.name)
        if err:=os.WriteFile(path,[]byte("test"),0600);err!=nil{t.Fatal(err)}
        stamp:=now
        if tc.old{stamp=now.Add(-podcastAbandonedPartGrace-time.Minute)}
        if err:=os.Chtimes(path,stamp,stamp);err!=nil{t.Fatal(err)}
    }
    a.cleanupPodcastAbandonedParts(dir,now)
    for _,tc:=range files{
        _,err:=os.Lstat(filepath.Join(dir,tc.name))
        if tc.removed && !errors.Is(err,os.ErrNotExist){t.Fatalf("abandoned temp not removed: %s %v",tc.name,err)}
        if !tc.removed && err!=nil{t.Fatalf("non-owned/recent file removed: %s %v",tc.name,err)}
    }
}

// The disk preflight needs no genuine 512 MiB fixture. Simulate a volume with
// less than the remaining file size plus the reserved OS safety margin.
func TestPodcastDownloadDiskFreePreflightRejectsLowSpaceAndProbeErrors(t *testing.T){
 for _,tc:=range []struct{name string;available uint64;err error}{
    {"low space",podcastDiskFreeReserve+3,nil},
    {"unknown volume",0,errors.New("volume probe denied")},
 }{
    t.Run(tc.name,func(t *testing.T){
        d,id:=seedPodcastDownload(t);defer d.Close()
        calls:=0
        a:=&API{db:d,dataDir:t.TempDir(),
          podcastDiskFreeBytes:func(string)(uint64,error){calls++;return tc.available,tc.err},
          podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
            return podcast.MediaSegment{Start:0,End:3,Total:4,MIME:"audio/mpeg",Data:[]byte("ID3!")},nil
          }}
        req:=httptest.NewRecorder()
        a.PodcastRoutes().ServeHTTP(req,httptest.NewRequest(http.MethodPost,"/episodes/"+id+"/download",nil))
        if req.Code!=http.StatusAccepted{t.Fatalf("queue: %d",req.Code)}
        a.processPodcastDownload(context.Background())
        row,err:=d.GetPodcastDownload(context.Background(),id)
        if err!=nil||row.Status!="failed"||row.ErrorCode!="download_disk_space_low"{
            t.Fatalf("low disk not safely reported: %+v %v",row,err)
        }
        if calls!=1{t.Fatalf("expected one disk probe: %d",calls)}
        path:=filepath.Join(a.dataDir,"podcasts","episodes",id+".audio")
        if _,err:=os.Lstat(path);!errors.Is(err,os.ErrNotExist){
            t.Fatalf("insufficient-space download published: %v",err)
        }
    })
 }
}

func TestPodcastDownloadDiskFreePreflightAcceptsBudgetedSpace(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 calls:=0
 a:=&API{db:d,dataDir:t.TempDir(),
    podcastDiskFreeBytes:func(string)(uint64,error){calls++;return podcastDiskFreeReserve+8,nil},
    podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
        return podcast.MediaSegment{Start:0,End:3,Total:4,MIME:"audio/mpeg",Data:[]byte("ID3!")},nil
    }}
 r:=httptest.NewRecorder()
 a.PodcastRoutes().ServeHTTP(r,httptest.NewRequest(http.MethodPost,"/episodes/"+id+"/download",nil))
 if r.Code!=http.StatusAccepted{t.Fatalf("queue: %d",r.Code)}
 a.processPodcastDownload(context.Background())
 row,err:=d.GetPodcastDownload(context.Background(),id)
 if err!=nil||row.Status!="completed"||calls!=1{
    t.Fatalf("ample disk unexpectedly refused: %+v, probes %d, err %v",row,calls,err)
 }
}

func TestPodcastUnfollowReclaimsCompletedFileAndRetainsHistory(t *testing.T){
    d,id:=seedPodcastDownload(t);defer d.Close()
    ctx:=context.Background()
    a:=&API{db:d,dataDir:t.TempDir(),podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
        return podcast.MediaSegment{Start:0,End:3,Total:4,MIME:"audio/mpeg",Data:[]byte("ID3!")},nil
    }}
    subs,err:=d.ListSubscribedPodcasts(ctx)
    if err!=nil||len(subs)!=1{t.Fatalf("subscriptions: %+v %v",subs,err)}
    feedID:=subs[0].ID
    router:=a.PodcastRoutes()
    req:=func(method,path string)*httptest.ResponseRecorder{
        t.Helper()
        out:=httptest.NewRecorder()
        router.ServeHTTP(out,httptest.NewRequest(method,path,nil))
        return out
    }
    if got:=req("POST","/episodes/"+id+"/download");got.Code!=202{t.Fatalf("queue: %d",got.Code)}
    a.processPodcastDownload(ctx)
    file,err:=d.PodcastCompletedDownload(ctx,id);if err!=nil{t.Fatal(err)}
    path:=filepath.Join(a.dataDir,"podcasts","episodes",file.Filename)
    if _,err=os.Lstat(path);err!=nil{t.Fatal(err)}
    if _,err:=d.SetPodcastProgress(ctx,id,18000,22000,0,false);err!=nil{t.Fatal(err)}
    if got:=req("DELETE","/feeds/"+feedID);got.Code!=204{
        t.Fatalf("unsubscribe: %d %s",got.Code,got.Body.String())
    }
    if _,err=os.Lstat(path);!errors.Is(err,os.ErrNotExist){
        t.Fatalf("unfollowed file remains: %v",err)
    }
    used,err:=d.PodcastOfflineStorage(ctx)
    if err!=nil||used!=0{t.Fatalf("unfollow storage not released: %d %v",used,err)}
    if got:=req("GET","/episodes/"+id+"/download");got.Code!=404{
        t.Fatalf("unfollow download still visible: %d",got.Code)
    }
    progress,err:=d.GetPodcastProgress(ctx,id)
    if err!=nil||progress.PositionMs!=18000||progress.ListenedMs!=22000{
        t.Fatalf("unfollow changed listening history: %+v %v",progress,err)
    }
}

func TestPodcastUnfollowRevokesActiveTransferLease(t *testing.T){
    d,id:=seedPodcastDownload(t);defer d.Close()
    ctx:=context.Background()
    if _,err:=d.EnqueuePodcastDownload(ctx,id);err!=nil{t.Fatal(err)}
    claimed,err:=d.ClaimPodcastDownload(ctx,"worker-active",time.Now())
    if err!=nil||claimed!=id{t.Fatalf("claim: %q %v",claimed,err)}
    subs,err:=d.ListSubscribedPodcasts(ctx)
    if err!=nil||len(subs)!=1{t.Fatalf("subscriptions: %+v %v",subs,err)}
    if err:=d.UnsubscribePodcast(ctx,subs[0].ID);err!=nil{t.Fatal(err)}
    if err:=d.UpdatePodcastDownloadBytes(ctx,id,"worker-active",1,4);!errors.Is(err,sql.ErrNoRows){
        t.Fatalf("unfollow did not revoke lease: %v",err)
    }
    if err:=d.CompletePodcastDownload(ctx,id,"worker-active",id+".audio",
        "audio/mpeg",strings.Repeat("a",64),4);!errors.Is(err,sql.ErrNoRows){
        t.Fatalf("unfollowed worker completed audio: %v",err)
    }
    used,err:=d.PodcastOfflineStorage(ctx)
    if err!=nil||used!=0{t.Fatalf("unfollowed transfer reserved disk: %d %v",used,err)}
}

func TestPodcastUnfollowCleanupRefusesUnsafeFile(t *testing.T){
    d,id:=seedPodcastDownload(t);defer d.Close()
    ctx:=context.Background()
    a:=&API{db:d,dataDir:t.TempDir(),podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
        return podcast.MediaSegment{Start:0,End:3,Total:4,MIME:"audio/mpeg",Data:[]byte("ID3!")},nil
    }}
    router:=a.PodcastRoutes()
    out:=httptest.NewRecorder()
    router.ServeHTTP(out,httptest.NewRequest("POST","/episodes/"+id+"/download",nil))
    if out.Code!=202{t.Fatalf("queue: %d",out.Code)}
    a.processPodcastDownload(ctx)
    path:=filepath.Join(a.dataDir,"podcasts","episodes",id+".audio")
    if err:=os.Remove(path);err!=nil{t.Fatal(err)}
    if err:=os.Mkdir(path,0700);err!=nil{t.Fatal(err)}
    subs,err:=d.ListSubscribedPodcasts(ctx)
    if err!=nil||len(subs)!=1{t.Fatal(err)}
    if err:=d.UnsubscribePodcast(ctx,subs[0].ID);err!=nil{t.Fatal(err)}
    a.cleanupUnsubscribedPodcastFiles(ctx)
    info,err:=os.Stat(path)
    if err!=nil||!info.IsDir(){t.Fatalf("unsafe object removed: %v",err)}
    used,err:=d.PodcastOfflineStorage(ctx)
    if err!=nil||used!=4{t.Fatalf("unsafe orphan should remain flagged: %d %v",used,err)}
}

func TestPodcastDownloadRejectsPublisherRepresentationChangedMidTransfer(t *testing.T){
    d,id:=seedPodcastDownload(t);defer d.Close()
    calls:=0
    a:=&API{db:d,dataDir:t.TempDir(),podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
        calls++
        if calls==1{
            return podcast.MediaSegment{Start:0,End:3,Total:8,MIME:"audio/mpeg",ETag:"\"first\"",Data:[]byte("ID3a")},nil
        }
        return podcast.MediaSegment{Start:4,End:7,Total:8,MIME:"audio/mpeg",ETag:"\"changed\"",Data:[]byte("bcde")},nil
    }}
    w:=httptest.NewRecorder()
    a.PodcastRoutes().ServeHTTP(w,httptest.NewRequest(http.MethodPost,"/episodes/"+id+"/download",nil))
    if w.Code!=202{t.Fatalf("queue: %d",w.Code)}
    a.processPodcastDownload(context.Background())
    record,err:=d.GetPodcastDownload(context.Background(),id)
    if err!=nil||record.Status!="failed"||record.ErrorCode!="download_publisher_changed"{
        t.Fatalf("publisher version transition was not rejected: %+v %v",record,err)
    }
    if calls!=2{t.Fatalf("expected two chunks, got %d",calls)}
    path:=filepath.Join(a.dataDir,"podcasts","episodes",id+".audio")
    if _,err:=os.Lstat(path);!errors.Is(err,os.ErrNotExist){
        t.Fatalf("mixed-version audio published: %v",err)
    }
}

func TestPodcastFull200OfflineDownloadAndLocalSeeking(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 calls:=0
 a:=&API{db:d,dataDir:t.TempDir(),
  podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
    return podcast.MediaSegment{},podcast.ErrPublisherNoRange
  },
  podcastDownloadFullFetch:func(_ context.Context,raw,declared string,dst io.Writer,maxBytes int64)(podcast.FullMedia,error){
    calls++
    if !strings.Contains(raw,"signature=keep-private")||declared!="audio/mpeg"||maxBytes!=db.MaxPodcastDownloadBytes{
        t.Fatal("unsafe full fetch inputs")
    }
    data:=[]byte("ID3abcdefg")
    if _,err:=dst.Write(data);err!=nil{return podcast.FullMedia{},err}
    return podcast.FullMedia{MIME:"audio/mpeg",Size:int64(len(data))},nil
  },
 }
 router:=a.PodcastRoutes()
 request:=func(method,path string)*httptest.ResponseRecorder{
    t.Helper()
    result:=httptest.NewRecorder()
    router.ServeHTTP(result,httptest.NewRequest(method,path,nil))
    return result
 }
 if got:=request("POST","/episodes/"+id+"/download");got.Code!=202{t.Fatalf("queue: %d",got.Code)}
 a.processPodcastDownload(context.Background())
 row,err:=d.GetPodcastDownload(context.Background(),id)
 if err!=nil||row.Status!="completed"||row.VerifiedSize!=10||calls!=1{
    t.Fatalf("full 200 publish: %+v calls=%d err=%v",row,calls,err)
 }
 a.podcastMediaFetch=func(context.Context,string,string)(podcast.MediaSegment,error){
    t.Error("offline playback accessed publisher")
    return podcast.MediaSegment{},errors.New("disconnected")
 }
 media:=httptest.NewRecorder()
 req:=httptest.NewRequest(http.MethodGet,"/episodes/"+id+"/media",nil)
 req.Header.Set("Range","bytes=3-6")
 router.ServeHTTP(media,req)
 if media.Code!=206||media.Body.String()!="abcd"{
    t.Fatalf("offline seek on downloaded 200 media: %d %q",media.Code,media.Body.String())
 }
}

func TestPodcastFull200CancellationDoesNotPublish(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 a:=&API{db:d,dataDir:t.TempDir(),
  podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
    return podcast.MediaSegment{},podcast.ErrPublisherNoRange
  },
  podcastDownloadFullFetch:func(ctx context.Context,_ string,_ string,dst io.Writer,_ int64)(podcast.FullMedia,error){
    if _,err:=dst.Write([]byte("ID3x"));err!=nil{return podcast.FullMedia{},err}
    if _,err:=d.CancelPodcastDownload(ctx,id);err!=nil{return podcast.FullMedia{},err}
    return podcast.FullMedia{MIME:"audio/mpeg",Size:4},nil
  },
 }
 router:=a.PodcastRoutes()
 queued:=httptest.NewRecorder()
 router.ServeHTTP(queued,httptest.NewRequest(http.MethodPost,"/episodes/"+id+"/download",nil))
 if queued.Code!=202{t.Fatalf("queue: %d",queued.Code)}
 a.processPodcastDownload(context.Background())
 row,err:=d.GetPodcastDownload(context.Background(),id)
 if err!=nil||row.Status!="canceled"{t.Fatalf("canceled 200: %+v %v",row,err)}
 if _,err:=os.Stat(filepath.Join(a.dataDir,"podcasts","episodes",id+".audio"));!errors.Is(err,os.ErrNotExist){
    t.Fatalf("canceled 200 downloaded audio was published: %v",err)
 }
}

func TestPodcastFull200UserCancellationStopsBlockedStream(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 ctx:=context.Background()
 entered:=make(chan struct{})
 exited:=make(chan struct{})
 a:=&API{db:d,dataDir:t.TempDir(),
   podcastDownloadFetch:func(context.Context,string,string,string)(podcast.MediaSegment,error){
     return podcast.MediaSegment{},podcast.ErrPublisherNoRange
   },
   podcastDownloadFullFetch:func(fetchCtx context.Context,_ string,_ string,_ io.Writer,_ int64)(podcast.FullMedia,error){
     close(entered)
     <-fetchCtx.Done()
     close(exited)
     return podcast.FullMedia{},fetchCtx.Err()
   },
 }
 queued:=httptest.NewRecorder()
 a.PodcastRoutes().ServeHTTP(queued,httptest.NewRequest(http.MethodPost,
     "/episodes/"+id+"/download",nil))
 if queued.Code!=http.StatusAccepted{t.Fatalf("queue: %d",queued.Code)}
 finished:=make(chan struct{})
 go func(){a.processPodcastDownload(ctx);close(finished)}()
 select{
 case <-entered:
 case <-time.After(4*time.Second):t.Fatal("full transfer not entered")
 }
 if _,err:=d.CancelPodcastDownload(ctx,id);err!=nil{t.Fatal(err)}
 select{
 case <-finished:
 case <-time.After(6*time.Second):t.Fatal("canceled full 200 transfer held network open")
 }
 select{
 case <-exited:
 default:t.Fatal("publisher stream did not receive cancellation context")
 }
 row,err:=d.GetPodcastDownload(ctx,id)
 if err!=nil||row.Status!="canceled"{t.Fatalf("canceled stream published: %+v %v",row,err)}
 if _,err:=os.Stat(filepath.Join(a.dataDir,"podcasts","episodes",id+".audio"));!errors.Is(err,os.ErrNotExist){
    t.Fatalf("canceled stream published local file: %v",err)
 }
}


// A crashed worker left a byte-verified prefix in a real private file.
// The recovering worker MUST issue its first request at the saved offset,
// hash the copied prefix, and preserve offline seeking without publisher IO.
func TestPodcastResumeFromVerifiedStagedFileAfterLeaseReclaim(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 ctx:=context.Background()
 a:=&API{db:d,dataDir:t.TempDir()}
 dir,err:=a.podcastStorageDir();if err!=nil{t.Fatal(err)}
 staged,err:=os.CreateTemp(dir,".part-*");if err!=nil{t.Fatal(err)}
 prefix:=[]byte("ID3a")
 if _,err:=staged.Write(prefix);err!=nil{t.Fatal(err)}
 if err:=staged.Sync();err!=nil{t.Fatal(err)}
 if err:=staged.Close();err!=nil{t.Fatal(err)}
 raw,_,err:=d.PodcastMediaSource(ctx,id);if err!=nil{t.Fatal(err)}
 sourceDigest:=sha256.Sum256([]byte(raw))
 chunkDigest:=sha256.Sum256(prefix)
 cp:=db.PodcastResumeCheckpoint{
    TempFilename:filepath.Base(staged.Name()),Bytes:4,Total:8,ETag:"\"episode-v1\"",
    MIME:"audio/mpeg",SHA256:hex.EncodeToString(chunkDigest[:]),
    SourceSHA256:hex.EncodeToString(sourceDigest[:]),
 }
 if _,err:=d.EnqueuePodcastDownload(ctx,id);err!=nil{t.Fatal(err)}
 claimed,err:=d.ClaimPodcastDownload(ctx,"old-worker",time.Now())
 if err!=nil||claimed!=id{t.Fatalf("claim: %s %v",claimed,err)}
 if err:=d.SavePodcastDownloadCheckpoint(ctx,id,"old-worker",cp);err!=nil{t.Fatal(err)}
 expired,err:=d.ClaimPodcastDownload(ctx,"new-worker",time.Now().Add(5*time.Minute))
 if err!=nil||expired!=id{t.Fatalf("reclaim: %s %v",expired,err)}
 calls:=0
 a.podcastDownloadFetch=func(_ context.Context,_ string,rangeHeader string,_ string)(podcast.MediaSegment,error){
    calls++
    if rangeHeader!="bytes=4-2097155"{t.Fatalf("did not resume saved offset: %q",rangeHeader)}
    return podcast.MediaSegment{
      Start:4,End:7,Total:8,MIME:"audio/mpeg",ETag:"\"episode-v1\"",Data:[]byte("bcde"),
    },nil
 }
 if err:=a.transferPodcastDownload(ctx,id,"new-worker");err!=nil{t.Fatal(err)}
 if calls!=1{t.Fatalf("expected only one suffix fetch, got %d",calls)}
 file,err:=d.PodcastCompletedDownload(ctx,id);if err!=nil{t.Fatal(err)}
 data,err:=os.ReadFile(filepath.Join(dir,file.Filename))
 if err!=nil||string(data)!="ID3abcde"{t.Fatalf("resumed bytes invalid: %q %v",data,err)}
 // Stale source may remain but is no longer referenced; the new file owns
 // all completed bytes. A fresh local playback needs no remote publisher.
 a.podcastMediaFetch=func(context.Context,string,string)(podcast.MediaSegment,error){
    t.Fatal("offline resume unexpectedly used the network")
    return podcast.MediaSegment{},errors.New("unexpected network")
 }
 media:=httptest.NewRecorder()
 request:=httptest.NewRequest("GET","/episodes/"+id+"/media",nil)
 request.Header.Set("Range","bytes=3-6")
 a.PodcastRoutes().ServeHTTP(media,request)
 if media.Code!=206||media.Body.String()!="abcd"{
    t.Fatalf("resumed local seek %d %q",media.Code,media.Body.String())
 }
}

func TestPodcastResumeCorruptOrRotatedPrefixRestartsCleanly(t *testing.T){
 for _,tc:=range []struct{name string; corrupt bool; rotate bool}{
   {"tampered prefix",true,false},{"enclosure rotated",false,true},
 }{
  t.Run(tc.name,func(t *testing.T){
    d,id:=seedPodcastDownload(t);defer d.Close()
    ctx:=context.Background()
    a:=&API{db:d,dataDir:t.TempDir()}
    dir,err:=a.podcastStorageDir();if err!=nil{t.Fatal(err)}
    stage,err:=os.CreateTemp(dir,".part-*");if err!=nil{t.Fatal(err)}
    original:=[]byte("ID3a")
    actual:=original
    if tc.corrupt{actual=[]byte("BAD!")}
    if _,err:=stage.Write(actual);err!=nil{t.Fatal(err)}
    if err:=stage.Sync();err!=nil{t.Fatal(err)}
    if err:=stage.Close();err!=nil{t.Fatal(err)}
    raw,_,err:=d.PodcastMediaSource(ctx,id);if err!=nil{t.Fatal(err)}
    sourceHash:=sha256.Sum256([]byte(raw))
    digest:=sha256.Sum256(original)
    srcHash:=hex.EncodeToString(sourceHash[:])
    if tc.rotate{srcHash=strings.Repeat("0",64)}
    cp:=db.PodcastResumeCheckpoint{
      TempFilename:filepath.Base(stage.Name()),Bytes:4,Total:8,ETag:"\"episode-v1\"",
      MIME:"audio/mpeg",SHA256:hex.EncodeToString(digest[:]),SourceSHA256:srcHash,
    }
    if _,err:=d.EnqueuePodcastDownload(ctx,id);err!=nil{t.Fatal(err)}
    if _,err:=d.ClaimPodcastDownload(ctx,"old",time.Now());err!=nil{t.Fatal(err)}
    if err:=d.SavePodcastDownloadCheckpoint(ctx,id,"old",cp);err!=nil{t.Fatal(err)}
    if _,err:=d.ClaimPodcastDownload(ctx,"new",time.Now().Add(5*time.Minute));err!=nil{t.Fatal(err)}
    calls:=0
    a.podcastDownloadFetch=func(_ context.Context,_ string,rangeHeader string,_ string)(podcast.MediaSegment,error){
       calls++
       if rangeHeader!="bytes=0-2097151"{t.Fatalf("unsafe resume used range %q",rangeHeader)}
       return podcast.MediaSegment{Start:0,End:7,Total:8,
         MIME:"audio/mpeg",ETag:"\"episode-v1\"",Data:[]byte("ID3abcde")},nil
    }
    if err:=a.transferPodcastDownload(ctx,id,"new");err!=nil{t.Fatal(err)}
    if calls!=1{t.Fatalf("unexpected requests %d",calls)}
    file,err:=d.PodcastCompletedDownload(ctx,id);if err!=nil{t.Fatal(err)}
    bytes,err:=os.ReadFile(filepath.Join(dir,file.Filename))
    if err!=nil||string(bytes)!="ID3abcde"{t.Fatalf("unsafe bytes: %q %v",bytes,err)}
  })
 }
}

func TestPodcastStagedResumeFileSurvivesAgedTempSweep(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 ctx:=context.Background()
 a:=&API{db:d,dataDir:t.TempDir()}
 dir,err:=a.podcastStorageDir();if err!=nil{t.Fatal(err)}
 f,err:=os.CreateTemp(dir,".part-*");if err!=nil{t.Fatal(err)}
 if _,err:=f.Write([]byte("ID3a"));err!=nil{t.Fatal(err)}
 if err:=f.Close();err!=nil{t.Fatal(err)}
 raw,_,err:=d.PodcastMediaSource(ctx,id);if err!=nil{t.Fatal(err)}
 src:=sha256.Sum256([]byte(raw))
 sha:=sha256.Sum256([]byte("ID3a"))
 if _,err:=d.EnqueuePodcastDownload(ctx,id);err!=nil{t.Fatal(err)}
 if _,err:=d.ClaimPodcastDownload(ctx,"owner",time.Now());err!=nil{t.Fatal(err)}
 if err:=d.SavePodcastDownloadCheckpoint(ctx,id,"owner",db.PodcastResumeCheckpoint{
    TempFilename:filepath.Base(f.Name()),Bytes:4,Total:8,ETag:"\"v1\"",
    MIME:"audio/mpeg",SHA256:hex.EncodeToString(sha[:]),SourceSHA256:hex.EncodeToString(src[:]),
 });err!=nil{t.Fatal(err)}
 now:=time.Now()
 old:=now.Add(-podcastAbandonedPartGrace-time.Minute)
 if err:=os.Chtimes(f.Name(),old,old);err!=nil{t.Fatal(err)}
 a.cleanupPodcastAbandonedParts(dir,now)
 if _,err:=os.Stat(f.Name());err!=nil{t.Fatalf("checkpoint swept: %v",err)}
}

func TestPodcastResumeRecovers416AndChangedFirstRangeByOneCleanRestart(t *testing.T){
 for _,tc:=range []struct{name string;staleError error;changedETag bool}{
    {"416 range no longer valid",podcast.ErrInvalidMediaRange,false},
    {"strong ETag changed",nil,true},
 }{
  t.Run(tc.name,func(t *testing.T){
    d,id:=seedPodcastDownload(t);defer d.Close()
    ctx:=context.Background()
    a:=&API{db:d,dataDir:t.TempDir()}
    dir,err:=a.podcastStorageDir();if err!=nil{t.Fatal(err)}
    f,err:=os.CreateTemp(dir,".part-*");if err!=nil{t.Fatal(err)}
    prefix:=[]byte("ID3a")
    if _,err:=f.Write(prefix);err!=nil{t.Fatal(err)}
    if err:=f.Sync();err!=nil{t.Fatal(err)}
    if err:=f.Close();err!=nil{t.Fatal(err)}
    source,_,err:=d.PodcastMediaSource(ctx,id);if err!=nil{t.Fatal(err)}
    sourceHash:=sha256.Sum256([]byte(source))
    prefixHash:=sha256.Sum256(prefix)
    if _,err:=d.EnqueuePodcastDownload(ctx,id);err!=nil{t.Fatal(err)}
    if _,err:=d.ClaimPodcastDownload(ctx,"old",time.Now());err!=nil{t.Fatal(err)}
    if err:=d.SavePodcastDownloadCheckpoint(ctx,id,"old",db.PodcastResumeCheckpoint{
      TempFilename:filepath.Base(f.Name()),Bytes:4,Total:8,ETag:"\"old\"",
      MIME:"audio/mpeg",SHA256:hex.EncodeToString(prefixHash[:]),
      SourceSHA256:hex.EncodeToString(sourceHash[:]),
    });err!=nil{t.Fatal(err)}
    claimed,err:=d.ClaimPodcastDownload(ctx,"new",time.Now().Add(5*time.Minute))
    if err!=nil||claimed!=id{t.Fatalf("reclaimed: %s %v",claimed,err)}
    calls:=0
    a.podcastDownloadFetch=func(_ context.Context,_ string,header string,_ string)(podcast.MediaSegment,error){
        calls++
        if calls==1{
            if header!="bytes=4-2097155"{t.Fatalf("expected resumed range, got %q",header)}
            if tc.staleError!=nil{return podcast.MediaSegment{},tc.staleError}
            if tc.changedETag{return podcast.MediaSegment{
                Start:4,End:7,Total:8,MIME:"audio/mpeg",
                ETag:"\"new\"",Data:[]byte("bcde")},nil}
        }
        if calls==2{
            if header!="bytes=0-2097151"{t.Fatalf("not restarted from zero: %q",header)}
            return podcast.MediaSegment{Start:0,End:7,Total:8,
                MIME:"audio/mpeg",ETag:"\"new\"",Data:[]byte("ID3abcde")},nil
        }
        t.Fatalf("unbounded publisher retry %d",calls)
        return podcast.MediaSegment{},errors.New("unbounded requests")
    }
    if err:=a.transferPodcastDownload(ctx,id,"new");err!=nil{t.Fatal(err)}
    if calls!=2{t.Fatalf("want single clean restart, calls=%d",calls)}
    file,err:=d.PodcastCompletedDownload(ctx,id);if err!=nil{t.Fatal(err)}
    data,err:=os.ReadFile(filepath.Join(dir,file.Filename))
    if err!=nil||string(data)!="ID3abcde"{t.Fatalf("mixed/stale bytes published: %q %v",data,err)}
  })
 }
}

func TestPodcastResume416RetryOccursAtMostOnce(t *testing.T){
 d,id:=seedPodcastDownload(t);defer d.Close()
 ctx:=context.Background()
 a:=&API{db:d,dataDir:t.TempDir()}
 dir,err:=a.podcastStorageDir();if err!=nil{t.Fatal(err)}
 f,err:=os.CreateTemp(dir,".part-*");if err!=nil{t.Fatal(err)}
 if _,err:=f.Write([]byte("ID3a"));err!=nil{t.Fatal(err)}
 if err:=f.Close();err!=nil{t.Fatal(err)}
 source,_,err:=d.PodcastMediaSource(ctx,id);if err!=nil{t.Fatal(err)}
 sourceHash:=sha256.Sum256([]byte(source))
 prefixHash:=sha256.Sum256([]byte("ID3a"))
 if _,err:=d.EnqueuePodcastDownload(ctx,id);err!=nil{t.Fatal(err)}
 if _,err:=d.ClaimPodcastDownload(ctx,"old",time.Now());err!=nil{t.Fatal(err)}
 if err:=d.SavePodcastDownloadCheckpoint(ctx,id,"old",db.PodcastResumeCheckpoint{
    TempFilename:filepath.Base(f.Name()),Bytes:4,Total:8,
    MIME:"audio/mpeg",ETag:"\"old\"",
    SHA256:hex.EncodeToString(prefixHash[:]),SourceSHA256:hex.EncodeToString(sourceHash[:]),
 });err!=nil{t.Fatal(err)}
 if _,err:=d.ClaimPodcastDownload(ctx,"new",time.Now().Add(5*time.Minute));err!=nil{t.Fatal(err)}
 calls:=0
 a.podcastDownloadFetch=func(_ context.Context,_ string,_ string,_ string)(podcast.MediaSegment,error){
     calls++
     return podcast.MediaSegment{},podcast.ErrInvalidMediaRange
 }
 if err:=a.transferPodcastDownload(ctx,id,"new");!errors.Is(err,podcast.ErrInvalidMediaRange){
     t.Fatalf("expected final 416 after one restart, got %v",err)
 }
 if calls!=2{t.Fatalf("expected bounded two requests, got %d",calls)}
 if _,err:=os.Lstat(filepath.Join(dir,id+".audio"));!errors.Is(err,os.ErrNotExist){
    t.Fatalf("failed resume published audio: %v",err)
 }
}

func TestPodcastTransientNetworkResumesVerifiedPartialWithoutRestart(t *testing.T){
    d,id:=seedPodcastDownload(t);defer d.Close()
    ctx:=context.Background()
    a:=&API{db:d,dataDir:t.TempDir(),podcastDownloadOwner:"transient-old"}
    fetchCalls:=0
    a.podcastDownloadFetch=func(_ context.Context,_ string,header string,_ string)(podcast.MediaSegment,error){
        fetchCalls++
        switch fetchCalls{
        case 1:
            if header!="bytes=0-2097151"{t.Fatalf("initial range %q",header)}
            return podcast.MediaSegment{Start:0,End:3,Total:8,MIME:"audio/mpeg",
                ETag:"\"stable-v1\"",Data:[]byte("ID3a")},nil
        case 2:
            if header!="bytes=4-2097155"{t.Fatalf("interrupted range %q",header)}
            return podcast.MediaSegment{},podcast.ErrMediaUnavailable
        case 3:
            if header!="bytes=4-2097155"{t.Fatalf("retry did not reuse prefix: %q",header)}
            return podcast.MediaSegment{Start:4,End:7,Total:8,MIME:"audio/mpeg",
                ETag:"\"stable-v1\"",Data:[]byte("bcde")},nil
        default:
            t.Fatalf("unbounded range requests: %d",fetchCalls)
            return podcast.MediaSegment{},errors.New("unexpected")
        }
    }
    route:=a.PodcastRoutes()
    enqueue:=httptest.NewRecorder()
    route.ServeHTTP(enqueue,httptest.NewRequest(http.MethodPost,"/episodes/"+id+"/download",nil))
    if enqueue.Code!=http.StatusAccepted{t.Fatalf("queue: %d",enqueue.Code)}
    a.processPodcastDownload(ctx)
    paused,err:=d.GetPodcastDownload(ctx,id)
    if err!=nil||paused.Status!="paused"||paused.BytesReceived!=4||
       paused.ErrorCode!="download_unavailable"||paused.RetryAfter<=time.Now().UnixMilli(){
        t.Fatalf("publisher outage lost checkpoint: %+v %v",paused,err)
    }
    // The old fsynced staged file is still present; a new lease must copy
    // and verify it, not append to a possibly still-open old worker's file.
    protected,err:=d.ActivePodcastStagedFilenames(ctx)
    if err!=nil||len(protected)!=1{t.Fatalf("staged checkpoint not protected: %v %v",protected,err)}
    dir,err:=a.podcastStorageDir();if err!=nil{t.Fatal(err)}
    for filename:=range protected {
        contents,err:=os.ReadFile(filepath.Join(dir,filename))
        if err!=nil||string(contents)!="ID3a"{t.Fatalf("staged prefix lost: %q %v",contents,err)}
    }
    if got,err:=d.ClaimPodcastDownload(ctx,"too-soon",time.Now());err!=nil||got!=""{
        t.Fatalf("retried before deadline: %q %v",got,err)
    }
    newOwner:="transient-new"
    if got,err:=d.ClaimPodcastDownload(ctx,newOwner,time.UnixMilli(paused.RetryAfter));err!=nil||got!=id{
        t.Fatalf("due retry not reclaimed: %q %v",got,err)
    }
    if err:=a.transferPodcastDownload(ctx,id,newOwner);err!=nil{t.Fatal(err)}
    if fetchCalls!=3{t.Fatalf("redownloaded saved prefix: %d requests",fetchCalls)}
    done,err:=d.GetPodcastDownload(ctx,id)
    if err!=nil||done.Status!="completed"||done.VerifiedSize!=8{
        t.Fatalf("transient resume did not publish: %+v %v",done,err)
    }
    file,err:=d.PodcastCompletedDownload(ctx,id)
    if err!=nil{t.Fatal(err)}
    final,err:=os.ReadFile(filepath.Join(dir,file.Filename))
    if err!=nil||string(final)!="ID3abcde"{t.Fatalf("wrong audio: %q %v",final,err)}
}
