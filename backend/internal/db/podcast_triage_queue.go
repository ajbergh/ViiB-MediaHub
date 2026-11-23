// podcast_triage_queue.go owns episode triage views, cross-show local search,
// the durable podcast playback queue, and per-show download retention policies.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MaxPodcastKeepLatestCount = 50
	MaxPodcastQueueSize       = 500
)

var (
	ErrInvalidPodcastView         = errors.New("invalid podcast episode view")
	ErrInvalidPodcastEpisodePatch = errors.New("invalid podcast episode state update")
	ErrInvalidPodcastQueue        = errors.New("invalid podcast queue request")
	ErrPodcastEpisodeNoAudio      = errors.New("podcast episode has no audio enclosure")
	ErrInvalidPodcastRetention    = errors.New("invalid podcast retention policy")
)

// PodcastEpisodeState captures the persisted playback, triage, and CAS revision state for an episode.
type PodcastEpisodeState struct {
	EpisodeID    string `json:"episodeId"`
	PositionMs   int64  `json:"positionMs"`
	ListenedMs   int64  `json:"listenedMs"`
	IsPlayed     bool   `json:"isPlayed"`
	CompletedAt  int64  `json:"completedAt"`
	LastPlayedAt int64  `json:"lastPlayedAt"`
	Starred      bool   `json:"starred"`
	Archived     bool   `json:"archived"`
	Revision     int64  `json:"revision"`
}

type PodcastViewEpisode struct {
	PodcastEpisodeRecord
	FeedTitle      string `json:"feedTitle"`
	FeedAuthor     string `json:"feedAuthor"`
	Starred        bool   `json:"starred"`
	Archived       bool   `json:"archived"`
	DownloadStatus string `json:"downloadStatus"`
}

// PodcastQueueItem represents an entry in the durable podcast_queue ordered by sort_key.
type PodcastQueueItem struct {
	PodcastViewEpisode
	SortKey float64 `json:"sortKey"`
	AddedAt int64   `json:"addedAt"`
}

// PodcastRetentionPolicy configures how many newest completed downloads a show retains.
type PodcastRetentionPolicy struct {
	KeepLatestCount int `json:"keepLatestCount"`
}

type PodcastExpiredDownload struct {
	EpisodeID    string
	RelativePath string
}

