package api

import (
 "bytes"
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "net/http"
 "net/http/httptest"
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
   return podcast.MediaSegment{Start:0,End:3,Total:8,MIME:"audio/mpeg",Data:[]byte("ID3a")},nil
  }
  if calls==2 {
   if rangeHeader!="bytes=4-2097155"{t.Fatalf("second range %q",rangeHeader)}
   return podcast.MediaSegment{Start:4,End:7,Total:8,MIME:"audio/mpeg",Data:[]byte("bcde")},nil
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
