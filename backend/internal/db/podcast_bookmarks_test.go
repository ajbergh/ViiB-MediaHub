package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/podcast"
)

func TestPodcastEpisodeDetailAndRichMetadataRoundTrip(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "detail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	feedID, err := database.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:    "https://example.org/feed.xml",
		Title:  "Deep Tech Show",
		Author: "Host Name",
		Episodes: []podcast.Episode{{
			GUID:            "ep-1",
			Title:           "Episode One",
			Description:     "Plain summary",
			RichDescription: `<p>Rich <strong>notes</strong> <a href="https://example.org/link">ref</a></p><script>bad()</script>`,
			EnclosureURL:    "https://cdn.example.org/ep1.mp3",
			EnclosureType:   "audio/mpeg",
			EnclosureBytes:  1048576,
			WebsiteURL:      "https://example.org/ep1",
			EpisodeNumber:   1,
			SeasonNumber:    2,
			ChaptersURL:     "https://cdn.example.org/ep1.chapters.json",
			ChaptersType:    "application/json+chapters",
			HasChapters:     true,
			TranscriptURL:   "https://cdn.example.org/ep1.vtt",
			TranscriptType:  "text/vtt",
			HasTranscript:   true,
			PublishedAt:     time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
			DurationSeconds: 1800,
		}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}

	episodes, _, err := database.ListPodcastEpisodesPage(ctx, feedID, 10, 0)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("ListPodcastEpisodesPage: %#v err=%v", episodes, err)
	}
	ep := episodes[0]
	if !ep.HasChapters || !ep.HasTranscript || ep.WebsiteURL != "https://example.org/ep1" ||
		ep.EnclosureBytes != 1048576 || ep.EpisodeNumber != 1 || ep.SeasonNumber != 2 {
		t.Fatalf("episode metadata mismatch: %#v", ep)
	}

	detail, err := database.GetSubscribedEpisodeDetail(ctx, ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.FeedTitle != "Deep Tech Show" || detail.FeedAuthor != "Host Name" ||
		detail.FeedID != feedID || detail.BookmarkCount != 0 ||
		strings.Contains(detail.RichDescription, "<script") {
		t.Fatalf("detail mismatch: %#v", detail)
	}
	rawJSON, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	s := string(rawJSON)
	if strings.Contains(s, "ep1.mp3") || strings.Contains(s, "ep1.chapters.json") || strings.Contains(s, "ep1.vtt") {
		t.Fatalf("raw internal URL leaked in detail JSON: %s", s)
	}

	chURL, chType, err := database.PodcastEpisodeChaptersSource(ctx, ep.ID)
	if err != nil || chURL != "https://cdn.example.org/ep1.chapters.json" || chType != "application/json+chapters" {
		t.Fatalf("chapters source mismatch: %q %q %v", chURL, chType, err)
	}
	trURL, trType, err := database.PodcastEpisodeTranscriptSource(ctx, ep.ID)
	if err != nil || trURL != "https://cdn.example.org/ep1.vtt" || trType != "text/vtt" {
		t.Fatalf("transcript source mismatch: %q %q %v", trURL, trType, err)
	}

	// Unsubscribing blocks detail and internal source reads.
	if err := database.UnsubscribePodcast(ctx, feedID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetSubscribedEpisodeDetail(ctx, ep.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows after unsubscribe, got %v", err)
	}
	if _, _, err := database.PodcastEpisodeChaptersSource(ctx, ep.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows for chapters after unsubscribe, got %v", err)
	}
}

func TestPodcastBookmarksCRUDAndValidation(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "bookmarks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	feedID, err := database.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:      "https://example.org/feed.xml",
		Title:    "Show",
		Episodes: []podcast.Episode{{GUID: "ep-b", Title: "Ep B"}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := database.ListPodcastEpisodes(ctx, feedID, 5)
	if err != nil || len(episodes) != 1 {
		t.Fatal(err)
	}
	epID := episodes[0].ID

	if _, err := database.CreateEpisodeBookmark(ctx, "unknown", 1000, "note"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows for unknown episode, got %v", err)
	}
	if _, err := database.CreateEpisodeBookmark(ctx, epID, -1, "note"); !errors.Is(err, ErrInvalidBookmark) {
		t.Fatalf("expected ErrInvalidBookmark for negative position, got %v", err)
	}
	if _, err := database.CreateEpisodeBookmark(ctx, epID, 1000, strings.Repeat("é", MaxPodcastBookmarkNoteRunes+1)); !errors.Is(err, ErrInvalidBookmark) {
		t.Fatalf("expected ErrInvalidBookmark for >500 runes, got %v", err)
	}

	b1, err := database.CreateEpisodeBookmark(ctx, epID, 90000, "Second chronologically")
	if err != nil {
		t.Fatal(err)
	}
	b2, err := database.CreateEpisodeBookmark(ctx, epID, 15000, "First chronologically")
	if err != nil {
		t.Fatal(err)
	}

	list, err := database.ListEpisodeBookmarks(ctx, epID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list mismatch: %#v err=%v", list, err)
	}
	if list[0].ID != b2.ID || list[1].ID != b1.ID {
		t.Fatalf("expected bookmarks ordered by positionMs ascending: %#v", list)
	}

	detail, err := database.GetSubscribedEpisodeDetail(ctx, epID)
	if err != nil || detail.BookmarkCount != 2 {
		t.Fatalf("bookmarkCount mismatch: %#v err=%v", detail, err)
	}

	if err := database.DeleteEpisodeBookmark(ctx, epID, b2.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteEpisodeBookmark(ctx, epID, b2.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows on repeat delete, got %v", err)
	}
}
