// podcast_schema.go owns additive podcast-only SQLite schema. No existing
// music, Spotify, library, or DJ tables are modified.
package db

import (
    "fmt"
    "strings"
    "sync"
)

type podcastSchemaResult struct{ err error }
var (
    podcastSchemaMu sync.Mutex
    podcastSchemas sync.Map // map[*DB]podcastSchemaResult
)

// EnsurePodcastSchema follows the repository's per-domain lazy schema model.
// A mutex ensures concurrent callers cannot observe a half-created schema.
func (d *DB) EnsurePodcastSchema() error {
    podcastSchemaMu.Lock()
    defer podcastSchemaMu.Unlock()
    if cached, ok := podcastSchemas.Load(d); ok {
        return cached.(podcastSchemaResult).err
    }
    _, err := d.conn.Exec(`
CREATE TABLE IF NOT EXISTS podcast_feeds (
    id TEXT PRIMARY KEY,
    canonical_url TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    author TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    image_url TEXT NOT NULL DEFAULT '',
    language TEXT NOT NULL DEFAULT '',
    etag TEXT,
    last_modified TEXT,
    last_checked_at INTEGER,
    last_success_at INTEGER,
    next_refresh_at INTEGER,
    last_error_code TEXT,
    refresh_failures INTEGER NOT NULL DEFAULT 0,
    refresh_lease_until INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS podcast_feed_aliases (
    original_url TEXT PRIMARY KEY,
    feed_id TEXT NOT NULL REFERENCES podcast_feeds(id)
);
CREATE TABLE IF NOT EXISTS podcast_subscriptions (
    feed_id TEXT PRIMARY KEY REFERENCES podcast_feeds(id),
    subscribed_at INTEGER NOT NULL,
    auto_download_policy TEXT NOT NULL DEFAULT 'off',
    keep_latest_count INTEGER NOT NULL DEFAULT 0,
    auto_queue_policy TEXT NOT NULL DEFAULT 'off',
    default_speed REAL NOT NULL DEFAULT 1.0
);
CREATE TABLE IF NOT EXISTS podcast_episodes (
    id TEXT PRIMARY KEY,
    feed_id TEXT NOT NULL REFERENCES podcast_feeds(id),
    identity_key TEXT NOT NULL,
    guid TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enclosure_url TEXT NOT NULL DEFAULT '',
    enclosure_mime TEXT NOT NULL DEFAULT '',
    published_at INTEGER,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    listed_in_feed INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(feed_id, identity_key)
);
CREATE TABLE IF NOT EXISTS podcast_episode_state (
    episode_id TEXT PRIMARY KEY REFERENCES podcast_episodes(id),
    position_ms INTEGER NOT NULL DEFAULT 0,
    listened_ms INTEGER NOT NULL DEFAULT 0,
    is_played INTEGER NOT NULL DEFAULT 0,
    completed_at INTEGER,
    last_played_at INTEGER,
    starred INTEGER NOT NULL DEFAULT 0,
    archived INTEGER NOT NULL DEFAULT 0,
    revision INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS podcast_queue (
    episode_id TEXT PRIMARY KEY REFERENCES podcast_episodes(id),
    sort_key REAL NOT NULL,
    added_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS podcast_downloads (
    id TEXT PRIMARY KEY,
    episode_id TEXT NOT NULL UNIQUE REFERENCES podcast_episodes(id),
    status TEXT NOT NULL CHECK(status IN ('queued','downloading','paused','completed','failed','canceled')),
    destination_relative_path TEXT,
    temp_relative_path TEXT,
    bytes_received INTEGER NOT NULL DEFAULT 0,
    expected_bytes INTEGER,
    etag TEXT,
    last_modified TEXT,
    verified_size INTEGER,
    sha256 TEXT,
    error_code TEXT,
    retry_after INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    completed_at INTEGER,
    lease_owner TEXT,
    lease_expires_at INTEGER
);
CREATE TABLE IF NOT EXISTS podcast_bookmarks (
    id TEXT PRIMARY KEY,
    episode_id TEXT NOT NULL REFERENCES podcast_episodes(id),
    position_ms INTEGER NOT NULL CHECK(position_ms >= 0),
    note TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_podcast_episode_feed_date
    ON podcast_episodes(feed_id, published_at DESC, id);
CREATE INDEX IF NOT EXISTS idx_podcast_refresh_due
    ON podcast_feeds(next_refresh_at);
CREATE INDEX IF NOT EXISTS idx_podcast_subscribed_date
    ON podcast_subscriptions(subscribed_at DESC);
CREATE INDEX IF NOT EXISTS idx_podcast_download_state
    ON podcast_downloads(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_podcast_queue_order
    ON podcast_queue(sort_key, added_at);
`)
    // Older preview databases lack the refresh columns. Add them after the
    // base schema exists; repeated Ensure calls remain idempotent.
    if err == nil {
        err = addPodcastColumns(d,
            "ALTER TABLE podcast_feeds ADD COLUMN refresh_failures INTEGER NOT NULL DEFAULT 0",
            "ALTER TABLE podcast_feeds ADD COLUMN refresh_lease_until INTEGER NOT NULL DEFAULT 0",
        )
    }
    if err != nil { err = fmt.Errorf("create podcast schema: %w", err) }
    podcastSchemas.Store(d, podcastSchemaResult{err: err})
    return err
}

func addPodcastColumns(d *DB, statements ...string) error {
    for _, stmt := range statements {
        if _, err := d.conn.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
            return err
        }
    }
    return nil
}
