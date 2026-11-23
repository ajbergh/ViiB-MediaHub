package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/podcast"
	"github.com/go-chi/chi/v5"
)

func newPodcastTriageTestRouter(a *API) chi.Router {
	r := chi.NewRouter()
	r.Patch("/episodes/{id}/state", a.patchPodcastEpisodeState)
	r.Get("/episodes", a.listPodcastEpisodesByView)
	r.Get("/episodes/search", a.searchPodcastEpisodes)
	r.Get("/queue", a.getPodcastQueue)
	r.Post("/queue", a.addPodcastQueue)
	r.Put("/queue", a.replacePodcastQueue)
	r.Delete("/queue/{episodeId}", a.deletePodcastQueueItem)
	r.Get("/feeds/{id}/retention", a.getPodcastRetention)
	r.Put("/feeds/{id}/retention", a.putPodcastRetention)
	return r
}

func TestPodcastEpisodeStatePatchAndViewsAPI(t *testing.T) {
	ctx := context.Background()
	d, err := db.New(filepath.Join(t.TempDir(), "triage-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	base := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	feedID, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:    "https://example.org/show.xml",
		Title:  "Morning Brief",
		Author: "Reporter",
		Episodes: []podcast.Episode{
			{GUID: "e1", Title: "First 100% Story", Description: "Summary A", EnclosureURL: "https://cdn.example.org/e1.mp3?token=secret", EnclosureType: "audio/mpeg", PublishedAt: base.Add(2 * time.Hour)},
			{GUID: "e2", Title: "Second Story", Description: "Summary B", EnclosureURL: "https://cdn.example.org/e2.mp3", EnclosureType: "audio/mpeg", PublishedAt: base.Add(time.Hour)},
			{GUID: "e3", Title: "Third Story", Description: "No audio note", EnclosureURL: "", PublishedAt: base},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	eps, err := d.ListPodcastEpisodes(ctx, feedID, 10)
	if err != nil || len(eps) != 3 {
		t.Fatalf("episodes: %+v %v", eps, err)
	}

	router := newPodcastTriageTestRouter(&API{db: d, dataDir: t.TempDir()})
	doReq := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// Reject malformed / invalid state patches.
	for _, bad := range []string{
		`{}`,
		`{"starred":true,"extra":"bad"}`,
		`{"starred":true}{}`,
		`{"positionMs":-1}`,
		`{"expectedRevision":-1,"starred":true}`,
	} {
		res := doReq(http.MethodPatch, "/episodes/"+eps[0].ID+"/state", bad)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("accepted invalid patch %s: %d %s", bad, res.Code, res.Body.String())
		}
	}
	// Reject > 1 KiB body.
	bigBody := `{"starred":true,"archived":` + strings.Repeat(" ", 1100) + `false}`
	if res := doReq(http.MethodPatch, "/episodes/"+eps[0].ID+"/state", bigBody); res.Code != http.StatusBadRequest {
		t.Fatalf("oversized patch accepted: %d", res.Code)
	}

	// 404 on missing episode.
	if res := doReq(http.MethodPatch, "/episodes/missing/state", `{"starred":true}`); res.Code != http.StatusNotFound {
		t.Fatalf("missing episode patch returned %d", res.Code)
	}

	// Valid patch on eps[0].
	ok := doReq(http.MethodPatch, "/episodes/"+eps[0].ID+"/state", `{"expectedRevision":0,"starred":true,"positionMs":20000,"listenedMs":20000}`)
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), `"starred":true`) || !strings.Contains(ok.Body.String(), `"revision":1`) {
		t.Fatalf("patch failed: %d %s", ok.Code, ok.Body.String())
	}

	// Conflict on stale revision.
	conflict := doReq(http.MethodPatch, "/episodes/"+eps[0].ID+"/state", `{"expectedRevision":0,"archived":true}`)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected 409 conflict, got %d %s", conflict.Code, conflict.Body.String())
	}

	// Archive eps[1].
	if res := doReq(http.MethodPatch, "/episodes/"+eps[1].ID+"/state", `{"archived":true}`); res.Code != http.StatusOK {
		t.Fatalf("archive failed: %d %s", res.Code, res.Body.String())
	}

	// Views & validation.
	if res := doReq(http.MethodGet, "/episodes?view=invalid", ""); res.Code != http.StatusBadRequest {
		t.Fatalf("invalid view returned %d", res.Code)
	}
	if res := doReq(http.MethodGet, "/episodes?limit=101", ""); res.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit returned %d", res.Code)
	}

	starredRes := doReq(http.MethodGet, "/episodes?view=starred", "")
	if starredRes.Code != http.StatusOK || !strings.Contains(starredRes.Body.String(), eps[0].ID) || strings.Contains(starredRes.Body.String(), "secret") {
		t.Fatalf("starred view: %d %s", starredRes.Code, starredRes.Body.String())
	}

	inProgressRes := doReq(http.MethodGet, "/episodes?view=in_progress", "")
	var viewBody struct {
		Episodes   []db.PodcastViewEpisode `json:"episodes"`
		NextOffset *int                    `json:"nextOffset"`
	}
	if err := json.Unmarshal(inProgressRes.Body.Bytes(), &viewBody); err != nil || len(viewBody.Episodes) != 1 || viewBody.Episodes[0].ID != eps[0].ID {
		t.Fatalf("in_progress view: %+v %v", viewBody, err)
	}

	// Search endpoint with wildcard escaping and pagination.
	searchRes := doReq(http.MethodGet, "/episodes/search?q=100%25", "")
	if searchRes.Code != http.StatusOK || !strings.Contains(searchRes.Body.String(), eps[0].ID) || strings.Contains(searchRes.Body.String(), eps[1].ID) {
		t.Fatalf("search 100%% failed: %d %s", searchRes.Code, searchRes.Body.String())
	}
	paged := doReq(http.MethodGet, "/episodes/search?q=Story&limit=1&offset=0", "")
	var searchBody struct {
		Episodes   []db.PodcastViewEpisode `json:"episodes"`
		NextOffset *int                    `json:"nextOffset"`
	}
	if err := json.Unmarshal(paged.Body.Bytes(), &searchBody); err != nil || len(searchBody.Episodes) != 1 || searchBody.NextOffset == nil || *searchBody.NextOffset != 1 {
		t.Fatalf("paged search: %+v %v", searchBody, err)
	}
}

