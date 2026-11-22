package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ajbergh/viib-mediahub/internal/podcast"
	"github.com/google/uuid"
)

const (
	MaxPodcastBookmarkNoteRunes = 500
	maxPodcastBookmarksPerEp    = 500
)

var ErrInvalidBookmark = errors.New("invalid podcast bookmark")

type PodcastEpisodeDetail struct {
	PodcastEpisodeRecord
	FeedTitle        string  `json:"feedTitle"`
	FeedAuthor       string  `json:"feedAuthor"`
	RichDescription  string  `json:"richDescription"`
	ChaptersType     string  `json:"chaptersType,omitempty"`
	TranscriptType   string  `json:"transcriptType,omitempty"`
	BookmarkCount    int64   `json:"bookmarkCount"`
	DownloadStatus   string  `json:"downloadStatus,omitempty"`
	DefaultSpeed     float64 `json:"defaultSpeed,omitempty"`
	SkipIntroSeconds int     `json:"skipIntroSeconds,omitempty"`
	SkipOutroSeconds int     `json:"skipOutroSeconds,omitempty"`
}

type PodcastBookmark struct {
	ID         string `json:"id"`
	EpisodeID  string `json:"episodeId"`
	PositionMs int64  `json:"positionMs"`
	Note       string `json:"note"`
	CreatedAt  int64  `json:"createdAt"`
}

// GetSubscribedEpisodeDetail returns rich episode metadata, show identity,
// bookmark count, and download status only for an actively subscribed show.
// Raw enclosure, chapters, and transcript URLs are never exposed.
func (d *DB) GetSubscribedEpisodeDetail(ctx context.Context, episodeID string) (PodcastEpisodeDetail, error) {
	var detail PodcastEpisodeDetail
	if err := d.EnsurePodcastSchema(); err != nil {
		return detail, err
	}
	var chaptersURL, transcriptURL string
	var played, starred, archived int64
	var customSpeed int
	var rawSpeed float64
	e := &detail.PodcastEpisodeRecord
	err := d.conn.QueryRowContext(ctx, `
SELECT e.id,e.feed_id,e.guid,e.title,e.description,e.rich_description,
 e.enclosure_url,e.enclosure_mime,e.enclosure_bytes,e.website_url,
 e.episode_number,e.season_number,e.chapters_url,e.chapters_type,
 e.transcript_url,e.transcript_type,COALESCE(e.published_at,0),e.duration_ms,
 COALESCE(s.is_played,0),COALESCE(s.position_ms,0),COALESCE(s.listened_ms,0),
 COALESCE(s.revision,0),COALESCE(s.starred,0),COALESCE(s.archived,0),
 f.title,f.author,
 sub.playback_speed_custom,sub.default_speed,sub.skip_intro_seconds,sub.skip_outro_seconds,
 COALESCE(d.status,''),
 (SELECT COUNT(*) FROM podcast_bookmarks b WHERE b.episode_id=e.id)
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id=e.feed_id
JOIN podcast_feeds f ON f.id=e.feed_id
LEFT JOIN podcast_episode_state s ON s.episode_id=e.id
LEFT JOIN podcast_downloads d ON d.episode_id=e.id
WHERE e.id=?`, episodeID).Scan(
		&e.ID, &e.FeedID, &e.GUID, &e.Title, &e.Description, &detail.RichDescription,
		&e.EnclosureURL, &e.EnclosureType, &e.EnclosureBytes, &e.WebsiteURL,
		&e.EpisodeNumber, &e.SeasonNumber, &chaptersURL, &detail.ChaptersType,
		&transcriptURL, &detail.TranscriptType, &e.PublishedAt, &e.DurationMs,
		&played, &e.PositionMs, &e.ListenedMs,
		&e.ProgressRevision, &starred, &archived,
		&detail.FeedTitle, &detail.FeedAuthor,
		&customSpeed, &rawSpeed, &detail.SkipIntroSeconds, &detail.SkipOutroSeconds,
		&detail.DownloadStatus, &detail.BookmarkCount,
	)
	if err != nil {
		return PodcastEpisodeDetail{}, err
	}
	e.IsPlayed = played == 1
	e.Starred = starred == 1
	e.Archived = archived == 1
	e.HasAudio = e.EnclosureURL != ""
	e.HasChapters = chaptersURL != ""
	e.HasTranscript = transcriptURL != ""
	detail.DefaultSpeed = normalizePodcastShowSpeed(customSpeed, rawSpeed)
	detail.SkipIntroSeconds = clampSkipTrim(detail.SkipIntroSeconds)
	detail.SkipOutroSeconds = clampSkipTrim(detail.SkipOutroSeconds)
	return detail, nil
}

