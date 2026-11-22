package db

import (
    "context"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

// PodcastMediaSource resolves publisher enclosure metadata only for an
// actively followed show. This protects the media endpoint from becoming a
// general-purpose arbitrary-URL network proxy.
func (d *DB) PodcastMediaSource(ctx context.Context,episodeID string) (string,string,error){
    if err:=d.EnsurePodcastSchema();err!=nil{return "","",err}
    var raw,declared string
    err:=d.conn.QueryRowContext(ctx,`
SELECT e.enclosure_url,e.enclosure_mime
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id=e.feed_id
WHERE e.id=?`,episodeID).Scan(&raw,&declared)
    if err!=nil{return "","",err}
    if _,err=podcast.ValidateMediaURL(raw,false);err!=nil{return "","",err}
    // Publisher MIME may be missing in older RSS. An explicit non-audio
    // declared type is not playable even if a remote server says otherwise.
    if declared!="" && podcast.SupportedPodcastAudioMIME(declared)==""{
        return "","",podcast.ErrInvalidMedia
    }
    return raw,declared,nil
}
