// Shared podcast episode upsert, including conservative publisher GUID repair.
package db

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "strings"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func podcastEpisodeIdentity(e podcast.Episode)(string,any){
    guid:=strings.TrimSpace(e.GUID)
    key:=""
    if guid!=""{key="guid:"+guid}else if strings.TrimSpace(e.EnclosureURL)!=""{
        key="enclosure:"+strings.TrimSpace(e.EnclosureURL)
    }
    var published any
    if !e.PublishedAt.IsZero(){published=e.PublishedAt.UnixMilli()}
    return key,published
}

// upsertPodcastEpisodes never edits playback, queue, download or bookmark
// records. A changed GUID migrates an ID only if unique old enclosure, title,
// date and positive duration all match and the prior GUID is absent from
// this snapshot. Ambiguous cases deliberately remain separate episodes.
func upsertPodcastEpisodes(ctx context.Context,tx *sql.Tx,feedID string,episodes []podcast.Episode,now int64)error{
    incoming:=make(map[string]struct{},len(episodes))
    for _,e:=range episodes{
        if strings.TrimSpace(e.Title)==""{continue}
        key,_:=podcastEpisodeIdentity(e)
        if key!=""{incoming[key]=struct{}{}}
    }
    migrated:=make(map[string]struct{})
    for _,e:=range episodes{
        if strings.TrimSpace(e.Title)==""{continue}
        key,published:=podcastEpisodeIdentity(e)
        if key==""{continue}
        episodeID:=podcastStableID(feedID,key)
        var existingID string
        err:=tx.QueryRowContext(ctx,"SELECT id FROM podcast_episodes WHERE feed_id=? AND identity_key=?",feedID,key).Scan(&existingID)
        if err!=nil && !errors.Is(err,sql.ErrNoRows){return fmt.Errorf("lookup podcast episode: %w",err)}
        if err==nil{
            episodeID=existingID
        }else if strings.HasPrefix(key,"guid:") && strings.TrimSpace(e.EnclosureURL)!="" &&
            published!=nil && e.DurationSeconds>0 {
            rows,queryErr:=tx.QueryContext(ctx,
                "SELECT id,identity_key FROM podcast_episodes WHERE feed_id=? AND enclosure_url=? AND title=? AND published_at=? AND duration_ms=? AND identity_key LIKE 'guid:%' LIMIT 2",
                feedID,strings.TrimSpace(e.EnclosureURL),e.Title,published,e.DurationSeconds*1000)
            if queryErr!=nil{return fmt.Errorf("lookup old GUID: %w",queryErr)}
            type candidate struct{id,key string}
            matches:=[]candidate{}
            for rows.Next(){
                var c candidate
                if queryErr=rows.Scan(&c.id,&c.key);queryErr!=nil{break}
                matches=append(matches,c)
            }
            if queryErr==nil{queryErr=rows.Err()}
            rows.Close()
            if queryErr!=nil{return fmt.Errorf("scan old GUID: %w",queryErr)}
            if len(matches)==1 {
                old:=matches[0]
                _,stillListed:=incoming[old.key]
                _,claimed:=migrated[old.id]
                if !stillListed&&!claimed {
                    _,err=tx.ExecContext(ctx,"UPDATE podcast_episodes SET identity_key=?,guid=?,updated_at=? WHERE id=?",key,e.GUID,now,old.id)
                    if err!=nil{return fmt.Errorf("migrate podcast GUID: %w",err)}
                    episodeID=old.id
                    migrated[old.id]=struct{}{}
                }
            }
        }
        richDesc:=podcast.SanitizeRichHTML(e.RichDescription)
        _,err=tx.ExecContext(ctx,
            "INSERT INTO podcast_episodes (id,feed_id,identity_key,guid,title,description,enclosure_url,enclosure_mime,published_at,duration_ms,"+
            "chapters_url,chapters_type,transcript_url,transcript_type,website_url,enclosure_bytes,episode_number,season_number,rich_description,created_at,updated_at) "+
            "VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(feed_id,identity_key) DO UPDATE SET "+
            "title=excluded.title,description=excluded.description,enclosure_url=excluded.enclosure_url,enclosure_mime=excluded.enclosure_mime,"+
            "published_at=excluded.published_at,duration_ms=excluded.duration_ms,"+
            "chapters_url=excluded.chapters_url,chapters_type=excluded.chapters_type,"+
            "transcript_url=excluded.transcript_url,transcript_type=excluded.transcript_type,"+
            "website_url=excluded.website_url,enclosure_bytes=excluded.enclosure_bytes,"+
            "episode_number=excluded.episode_number,season_number=excluded.season_number,"+
            "rich_description=excluded.rich_description,listed_in_feed=1,updated_at=excluded.updated_at",
            episodeID,feedID,key,e.GUID,e.Title,e.Description,e.EnclosureURL,e.EnclosureType,published,e.DurationSeconds*1000,
            strings.TrimSpace(e.ChaptersURL),strings.TrimSpace(e.ChaptersType),
            strings.TrimSpace(e.TranscriptURL),strings.TrimSpace(e.TranscriptType),
            strings.TrimSpace(e.WebsiteURL),e.EnclosureBytes,e.EpisodeNumber,e.SeasonNumber,richDesc,now,now)
        if err!=nil{return fmt.Errorf("save podcast episode: %w",err)}
    }
    return nil
}
