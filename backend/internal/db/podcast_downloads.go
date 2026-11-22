// Podcast download reservations are a separate, durable SQLite domain.
// No music download, catalog, or playlist tables are touched.
package db

import (
    "context"
    "database/sql"
    "errors"
    "time"
)

const podcastDownloadLease = 90 * time.Second
const MaxPodcastDownloadBytes int64 = 512 << 20 // bounded early-access offline MVP
// Includes completed and currently reserved work, even for unfollowed shows.
const MaxPodcastOfflineLibraryBytes int64 = 2 << 30

var (
    ErrPodcastDownloadNotCancelable = errors.New("podcast download cannot be canceled")
    ErrPodcastOfflineQuotaExceeded = errors.New("podcast offline storage quota exceeded")
)

type PodcastDownloadRecord struct {
    ID string `json:"id"`
    EpisodeID string `json:"episodeId"`
    FeedID string `json:"feedId,omitempty"`
    EpisodeTitle string `json:"episodeTitle,omitempty"`
    FeedTitle string `json:"feedTitle,omitempty"`
    Status string `json:"status"`
    BytesReceived int64 `json:"bytesReceived"`
    ExpectedBytes int64 `json:"expectedBytes"`
    VerifiedSize int64 `json:"verifiedSize"`
    ErrorCode string `json:"errorCode,omitempty"`
    CreatedAt int64 `json:"createdAt"`
    UpdatedAt int64 `json:"updatedAt"`
    CompletedAt int64 `json:"completedAt"`
}

type PodcastDownloadedFile struct {
    EpisodeID string
    Filename string
    MIME string
    Size int64
    SHA256 string
}

const podcastDownloadColumns = `
d.id,d.episode_id,d.status,d.bytes_received,
COALESCE(d.expected_bytes,0),COALESCE(d.verified_size,0),
COALESCE(d.error_code,''),d.created_at,d.updated_at,COALESCE(d.completed_at,0)`

func scanPodcastDownload(scanner interface{ Scan(...any) error }) (PodcastDownloadRecord,error) {
    var row PodcastDownloadRecord
    err := scanner.Scan(&row.ID,&row.EpisodeID,&row.Status,&row.BytesReceived,
        &row.ExpectedBytes,&row.VerifiedSize,&row.ErrorCode,&row.CreatedAt,
        &row.UpdatedAt,&row.CompletedAt)
    return row,err
}

// All public reads require an active subscription, even when historical
// downloaded files remain on disk after an unsubscribe.
func (d *DB) GetPodcastDownload(ctx context.Context, episodeID string) (PodcastDownloadRecord,error) {
    if err:=d.EnsurePodcastSchema();err!=nil{return PodcastDownloadRecord{},err}
    return scanPodcastDownload(d.conn.QueryRowContext(ctx,`
SELECT `+podcastDownloadColumns+` FROM podcast_downloads d
JOIN podcast_episodes e ON e.id=d.episode_id
JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
WHERE d.episode_id=?`,episodeID))
}

func (d *DB) ListPodcastDownloads(ctx context.Context) ([]PodcastDownloadRecord,error) {
    if err:=d.EnsurePodcastSchema();err!=nil{return nil,err}
    rows,err:=d.conn.QueryContext(ctx,`
SELECT `+podcastDownloadColumns+`,e.feed_id,e.title,f.title FROM podcast_downloads d
JOIN podcast_episodes e ON e.id=d.episode_id
JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
JOIN podcast_feeds f ON f.id=e.feed_id
ORDER BY d.updated_at DESC,d.episode_id LIMIT 100`)
    if err!=nil{return nil,err}
    defer rows.Close()
    result:=make([]PodcastDownloadRecord,0)
    for rows.Next(){
        var row PodcastDownloadRecord
        err:=rows.Scan(&row.ID,&row.EpisodeID,&row.Status,&row.BytesReceived,
            &row.ExpectedBytes,&row.VerifiedSize,&row.ErrorCode,&row.CreatedAt,
            &row.UpdatedAt,&row.CompletedAt,&row.FeedID,&row.EpisodeTitle,&row.FeedTitle)
        if err!=nil{return nil,err}
        result=append(result,row)
    }
    return result,rows.Err()
}

