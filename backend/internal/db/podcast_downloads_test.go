package db

import (
 "context"
 "database/sql"
 "errors"
 "path/filepath"
 "testing"
 "time"
 "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastDownloadLifecycleSurvivesRestartAndValidatesLease(t *testing.T) {
 ctx:=context.Background()
 path:=filepath.Join(t.TempDir(),"podcast-download.db")
 d,err:=New(path);if err!=nil{t.Fatal(err)}
 feedID,err:=d.SavePodcastFeedSnapshot(ctx,podcast.Feed{
  URL:"https://publisher.example/rss", Title:"Show",
  Episodes:[]podcast.Episode{{GUID:"one",Title:"Audio",EnclosureURL:"https://cdn.example/one.mp3",EnclosureType:"audio/mpeg"}},
 },true);if err!=nil{t.Fatal(err)}
 eps,err:=d.ListPodcastEpisodes(ctx,feedID,10);if err!=nil||len(eps)!=1{t.Fatalf("%v %#v",err,eps)}
 id:=eps[0].ID
 queued,err:=d.EnqueuePodcastDownload(ctx,id);if err!=nil||queued.Status!="queued"{t.Fatalf("%v %#v",err,queued)}
 again,err:=d.EnqueuePodcastDownload(ctx,id);if err!=nil||again.ID!=queued.ID{t.Fatalf("duplicate enqueue: %v %+v",err,again)}
 owner:="worker-a"
 claimed,err:=d.ClaimPodcastDownload(ctx,owner,time.Now())
 if err!=nil||claimed!=id{t.Fatalf("claim: %q %v",claimed,err)}
 claimed,err=d.ClaimPodcastDownload(ctx,"worker-b",time.Now())
 if err!=nil||claimed!=""{t.Fatalf("double claim: %q %v",claimed,err)}
 if err:=d.UpdatePodcastDownloadBytes(ctx,id,owner,2,4);err!=nil{t.Fatal(err)}
 canceled,err:=d.CancelPodcastDownload(ctx,id);if err!=nil||canceled.Status!="canceled"{t.Fatalf("%v %#v",err,canceled)}
 if err=d.UpdatePodcastDownloadBytes(ctx,id,owner,4,4);!errors.Is(err,sql.ErrNoRows){t.Fatalf("cancel did not revoke lease: %v",err)}
 retry,err:=d.EnqueuePodcastDownload(ctx,id);if err!=nil||retry.Status!="queued"{t.Fatalf("retry: %v %+v",err,retry)}
 if err:=d.Close();err!=nil{t.Fatal(err)}
 d,err=New(path);if err!=nil{t.Fatal(err)}
 defer d.Close()
 persisted,err:=d.GetPodcastDownload(ctx,id);if err!=nil||persisted.Status!="queued"{t.Fatalf("restart: %v %+v",err,persisted)}
 claimed,err=d.ClaimPodcastDownload(ctx,"worker-c",time.Now());if err!=nil||claimed!=id{t.Fatalf("claim after restart: %v %q",err,claimed)}
 if err:=d.UpdatePodcastDownloadBytes(ctx,id,"worker-c",4,4);err!=nil{t.Fatal(err)}
 if err:=d.CompletePodcastDownload(ctx,id,"worker-a","file.audio","audio/mpeg","0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",4);!errors.Is(err,sql.ErrNoRows){t.Fatalf("stale owner completed: %v",err)}
 digest:="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
 if err:=d.CompletePodcastDownload(ctx,id,"worker-c",id+".audio","audio/mpeg",digest,4);err!=nil{t.Fatal(err)}
 file,err:=d.PodcastCompletedDownload(ctx,id);if err!=nil||file.Size!=4||file.MIME!="audio/mpeg"{t.Fatalf("%v %+v",err,file)}
 done,err:=d.EnqueuePodcastDownload(ctx,id);if err!=nil||done.Status!="completed"{t.Fatalf("completed download overwritten: %v %+v",err,done)}
 if _,err=d.CancelPodcastDownload(ctx,id);!errors.Is(err,ErrPodcastDownloadNotCancelable){t.Fatalf("cancel completed: %v",err)}
 if err:=d.UnsubscribePodcast(ctx,feedID);err!=nil{t.Fatal(err)}
 if _,err=d.GetPodcastDownload(ctx,id);!errors.Is(err,sql.ErrNoRows){t.Fatalf("unfollowed download exposed: %v",err)}
 if _,err=d.PodcastCompletedDownload(ctx,id);!errors.Is(err,sql.ErrNoRows){t.Fatalf("unfollowed file exposed: %v",err)}
}

func TestPodcastDownloadClaimExpiredLeaseAndInvalidEpisode(t *testing.T) {
 ctx:=context.Background()
 d,err:=New(filepath.Join(t.TempDir(),"download-empty.db"));if err!=nil{t.Fatal(err)}
 defer d.Close()
 if _,err=d.EnqueuePodcastDownload(ctx,"not-an-episode");!errors.Is(err,sql.ErrNoRows){t.Fatalf("unknown job queued: %v",err)}
 feedID,err:=d.SavePodcastFeedSnapshot(ctx,podcast.Feed{URL:"https://example.org/rss",Title:"Show",
  Episodes:[]podcast.Episode{{GUID:"x",Title:"X",EnclosureURL:"https://example.org/x.mp3",EnclosureType:"audio/mpeg"}}},true)
 if err!=nil{t.Fatal(err)}
 episodes,err:=d.ListPodcastEpisodes(ctx,feedID,10);if err!=nil{t.Fatal(err)}
 id:=episodes[0].ID
 if _,err=d.EnqueuePodcastDownload(ctx,id);err!=nil{t.Fatal(err)}
 then:=time.Now().Add(-5*time.Minute)
 got,err:=d.ClaimPodcastDownload(ctx,"crashed-worker",then)
 if err!=nil||got!=id{t.Fatalf("initial claim: %q %v",got,err)}
 got,err=d.ClaimPodcastDownload(ctx,"recovered-worker",time.Now())
 if err!=nil||got!=id{t.Fatalf("expired lease not recovered: %q %v",got,err)}
 if err=d.UpdatePodcastDownloadBytes(ctx,id,"crashed-worker",1,2);!errors.Is(err,sql.ErrNoRows){t.Fatalf("old owner changed recovered work: %v",err)}
}
