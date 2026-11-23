package podcast

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseJSONChaptersSanitizesAndValidates(t *testing.T) {
	payload := `{
		"version": "1.2.0",
		"chapters": [
			{
				"startTime": 120.5,
				"endTime": 240,
				"title": "Deep <b>Dive</b> <script>alert(1)</script>",
				"img": "https://cdn.example.org/ch2.png?width=300",
				"url": "https://example.org/ch2",
				"toc": false
			},
			{
				"startTime": 0,
				"title": "Intro",
				"img": "https://127.0.0.1/private.png",
				"url": "javascript:alert(1)"
			},
			{
				"startTime": 30,
				"title": "Middle",
				"img": "https://cdn.example.org/ch1.jpg?token=secret",
				"url": "http://insecure.example.org/ch1"
			}
		]
	}`
	chapters, err := ParseJSONChapters(strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "Intro" || chapters[0].StartTime != 0 || chapters[0].Img != "" || chapters[0].URL != "" {
		t.Fatalf("chapter 0 unsafe fields not stripped: %#v", chapters[0])
	}
	if chapters[1].Title != "Middle" || chapters[1].Img != "" || chapters[1].URL != "" {
		t.Fatalf("chapter 1 unsafe fields not stripped: %#v", chapters[1])
	}
	if chapters[2].Title != "Deep Dive" || chapters[2].StartTime != 120.5 ||
		chapters[2].EndTime == nil || *chapters[2].EndTime != 240 ||
		chapters[2].Img != "https://cdn.example.org/ch2.png?width=300" ||
		chapters[2].URL != "https://example.org/ch2" ||
		chapters[2].TOC == nil || *chapters[2].TOC != false {
		t.Fatalf("chapter 2 mismatch: %#v", chapters[2])
	}
}

func TestParseJSONChaptersRejectsOversizeAndTooMany(t *testing.T) {
	oversize := strings.Repeat("a", int(MaxChaptersBytes)+1)
	if _, err := ParseJSONChapters(strings.NewReader(oversize)); !errors.Is(err, ErrInvalidChapters) {
		t.Fatalf("expected ErrInvalidChapters for oversize, got %v", err)
	}

	var sb strings.Builder
	sb.WriteString(`{"version":"1.2.0","chapters":[`)
	for i := 0; i <= MaxChaptersCount; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"startTime":1,"title":"x"}`)
	}
	sb.WriteString(`]}`)
	if _, err := ParseJSONChapters(strings.NewReader(sb.String())); !errors.Is(err, ErrInvalidChapters) {
		t.Fatalf("expected ErrInvalidChapters for >1000 chapters, got %v", err)
	}
}

func TestParsePodloveChaptersXML(t *testing.T) {
	xmlData := `<psc:chapters version="1.2" xmlns:psc="http://podlove.org/simple-chapters">
		<psc:chapter start="00:05:10.500" title="Main Topic" href="https://example.org/main" image="https://cdn.example.org/m.jpg"/>
		<psc:chapter start="00:00:00" title="Opening" href="http://insecure.example.org/" image="https://127.0.0.1/bad.png"/>
	</psc:chapters>`
	chapters, err := ParsePodloveChaptersXML(strings.NewReader(xmlData))
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "Opening" || chapters[0].StartTime != 0 || chapters[0].URL != "" || chapters[0].Img != "" {
		t.Fatalf("opening chapter mismatch: %#v", chapters[0])
	}
	if chapters[1].Title != "Main Topic" || chapters[1].StartTime != 310.5 ||
		chapters[1].URL != "https://example.org/main" || chapters[1].Img != "https://cdn.example.org/m.jpg" {
		t.Fatalf("main chapter mismatch: %#v", chapters[1])
	}
}

func TestFetchChaptersSSRFAndRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chapters.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"1.2.0","chapters":[{"startTime":0,"title":"Start"}]}`))
		case "/redirect":
			http.Redirect(w, r, "http://127.0.0.1/secret", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	f := testFetcher(t, server)
	chapters, err := f.FetchChapters(context.Background(), "http://public.example/chapters.json")
	if err != nil || len(chapters) != 1 || chapters[0].Title != "Start" {
		t.Fatalf("FetchChapters failed: %#v err=%v", chapters, err)
	}
	if _, err := f.FetchChapters(context.Background(), "http://public.example/redirect"); !errors.Is(err, ErrBlockedURL) {
		t.Fatalf("expected ErrBlockedURL on redirect, got %v", err)
	}
	if _, err := f.FetchChapters(context.Background(), "http://127.0.0.1/chapters.json"); !errors.Is(err, ErrBlockedURL) {
		t.Fatalf("expected ErrBlockedURL on private IP, got %v", err)
	}
}
