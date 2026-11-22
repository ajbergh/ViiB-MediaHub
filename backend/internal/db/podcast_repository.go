// podcast_repository.go persists publisher feed snapshots and subscribed
// episodes without mutating any music/Spotify objects or user listening state.
package db

import (
    "context"
    "crypto/sha256"
    "database/sql"
    "encoding/hex"
    "errors"
    "fmt"
    "strings"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

type PodcastFeedRecord struct {
    ID string `json:"id"`
    URL string `json:"url"`
    Title string `json:"title"`
    Author string `json:"author"`
    Description string `json:"description"`
    ImageURL string `json:"imageUrl"`
    Language string `json:"language"`
    EpisodeCount int64 `json:"episodeCount"`
}

type PodcastEpisodeRecord struct {
    ID string `json:"id"`
    FeedID string `json:"feedId"`
    GUID string `json:"guid"`
    Title string `json:"title"`
    Description string `json:"description"`
    EnclosureURL string `json:"enclosureUrl"`
    EnclosureType string `json:"enclosureType"`
    PublishedAt int64 `json:"publishedAt"`
    DurationMs int64 `json:"durationMs"`
}

func podcastStableID(parts ...string) string {
    sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
    return hex.EncodeToString(sum[:20])
}

// SavePodcastFeedSnapshot atomically reconciles RSS metadata; idempotent
// upserts deliberately do NOT clear user progress, bookmarks or downloaded
// files. Subscription is optional so preview/import use cases can differ.
// SavePodcastFeedSnapshot preserves the existing API for locally supplied
// snapshots. Network subscriptions should use SavePodcastSubscriptionFromFetch
// to retain redirect aliases and HTTP validators in the same transaction.
func (d *DB) SavePodcastFeedSnapshot(ctx context.Context, feed podcast.Feed, subscribe bool) (string, error) {
    return d.savePodcastFeed(ctx, feed.URL, feed, subscribe, "", "", false)
}

// SavePodcastSubscriptionFromFetch retains the original requested URL as an
// alias of the final public redirect destination. If the publisher redirects
// again later, a subscribed show's stable identity and episode progress are
// retained instead of producing a duplicate show.
func (d *DB) SavePodcastSubscriptionFromFetch(ctx context.Context, requestedURL string, fetched podcast.FetchResult) (string, error) {
    if fetched.NotModified { return "", podcast.ErrInvalidFeed }
    return d.savePodcastFeed(ctx, requestedURL, fetched.Feed, true, fetched.ETag, fetched.LastModified, true)
}

func (d *DB) savePodcastFeed(ctx context.Context, requestedURL string, feed podcast.Feed, subscribe bool, etag, lastModified string, schedule bool) (string, error) {
    if err := d.EnsurePodcastSchema(); err != nil { return "", err }
    source, err := podcast.ValidatePublicURL(requestedURL, false)
    if err != nil { return "", err }
    target, err := podcast.ValidatePublicURL(feed.URL, false)
    if err != nil { return "", err }
    requestedURL = source.String()
    feed.URL = target.String()
    if strings.TrimSpace(feed.Title) == "" { return "", podcast.ErrInvalidFeed }

    now := time.Now().UnixMilli()
    tx, err := d.conn.BeginTx(ctx, nil)
    if err != nil { return "", err }
    defer tx.Rollback()

    // Prefer the current canonical destination, then the original user's
    // alias, then historical canonical/alias entries. This also deduplicates
    // different subscription URLs that resolve to the same public feed.
    choices := []struct{ query, value string }{
        {`SELECT id,canonical_url FROM podcast_feeds WHERE canonical_url=?`, feed.URL},
        {`SELECT f.id,f.canonical_url FROM podcast_feed_aliases a JOIN podcast_feeds f ON f.id=a.feed_id WHERE a.original_url=?`, requestedURL},
        {`SELECT id,canonical_url FROM podcast_feeds WHERE canonical_url=?`, requestedURL},
        {`SELECT f.id,f.canonical_url FROM podcast_feed_aliases a JOIN podcast_feeds f ON f.id=a.feed_id WHERE a.original_url=?`, feed.URL},
    }
    var feedID,oldCanonical string
    for _, choice := range choices {
        err=tx.QueryRowContext(ctx,choice.query,choice.value).Scan(&feedID,&oldCanonical)
        if err==nil{break}
        if !errors.Is(err,sql.ErrNoRows){return "",fmt.Errorf("resolve podcast alias: %w",err)}
        feedID=""
    }
    if feedID==""{ feedID=podcastStableID(feed.URL) }

    _,err=tx.ExecContext(ctx,`
INSERT INTO podcast_feeds (
 id,canonical_url,title,author,description,image_url,language,
 created_at,updated_at,last_checked_at,last_success_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
 canonical_url=excluded.canonical_url,title=excluded.title,
 author=excluded.author,description=excluded.description,
 image_url=excluded.image_url,language=excluded.language,
 last_checked_at=excluded.last_checked_at,
 last_success_at=excluded.last_success_at,updated_at=excluded.updated_at
`,feedID,feed.URL,feed.Title,feed.Author,feed.Description,feed.ImageURL,
        feed.Language,now,now,now,now)
    if err != nil { return "", fmt.Errorf("save podcast feed: %w",err) }

    // Persist both the URL the user entered and the previous redirect target
    // when a publisher changes addresses. Never store unvalidated private
    // feed URLs or authentication query parameters in the alias table.
    aliases:=[]string{requestedURL}
    if oldCanonical!="" && oldCanonical!=feed.URL {
        aliases=append(aliases,oldCanonical)
    }
    for _, alias:=range aliases {
        if alias==feed.URL{continue}
        if _,err=podcast.ValidatePublicURL(alias,false);err!=nil{return "",err}
        _,err=tx.ExecContext(ctx,`
INSERT INTO podcast_feed_aliases(original_url,feed_id) VALUES (?,?)
ON CONFLICT(original_url) DO UPDATE SET feed_id=excluded.feed_id
`,alias,feedID)
        if err!=nil{return "",fmt.Errorf("save podcast redirect alias: %w",err)}
    }

    if subscribe {
        _, err = tx.ExecContext(ctx,`
INSERT INTO podcast_subscriptions(feed_id,subscribed_at) VALUES (?,?)
ON CONFLICT(feed_id) DO NOTHING`,feedID,now)
        if err != nil { return "", fmt.Errorf("subscribe podcast feed: %w",err) }
    }

    for _, episode := range feed.Episodes {
        if strings.TrimSpace(episode.Title) == "" { continue }
        key := strings.TrimSpace(episode.GUID)
        if key != "" {
            key = "guid:" + key
        } else if strings.TrimSpace(episode.EnclosureURL) != "" {
            key = "enclosure:" + strings.TrimSpace(episode.EnclosureURL)
        } else {
            // A feed item without a publisher identity is not safely durable.
            continue
        }
        episodeID := podcastStableID(feedID,key)
        var published any
        if !episode.PublishedAt.IsZero() { published=episode.PublishedAt.UnixMilli() }
        _, err = tx.ExecContext(ctx,`
INSERT INTO podcast_episodes (
 id,feed_id,identity_key,guid,title,description,enclosure_url,
 enclosure_mime,published_at,duration_ms,created_at,updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(feed_id,identity_key) DO UPDATE SET
 title=excluded.title,description=excluded.description,
 enclosure_url=excluded.enclosure_url,enclosure_mime=excluded.enclosure_mime,
 published_at=excluded.published_at,duration_ms=excluded.duration_ms,
 listed_in_feed=1,updated_at=excluded.updated_at
`,episodeID,feedID,key,episode.GUID,episode.Title,episode.Description,
            episode.EnclosureURL,episode.EnclosureType,published,
            episode.DurationSeconds*1000,now,now)
        if err != nil { return "", fmt.Errorf("save podcast episode: %w",err) }
    }

    if schedule {
        // Never overwrite a queued manual refresh (next_refresh_at=0).
        // Retain validators on repeats until this successful fetch supersedes
        // them. The requested and redirected URL updates happen atomically.
        _,err=tx.ExecContext(ctx,`
UPDATE podcast_feeds SET etag=?,last_modified=?,
 next_refresh_at=CASE WHEN next_refresh_at IS NULL THEN ? ELSE next_refresh_at END
WHERE id=?`,etag,lastModified,nextPodcastRefreshAt(feedID,time.Now()),feedID)
        if err!=nil{return "",fmt.Errorf("schedule podcast feed: %w",err)}
    }
    if err := tx.Commit(); err != nil { return "", err }
    return feedID,nil
}

func (d *DB) ListSubscribedPodcasts(ctx context.Context) ([]PodcastFeedRecord,error) {
    if err:=d.EnsurePodcastSchema();err!=nil{return nil,err}
    rows,err:=d.conn.QueryContext(ctx,`
SELECT f.id,f.canonical_url,f.title,f.author,f.description,f.image_url,f.language,
 (SELECT COUNT(*) FROM podcast_episodes e WHERE e.feed_id=f.id)
FROM podcast_subscriptions s JOIN podcast_feeds f ON f.id=s.feed_id
ORDER BY s.subscribed_at DESC,f.id`)
    if err!=nil{return nil,err}
    defer rows.Close()
    result:=make([]PodcastFeedRecord,0)
    for rows.Next(){
        var record PodcastFeedRecord
        if err:=rows.Scan(&record.ID,&record.URL,&record.Title,&record.Author,
            &record.Description,&record.ImageURL,&record.Language,&record.EpisodeCount);err!=nil{
            return nil,err
        }
        result=append(result,record)
    }
    return result,rows.Err()
}

// ListPodcastEpisodes keeps legacy in-process callers working while page
// requests expose a stable, bounded response shape to the public API.
func (d *DB) ListPodcastEpisodes(ctx context.Context,feedID string,limit int) ([]PodcastEpisodeRecord,error) {
    episodes,_,err:=d.ListPodcastEpisodesPage(ctx,feedID,limit,0)
    return episodes,err
}

// ListPodcastEpisodesPage reads at most limit+1 rows to detect additional
// episodes without a second COUNT query. Sort order is explicit and stable
// for unchanged publisher snapshots. Callers should reset pagination after
// refresh because newly published episodes can shift offsets.
func (d *DB) ListPodcastEpisodesPage(ctx context.Context,feedID string,limit,offset int) ([]PodcastEpisodeRecord,bool,error) {
    if err:=d.EnsurePodcastSchema();err!=nil{return nil,false,err}
    if limit<=0 || limit>100 {limit=50}
    if offset<0 {offset=0}
    rows,err:=d.conn.QueryContext(ctx,`
SELECT id,feed_id,guid,title,description,enclosure_url,enclosure_mime,
 COALESCE(published_at,0),duration_ms
FROM podcast_episodes WHERE feed_id=?
ORDER BY COALESCE(published_at,0) DESC,id ASC LIMIT ? OFFSET ?
`,feedID,limit+1,offset)
    if err!=nil{return nil,false,err}
    defer rows.Close()
    result:=make([]PodcastEpisodeRecord,0,limit+1)
    for rows.Next(){
        var e PodcastEpisodeRecord
        if err:=rows.Scan(&e.ID,&e.FeedID,&e.GUID,&e.Title,&e.Description,
            &e.EnclosureURL,&e.EnclosureType,&e.PublishedAt,&e.DurationMs);err!=nil{
            return nil,false,err
        }
        result=append(result,e)
    }
    if err=rows.Err();err!=nil{return nil,false,err}
    more:=len(result)>limit
    if more{result=result[:limit]}
    return result,more,nil
}

func (d *DB) UnsubscribePodcast(ctx context.Context,feedID string) error {
    if err:=d.EnsurePodcastSchema();err!=nil{return err}
    res,err:=d.conn.ExecContext(ctx,`DELETE FROM podcast_subscriptions WHERE feed_id=?`,feedID)
    if err!=nil{return err}
    affected,err:=res.RowsAffected()
    if err!=nil{return err}
    if affected==0{return sql.ErrNoRows}
    return nil
}

// PodcastSubscribed reports whether the feed is subscribed, rather than merely
// cached. It does not expose untrusted metadata to the API.
func (d *DB) PodcastSubscribed(ctx context.Context,feedID string) (bool,error){
    if err:=d.EnsurePodcastSchema();err!=nil{return false,err}
    var id string
    err:=d.conn.QueryRowContext(ctx,`SELECT feed_id FROM podcast_subscriptions WHERE feed_id=?`,feedID).Scan(&id)
    if errors.Is(err,sql.ErrNoRows){return false,nil}
    return err==nil,err
}
