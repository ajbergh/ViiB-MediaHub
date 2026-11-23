package podcast

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseTranscriptJSON(t *testing.T) {
	raw := `{
		"version": "1.0.0",
		"segments": [
			{"speaker": "Alice <b>Host</b>", "startTime": 0.5, "endTime": 3.25, "body": "Welcome <script>x()</script> everyone!"},
			{"speaker": "Bob", "startTime": 4.0, "endTime": 6.0, "body": "Glad to be here."}
		]
	}`
	cues, format, err := ParseTranscript(strings.NewReader(raw), "application/json")
	if err != nil {
		t.Fatal(err)
	}
	if format != "json" || len(cues) != 2 {
		t.Fatalf("unexpected result format=%s cues=%#v", format, cues)
	}
	if cues[0].StartMs != 500 || cues[0].EndMs != 3250 || cues[0].Speaker != "Alice Host" || cues[0].Text != "Welcome everyone!" {
		t.Fatalf("cue 0 mismatch: %#v", cues[0])
	}
}

func TestParseTranscriptVTT(t *testing.T) {
	raw := "WEBVTT\n\nNOTE ignore this comment\n\n00:00:01.500 --> 00:00:04.000\n<v Alice>Hello <i>world</i></v>\n\n01:02.250 --> 01:05.000\nSecond cue\nline two\n"
	cues, format, err := ParseTranscript(strings.NewReader(raw), "text/vtt")
	if err != nil {
		t.Fatal(err)
	}
	if format != "vtt" || len(cues) != 2 {
		t.Fatalf("unexpected vtt format=%s cues=%#v", format, cues)
	}
	if cues[0].StartMs != 1500 || cues[0].EndMs != 4000 || cues[0].Speaker != "Alice" || cues[0].Text != "Hello world" {
		t.Fatalf("vtt cue 0 mismatch: %#v", cues[0])
	}
	if cues[1].StartMs != 62250 || cues[1].EndMs != 65000 || cues[1].Text != "Second cue line two" {
		t.Fatalf("vtt cue 1 mismatch: %#v", cues[1])
	}
}

func TestParseTranscriptSRT(t *testing.T) {
	raw := "1\r\n00:00:02,000 --> 00:00:05,500\r\nFirst line.\r\n\r\n2\r\n00:01:00,000 --> 00:01:03,250\r\nSecond line.\r\n"
	cues, format, err := ParseTranscript(strings.NewReader(raw), "application/x-subrip")
	if err != nil {
		t.Fatal(err)
	}
	if format != "srt" || len(cues) != 2 {
		t.Fatalf("unexpected srt format=%s cues=%#v", format, cues)
	}
	if cues[0].StartMs != 2000 || cues[0].EndMs != 5500 || cues[0].Text != "First line." {
		t.Fatalf("srt cue 0 mismatch: %#v", cues[0])
	}
	if cues[1].StartMs != 60000 || cues[1].EndMs != 63250 || cues[1].Text != "Second line." {
		t.Fatalf("srt cue 1 mismatch: %#v", cues[1])
	}
}

func TestParseTranscriptPlainTextAndRejections(t *testing.T) {
	cues, format, err := ParseTranscript(strings.NewReader("Line one.\n\nLine two.\n"), "text/plain")
	if err != nil || format != "text" || len(cues) != 2 {
		t.Fatalf("plain text failed: format=%s cues=%#v err=%v", format, cues, err)
	}
	if _, _, err := ParseTranscript(strings.NewReader("<html><script>alert(1)</script></html>"), "text/html"); !errors.Is(err, ErrInvalidTranscript) {
		t.Fatalf("HTML transcript not rejected: %v", err)
	}
	if _, _, err := ParseTranscript(strings.NewReader(strings.Repeat("a", int(MaxTranscriptBytes)+1)), "text/plain"); !errors.Is(err, ErrInvalidTranscript) {
		t.Fatalf("oversize transcript not rejected: %v", err)
	}
	var tooMany strings.Builder
	for i := 0; i <= MaxTranscriptCues; i++ {
		tooMany.WriteString("line\n")
	}
	if _, _, err := ParseTranscript(strings.NewReader(tooMany.String()), "text/plain"); !errors.Is(err, ErrInvalidTranscript) {
		t.Fatalf(">10000 cues not rejected: %v", err)
	}
}

func TestFetchTranscriptSSRFAndRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tr.vtt":
			w.Header().Set("Content-Type", "text/vtt")
			_, _ = w.Write([]byte("WEBVTT\n\n00:00.000 --> 00:02.000\nHello\n"))
		case "/redirect":
			http.Redirect(w, r, "http://127.0.0.1/private.vtt", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	f := testFetcher(t, server)
	cues, format, err := f.FetchTranscript(context.Background(), "http://public.example/tr.vtt", "")
	if err != nil || format != "vtt" || len(cues) != 1 || cues[0].Text != "Hello" {
		t.Fatalf("FetchTranscript failed: format=%s cues=%#v err=%v", format, cues, err)
	}
	if _, _, err := f.FetchTranscript(context.Background(), "http://public.example/redirect", ""); !errors.Is(err, ErrBlockedURL) {
		t.Fatalf("redirect not blocked: %v", err)
	}
	if _, _, err := f.FetchTranscript(context.Background(), "http://127.0.0.1/tr.vtt", ""); !errors.Is(err, ErrBlockedURL) {
		t.Fatalf("private IP not blocked: %v", err)
	}
}