// Enqueue coalesces duplicates. Completed and currently in-flight downloads
// are never reset by another click. A failed/canceled task can be retried.
func (d *DB) EnqueuePodcastDownload(ctx context.Context,episodeID string)(PodcastDownloadRecord,error){
    if err:=d.EnsurePodcastSchema();err!=nil{return PodcastDownloadRecord{},err}
    now:=time.Now().UnixMilli()
    result,err:=d.conn.ExecContext(ctx,`
INSERT INTO podcast_downloads(id,episode_id,status,created_at,updated_at)
SELECT e.id,e.id,'queued',?,? FROM podcast_episodes e
JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
WHERE e.id=? AND e.enclosure_url<>''
ON CONFLICT(episode_id) DO UPDATE SET
 status=CASE WHEN podcast_downloads.status IN ('failed','canceled','paused') THEN 'queued' ELSE podcast_downloads.status END,
 error_code=CASE WHEN podcast_downloads.status IN ('failed','canceled','paused') THEN NULL ELSE podcast_downloads.error_code END,
 bytes_received=CASE WHEN podcast_downloads.status IN ('failed','canceled','paused') THEN 0 ELSE podcast_downloads.bytes_received END,
 expected_bytes=CASE WHEN podcast_downloads.status IN ('failed','canceled','paused') THEN NULL ELSE podcast_downloads.expected_bytes END,
 updated_at=CASE WHEN podcast_downloads.status IN ('failed','canceled','paused') THEN excluded.updated_at ELSE podcast_downloads.updated_at END
`,now,now,episodeID)
    if err!=nil{return PodcastDownloadRecord{},err}
    affected,err:=result.RowsAffected()
    if err!=nil{return PodcastDownloadRecord{},err}
    if affected==0{return PodcastDownloadRecord{},sql.ErrNoRows}
    return d.GetPodcastDownload(ctx,episodeID)
}

// A canceled worker loses its lease immediately. No new mutation can revive
// the old job; only an explicit later enqueue can do that.
func (d *DB) CancelPodcastDownload(ctx context.Context,episodeID string)(PodcastDownloadRecord,error){
    if err:=d.EnsurePodcastSchema();err!=nil{return PodcastDownloadRecord{},err}
    result,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_downloads SET status='canceled',lease_owner=NULL,lease_expires_at=NULL,
 bytes_received=0,expected_bytes=NULL,error_code=NULL,updated_at=?
WHERE episode_id=? AND status IN ('queued','downloading') AND EXISTS(
 SELECT 1 FROM podcast_episodes e JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
 WHERE e.id=podcast_downloads.episode_id)`,time.Now().UnixMilli(),episodeID)
    if err!=nil{return PodcastDownloadRecord{},err}
    n,err:=result.RowsAffected()
    if err!=nil{return PodcastDownloadRecord{},err}
    if n==0 {
        old,err:=d.GetPodcastDownload(ctx,episodeID)
        if err!=nil{return PodcastDownloadRecord{},err}
        if old.Status!="canceled"{return PodcastDownloadRecord{},ErrPodcastDownloadNotCancelable}
    }
    return d.GetPodcastDownload(ctx,episodeID)
}

// Claim exactly one job; an interrupted process can resume from zero after
// its lease expires. No partially validated files are reused.
func (d *DB) ClaimPodcastDownload(ctx context.Context,owner string,now time.Time)(string,error){
    if err:=d.EnsurePodcastSchema();err!=nil{return "",err}
    millis:=now.UnixMilli()
    var episodeID string
    err:=d.conn.QueryRowContext(ctx,`
UPDATE podcast_downloads SET status='downloading',lease_owner=?,
 lease_expires_at=?,bytes_received=0,expected_bytes=NULL,error_code=NULL,updated_at=?
WHERE episode_id=(
 SELECT d.episode_id FROM podcast_downloads d
 JOIN podcast_episodes e ON e.id=d.episode_id
 JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
 WHERE d.status='queued' OR (d.status='downloading' AND COALESCE(d.lease_expires_at,0)<?)
 ORDER BY d.created_at,d.episode_id LIMIT 1
)
RETURNING episode_id`,owner,millis+podcastDownloadLease.Milliseconds(),millis,millis).Scan(&episodeID)
    if errors.Is(err,sql.ErrNoRows){return "",nil}
    return episodeID,err
}

// PodcastOfflineStorage reports committed and in-flight reservations across
// ALL downloads, including unfollowed shows whose files still consume disk.
func (d *DB) PodcastOfflineStorage(ctx context.Context)(int64,error){
    if err:=d.EnsurePodcastSchema();err!=nil{return 0,err}
    var used int64
    err:=d.conn.QueryRowContext(ctx,`
SELECT COALESCE(SUM(CASE WHEN status='completed' THEN COALESCE(verified_size,0)
 ELSE COALESCE(expected_bytes,0) END),0)
FROM podcast_downloads WHERE status IN ('completed','downloading')`).Scan(&used)
    return used,err
}

func (d *DB) UpdatePodcastDownloadBytes(ctx context.Context,id,owner string,received,total int64) error {
    if received<0||total<=0||total>MaxPodcastDownloadBytes||received>total{return errors.New("invalid podcast byte count")}
    result,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_downloads SET bytes_received=?,expected_bytes=?,updated_at=?,
 lease_expires_at=?
WHERE episode_id=? AND status='downloading' AND lease_owner=?
AND EXISTS (SELECT 1 FROM podcast_episodes e JOIN podcast_subscriptions s
  ON s.feed_id=e.feed_id WHERE e.id=podcast_downloads.episode_id)
AND (SELECT COALESCE(SUM(CASE WHEN status='completed' THEN COALESCE(verified_size,0)
 ELSE COALESCE(expected_bytes,0) END),0)
 FROM podcast_downloads WHERE episode_id<>? AND status IN ('completed','downloading')) <= ?`,
received,total,time.Now().UnixMilli(),time.Now().Add(podcastDownloadLease).UnixMilli(),
id,owner,id,MaxPodcastOfflineLibraryBytes-total)
    if err!=nil{return err}
    n,err:=result.RowsAffected()
    if err!=nil{return err}
    if n==0 {
        var status,leaseOwner string
        err:=d.conn.QueryRowContext(ctx,`SELECT status,COALESCE(lease_owner,'')
            FROM podcast_downloads WHERE episode_id=?`,id).Scan(&status,&leaseOwner)
        if err==nil && status=="downloading" && leaseOwner==owner {
            return ErrPodcastOfflineQuotaExceeded
        }
        return sql.ErrNoRows
    }
    return nil
}