func TestPodcastQueueAPI(t *testing.T) {
	ctx := context.Background()
	d, err := db.New(filepath.Join(t.TempDir(), "queue-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	feedID, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:    "https://example.org/q.xml",
		Title:  "Queue Pod",
		Author: "Host",
		Episodes: []podcast.Episode{
			{GUID: "1", Title: "One", EnclosureURL: "https://cdn.example.org/1.mp3", EnclosureType: "audio/mpeg"},
			{GUID: "2", Title: "Two", EnclosureURL: "https://cdn.example.org/2.mp3", EnclosureType: "audio/mpeg"},
			{GUID: "3", Title: "No Audio", EnclosureURL: ""},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	eps, err := d.ListPodcastEpisodes(ctx, feedID, 10)
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]string{}
	for _, e := range eps {
		byTitle[e.Title] = e.ID
	}

	router := newPodcastTriageTestRouter(&API{db: d})
	doReq := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	if res := doReq(http.MethodPost, "/queue", `{"episodeId":"`+byTitle["No Audio"]+`","position":"end"}`); res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for no-audio episode, got %d %s", res.Code, res.Body.String())
	}
	if res := doReq(http.MethodPost, "/queue", `{"episodeId":"missing","position":"end"}`); res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing episode, got %d", res.Code)
	}
	if res := doReq(http.MethodPost, "/queue", `{"episodeId":"`+byTitle["One"]+`","position":"bad"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid position, got %d", res.Code)
	}

	if res := doReq(http.MethodPost, "/queue", `{"episodeId":"`+byTitle["One"]+`","position":"end"}`); res.Code != http.StatusOK {
		t.Fatalf("add One failed: %d %s", res.Code, res.Body.String())
	}
	resNext := doReq(http.MethodPost, "/queue", `{"episodeId":"`+byTitle["Two"]+`","position":"next"}`)
	if resNext.Code != http.StatusOK {
		t.Fatalf("add Two next failed: %d %s", resNext.Code, resNext.Body.String())
	}

	var queueResp struct {
		Queue []db.PodcastQueueItem `json:"queue"`
	}
	if err := json.Unmarshal(resNext.Body.Bytes(), &queueResp); err != nil || len(queueResp.Queue) != 2 ||
		queueResp.Queue[0].ID != byTitle["Two"] || queueResp.Queue[1].ID != byTitle["One"] {
		t.Fatalf("unexpected queue order: %+v %v", queueResp, err)
	}

	// Replace queue order.
	resPut := doReq(http.MethodPut, "/queue", `{"episodeIds":["`+byTitle["One"]+`","`+byTitle["Two"]+`"]}`)
	if resPut.Code != http.StatusOK {
		t.Fatalf("put queue failed: %d %s", resPut.Code, resPut.Body.String())
	}
	if err := json.Unmarshal(resPut.Body.Bytes(), &queueResp); err != nil || len(queueResp.Queue) != 2 ||
		queueResp.Queue[0].ID != byTitle["One"] || queueResp.Queue[1].ID != byTitle["Two"] {
		t.Fatalf("unexpected replaced queue: %+v %v", queueResp, err)
	}

	// Delete item.
	if res := doReq(http.MethodDelete, "/queue/"+byTitle["One"], ""); res.Code != http.StatusNoContent {
		t.Fatalf("delete queue item returned %d", res.Code)
	}
	getRes := doReq(http.MethodGet, "/queue", "")
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), byTitle["Two"]) || strings.Contains(getRes.Body.String(), byTitle["One"]) {
		t.Fatalf("unexpected queue after delete: %d %s", getRes.Code, getRes.Body.String())
	}
}

func TestPodcastRetentionAPIAndDiskCleanup(t *testing.T) {
	ctx := context.Background()
	d, err := db.New(filepath.Join(t.TempDir(), "retention-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	base := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	feedID, err := d.SavePodcastFeedSnapshot(ctx, podcast.Feed{
		URL:   "https://example.org/ret-api.xml",
		Title: "Retention Pod",
		Episodes: []podcast.Episode{
			{GUID: "ep1", Title: "Newest", EnclosureURL: "https://cdn.example.org/ep1.mp3", EnclosureType: "audio/mpeg", PublishedAt: base.Add(3 * time.Hour)},
			{GUID: "ep2", Title: "Middle Unstarred", EnclosureURL: "https://cdn.example.org/ep2.mp3", EnclosureType: "audio/mpeg", PublishedAt: base.Add(2 * time.Hour)},
			{GUID: "ep3", Title: "Old Starred", EnclosureURL: "https://cdn.example.org/ep3.mp3", EnclosureType: "audio/mpeg", PublishedAt: base.Add(time.Hour)},
			{GUID: "ep4", Title: "Oldest Unstarred", EnclosureURL: "https://cdn.example.org/ep4.mp3", EnclosureType: "audio/mpeg", PublishedAt: base},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	eps, err := d.ListPodcastEpisodes(ctx, feedID, 10)
	if err != nil || len(eps) != 4 {
		t.Fatalf("episodes: %+v %v", eps, err)
	}

	dataDir := t.TempDir()
	epDir := filepath.Join(dataDir, "podcasts", "episodes")
	if err := os.MkdirAll(epDir, 0700); err != nil {
		t.Fatal(err)
	}
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, ep := range eps {
		if _, err := d.EnqueuePodcastDownload(ctx, ep.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ClaimPodcastDownload(ctx, "worker", time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(epDir, ep.ID+".audio"), []byte("audio-bytes"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := d.CompletePodcastDownload(ctx, ep.ID, "worker", ep.ID+".audio", "audio/mpeg", digest, 11); err != nil {
			t.Fatal(err)
		}
	}

	// Star eps[2] and save listening progress on eps[1].
	starred := true
	pos := int64(33000)
	if _, err := d.PatchPodcastEpisodeState(ctx, eps[2].ID, nil, &starred, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PatchPodcastEpisodeState(ctx, eps[1].ID, nil, nil, nil, nil, &pos, &pos); err != nil {
		t.Fatal(err)
	}

	api := &API{db: d, dataDir: dataDir}
	router := newPodcastTriageTestRouter(api)
	doReq := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// Validation & 404s.
	if res := doReq(http.MethodGet, "/feeds/missing/retention", ""); res.Code != http.StatusNotFound {
		t.Fatalf("missing feed retention GET: %d", res.Code)
	}
	for _, bad := range []string{`{}`, `{"keepLatestCount":-1}`, `{"keepLatestCount":51}`, `{"keepLatestCount":1,"extra":1}`, `{"keepLatestCount":1}{}`} {
		if res := doReq(http.MethodPut, "/feeds/"+feedID+"/retention", bad); res.Code != http.StatusBadRequest {
			t.Fatalf("accepted invalid retention %s: %d %s", bad, res.Code, res.Body.String())
		}
	}

	// First test enforcePodcastRetentionForEpisode protecting eps[3] when keep_latest_count=2.
	if _, err := d.SetPodcastRetentionPolicy(ctx, feedID, 2); err != nil {
		t.Fatal(err)
	}
	api.enforcePodcastRetentionForEpisode(ctx, eps[3].ID)
	// With keep=2: eps[0], eps[1] are in top 2; eps[2] is starred; eps[3] is protected by excludeEpisodeID.
	if _, err := os.Stat(filepath.Join(epDir, eps[3].ID+".audio")); err != nil {
		t.Fatalf("excluded episode file should remain: %v", err)
	}

	// Now PUT keepLatestCount=1 via API -> immediately cleans up eps[1] and eps[3], keeping eps[0] (newest) and eps[2] (starred).
	putRes := doReq(http.MethodPut, "/feeds/"+feedID+"/retention", `{"keepLatestCount":1}`)
	if putRes.Code != http.StatusOK || !strings.Contains(putRes.Body.String(), `"keepLatestCount":1`) {
		t.Fatalf("put retention failed: %d %s", putRes.Code, putRes.Body.String())
	}

	for _, kept := range []string{eps[0].ID, eps[2].ID} {
		if _, err := os.Stat(filepath.Join(epDir, kept+".audio")); err != nil {
			t.Fatalf("expected %s file kept, got %v", kept, err)
		}
		if _, err := d.GetPodcastDownload(ctx, kept); err != nil {
			t.Fatalf("expected %s download record kept, got %v", kept, err)
		}
	}
	for _, removed := range []string{eps[1].ID, eps[3].ID} {
		if _, err := os.Stat(filepath.Join(epDir, removed+".audio")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s file removed, got %v", removed, err)
		}
		if _, err := d.GetPodcastDownload(ctx, removed); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected %s download record deleted, got %v", removed, err)
		}
	}

	// Listening state on removed eps[1] must be preserved.
	prog, err := d.GetPodcastProgress(ctx, eps[1].ID)
	if err != nil || prog.PositionMs != 33000 {
		t.Fatalf("progress lost on retention cleanup: %+v %v", prog, err)
	}

	getRes := doReq(http.MethodGet, "/feeds/"+feedID+"/retention", "")
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), `"keepLatestCount":1`) {
		t.Fatalf("get retention failed: %d %s", getRes.Code, getRes.Body.String())
	}
}