func nullableUnixMillis(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func boolToSQLiteInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// PatchPodcastEpisodeState updates or inserts podcast_episode_state atomically
// for a subscribed episode, enforcing optimistic expectedRevision when supplied.
func (d *DB) PatchPodcastEpisodeState(
	ctx context.Context,
	episodeID string,
	expectedRevision *int64,
	starred *bool,
	archived *bool,
	isPlayed *bool,
	positionMs *int64,
	listenedMs *int64,
) (PodcastEpisodeState, error) {
	if starred == nil && archived == nil && isPlayed == nil && positionMs == nil && listenedMs == nil {
		return PodcastEpisodeState{}, ErrInvalidPodcastEpisodePatch
	}
	if expectedRevision != nil && *expectedRevision < 0 {
		return PodcastEpisodeState{}, ErrInvalidPodcastEpisodePatch
	}
	if positionMs != nil && (*positionMs < 0 || *positionMs > maxPodcastProgressMillis) {
		return PodcastEpisodeState{}, ErrInvalidPodcastEpisodePatch
	}
	if listenedMs != nil && (*listenedMs < 0 || *listenedMs > maxPodcastProgressMillis) {
		return PodcastEpisodeState{}, ErrInvalidPodcastEpisodePatch
	}
	if err := d.EnsurePodcastSchema(); err != nil {
		return PodcastEpisodeState{}, err
	}

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return PodcastEpisodeState{}, err
	}
	defer tx.Rollback()

	var subscribedID string
	err = tx.QueryRowContext(ctx, `
SELECT e.id
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id = e.feed_id
WHERE e.id = ?`, episodeID).Scan(&subscribedID)
	if err != nil {
		return PodcastEpisodeState{}, err
	}

	var cur PodcastEpisodeState
	var curPlayed, curStarred, curArchived int64
	rowExists := true
	err = tx.QueryRowContext(ctx, `
SELECT episode_id, position_ms, listened_ms, is_played,
       COALESCE(completed_at, 0), COALESCE(last_played_at, 0),
       starred, archived, revision
FROM podcast_episode_state
WHERE episode_id = ?`, episodeID).Scan(
		&cur.EpisodeID, &cur.PositionMs, &cur.ListenedMs, &curPlayed,
		&cur.CompletedAt, &cur.LastPlayedAt, &curStarred, &curArchived, &cur.Revision,
	)
	if errors.Is(err, sql.ErrNoRows) {
		rowExists = false
		cur = PodcastEpisodeState{EpisodeID: episodeID}
	} else if err != nil {
		return PodcastEpisodeState{}, err
	} else {
		cur.IsPlayed = curPlayed == 1
		cur.Starred = curStarred == 1
		cur.Archived = curArchived == 1
	}

	if expectedRevision != nil && *expectedRevision != cur.Revision {
		return PodcastEpisodeState{}, ErrPodcastProgressConflict
	}

	now := time.Now().UnixMilli()
	next := cur
	next.Revision = cur.Revision + 1

	if positionMs != nil {
		next.PositionMs = *positionMs
	}
	if listenedMs != nil && *listenedMs > next.ListenedMs {
		next.ListenedMs = *listenedMs
	}
	if starred != nil {
		next.Starred = *starred
	}
	if archived != nil {
		next.Archived = *archived
	}
	if isPlayed != nil {
		next.IsPlayed = *isPlayed
		if *isPlayed {
			if next.CompletedAt == 0 {
				next.CompletedAt = now
			}
		} else {
			next.CompletedAt = 0
		}
	}
	if positionMs != nil || listenedMs != nil || (isPlayed != nil && *isPlayed) {
		next.LastPlayedAt = now
	}

	if !rowExists {
		res, execErr := tx.ExecContext(ctx, `
INSERT INTO podcast_episode_state (
	episode_id, position_ms, listened_ms, is_played,
	completed_at, last_played_at, starred, archived, revision
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(episode_id) DO NOTHING`,
			next.EpisodeID,
			next.PositionMs,
			next.ListenedMs,
			boolToSQLiteInt(next.IsPlayed),
			nullableUnixMillis(next.CompletedAt),
			nullableUnixMillis(next.LastPlayedAt),
			boolToSQLiteInt(next.Starred),
			boolToSQLiteInt(next.Archived),
			next.Revision,
		)
		if execErr != nil {
			return PodcastEpisodeState{}, fmt.Errorf("insert podcast episode state: %w", execErr)
		}
		n, raErr := res.RowsAffected()
		if raErr != nil {
			return PodcastEpisodeState{}, raErr
		}
		if n == 0 {
			return PodcastEpisodeState{}, ErrPodcastProgressConflict
		}
	} else {
		res, execErr := tx.ExecContext(ctx, `
UPDATE podcast_episode_state
SET position_ms = ?,
    listened_ms = ?,
    is_played = ?,
    completed_at = ?,
    last_played_at = ?,
    starred = ?,
    archived = ?,
    revision = ?
WHERE episode_id = ? AND revision = ?`,
			next.PositionMs,
			next.ListenedMs,
			boolToSQLiteInt(next.IsPlayed),
			nullableUnixMillis(next.CompletedAt),
			nullableUnixMillis(next.LastPlayedAt),
			boolToSQLiteInt(next.Starred),
			boolToSQLiteInt(next.Archived),
			next.Revision,
			episodeID,
			cur.Revision,
		)
		if execErr != nil {
			return PodcastEpisodeState{}, fmt.Errorf("update podcast episode state: %w", execErr)
		}
		n, raErr := res.RowsAffected()
		if raErr != nil {
			return PodcastEpisodeState{}, raErr
		}
		if n == 0 {
			return PodcastEpisodeState{}, ErrPodcastProgressConflict
		}
	}

	if err := tx.Commit(); err != nil {
		return PodcastEpisodeState{}, err
	}
	return next, nil
}

