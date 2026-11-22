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

var ErrPodcastDownloadNotCancelable = errors.New("podcast download cannot be canceled")

type PodcastDownloadRecord struct {
    ID string `json:"id"`
    EpisodeID string `json:"episodeId"`
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
SELECT `+podcastDownloadColumns+` FROM podcast_downloads d
JOIN podcast_episodes e ON e.id=d.episode_id
JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
ORDER BY d.updated_at DESC,d.episode_id LIMIT 100`)
    if err!=nil{return nil,err}
    defer rows.Close()
    result:=make([]PodcastDownloadRecord,0)
    for rows.Next(){
        row,err:=scanPodcastDownload(rows)
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

func (d *DB) UpdatePodcastDownloadBytes(ctx context.Context,id,owner string,received,total int64) error {
    if received<0||total<=0||total>MaxPodcastDownloadBytes||received>total{return errors.New("invalid podcast byte count")}
    result,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_downloads SET bytes_received=?,expected_bytes=?,updated_at=?,
 lease_expires_at=?
WHERE episode_id=? AND status='downloading' AND lease_owner=?`,
received,total,time.Now().UnixMilli(),time.Now().Add(podcastDownloadLease).UnixMilli(),id,owner)
    if err!=nil{return err}
    n,err:=result.RowsAffected()
    if err!=nil{return err}
    if n==0{return sql.ErrNoRows}
    return nil
}

func (d *DB) CompletePodcastDownload(ctx context.Context,id,owner,filename,mime,digest string,size int64) error {
    if size<=0||size>MaxPodcastDownloadBytes||filename==""||mime==""||len(digest)!=64{return errors.New("invalid verified podcast file")}
    result,err:=d.conn.ExecContext(ctx,`
UPDATE podcast_downloads SET status='completed',destination_relative_path=?,
 temp_relative_path=NULL,bytes_received=?,expected_bytes=?,verified_size=?,sha256=?,mime=?,
 completed_at=?,updated_at=?,lease_owner=NULL,lease_expires_at=NULL,error_code=NULL
WHERE episode_id=? AND status='downloading' AND lease_owner=?`,
filename,size,size,size,digest,mime,time.Now().UnixMilli(),time.Now().UnixMilli(),id,owner)
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
 destination_relative_path=NULL,mime=NULL,sha256=NULL,verified_size=NULL,updated_at=?
WHERE episode_id=? AND status='completed'`,time.Now().UnixMilli(),id)
    return err
}
