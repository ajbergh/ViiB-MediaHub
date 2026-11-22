package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/podcast"
	"github.com/go-chi/chi/v5"
)

func testRichEpisodeRouter(a *API) chi.Router {
	r := chi.NewRouter()
	r.Get("/episodes/{id}", a.getPodcastEpisodeDetail)
	r.Get("/episodes/{id}/chapters", a.getPodcastEpisodeChapters)
	r.Get("/episodes/{id}/transcript", a.getPodcastEpisodeTranscript)
	r.Get("/episodes/{id}/bookmarks", a.listPodcastEpisodeBookmarks)
	r.Post("/episodes/{id}/bookmarks", a.createPodcastEpisodeBookmark)
	r.Delete("/episodes/{id}/bookmarks/{bookmarkId}", a.deletePodcastEpisodeBookmark)
	return r
}

func TestPodcastEpisodeDetailChaptersTranscriptAPI(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "rich.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	feedID, err := database.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:    "https://example.org/rss.xml",
		Title:  "Rich Show",
		Author: "Host",
		Episodes: []podcast.Episode{
			{
				GUID:            "ep-rich",
				Title:           "Rich Episode",
				Description:     "Plain",
				RichDescription: `<p>Safe <strong>HTML</strong></p><script>evil()</script>`,
				EnclosureURL:    "https://cdn.example.org/ep.mp3",
				EnclosureType:   "audio/mpeg",
				EnclosureBytes:  2048,
				WebsiteURL:      "https://example.org/ep-rich",
				EpisodeNumber:   7,
				SeasonNumber:    1,
				ChaptersURL:     "https://cdn.example.org/chapters.json",
				ChaptersType:    "application/json+chapters",
				HasChapters:     true,
				TranscriptURL:   "https://cdn.example.org/tr.vtt",
				TranscriptType:  "text/vtt",
				HasTranscript:   true,
				DurationSeconds: 600,
			},
			{
				GUID:  "ep-bare",
				Title: "Bare Episode",
			},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := database.ListPodcastEpisodes(ctx, feedID, 10)
	if err != nil || len(episodes) != 2 {
		t.Fatalf("episodes: %#v err=%v", episodes, err)
	}
	var richID, bareID string
	for _, ep := range episodes {
		if ep.GUID == "ep-rich" {
			richID = ep.ID
		} else {
			bareID = ep.ID
		}
	}

	a := &API{db: database}
	chCalled := 0
	t.Cleanup(setPodcastChaptersFetcher(a, func(_ context.Context, raw string) ([]podcast.Chapter, error) {
		chCalled++
		if raw != "https://cdn.example.org/chapters.json" {
			t.Fatalf("unexpected chapters URL: %q", raw)
		}
		return []podcast.Chapter{{StartTime: 0, Title: "Intro"}}, nil
	}))
	trCalled := 0
	t.Cleanup(setPodcastTranscriptFetcher(a, func(_ context.Context, raw, declared string) ([]podcast.TranscriptCue, string, error) {
		trCalled++
		if raw != "https://cdn.example.org/tr.vtt" || declared != "text/vtt" {
			t.Fatalf("unexpected transcript args: %q %q", raw, declared)
		}
		return []podcast.TranscriptCue{{StartMs: 0, EndMs: 2000, Speaker: "Host", Text: "Hello"}}, "vtt", nil
	}))

	router := testRichEpisodeRouter(a)

	// Detail 200 without leaking raw URLs
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/episodes/"+richID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "chapters.json") || strings.Contains(body, "tr.vtt") || strings.Contains(body, "ep.mp3") || strings.Contains(body, "<script") {
		t.Fatalf("unsafe data leaked in detail response: %s", body)
	}
	if !strings.Contains(body, `"feedTitle":"Rich Show"`) || !strings.Contains(body, `"hasChapters":true`) {
		t.Fatalf("expected detail fields missing: %s", body)
	}

	// Detail 404
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/episodes/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing episode detail status=%d", rec.Code)
	}

	// Chapters 200 (podcast2) & 200 (none)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/episodes/"+richID+"/chapters", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"source":"podcast2"`) || chCalled != 1 {
		t.Fatalf("chapters response=%d body=%s chCalled=%d", rec.Code, rec.Body.String(), chCalled)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/episodes/"+bareID+"/chapters", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"source":"none"`) || !strings.Contains(rec.Body.String(), `"chapters":[]`) {
		t.Fatalf("empty chapters response=%d body=%s", rec.Code, rec.Body.String())
	}

	// Transcript 200 (vtt) & 200 (none)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/episodes/"+richID+"/transcript", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"format":"vtt"`) || trCalled != 1 {
		t.Fatalf("transcript response=%d body=%s trCalled=%d", rec.Code, rec.Body.String(), trCalled)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/episodes/"+bareID+"/transcript", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"format":"none"`) || !strings.Contains(rec.Body.String(), `"cues":[]`) {
		t.Fatalf("empty transcript response=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPodcastBookmarksAPIStrictValidationAndCRUD(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "bookmarks-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	feedID, err := database.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:      "https://example.org/rss.xml",
		Title:    "Show",
		Episodes: []podcast.Episode{{GUID: "ep-1", Title: "Episode 1"}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := database.ListPodcastEpisodes(ctx, feedID, 5)
	if err != nil || len(episodes) != 1 {
		t.Fatal(err)
	}
	epID := episodes[0].ID

	router := testRichEpisodeRouter(&API{db: database})

	post := func(payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/episodes/"+epID+"/bookmarks", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// Reject unknown fields, negative position, missing positionMs, >1 KiB, >500 runes
	for _, bad := range []string{
		`{"positionMs":100,"note":"ok","extra":1}`,
		`{"positionMs":-5,"note":"ok"}`,
		`{"note":"missing position"}`,
		`{"positionMs":100,"note":"` + strings.Repeat("a", 501) + `"}`,
		`{"positionMs":100,"note":"` + strings.Repeat("x", 1100) + `"}`,
	} {
		rec := post(bad)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unsafe/invalid payload accepted (%s): status=%d body=%s", bad, rec.Code, rec.Body.String())
		}
	}

	// Create valid bookmark
	rec := post(`{"positionMs":42000,"note":"Key point"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Bookmark db.PodcastBookmark `json:"bookmark"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Bookmark.ID == "" {
		t.Fatalf("decode created bookmark: %v body=%s", err, rec.Body.String())
	}

	// List bookmarks
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/episodes/"+epID+"/bookmarks", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Key point") {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Delete bookmark
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/episodes/"+epID+"/bookmarks/"+created.Bookmark.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Repeat delete returns 404
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/episodes/"+epID+"/bookmarks/"+created.Bookmark.ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("repeat delete status=%d", rec.Code)
	}
}