const podcastViewEpisodeColumns = `
e.id, e.feed_id, e.guid, e.title, e.description, e.enclosure_url, e.enclosure_mime,
COALESCE(e.published_at, 0), e.duration_ms,
COALESCE(s.is_played, 0), COALESCE(s.position_ms, 0), COALESCE(s.listened_ms, 0), COALESCE(s.revision, 0),
f.title, f.author, COALESCE(s.starred, 0), COALESCE(s.archived, 0), COALESCE(dl.status, '')`

func scanPodcastViewEpisode(scanner interface{ Scan(...any) error }) (PodcastViewEpisode, error) {
	var item PodcastViewEpisode
	var played, starred, archived int64
	e := &item.PodcastEpisodeRecord
	err := scanner.Scan(
		&e.ID, &e.FeedID, &e.GUID, &e.Title, &e.Description,
		&e.EnclosureURL, &e.EnclosureType, &e.PublishedAt, &e.DurationMs,
		&played, &e.PositionMs, &e.ListenedMs, &e.ProgressRevision,
		&item.FeedTitle, &item.FeedAuthor, &starred, &archived, &item.DownloadStatus,
	)
	if err != nil {
		return PodcastViewEpisode{}, err
	}
	e.IsPlayed = played == 1
	e.HasAudio = e.EnclosureURL != ""
	item.Starred = starred == 1
	item.Archived = archived == 1
	return item, nil
}

