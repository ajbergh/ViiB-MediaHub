package db

import "context"

// PodcastContinueEpisodeRecord contains only followed podcast episodes with
// saved, uncompleted progress. Never scan the music/library song domain.
type PodcastContinueEpisodeRecord struct {
    PodcastEpisodeRecord
    FeedTitle string `json:"feedTitle"`
}

// ListContinueListening uses bounded persisted state; it remains useful
// after restarts and never requests network resources.
func (d *DB) ListContinueListening(ctx context.Context,limit int) ([]PodcastContinueEpisodeRecord,error) {
    if err:=d.EnsurePodcastSchema();err!=nil{return nil,err}
    if limit<1||limit>50{limit=20}
    rows,err:=d.conn.QueryContext(ctx,`
SELECT e.id,e.feed_id,e.guid,e.title,e.description,e.enclosure_url,e.enclosure_mime,
 COALESCE(e.published_at,0),e.duration_ms,s.position_ms,s.listened_ms,s.revision,f.title
FROM podcast_episode_state s
JOIN podcast_episodes e ON e.id=s.episode_id
JOIN podcast_subscriptions sub ON sub.feed_id=e.feed_id
JOIN podcast_feeds f ON f.id=e.feed_id
WHERE s.position_ms>0 AND s.is_played=0
ORDER BY COALESCE(s.last_played_at,0) DESC,e.id ASC LIMIT ?
`,limit)
    if err!=nil{return nil,err}
    defer rows.Close()
    result:=make([]PodcastContinueEpisodeRecord,0,limit)
    for rows.Next(){
        var item PodcastContinueEpisodeRecord
        e:=&item.PodcastEpisodeRecord
        if err:=rows.Scan(&e.ID,&e.FeedID,&e.GUID,&e.Title,&e.Description,
            &e.EnclosureURL,&e.EnclosureType,&e.PublishedAt,&e.DurationMs,
            &e.PositionMs,&e.ListenedMs,&e.ProgressRevision,&item.FeedTitle);err!=nil{
            return nil,err
        }
        result=append(result,item)
    }
    return result,rows.Err()
}
