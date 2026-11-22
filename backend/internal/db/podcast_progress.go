// podcast_progress.go owns durable, isolated listening positions. A future
// player will throttle calls; these primitives provide optimistic revision
// checks so late/older sessions cannot rewind a newer session's progress.
package db

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "time"
)

var ErrPodcastProgressConflict = errors.New("podcast progress revision conflict")

const maxPodcastProgressMillis int64 = 30 * 24 * 60 * 60 * 1000

type PodcastProgress struct{
    EpisodeID string
    PositionMs int64
    ListenedMs int64
    IsPlayed bool
    Revision int64
    CompletedAt int64
    LastPlayedAt int64
}

// GetPodcastProgress returns a zero-revision state for a known episode that
// has never been played. Unknown episode IDs are not silently created.
func (d *DB) GetPodcastProgress(ctx context.Context,episodeID string)(PodcastProgress,error){
    var p PodcastProgress
    if err:=d.EnsurePodcastSchema();err!=nil{return p,err}
    var played int64
    err:=d.conn.QueryRowContext(ctx,
        "SELECT e.id,COALESCE(s.position_ms,0),COALESCE(s.listened_ms,0),"+
        "COALESCE(s.is_played,0),COALESCE(s.revision,0),COALESCE(s.completed_at,0),"+
        "COALESCE(s.last_played_at,0) FROM podcast_episodes e "+
        "LEFT JOIN podcast_episode_state s ON s.episode_id=e.id WHERE e.id=?",episodeID).
        Scan(&p.EpisodeID,&p.PositionMs,&p.ListenedMs,&played,&p.Revision,&p.CompletedAt,&p.LastPlayedAt)
    if err!=nil{return PodcastProgress{},err}
    p.IsPlayed=played==1
    return p,nil
}

// SetPodcastProgress accepts absolute cumulative listened time, not an
// increment. The greatest listened total wins while position may move
// backward after intentional seeking. expectedRevision prevents stale
// sessions from overwriting newer positions. Manual played/unplayed is
// explicit: completion thresholds remain a future player policy decision.
func (d *DB) SetPodcastProgress(ctx context.Context,episodeID string,positionMs,listenedMs,expectedRevision int64,played bool)(PodcastProgress,error){
    if positionMs<0||positionMs>maxPodcastProgressMillis||
        listenedMs<0||listenedMs>maxPodcastProgressMillis||expectedRevision<0{
        return PodcastProgress{},fmt.Errorf("invalid podcast progress")
    }
    if err:=d.EnsurePodcastSchema();err!=nil{return PodcastProgress{},err}
    // A first write must start at revision zero. This also prevents callers
    // from skipping arbitrary revision numbers when no state row exists yet.
    var existingRevision int64
    checkErr:=d.conn.QueryRowContext(ctx,
        "SELECT revision FROM podcast_episode_state WHERE episode_id=?",episodeID).Scan(&existingRevision)
    if checkErr!=nil && !errors.Is(checkErr,sql.ErrNoRows){return PodcastProgress{},checkErr}
    if errors.Is(checkErr,sql.ErrNoRows) && expectedRevision!=0{
        if _,err:=d.GetPodcastProgress(ctx,episodeID);err!=nil{return PodcastProgress{},err}
        return PodcastProgress{},ErrPodcastProgressConflict
    }
    playedValue:=0
    var completion any
    now:=time.Now().UnixMilli()
    if played{playedValue=1;completion=now}
    // A single statement performs the CAS and updates no other user state.
    // INSERT...SELECT avoids FK ambiguity for unknown episode IDs.
    var revision int64
    err:=d.conn.QueryRowContext(ctx,
        "INSERT INTO podcast_episode_state "+
        "(episode_id,position_ms,listened_ms,is_played,completed_at,last_played_at,revision) "+
        "SELECT id,?,?,?,?,?,1 FROM podcast_episodes WHERE id=? "+
        "ON CONFLICT(episode_id) DO UPDATE SET "+
        "position_ms=excluded.position_ms,"+
        "listened_ms=MAX(podcast_episode_state.listened_ms,excluded.listened_ms),"+
        "is_played=excluded.is_played,"+
        "completed_at=CASE WHEN excluded.is_played=0 THEN NULL "+
        "ELSE COALESCE(podcast_episode_state.completed_at,excluded.completed_at) END,"+
        "last_played_at=excluded.last_played_at,"+
        "revision=podcast_episode_state.revision+1 "+
        "WHERE podcast_episode_state.revision=? RETURNING revision",
        positionMs,listenedMs,playedValue,completion,now,episodeID,expectedRevision).
        Scan(&revision)
    if errors.Is(err,sql.ErrNoRows){
        // Distinguish unknown episodes from stale revision writes.
        var id string
        checkErr:=d.conn.QueryRowContext(ctx,"SELECT id FROM podcast_episodes WHERE id=?",episodeID).Scan(&id)
        if errors.Is(checkErr,sql.ErrNoRows){return PodcastProgress{},sql.ErrNoRows}
        if checkErr!=nil{return PodcastProgress{},checkErr}
        return PodcastProgress{},ErrPodcastProgressConflict
    }
    if err!=nil{return PodcastProgress{},fmt.Errorf("save podcast progress: %w",err)}
    return d.GetPodcastProgress(ctx,episodeID)
}