// PodcastEpisodeChaptersSource returns the backend-private chapters URL and
// declared MIME type only for an actively subscribed episode.
func (d *DB) PodcastEpisodeChaptersSource(ctx context.Context, episodeID string) (string, string, error) {
	if err := d.EnsurePodcastSchema(); err != nil {
		return "", "", err
	}
	var raw, declared string
	err := d.conn.QueryRowContext(ctx, `
SELECT e.chapters_url,e.chapters_type
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id=e.feed_id
WHERE e.id=?`, episodeID).Scan(&raw, &declared)
	if err != nil {
		return "", "", err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	u, err := podcast.ValidatePublicURL(raw, false)
	if err != nil {
		return "", "", err
	}
	return u.String(), declared, nil
}

// PodcastEpisodeTranscriptSource returns the backend-private transcript URL
// and declared MIME type only for an actively subscribed episode.
func (d *DB) PodcastEpisodeTranscriptSource(ctx context.Context, episodeID string) (string, string, error) {
	if err := d.EnsurePodcastSchema(); err != nil {
		return "", "", err
	}
	var raw, declared string
	err := d.conn.QueryRowContext(ctx, `
SELECT e.transcript_url,e.transcript_type
FROM podcast_episodes e
JOIN podcast_subscriptions sub ON sub.feed_id=e.feed_id
WHERE e.id=?`, episodeID).Scan(&raw, &declared)
	if err != nil {
		return "", "", err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	u, err := podcast.ValidatePublicURL(raw, false)
	if err != nil {
		return "", "", err
	}
	return u.String(), declared, nil
}

func (d *DB) verifySubscribedPodcastEpisode(ctx context.Context, episodeID string) error {
	var id string
	return d.conn.QueryRowContext(ctx, `
SELECT e.id FROM podcast_episodes e
JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
WHERE e.id=?`, episodeID).Scan(&id)
}

func (d *DB) ListEpisodeBookmarks(ctx context.Context, episodeID string) ([]PodcastBookmark, error) {
	if err := d.EnsurePodcastSchema(); err != nil {
		return nil, err
	}
	if err := d.verifySubscribedPodcastEpisode(ctx, episodeID); err != nil {
		return nil, err
	}
	rows, err := d.conn.QueryContext(ctx, `
SELECT id,episode_id,position_ms,note,created_at
FROM podcast_bookmarks
WHERE episode_id=?
ORDER BY position_ms ASC, created_at ASC, id ASC
LIMIT ?`, episodeID, maxPodcastBookmarksPerEp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PodcastBookmark, 0)
	for rows.Next() {
		var b PodcastBookmark
		if err := rows.Scan(&b.ID, &b.EpisodeID, &b.PositionMs, &b.Note, &b.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

func (d *DB) CreateEpisodeBookmark(ctx context.Context, episodeID string, positionMs int64, note string) (PodcastBookmark, error) {
	if positionMs < 0 || positionMs > maxPodcastProgressMillis {
		return PodcastBookmark{}, fmt.Errorf("%w: position out of range", ErrInvalidBookmark)
	}
	if !utf8.ValidString(note) || strings.ContainsRune(note, '\x00') {
		return PodcastBookmark{}, fmt.Errorf("%w: invalid note text", ErrInvalidBookmark)
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > MaxPodcastBookmarkNoteRunes {
		return PodcastBookmark{}, fmt.Errorf("%w: note exceeds %d characters", ErrInvalidBookmark, MaxPodcastBookmarkNoteRunes)
	}
	if err := d.EnsurePodcastSchema(); err != nil {
		return PodcastBookmark{}, err
	}
	if err := d.verifySubscribedPodcastEpisode(ctx, episodeID); err != nil {
		return PodcastBookmark{}, err
	}
	bookmark := PodcastBookmark{
		ID:         uuid.NewString(),
		EpisodeID:  episodeID,
		PositionMs: positionMs,
		Note:       note,
		CreatedAt:  time.Now().UnixMilli(),
	}
	_, err := d.conn.ExecContext(ctx, `
INSERT INTO podcast_bookmarks (id,episode_id,position_ms,note,created_at)
VALUES (?,?,?,?,?)`,
		bookmark.ID, bookmark.EpisodeID, bookmark.PositionMs, bookmark.Note, bookmark.CreatedAt)
	if err != nil {
		return PodcastBookmark{}, fmt.Errorf("create podcast bookmark: %w", err)
	}
	return bookmark, nil
}

func (d *DB) DeleteEpisodeBookmark(ctx context.Context, episodeID, bookmarkID string) error {
	if err := d.EnsurePodcastSchema(); err != nil {
		return err
	}
	res, err := d.conn.ExecContext(ctx, `
DELETE FROM podcast_bookmarks
WHERE id=? AND episode_id=?
  AND EXISTS (
    SELECT 1 FROM podcast_episodes e
    JOIN podcast_subscriptions s ON s.feed_id=e.feed_id
    WHERE e.id=podcast_bookmarks.episode_id
  )`, bookmarkID, episodeID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
