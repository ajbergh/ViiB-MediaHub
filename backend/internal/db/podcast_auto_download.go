// podcast_auto_download.go stores explicit, conservative per-show automation.
// The existing podcast-only subscription row owns the setting; no music,
// Spotify or user playback-state tables participate.
package db

import (
    "context"
    "database/sql"
    "errors"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

const podcastMaxAutoDownloadPerRefresh = 3
const podcastMaxAutoDownloadPendingGlobal = 12
const podcastMaxAutoDownloadPendingPerShow = 3

type PodcastAutoDownloadPolicy struct {
    Enabled bool `json:"enabled"`
    NewestCount int `json:"newestCount"`
}

// Only active subscriptions may inspect/configure automatic downloading.
// Enabling is never implicit and previously published episodes are excluded.
func (d *DB) GetPodcastAutoDownloadPolicy(ctx context.Context,feedID string)(PodcastAutoDownloadPolicy,error){
    var out PodcastAutoDownloadPolicy
    if err:=d.EnsurePodcastSchema();err!=nil{return out,err}
    var policy string
    err:=d.conn.QueryRowContext(ctx,`
SELECT auto_download_policy,auto_download_newest_count
FROM podcast_subscriptions WHERE feed_id=?`,feedID).Scan(&policy,&out.NewestCount)
    if err!=nil{return out,err}
    out.Enabled=policy=="new"
    if out.NewestCount<1||out.NewestCount>podcastMaxAutoDownloadPerRefresh {
        out.NewestCount=1
    }
    return out,nil
}

// Every PUT re-arms the policy from now; it cannot enqueue historical audio.
// Setting the policy off does not delete the user's manual/existing downloads.
func (d *DB) UpdatePodcastAutoDownloadPolicy(ctx context.Context,feedID string,
    policy PodcastAutoDownloadPolicy)(PodcastAutoDownloadPolicy,error){
    if policy.NewestCount<1||policy.NewestCount>podcastMaxAutoDownloadPerRefresh{
        return PodcastAutoDownloadPolicy{},errors.New("invalid podcast auto-download count")
    }
    if err:=d.EnsurePodcastSchema();err!=nil{return PodcastAutoDownloadPolicy{},err}
    mode:="off"
    if policy.Enabled{mode="new"}
    result,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_subscriptions SET auto_download_policy=?,auto_download_newest_count=?,
 auto_download_armed_at=? WHERE feed_id=?`,
 mode,policy.NewestCount,time.Now().UnixMilli(),feedID)
    if err!=nil{return PodcastAutoDownloadPolicy{},err}
    n,err:=result.RowsAffected()
    if err!=nil{return PodcastAutoDownloadPolicy{},err}
    if n==0{return PodcastAutoDownloadPolicy{},sql.ErrNoRows}
    return policy,nil
}

// Called *inside* the successful RSS refresh transaction, after episode
// reconciliation. This queues only brand-new episodes from THIS refresh,
// never the already cached backlog or an HTTP 304 response.
//
// Auto-enqueue never fetches audio itself. The existing serial downloader
// enforces episode/library quotas, disk headroom, leased jobs, publisher DNS
// pinning, content checks and local-player reuse. Global/per-show queue limits
// prevent a flood of work after publisher/connection catch-up.
func enqueueNewPodcastAutoDownloads(ctx context.Context,tx *sql.Tx,feedID string,nowMillis int64)error{
    var mode string
    var newestCount int
    var armedAt int64
    err:=tx.QueryRowContext(ctx,`SELECT auto_download_policy,auto_download_newest_count,
auto_download_armed_at FROM podcast_subscriptions WHERE feed_id=?`,feedID).
        Scan(&mode,&newestCount,&armedAt)
    if err!=nil{return err}
    if mode!="new"||newestCount<1||armedAt>=nowMillis{return nil}
    if newestCount>podcastMaxAutoDownloadPerRefresh{newestCount=podcastMaxAutoDownloadPerRefresh}
    var globalPending,showPending int
    err=tx.QueryRowContext(ctx,`
SELECT COUNT(*) FROM podcast_downloads WHERE status IN ('queued','downloading','paused')`).Scan(&globalPending)
    if err!=nil{return err}
    err=tx.QueryRowContext(ctx,`
SELECT COUNT(*) FROM podcast_downloads d JOIN podcast_episodes e ON e.id=d.episode_id
WHERE e.feed_id=? AND d.status IN ('queued','downloading','paused')`,feedID).Scan(&showPending)
    if err!=nil{return err}
    capacity:=newestCount
    if n:=podcastMaxAutoDownloadPendingGlobal-globalPending;n<capacity{capacity=n}
    if n:=podcastMaxAutoDownloadPendingPerShow-showPending;n<capacity{capacity=n}
    if capacity<=0{return nil}

    // Read only 12 candidates from this particular successful refresh so
    // unsupported enclosures cannot displace all eligible newer audio.
    rows,err:=tx.QueryContext(ctx,`
SELECT e.id,e.enclosure_url,e.enclosure_mime
FROM podcast_episodes e
LEFT JOIN podcast_episode_state s ON s.episode_id=e.id
WHERE e.feed_id=? AND e.created_at=? AND e.created_at>?
  AND e.enclosure_url<>'' AND COALESCE(s.is_played,0)=0
ORDER BY COALESCE(e.published_at,0) DESC,e.id ASC LIMIT 12`,
 feedID,nowMillis,armedAt)
    if err!=nil{return err}
    type candidate struct{id,url,mime string}
    candidates:=make([]candidate,0,12)
    for rows.Next(){
        var c candidate
        if err=rows.Scan(&c.id,&c.url,&c.mime);err!=nil{break}
        candidates=append(candidates,c)
    }
    if err==nil{err=rows.Err()}
    rows.Close()
    if err!=nil{return err}
    for _,c:=range candidates{
        if capacity==0{break}
        if _,err=podcast.ValidateMediaURL(c.url,true);err!=nil{continue}
        if c.mime!=""&&podcast.SupportedPodcastAudioMIME(c.mime)==""{continue}
        result,insertErr:=tx.ExecContext(ctx,`
INSERT INTO podcast_downloads(id,episode_id,status,created_at,updated_at)
VALUES (?,?,'queued',?,?) ON CONFLICT(episode_id) DO NOTHING`,
 c.id,c.id,nowMillis,nowMillis)
        if insertErr!=nil{return insertErr}
        n,insertErr:=result.RowsAffected()
        if insertErr!=nil{return insertErr}
        if n>0{capacity--}
    }
    return nil
}