// ListPodcastEpisodesByView returns subscribed podcast episodes filtered by a
// named triage view and optional LIKE query, paginated via limit+1.
func (d *DB) ListPodcastEpisodesByView(
	ctx context.Context,
	view string,
	query string,
	limit int,
	offset int,
) ([]PodcastViewEpisode, bool, error) {
	if err := d.EnsurePodcastSchema(); err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var viewClause string
	switch view {
	case "inbox":
		viewClause = "COALESCE(s.is_played, 0) = 0 AND COALESCE(s.archived, 0) = 0"
	case "starred":
		viewClause = "COALESCE(s.starred, 0) = 1"
	case "in_progress":
		viewClause = "COALESCE(s.is_played, 0) = 0 AND COALESCE(s.position_ms, 0) > 0 AND COALESCE(s.archived, 0) = 0"
	case "played":
		viewClause = "COALESCE(s.is_played, 0) = 1"
	case "archived":
		viewClause = "COALESCE(s.archived, 0) = 1"
	case "all":
		viewClause = "1 = 1"
	default:
		return nil, false, ErrInvalidPodcastView
	}

	args := make([]any, 0, 5)
	where := viewClause
	if q := strings.TrimSpace(query); q != "" {
		pattern := "%" + escapeLikePattern(q) + "%"
		where += ` AND (e.title LIKE ? ESCAPE '\' OR e.description LIKE ? ESCAPE '\' OR f.title LIKE ? ESCAPE '\')`
		args = append(args, pattern, pattern, pattern)
	}
	args = append(args, limit+1, offset)

	rows, err := d.conn.QueryContext(ctx, `
SELECT `+podcastViewEpisodeColumns+`
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id = e.feed_id
JOIN podcast_feeds f ON f.id = e.feed_id
LEFT JOIN podcast_episode_state s ON s.episode_id = e.id
LEFT JOIN podcast_downloads dl ON dl.episode_id = e.id
WHERE `+where+`
ORDER BY COALESCE(e.published_at, 0) DESC, e.id ASC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	result := make([]PodcastViewEpisode, 0, limit+1)
	for rows.Next() {
		item, scanErr := scanPodcastViewEpisode(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

// SearchPodcastEpisodes performs local cross-show search across subscribed
// episodes and feeds matching title, description, or show title/author.
func (d *DB) SearchPodcastEpisodes(
	ctx context.Context,
	query string,
	limit int,
	offset int,
) ([]PodcastViewEpisode, bool, error) {
	if err := d.EnsurePodcastSchema(); err != nil {
		return nil, false, err
	}
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return []PodcastViewEpisode{}, false, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	pattern := "%" + escapeLikePattern(trimmed) + "%"
	rows, err := d.conn.QueryContext(ctx, `
SELECT `+podcastViewEpisodeColumns+`
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id = e.feed_id
JOIN podcast_feeds f ON f.id = e.feed_id
LEFT JOIN podcast_episode_state s ON s.episode_id = e.id
LEFT JOIN podcast_downloads dl ON dl.episode_id = e.id
WHERE e.title LIKE ? ESCAPE '\'
   OR e.description LIKE ? ESCAPE '\'
   OR f.title LIKE ? ESCAPE '\'
   OR f.author LIKE ? ESCAPE '\'
ORDER BY (CASE WHEN e.title LIKE ? ESCAPE '\' THEN 0 WHEN f.title LIKE ? ESCAPE '\' THEN 1 ELSE 2 END),
         COALESCE(e.published_at, 0) DESC, e.id ASC
LIMIT ? OFFSET ?`,
		pattern, pattern, pattern, pattern,
		pattern, pattern,
		limit+1, offset,
	)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	result := make([]PodcastViewEpisode, 0, limit+1)
	for rows.Next() {
		item, scanErr := scanPodcastViewEpisode(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

// ListPodcastQueue returns queued episodes for active subscriptions ordered by
// sort_key ASC, added_at ASC.
func (d *DB) ListPodcastQueue(ctx context.Context) ([]PodcastQueueItem, error) {
	if err := d.EnsurePodcastSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.QueryContext(ctx, `
SELECT `+podcastViewEpisodeColumns+`, q.sort_key, q.added_at
FROM podcast_queue q
JOIN podcast_episodes e ON e.id = q.episode_id
JOIN podcast_subscriptions sub ON sub.feed_id = e.feed_id
JOIN podcast_feeds f ON f.id = e.feed_id
LEFT JOIN podcast_episode_state s ON s.episode_id = e.id
LEFT JOIN podcast_downloads dl ON dl.episode_id = e.id
ORDER BY q.sort_key ASC, q.added_at ASC, e.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]PodcastQueueItem, 0)
	for rows.Next() {
		var item PodcastQueueItem
		var played, starred, archived int64
		e := &item.PodcastEpisodeRecord
		if err := rows.Scan(
			&e.ID, &e.FeedID, &e.GUID, &e.Title, &e.Description,
			&e.EnclosureURL, &e.EnclosureType, &e.PublishedAt, &e.DurationMs,
			&played, &e.PositionMs, &e.ListenedMs, &e.ProgressRevision,
			&item.FeedTitle, &item.FeedAuthor, &starred, &archived, &item.DownloadStatus,
			&item.SortKey, &item.AddedAt,
		); err != nil {
			return nil, err
		}
		e.IsPlayed = played == 1
		e.HasAudio = e.EnclosureURL != ""
		item.Starred = starred == 1
		item.Archived = archived == 1
		result = append(result, item)
	}
	return result, rows.Err()
}

