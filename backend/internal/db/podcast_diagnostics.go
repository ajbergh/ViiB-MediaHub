// podcast_diagnostics.go collects strictly redacted aggregate podcast metrics
// for support bundles and internal observability. Never include feed URLs,
// show/episode titles, episode IDs, file paths, or credentials.
package db

import (
	"context"
	"regexp"
	"time"
)

var safePodcastDiagCode = regexp.MustCompile(`^[a-z0-9_]{1,48}$`)

// PodcastSupportDiagnostics contains only aggregate counts and error-code
// histograms safe to include in exported support archives.
type PodcastSupportDiagnostics struct {
	SubscribedCount      int64            `json:"subscribedCount"`
	EpisodeCount         int64            `json:"episodeCount"`
	QueueCount           int64            `json:"queueCount"`
	BookmarkCount        int64            `json:"bookmarkCount"`
	DownloadBytesUsed    int64            `json:"downloadBytesUsed"`
	DownloadsByStatus    map[string]int64 `json:"downloadsByStatus"`
	FeedErrorsByCode     map[string]int64 `json:"feedErrorsByCode"`
	DownloadErrorsByCode map[string]int64 `json:"downloadErrorsByCode"`
}

func sanitizePodcastDiagCode(code string) string {
	if safePodcastDiagCode.MatchString(code) {
		return code
	}
	return "other"
}

// PodcastDiagnostics queries redacted aggregate counts and error histograms,
// initializing the podcast schema first so fresh databases query cleanly.
func (d *DB) PodcastDiagnostics(ctx context.Context) (PodcastSupportDiagnostics, error) {
	out := PodcastSupportDiagnostics{
		DownloadsByStatus:    map[string]int64{},
		FeedErrorsByCode:     map[string]int64{},
		DownloadErrorsByCode: map[string]int64{},
	}
	if err := d.EnsurePodcastSchema(); err != nil {
		return out, err
	}

	if err := d.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM podcast_subscriptions`).Scan(&out.SubscribedCount); err != nil {
		return out, err
	}
	if err := d.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM podcast_episodes`).Scan(&out.EpisodeCount); err != nil {
		return out, err
	}
	if err := d.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM podcast_queue`).Scan(&out.QueueCount); err != nil {
		return out, err
	}
	if err := d.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM podcast_bookmarks`).Scan(&out.BookmarkCount); err != nil {
		return out, err
	}

	used, err := d.PodcastOfflineStorage(ctx)
	if err != nil {
		return out, err
	}
	out.DownloadBytesUsed = used

	statusRows, err := d.conn.QueryContext(ctx, `SELECT status, COUNT(*) FROM podcast_downloads GROUP BY status`)
	if err != nil {
		return out, err
	}
	for statusRows.Next() {
		var status string
		var count int64
		if scanErr := statusRows.Scan(&status, &count); scanErr != nil {
			_ = statusRows.Close()
			return out, scanErr
		}
		out.DownloadsByStatus[sanitizePodcastDiagCode(status)] += count
	}
	if err := statusRows.Err(); err != nil {
		_ = statusRows.Close()
		return out, err
	}
	_ = statusRows.Close()

	feedErrRows, err := d.conn.QueryContext(ctx, `
		SELECT COALESCE(last_error_code, ''), COUNT(*)
		FROM podcast_feeds
		WHERE COALESCE(last_error_code, '') <> ''
		GROUP BY last_error_code`)
	if err != nil {
		return out, err
	}
	for feedErrRows.Next() {
		var code string
		var count int64
		if scanErr := feedErrRows.Scan(&code, &count); scanErr != nil {
			_ = feedErrRows.Close()
			return out, scanErr
		}
		out.FeedErrorsByCode[sanitizePodcastDiagCode(code)] += count
	}
	if err := feedErrRows.Err(); err != nil {
		_ = feedErrRows.Close()
		return out, err
	}
	_ = feedErrRows.Close()

	dlErrRows, err := d.conn.QueryContext(ctx, `
		SELECT COALESCE(error_code, ''), COUNT(*)
		FROM podcast_downloads
		WHERE COALESCE(error_code, '') <> ''
		GROUP BY error_code`)
	if err != nil {
		return out, err
	}
	for dlErrRows.Next() {
		var code string
		var count int64
		if scanErr := dlErrRows.Scan(&code, &count); scanErr != nil {
			_ = dlErrRows.Close()
			return out, scanErr
		}
		out.DownloadErrorsByCode[sanitizePodcastDiagCode(code)] += count
	}
	if err := dlErrRows.Err(); err != nil {
		_ = dlErrRows.Close()
		return out, err
	}
	_ = dlErrRows.Close()

	return out, nil
}

// CountActivePodcastRefreshes returns the number of subscribed feeds with an
// active in-flight refresh lease.
func (d *DB) CountActivePodcastRefreshes(ctx context.Context) (int64, error) {
	if err := d.EnsurePodcastSchema(); err != nil {
		return 0, err
	}
	var count int64
	err := d.conn.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM podcast_feeds f
		JOIN podcast_subscriptions s ON s.feed_id = f.id
		WHERE COALESCE(f.refresh_lease_until, 0) > ?`,
		time.Now().UnixMilli(),
	).Scan(&count)
	return count, err
}
