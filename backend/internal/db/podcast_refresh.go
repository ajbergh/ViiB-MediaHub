// podcast_refresh.go implements a crash-recoverable, low-impact RSS refresh
// queue independent of the music-analysis scheduler and podcast downloads.
package db

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "strings"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

const podcastRefreshLease = 3 * time.Minute

type PodcastRefreshTarget struct {
    ID string
    URL string
    ETag string
    LastModified string
    Failures int
}

type PodcastRefreshStatus struct {
    LastCheckedAt int64 `json:"lastCheckedAt"`
    LastSuccessAt int64 `json:"lastSuccessAt"`
    NextRefreshAt int64 `json:"nextRefreshAt"`
    LastErrorCode string `json:"lastErrorCode,omitempty"`
    Failures int `json:"failures"`
    Refreshing bool `json:"refreshing"`
}

func (d *DB) PodcastRefreshStatus(ctx context.Context, feedID string) (PodcastRefreshStatus, error) {
    var status PodcastRefreshStatus
    if err:=d.EnsurePodcastSchema(); err!=nil{return status,err}
    var checked, success, next, lease sql.NullInt64
    var code sql.NullString
    err:=d.conn.QueryRowContext(ctx,`
SELECT last_checked_at,last_success_at,next_refresh_at,last_error_code,refresh_failures,refresh_lease_until
FROM podcast_feeds WHERE id=? AND EXISTS
 (SELECT 1 FROM podcast_subscriptions s WHERE s.feed_id=podcast_feeds.id)
`,feedID).Scan(&checked,&success,&next,&code,&status.Failures,&lease)
    if err!=nil{return status,err}
    status.LastCheckedAt=checked.Int64
    status.LastSuccessAt=success.Int64
    status.NextRefreshAt=next.Int64
    status.LastErrorCode=code.String
    status.Refreshing=lease.Int64>time.Now().UnixMilli()
    return status,nil
}