func (d *DB) CompletePodcastDownload(ctx context.Context,id,owner,filename,mime,digest string,size int64) error {
    if size<=0||size>MaxPodcastDownloadBytes||filename==""||mime==""||len(digest)!=64{return errors.New("invalid verified podcast file")}
    result,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_downloads SET status='completed',destination_relative_path=?,
 temp_relative_path=NULL,bytes_received=?,expected_bytes=?,verified_size=?,sha256=?,mime=?,
 completed_at=?,updated_at=?,lease_owner=NULL,lease_expires_at=NULL,error_code=NULL
WHERE episode_id=? AND status='downloading' AND lease_owner=?
AND EXISTS (SELECT 1 FROM podcast_episodes e JOIN podcast_subscriptions s
  ON s.feed_id=e.feed_id WHERE e.id=podcast_downloads.episode_id)
AND (SELECT COALESCE(SUM(CASE WHEN status='completed' THEN COALESCE(verified_size,0)
 ELSE COALESCE(expected_bytes,0) END),0)
 FROM podcast_downloads WHERE episode_id<>? AND status IN ('completed','downloading')) <= ?`,
filename,size,size,size,digest,mime,time.Now().UnixMilli(),time.Now().UnixMilli(),
id,owner,id,MaxPodcastOfflineLibraryBytes-size)
    if err!=nil{return err}
    n,err:=result.RowsAffected()
    if err!=nil{return err}
    if n==0{return sql.ErrNoRows}
    return nil
}

func (d *DB) FailPodcastDownload(ctx context.Context,id,owner,code string) error {
    if code==""||len(code)>60{return errors.New("invalid safe error code")}
    _,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_downloads SET status='failed',bytes_received=0,expected_bytes=NULL,
 lease_owner=NULL,lease_expires_at=NULL,error_code=?,updated_at=?
WHERE episode_id=? AND status='downloading' AND lease_owner=?`,
code,time.Now().UnixMilli(),id,owner)
    return err
}