// AddToPodcastQueue inserts or repositions a subscribed episode with audio into
// podcast_queue at "next" (before minimum sort_key) or "end" (max + 1.0).
func (d *DB) AddToPodcastQueue(ctx context.Context, episodeID, position string) error {
	if position == "" {
		position = "end"
	}
	if position != "next" && position != "end" {
		return ErrInvalidPodcastQueue
	}
	if err := d.EnsurePodcastSchema(); err != nil {
		return err
	}

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var enclosureURL string
	err = tx.QueryRowContext(ctx, `
SELECT e.enclosure_url
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id = e.feed_id
WHERE e.id = ?`, episodeID).Scan(&enclosureURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(enclosureURL) == "" {
		return ErrPodcastEpisodeNoAudio
	}

	var existingSort float64
	alreadyQueued := true
	err = tx.QueryRowContext(ctx, `SELECT sort_key FROM podcast_queue WHERE episode_id = ?`, episodeID).Scan(&existingSort)
	if errors.Is(err, sql.ErrNoRows) {
		alreadyQueued = false
	} else if err != nil {
		return err
	}

	if alreadyQueued {
		if position == "end" {
			return tx.Commit()
		}
		var minOther sql.NullFloat64
		if err := tx.QueryRowContext(ctx, `
SELECT MIN(sort_key)
FROM podcast_queue
WHERE episode_id <> ?`, episodeID).Scan(&minOther); err != nil {
			return err
		}
		if minOther.Valid && existingSort >= minOther.Float64 {
			if _, err := tx.ExecContext(ctx, `
UPDATE podcast_queue SET sort_key = ? WHERE episode_id = ?`,
				minOther.Float64-1.0, episodeID); err != nil {
				return err
			}
		}
		return tx.Commit()
	}

	var sortKey float64
	if position == "next" {
		var minVal sql.NullFloat64
		if err := tx.QueryRowContext(ctx, `SELECT MIN(sort_key) FROM podcast_queue`).Scan(&minVal); err != nil {
			return err
		}
		if minVal.Valid {
			sortKey = minVal.Float64 - 1.0
		} else {
			sortKey = 1.0
		}
	} else {
		var maxVal sql.NullFloat64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(sort_key) FROM podcast_queue`).Scan(&maxVal); err != nil {
			return err
		}
		if maxVal.Valid {
			sortKey = maxVal.Float64 + 1.0
		} else {
			sortKey = 1.0
		}
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO podcast_queue (episode_id, sort_key, added_at)
VALUES (?, ?, ?)`, episodeID, sortKey, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplacePodcastQueue atomically replaces the entire podcast_queue after
// validating every episode belongs to a subscribed show and has audio.
func (d *DB) ReplacePodcastQueue(ctx context.Context, episodeIDs []string) error {
	if len(episodeIDs) > MaxPodcastQueueSize {
		return ErrInvalidPodcastQueue
	}
	seen := make(map[string]struct{}, len(episodeIDs))
	for _, id := range episodeIDs {
		if strings.TrimSpace(id) == "" {
			return ErrInvalidPodcastQueue
		}
		if _, dup := seen[id]; dup {
			return ErrInvalidPodcastQueue
		}
		seen[id] = struct{}{}
	}
	if err := d.EnsurePodcastSchema(); err != nil {
		return err
	}

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range episodeIDs {
		var enclosureURL string
		err := tx.QueryRowContext(ctx, `
SELECT e.enclosure_url
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id = e.feed_id
WHERE e.id = ?`, id).Scan(&enclosureURL)
		if err != nil {
			return err
		}
		if strings.TrimSpace(enclosureURL) == "" {
			return ErrPodcastEpisodeNoAudio
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM podcast_queue`); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for i, id := range episodeIDs {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO podcast_queue (episode_id, sort_key, added_at)
VALUES (?, ?, ?)`, id, float64(i+1), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveFromPodcastQueue removes an episode from podcast_queue.
func (d *DB) RemoveFromPodcastQueue(ctx context.Context, episodeID string) error {
	if err := d.EnsurePodcastSchema(); err != nil {
		return err
	}
	_, err := d.conn.ExecContext(ctx, `DELETE FROM podcast_queue WHERE episode_id = ?`, episodeID)
	return err
}

// GetPodcastRetentionPolicy returns the keep_latest_count policy for an active
// subscription (0 = keep all, 1..50 = keep newest N completed downloads).
func (d *DB) GetPodcastRetentionPolicy(ctx context.Context, feedID string) (PodcastRetentionPolicy, error) {
	var out PodcastRetentionPolicy
	if err := d.EnsurePodcastSchema(); err != nil {
		return out, err
	}
	err := d.conn.QueryRowContext(ctx, `
SELECT keep_latest_count
FROM podcast_subscriptions
WHERE feed_id = ?`, feedID).Scan(&out.KeepLatestCount)
	if err != nil {
		return out, err
	}
	if out.KeepLatestCount < 0 {
		out.KeepLatestCount = 0
	} else if out.KeepLatestCount > MaxPodcastKeepLatestCount {
		out.KeepLatestCount = MaxPodcastKeepLatestCount
	}
	return out, nil
}

// SetPodcastRetentionPolicy updates keep_latest_count (0..50) for an active
// subscription, returning sql.ErrNoRows if the feed is not subscribed.
func (d *DB) SetPodcastRetentionPolicy(ctx context.Context, feedID string, keepLatestCount int) (PodcastRetentionPolicy, error) {
	if keepLatestCount < 0 || keepLatestCount > MaxPodcastKeepLatestCount {
		return PodcastRetentionPolicy{}, ErrInvalidPodcastRetention
	}
	if err := d.EnsurePodcastSchema(); err != nil {
		return PodcastRetentionPolicy{}, err
	}
	res, err := d.conn.ExecContext(ctx, `
UPDATE podcast_subscriptions
SET keep_latest_count = ?
WHERE feed_id = ?`, keepLatestCount, feedID)
	if err != nil {
		return PodcastRetentionPolicy{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return PodcastRetentionPolicy{}, err
	}
	if n == 0 {
		return PodcastRetentionPolicy{}, sql.ErrNoRows
	}
	return PodcastRetentionPolicy{KeepLatestCount: keepLatestCount}, nil
}

// SelectExpiredCompletedDownloads returns completed downloads for feedID beyond
// the newest keep_latest_count episodes, excluding starred episodes and
// excludeEpisodeID.
func (d *DB) SelectExpiredCompletedDownloads(ctx context.Context, feedID, excludeEpisodeID string) ([]PodcastExpiredDownload, error) {
	if err := d.EnsurePodcastSchema(); err != nil {
		return nil, err
	}
	var keepLatest int
	err := d.conn.QueryRowContext(ctx, `
SELECT keep_latest_count
FROM podcast_subscriptions
WHERE feed_id = ?`, feedID).Scan(&keepLatest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if keepLatest <= 0 {
		return nil, nil
	}

	rows, err := d.conn.QueryContext(ctx, `
WITH ranked AS (
	SELECT d.episode_id,
	       COALESCE(d.destination_relative_path, '') AS relative_path,
	       COALESCE(s.starred, 0) AS starred,
	       ROW_NUMBER() OVER (
	           ORDER BY COALESCE(e.published_at, 0) DESC, COALESCE(d.completed_at, 0) DESC, e.id ASC
	       ) AS rn
	FROM podcast_downloads d
	JOIN podcast_episodes e ON e.id = d.episode_id
	JOIN podcast_subscriptions sub ON sub.feed_id = e.feed_id
	LEFT JOIN podcast_episode_state s ON s.episode_id = e.id
	WHERE e.feed_id = ? AND d.status = 'completed'
)
SELECT episode_id, relative_path
FROM ranked
WHERE rn > ? AND starred = 0 AND episode_id <> ?
ORDER BY rn ASC`, feedID, keepLatest, excludeEpisodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PodcastExpiredDownload, 0)
	for rows.Next() {
		var item PodcastExpiredDownload
		if err := rows.Scan(&item.EpisodeID, &item.RelativePath); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// DeleteCompletedDownloadRecord removes a completed podcast_downloads record
// after its disk file is deleted, preserving podcast_episode_state and progress.
func (d *DB) DeleteCompletedDownloadRecord(ctx context.Context, episodeID string) error {
	return d.DeletePodcastCompletedDownload(ctx, episodeID)
}

// PodcastEpisodeFeedID resolves the feed_id for a subscribed episode.
func (d *DB) PodcastEpisodeFeedID(ctx context.Context, episodeID string) (string, error) {
	if err := d.EnsurePodcastSchema(); err != nil {
		return "", err
	}
	var feedID string
	err := d.conn.QueryRowContext(ctx, `
SELECT e.feed_id
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id = e.feed_id
WHERE e.id = ?`, episodeID).Scan(&feedID)
	return feedID, err
}