// SchedulePodcastRefresh makes a followed feed immediately eligible, without
// changing a valid in-flight lease or discarding ETag/Last-Modified validators.
// Repeated calls coalesce into the existing pending work.
func (d *DB) SchedulePodcastRefresh(ctx context.Context,feedID string) error {
    if err:=d.EnsurePodcastSchema();err!=nil{return err}
    result,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_feeds SET next_refresh_at=0
WHERE id=? AND EXISTS(SELECT 1 FROM podcast_subscriptions s WHERE s.feed_id=podcast_feeds.id)
`,feedID)
    if err!=nil{return err}
    count,err:=result.RowsAffected()
    if err!=nil{return err}
    if count==0{return sql.ErrNoRows}
    return nil
}

// ClaimDuePodcastRefresh leases exactly one eligible subscribed feed atomically.
// The durable lease means a restart recovers work after the deadline. It does
// not compete for the generic analysis scheduler's scarce workers.
func (d *DB) ClaimDuePodcastRefresh(ctx context.Context,now time.Time) (*PodcastRefreshTarget,error) {
    if err:=d.EnsurePodcastSchema();err!=nil{return nil,err}
    millis:=now.UnixMilli()
    target:=new(PodcastRefreshTarget)
    var etag,modified sql.NullString
    err:=d.conn.QueryRowContext(ctx,`
UPDATE podcast_feeds SET refresh_lease_until=?
WHERE id = (
    SELECT f.id FROM podcast_feeds f
    JOIN podcast_subscriptions s ON s.feed_id=f.id
    WHERE COALESCE(f.next_refresh_at,0)<=?
      AND COALESCE(f.refresh_lease_until,0)<=?
    ORDER BY COALESCE(f.next_refresh_at,0),f.id LIMIT 1
)
RETURNING id,canonical_url,etag,last_modified,refresh_failures
`,millis+podcastRefreshLease.Milliseconds(),millis,millis).
    Scan(&target.ID,&target.URL,&etag,&modified,&target.Failures)
    if errors.Is(err,sql.ErrNoRows){return nil,nil}
    if err!=nil{return nil,err}
    target.ETag=etag.String
    target.LastModified=modified.String
    return target,nil
}

func nextPodcastRefreshAt(id string,now time.Time) int64 {
    // Deterministic 0-10 minute per-feed spread to prevent bursts after restart.
    var jitter int64
    for _,ch:=range id{
        jitter=(jitter*31+int64(ch))%600
    }
    return now.Add(2*time.Hour+time.Duration(jitter)*time.Second).UnixMilli()
}

func nextPodcastRetryAt(now time.Time, failures int) int64 {
    delay:=10*time.Minute
    for n:=1;n<failures && n<7;n++ {delay*=2}
    if delay>6*time.Hour {delay=6*time.Hour}
    return now.Add(delay).UnixMilli()
}

// FinishPodcastRefresh atomically applies a 200 snapshot or a 304 unchanged
// response; neither outcome erases playback state or cached historical rows.
// The feed ID stays stable even when the upstream RSS URL redirected.
func (d *DB) FinishPodcastRefresh(ctx context.Context,target PodcastRefreshTarget,result podcast.FetchResult,now time.Time) error {
    if err:=d.EnsurePodcastSchema();err!=nil{return err}
    tx,err:=d.conn.BeginTx(ctx,nil)
    if err!=nil{return err}
    defer tx.Rollback()
    var exists int
    err=tx.QueryRowContext(ctx,`SELECT 1 FROM podcast_subscriptions WHERE feed_id=?`,target.ID).Scan(&exists)
    if err!=nil{return err}
    millis:=now.UnixMilli()
    if !result.NotModified{
        if strings.TrimSpace(result.Feed.Title)=="" {return podcast.ErrInvalidFeed}
        // The public network fetcher validates every redirect hop. Move
        // canonical URL atomically while preserving the old URL as an alias,
        // so a publisher migration does not create a second subscription.
        // A destination already owned by another show is a genuine identity
        // conflict; roll back rather than merging history heuristically.
        newCanonical:=target.URL
        if result.Feed.URL!="" {
            normalized,validateErr:=podcast.ValidatePublicURL(result.Feed.URL,false)
            if validateErr!=nil{return validateErr}
            newCanonical=normalized.String()
        }
        var previousCanonical string
        err=tx.QueryRowContext(ctx,`SELECT canonical_url FROM podcast_feeds WHERE id=?`,target.ID).Scan(&previousCanonical)
        if err!=nil{return err}
        if newCanonical!=previousCanonical {
            var destinationOwner string
            lookupErr:=tx.QueryRowContext(ctx,`SELECT id FROM podcast_feeds WHERE canonical_url=?`,newCanonical).Scan(&destinationOwner)
            if lookupErr==nil && destinationOwner!=target.ID {
                return fmt.Errorf("podcast redirect identity conflict")
            }
            if lookupErr!=nil && !errors.Is(lookupErr,sql.ErrNoRows){return lookupErr}
            _,err=tx.ExecContext(ctx,`UPDATE podcast_feeds SET canonical_url=? WHERE id=?`,newCanonical,target.ID)
            if err!=nil{return err}
            _,err=tx.ExecContext(ctx,`
INSERT INTO podcast_feed_aliases(original_url,feed_id) VALUES (?,?)
ON CONFLICT(original_url) DO NOTHING
`,previousCanonical,target.ID)
            if err!=nil{return err}
        }
        _,err=tx.ExecContext(ctx,`
UPDATE podcast_feeds SET
 title=?,author=?,description=?,image_url=?,language=?
WHERE id=?`,result.Feed.Title,result.Feed.Author,result.Feed.Description,
            podcast.SafeArtworkURL(result.Feed.ImageURL),result.Feed.Language,target.ID)
        if err!=nil{return err}
        // Preserve history rows rather than deleting truncated back catalogs.
        if _,err=tx.ExecContext(ctx,`UPDATE podcast_episodes SET listed_in_feed=0 WHERE feed_id=?`,target.ID);err!=nil{return err}
        if err=upsertPodcastEpisodes(ctx,tx,target.ID,result.Feed.Episodes,millis);err!=nil{return err}
    }
    etag:=target.ETag
    modified:=target.LastModified
    if result.ETag!=""{etag=result.ETag}
    if result.LastModified!=""{modified=result.LastModified}
    _,err=tx.ExecContext(ctx,`
UPDATE podcast_feeds SET etag=?,last_modified=?,last_checked_at=?,
 last_success_at=?,last_error_code=NULL,refresh_failures=0,
 next_refresh_at=?,refresh_lease_until=0,updated_at=?
WHERE id=?`,etag,modified,millis,millis,nextPodcastRefreshAt(target.ID,now),millis,target.ID)
    if err!=nil{return err}
    return tx.Commit()
}

func (d *DB) FailPodcastRefresh(ctx context.Context,id,code string,now time.Time) error {
    if err:=d.EnsurePodcastSchema();err!=nil{return err}
    if code!="feed_invalid" && code!="blocked_remote_target" {code="feed_unavailable"}
    tx,err:=d.conn.BeginTx(ctx,nil)
    if err!=nil{return err}
    defer tx.Rollback()
    var failures int
    err=tx.QueryRowContext(ctx,`SELECT refresh_failures FROM podcast_feeds WHERE id=?`,id).Scan(&failures)
    if err!=nil{return err}
    failures++
    _,err=tx.ExecContext(ctx,`
UPDATE podcast_feeds SET
 last_checked_at=?,last_error_code=?,refresh_failures=?,
 refresh_lease_until=0,next_refresh_at=?,updated_at=?
WHERE id=?`,now.UnixMilli(),code,failures,nextPodcastRetryAt(now,failures),now.UnixMilli(),id)
    if err!=nil{return err}
    return tx.Commit()
}


// RecordInitialPodcastFetch saves validators from the already-successful
// subscribe fetch and avoids an unnecessary immediate second network request.
// An explicitly queued manual refresh (next_refresh_at=0) is not overridden.
func (d *DB) RecordInitialPodcastFetch(ctx context.Context,id,etag,modified string) error {
    if err:=d.EnsurePodcastSchema();err!=nil{return err}
    now:=time.Now()
    _,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_feeds
SET etag=?,last_modified=?,
 next_refresh_at=CASE WHEN next_refresh_at IS NULL THEN ? ELSE next_refresh_at END
WHERE id=?`,etag,modified,nextPodcastRefreshAt(id,now),id)
    return err
}
