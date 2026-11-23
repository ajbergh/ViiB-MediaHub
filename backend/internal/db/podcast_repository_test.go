package db

import (
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastSchemaAndSnapshotSurviveReopen(t *testing.T){
    filename:=filepath.Join(t.TempDir(),"viib.db")
    database,err:=New(filename)
    if err!=nil{t.Fatal(err)}
    ctx:=context.Background()
    if err:=database.EnsurePodcastSchema();err!=nil{t.Fatal(err)}
    if err:=database.EnsurePodcastSchema();err!=nil{t.Fatal(err)}
    data,err:=os.ReadFile("../podcast/testdata/basic.xml")
    if err!=nil{t.Fatal(err)}
    feed,err:=podcast.ParseRSS(strings.NewReader(string(data)),"https://example.org/rss.xml")
    if err!=nil{t.Fatal(err)}
    id,err:=database.SavePodcastFeedSnapshot(ctx,feed,true)
    if err!=nil{t.Fatal(err)}
    shows,err:=database.ListSubscribedPodcasts(ctx)
    if err!=nil{t.Fatal(err)}
    if len(shows)!=1||shows[0].EpisodeCount!=2{t.Fatalf("unexpected shows: %#v",shows)}
    episodes,err:=database.ListPodcastEpisodes(ctx,id,100)
    if err!=nil||len(episodes)!=2{t.Fatalf("episodes: %#v error=%v",episodes,err)}
    var target string
    for _,episode:=range episodes{
        if episode.GUID=="episode-001"{target=episode.ID}
    }
    if target==""{t.Fatal("missing episode identity")}
    _,err=database.conn.ExecContext(ctx,`
INSERT INTO podcast_episode_state(episode_id,position_ms,revision) VALUES (?,81234,1)`,target)
    if err!=nil{t.Fatal(err)}
    feed.Episodes[0].Title="Updated Title"
    if _,err:=database.SavePodcastFeedSnapshot(ctx,feed,true);err!=nil{t.Fatal(err)}
    // Re-imports must neither duplicate episodes nor reset playback progress.
    var count int
    if err:=database.conn.QueryRowContext(ctx,`SELECT COUNT(*) FROM podcast_episodes WHERE feed_id=?`,id).Scan(&count);err!=nil{t.Fatal(err)}
    if count!=2{t.Fatalf("duplicate episodes: %d",count)}
    var position int64
    if err:=database.conn.QueryRowContext(ctx,`SELECT position_ms FROM podcast_episode_state WHERE episode_id=?`,target).Scan(&position);err!=nil{t.Fatal(err)}
    if position!=81234{t.Fatalf("progress lost: %d",position)}
    if err:=database.Close();err!=nil{t.Fatal(err)}
    reopened,err:=New(filename)
    if err!=nil{t.Fatal(err)}
    defer reopened.Close()
    shows,err=reopened.ListSubscribedPodcasts(ctx)
    if err!=nil||len(shows)!=1||shows[0].EpisodeCount!=2{t.Fatalf("reopen failed: %#v error=%v",shows,err)}
    if err:=reopened.UnsubscribePodcast(ctx,id);err!=nil{t.Fatal(err)}
    followed,err:=reopened.PodcastSubscribed(ctx,id)
    if err!=nil||followed{t.Fatalf("unsubscribe failed: %v %v",followed,err)}
    // A subscription removal must not delete the episode or its user state.
    if err:=reopened.conn.QueryRowContext(ctx,`SELECT position_ms FROM podcast_episode_state WHERE episode_id=?`,target).Scan(&position);err!=nil{t.Fatal(err)}
    if position!=81234{t.Fatalf("unsubscribe erased history: %d",position)}
}

func TestPodcastRejectsPrivateURLAndKeepsMusicSchema(t *testing.T){
    database,err:=New(filepath.Join(t.TempDir(),"viib.db"))
    if err!=nil{t.Fatal(err)}
    defer database.Close()
    ctx:=context.Background()
    if _,err:=database.SavePodcastFeedSnapshot(ctx,podcast.Feed{
        URL:"https://example.org/podcast?token=secret",Title:"Sensitive",
    },true);err==nil{t.Fatal("tokenized public feed was persisted")}
    var songsTable string
    if err:=database.conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='songs'`).Scan(&songsTable);err!=nil{t.Fatal(err)}
    if songsTable!="songs"{t.Fatalf("music table damaged: %s",songsTable)}
}