// Completed files are selected only through active subscriptions. The
// filename itself is internal and must be revalidated by the file server.
func (d *DB) PodcastCompletedDownload(ctx context.Context,id string)(PodcastDownloadedFile,error){
    if err:=d.EnsurePodcastSchema();err!=nil{return PodcastDownloadedFile{},err}
    var f PodcastDownloadedFile
    err:=d.conn.QueryRowContext(ctx,`
SELECT d.episode_id,COALESCE(d.destination_relative_path,''),COALESCE(d.mime,''),
 COALESCE(d.verified_size,0),COALESCE(d.sha256,'')
FROM podcast_downloads d JOIN podcast_episodes e ON e.id=d.episode_id
JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
WHERE d.episode_id=? AND d.status='completed'`,id).Scan(&f.EpisodeID,&f.Filename,&f.MIME,&f.Size,&f.SHA256)
    return f,err
}

func (d *DB) InvalidatePodcastDownload(ctx context.Context,id string) error {
    if err:=d.EnsurePodcastSchema();err!=nil{return err}
    _,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_downloads SET status='failed',error_code='cached_media_missing',
 destination_relative_path=NULL,mime='',sha256=NULL,verified_size=NULL,updated_at=?
WHERE episode_id=? AND status='completed'`,time.Now().UnixMilli(),id)
    return err
}

// DeletePodcastCompletedDownload releases only a followed episode's completed
// metadata after the caller has safely removed its fixed-name local file.
// Listening history and publisher episode metadata remain untouched.
func (d *DB) DeletePodcastCompletedDownload(ctx context.Context,id string) error {
    if err:=d.EnsurePodcastSchema();err!=nil{return err}
    result,err:=d.conn.ExecContext(ctx,`
DELETE FROM podcast_downloads WHERE episode_id=? AND status='completed'
AND EXISTS (
  SELECT 1 FROM podcast_episodes e
  JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
  WHERE e.id=podcast_downloads.episode_id)`,id)
    if err!=nil{return err}
    n,err:=result.RowsAffected()
    if err!=nil{return err}
    if n==0{return sql.ErrNoRows}
    return nil
}

// A bounded inventory of completed offline files no longer covered by any
// subscription. Returned names are backend-only and MUST be revalidated by
// the filesystem cleanup routine before unlinking.
func (d *DB) ListUnsubscribedPodcastCompletedFiles(ctx context.Context,limit int)([]PodcastDownloadedFile,error){
    if err:=d.EnsurePodcastSchema();err!=nil{return nil,err}
    if limit<1||limit>32{limit=32}
    rows,err:=d.conn.QueryContext(ctx,`
SELECT d.episode_id,COALESCE(d.destination_relative_path,''),
 COALESCE(d.mime,''),COALESCE(d.verified_size,0),COALESCE(d.sha256,'')
FROM podcast_downloads d JOIN podcast_episodes e ON e.id=d.episode_id
WHERE d.status='completed' AND NOT EXISTS(
 SELECT 1 FROM podcast_subscriptions s WHERE s.feed_id=e.feed_id)
ORDER BY d.updated_at,d.episode_id LIMIT ?`,limit)
    if err!=nil{return nil,err}
    defer rows.Close()
    out:=make([]PodcastDownloadedFile,0)
    for rows.Next(){
        var f PodcastDownloadedFile
        if err:=rows.Scan(&f.EpisodeID,&f.Filename,&f.MIME,&f.Size,&f.SHA256);err!=nil{return nil,err}
        out=append(out,f)
    }
    return out,rows.Err()
}

// Clear a reclaimed record only if the episode has not been re-followed.
// This preserves playback and episode records, and releases quota space.
func (d *DB) DeleteUnsubscribedPodcastCompletedFile(ctx context.Context,id string)error{
    if err:=d.EnsurePodcastSchema();err!=nil{return err}
    res,err:=d.conn.ExecContext(ctx,`
DELETE FROM podcast_downloads WHERE episode_id=? AND status='completed'
AND EXISTS (
 SELECT 1 FROM podcast_episodes e WHERE e.id=podcast_downloads.episode_id
 AND NOT EXISTS(SELECT 1 FROM podcast_subscriptions s WHERE s.feed_id=e.feed_id))`,id)
    if err!=nil{return err}
    n,err:=res.RowsAffected()
    if err!=nil{return err}
    if n==0{return sql.ErrNoRows}
    return nil
}
