package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/podcast"
)

func boolPtr(v bool) *bool    { return &v }
func int64Ptr(v int64) *int64 { return &v }

func TestPodcastTriagePatchAndViews(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "triage-views.db")
	d, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	baseTime := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	feedA, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:    "https://example.org/show-a.xml",
		Title:  "Alpha Show",
		Author: "Alice Host",
		Episodes: []podcast.Episode{
			{GUID: "a1", Title: "Episode 100% Real", Description: "Intro episode", EnclosureURL: "https://cdn.example.org/a1.mp3", EnclosureType: "audio/mpeg", PublishedAt: baseTime.Add(4 * time.Hour)},
			{GUID: "a2", Title: "Episode_Two", Description: "Deep dive", EnclosureURL: "https://cdn.example.org/a2.mp3", EnclosureType: "audio/mpeg", PublishedAt: baseTime.Add(3 * time.Hour)},
			{GUID: "a3", Title: "Episode Three", Description: "Backslash \\ path", EnclosureURL: "https://cdn.example.org/a3.mp3", EnclosureType: "audio/mpeg", PublishedAt: baseTime.Add(2 * time.Hour)},
			{GUID: "a4", Title: "Episode Four", Description: "Archived ep", EnclosureURL: "https://cdn.example.org/a4.mp3", EnclosureType: "audio/mpeg", PublishedAt: baseTime.Add(1 * time.Hour)},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	// Unsubscribed show should never appear in views or allow patching.
	feedUnsub, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:   "https://example.org/unsub.xml",
		Title: "Hidden Show",
		Episodes: []podcast.Episode{
			{GUID: "u1", Title: "Unsubscribed Episode", EnclosureURL: "https://cdn.example.org/u1.mp3", EnclosureType: "audio/mpeg"},
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	unsubEps, err := d.ListPodcastEpisodes(ctx, feedUnsub, 10)
	if err != nil || len(unsubEps) != 1 {
		t.Fatalf("unsub eps: %+v %v", unsubEps, err)
	}
	if _, err := d.PatchPodcastEpisodeState(ctx, unsubEps[0].ID, nil, boolPtr(true), nil, nil, nil, nil); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows for unsubscribed episode, got %v", err)
	}

	eps, err := d.ListPodcastEpisodes(ctx, feedA, 10)
	if err != nil || len(eps) != 4 {
		t.Fatalf("episodes: %+v %v", eps, err)
	}
	ep1, ep2, ep3, ep4 := eps[0], eps[1], eps[2], eps[3]

	// ExpectedRevision conflict on new row (expected 1 != 0).
	if _, err := d.PatchPodcastEpisodeState(ctx, ep1.ID, int64Ptr(1), boolPtr(true), nil, nil, nil, nil); !errors.Is(err, ErrPodcastProgressConflict) {
		t.Fatalf("expected conflict on initial revision mismatch, got %v", err)
	}
	// Patch ep1: star + in-progress.
	st1, err := d.PatchPodcastEpisodeState(ctx, ep1.ID, int64Ptr(0), boolPtr(true), nil, nil, int64Ptr(15000), int64Ptr(15000))
	if err != nil || !st1.Starred || st1.PositionMs != 15000 || st1.ListenedMs != 15000 || st1.Revision != 1 || st1.Archived || st1.IsPlayed {
		t.Fatalf("unexpected st1: %+v %v", st1, err)
	}
	// Stale revision rejected.
	if _, err := d.PatchPodcastEpisodeState(ctx, ep1.ID, int64Ptr(0), boolPtr(false), nil, nil, nil, nil); !errors.Is(err, ErrPodcastProgressConflict) {
		t.Fatalf("expected conflict on stale revision, got %v", err)
	}
	// Patch ep1 again without expectedRevision: backward seek, monotonic listenedMs.
	st1b, err := d.PatchPodcastEpisodeState(ctx, ep1.ID, nil, nil, nil, nil, int64Ptr(5000), int64Ptr(10000))
	if err != nil || st1b.PositionMs != 5000 || st1b.ListenedMs != 15000 || st1b.Revision != 2 || !st1b.Starred {
		t.Fatalf("unexpected st1b: %+v %v", st1b, err)
	}

	// Patch ep2: played + starred.
	st2, err := d.PatchPodcastEpisodeState(ctx, ep2.ID, nil, boolPtr(true), nil, boolPtr(true), int64Ptr(60000), int64Ptr(60000))
	if err != nil || !st2.IsPlayed || st2.CompletedAt == 0 || !st2.Starred {
		t.Fatalf("unexpected st2: %+v %v", st2, err)
	}
	// Patch ep4: archived (with position > 0, archived should exclude it from inbox & in_progress).
	if _, err := d.PatchPodcastEpisodeState(ctx, ep4.ID, nil, nil, boolPtr(true), nil, int64Ptr(2000), nil); err != nil {
		t.Fatal(err)
	}

	// Mark ep3 as completed download so DownloadStatus surfaces in view.
	if _, err := d.EnqueuePodcastDownload(ctx, ep3.ID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := d.ClaimPodcastDownload(ctx, "owner-1", time.Now()); err != nil || claimed != ep3.ID {
		t.Fatalf("claim: %q %v", claimed, err)
	}
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := d.CompletePodcastDownload(ctx, ep3.ID, "owner-1", ep3.ID+".audio", "audio/mpeg", digest, 1024); err != nil {
		t.Fatal(err)
	}

	assertViewIDs := func(view, q string, wantIDs ...string) {
		t.Helper()
		got, more, err := d.ListPodcastEpisodesByView(ctx, view, q, 50, 0)
		if err != nil {
			t.Fatalf("view %s (%q) failed: %v", view, q, err)
		}
		if more {
			t.Fatalf("unexpected hasMore for view %s", view)
		}
		if len(got) != len(wantIDs) {
			t.Fatalf("view %s (%q) got %d items, want %d: %+v", view, q, len(got), len(wantIDs), got)
		}
		for i, want := range wantIDs {
			if got[i].ID != want {
				t.Fatalf("view %s[%d]: got %s, want %s", view, i, got[i].ID, want)
			}
		}
	}

	assertViewIDs("inbox", "", ep1.ID, ep3.ID)
	assertViewIDs("starred", "", ep1.ID, ep2.ID)
	assertViewIDs("in_progress", "", ep1.ID)
	assertViewIDs("played", "", ep2.ID)
	assertViewIDs("archived", "", ep4.ID)
	assertViewIDs("all", "", ep1.ID, ep2.ID, ep3.ID, ep4.ID)

	// Verify DownloadStatus and FeedAuthor in view result.
	allEps, _, err := d.ListPodcastEpisodesByView(ctx, "all", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if allEps[2].DownloadStatus != "completed" || allEps[0].FeedAuthor != "Alice Host" || allEps[0].FeedTitle != "Alpha Show" {
		t.Fatalf("unexpected view metadata: %+v", allEps)
	}

	// LIKE wildcard escaping in view filter.
	assertViewIDs("all", "100%", ep1.ID)
	assertViewIDs("all", "Episode_Two", ep2.ID)
	assertViewIDs("all", `\`, ep3.ID)
	assertViewIDs("all", "Alpha Show", ep1.ID, ep2.ID, ep3.ID, ep4.ID)

	// Pagination check.
	page1, hasMore, err := d.ListPodcastEpisodesByView(ctx, "all", "", 2, 0)
	if err != nil || !hasMore || len(page1) != 2 || page1[0].ID != ep1.ID || page1[1].ID != ep2.ID {
		t.Fatalf("page1: len=%d more=%v err=%v", len(page1), hasMore, err)
	}
	page2, hasMore, err := d.ListPodcastEpisodesByView(ctx, "all", "", 2, 2)
	if err != nil || hasMore || len(page2) != 2 || page2[0].ID != ep3.ID || page2[1].ID != ep4.ID {
		t.Fatalf("page2: len=%d more=%v err=%v", len(page2), hasMore, err)
	}

	// Invalid view rejected.
	if _, _, err := d.ListPodcastEpisodesByView(ctx, "bogus", "", 10, 0); !errors.Is(err, ErrInvalidPodcastView) {
		t.Fatalf("expected ErrInvalidPodcastView, got %v", err)
	}
}

func TestPodcastSearchEpisodesAcrossShowsAndWildcards(t *testing.T) {
	ctx := context.Background()
	d, err := New(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	_, err = d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:    "https://example.org/tech.xml",
		Title:  "Tech Weekly",
		Author: "Jane Engineer",
		Episodes: []podcast.Episode{
			{GUID: "t1", Title: "SQLite 100% Guide", Description: "Fast queries", EnclosureURL: "https://cdn.example.org/t1.mp3", PublishedAt: now},
			{GUID: "t2", Title: "Go Concurrency", Description: "Channels_and_locks", EnclosureURL: "https://cdn.example.org/t2.mp3", PublishedAt: now.Add(-time.Hour)},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:    "https://example.org/history.xml",
		Title:  "History Hour",
		Author: "Dr. Bob",
		Episodes: []podcast.Episode{
			{GUID: "h1", Title: "Ancient Rome", Description: "Includes SQLite mention in notes", EnclosureURL: "https://cdn.example.org/h1.mp3", PublishedAt: now.Add(-2 * time.Hour)},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	// Unsubscribed show matching "SQLite" must not be returned.
	_, err = d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:    "https://example.org/unsub-tech.xml",
		Title:  "Unsub Tech",
		Author: "Jane Engineer",
		Episodes: []podcast.Episode{
			{GUID: "u1", Title: "SQLite Secrets", EnclosureURL: "https://cdn.example.org/u1.mp3", PublishedAt: now},
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	// Cross-show search matches episode title first, then description.
	res, more, err := d.SearchPodcastEpisodes(ctx, "SQLite", 10, 0)
	if err != nil || more || len(res) != 2 {
		t.Fatalf("search SQLite: %+v more=%v err=%v", res, more, err)
	}
	if res[0].Title != "SQLite 100% Guide" || res[1].Title != "Ancient Rome" {
		t.Fatalf("unexpected ordering: %+v", res)
	}

	// Wildcard characters % and _ must not act as SQL wildcards.
	pct, _, err := d.SearchPodcastEpisodes(ctx, "100%", 10, 0)
	if err != nil || len(pct) != 1 || pct[0].Title != "SQLite 100% Guide" {
		t.Fatalf("wildcard %% escaped failed: %+v %v", pct, err)
	}
	under, _, err := d.SearchPodcastEpisodes(ctx, "_and_", 10, 0)
	if err != nil || len(under) != 1 || under[0].Title != "Go Concurrency" {
		t.Fatalf("wildcard _ escaped failed: %+v %v", under, err)
	}

	// Author match across shows.
	authorRes, _, err := d.SearchPodcastEpisodes(ctx, "Dr. Bob", 10, 0)
	if err != nil || len(authorRes) != 1 || authorRes[0].FeedTitle != "History Hour" {
		t.Fatalf("author search failed: %+v %v", authorRes, err)
	}

	// Blank query returns empty without error.
	empty, more, err := d.SearchPodcastEpisodes(ctx, "   ", 10, 0)
	if err != nil || more || len(empty) != 0 {
		t.Fatalf("blank search: %+v more=%v err=%v", empty, more, err)
	}
}

func TestPodcastDurableQueueOperations(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	d, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	feedID, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:    "https://example.org/q.xml",
		Title:  "Queue Show",
		Author: "Host Q",
		Episodes: []podcast.Episode{
			{GUID: "q1", Title: "Ep 1", EnclosureURL: "https://cdn.example.org/q1.mp3", EnclosureType: "audio/mpeg"},
			{GUID: "q2", Title: "Ep 2", EnclosureURL: "https://cdn.example.org/q2.mp3", EnclosureType: "audio/mpeg"},
			{GUID: "q3", Title: "Ep 3", EnclosureURL: "https://cdn.example.org/q3.mp3", EnclosureType: "audio/mpeg"},
			{GUID: "q4-no-audio", Title: "Text Note", EnclosureURL: ""},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	eps, err := d.ListPodcastEpisodes(ctx, feedID, 10)
	if err != nil || len(eps) != 4 {
		t.Fatalf("episodes: %+v %v", eps, err)
	}
	byTitle := map[string]PodcastEpisodeRecord{}
	for _, e := range eps {
		byTitle[e.Title] = e
	}
	e1, e2, e3, e4 := byTitle["Ep 1"].ID, byTitle["Ep 2"].ID, byTitle["Ep 3"].ID, byTitle["Text Note"].ID

	// Validation checks.
	if err := d.AddToPodcastQueue(ctx, "missing", "end"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing episode: %v", err)
	}
	if err := d.AddToPodcastQueue(ctx, e4, "end"); !errors.Is(err, ErrPodcastEpisodeNoAudio) {
		t.Fatalf("no-audio episode: %v", err)
	}
	if err := d.AddToPodcastQueue(ctx, e1, "middle"); !errors.Is(err, ErrInvalidPodcastQueue) {
		t.Fatalf("invalid position: %v", err)
	}

	// Add e1 to end, e2 to end, e3 to next -> [e3, e1, e2]
	if err := d.AddToPodcastQueue(ctx, e1, "end"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddToPodcastQueue(ctx, e2, "end"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddToPodcastQueue(ctx, e3, "next"); err != nil {
		t.Fatal(err)
	}
	// Adding e2 again with "end" is idempotent (keeps its order).
	if err := d.AddToPodcastQueue(ctx, e2, "end"); err != nil {
		t.Fatal(err)
	}
	// Adding e2 with "next" moves it to the front -> [e2, e3, e1]
	if err := d.AddToPodcastQueue(ctx, e2, "next"); err != nil {
		t.Fatal(err)
	}

	// Star e2 and set position on e1 to verify queue joins state.
	if _, err := d.PatchPodcastEpisodeState(ctx, e2, nil, boolPtr(true), nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PatchPodcastEpisodeState(ctx, e1, nil, nil, nil, nil, int64Ptr(12345), nil); err != nil {
		t.Fatal(err)
	}

	q, err := d.ListPodcastQueue(ctx)
	if err != nil || len(q) != 3 {
		t.Fatalf("queue: %+v %v", q, err)
	}
	if q[0].ID != e2 || q[1].ID != e3 || q[2].ID != e1 {
		t.Fatalf("unexpected queue order: %s, %s, %s", q[0].ID, q[1].ID, q[2].ID)
	}
	if !q[0].Starred || q[2].PositionMs != 12345 || q[0].FeedTitle != "Queue Show" || q[0].FeedAuthor != "Host Q" {
		t.Fatalf("queue joined metadata missing: %+v", q)
	}

	// ReplacePodcastQueue failure must leave existing queue intact.
	if err := d.ReplacePodcastQueue(ctx, []string{e1, e4}); !errors.Is(err, ErrPodcastEpisodeNoAudio) {
		t.Fatalf("expected ErrPodcastEpisodeNoAudio on replace, got %v", err)
	}
	if err := d.ReplacePodcastQueue(ctx, []string{e1, e1}); !errors.Is(err, ErrInvalidPodcastQueue) {
		t.Fatalf("expected ErrInvalidPodcastQueue on duplicate, got %v", err)
	}
	// Replace successfully -> [e3, e2]
	if err := d.ReplacePodcastQueue(ctx, []string{e3, e2}); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveFromPodcastQueue(ctx, e3); err != nil {
		t.Fatal(err)
	}

	// Verify durability across DB reopen.
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	qAfter, err := reopened.ListPodcastQueue(ctx)
	if err != nil || len(qAfter) != 1 || qAfter[0].ID != e2 {
		t.Fatalf("reopened queue mismatch: %+v %v", qAfter, err)
	}
}

func TestPodcastRetentionPolicyAndSelection(t *testing.T) {
	ctx := context.Background()
	d, err := New(filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	base := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	feedID, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:   "https://example.org/ret.xml",
		Title: "Retention Show",
		Episodes: []podcast.Episode{
			{GUID: "r1", Title: "Newest", EnclosureURL: "https://cdn.example.org/r1.mp3", EnclosureType: "audio/mpeg", PublishedAt: base.Add(4 * time.Hour)},
			{GUID: "r2", Title: "Second", EnclosureURL: "https://cdn.example.org/r2.mp3", EnclosureType: "audio/mpeg", PublishedAt: base.Add(3 * time.Hour)},
			{GUID: "r3", Title: "Third Starred", EnclosureURL: "https://cdn.example.org/r3.mp3", EnclosureType: "audio/mpeg", PublishedAt: base.Add(2 * time.Hour)},
			{GUID: "r4", Title: "Fourth Active", EnclosureURL: "https://cdn.example.org/r4.mp3", EnclosureType: "audio/mpeg", PublishedAt: base.Add(1 * time.Hour)},
			{GUID: "r5", Title: "Oldest", EnclosureURL: "https://cdn.example.org/r5.mp3", EnclosureType: "audio/mpeg", PublishedAt: base},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}

	// Policy CRUD & validation.
	pol, err := d.GetPodcastRetentionPolicy(ctx, feedID)
	if err != nil || pol.KeepLatestCount != 0 {
		t.Fatalf("initial retention: %+v %v", pol, err)
	}
	if _, err := d.SetPodcastRetentionPolicy(ctx, feedID, -1); !errors.Is(err, ErrInvalidPodcastRetention) {
		t.Fatalf("negative keep_latest_count accepted: %v", err)
	}
	if _, err := d.SetPodcastRetentionPolicy(ctx, feedID, 51); !errors.Is(err, ErrInvalidPodcastRetention) {
		t.Fatalf("keep_latest_count > 50 accepted: %v", err)
	}
	if _, err := d.SetPodcastRetentionPolicy(ctx, "missing", 2); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unfollowed retention updated: %v", err)
	}

	eps, err := d.ListPodcastEpisodes(ctx, feedID, 10)
	if err != nil || len(eps) != 5 {
		t.Fatalf("episodes: %+v %v", eps, err)
	}
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, ep := range eps {
		if _, err := d.EnqueuePodcastDownload(ctx, ep.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ClaimPodcastDownload(ctx, "w1", time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := d.CompletePodcastDownload(ctx, ep.ID, "w1", ep.ID+".audio", "audio/mpeg", digest, 512); err != nil {
			t.Fatal(err)
		}
	}

	// Star eps[2] ("Third Starred") and give eps[4] ("Oldest") saved listening progress.
	if _, err := d.PatchPodcastEpisodeState(ctx, eps[2].ID, nil, boolPtr(true), nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PatchPodcastEpisodeState(ctx, eps[4].ID, nil, nil, nil, nil, int64Ptr(42000), int64Ptr(42000)); err != nil {
		t.Fatal(err)
	}

	// keep_latest_count = 0 -> nothing expired.
	expired, err := d.SelectExpiredCompletedDownloads(ctx, feedID, "")
	if err != nil || len(expired) != 0 {
		t.Fatalf("keep=0 expired: %+v %v", expired, err)
	}

	// keep_latest_count = 1 -> eps[0] kept, eps[2] starred excluded, eps[3] active excluded when passed.
	if _, err := d.SetPodcastRetentionPolicy(ctx, feedID, 1); err != nil {
		t.Fatal(err)
	}
	expired, err = d.SelectExpiredCompletedDownloads(ctx, feedID, eps[3].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 2 || expired[0].EpisodeID != eps[1].ID || expired[1].EpisodeID != eps[4].ID {
		t.Fatalf("unexpected expired downloads: %+v", expired)
	}

	// Deleting completed download record preserves episode state/progress.
	if err := d.DeleteCompletedDownloadRecord(ctx, eps[4].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetPodcastDownload(ctx, eps[4].ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected download record deleted, got %v", err)
	}
	prog, err := d.GetPodcastProgress(ctx, eps[4].ID)
	if err != nil || prog.PositionMs != 42000 {
		t.Fatalf("episode progress lost after download cleanup: %+v %v", prog, err)
	}
}
