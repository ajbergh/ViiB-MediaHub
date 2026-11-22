package podcast

import (
    "errors"
    "os"
    "strings"
    "testing"
)

func TestParseRSSFixture(t *testing.T) {
    handle, err := os.Open("testdata/basic.xml")
    if err != nil { t.Fatal(err) }
    defer handle.Close()
    feed, err := ParseRSS(handle, "https://example.org/rss.xml")
    if err != nil { t.Fatal(err) }
    if feed.Title != "Example & Friends" || feed.Author != "A. Producer" ||
        feed.ImageURL != "https://cdn.example.org/show.jpg" || len(feed.Episodes) != 2 {
        t.Fatalf("unexpected feed: %#v", feed)
    }
    if strings.Contains(feed.Description, "<") || strings.Contains(feed.Description, "bad()") {
        t.Fatalf("unsafe HTML in description: %q", feed.Description)
    }
    if feed.Episodes[0].DurationSeconds != 3723 ||
        feed.Episodes[0].GUID != "episode-001" ||
        feed.Episodes[1].DurationSeconds != 312 {
        t.Fatalf("episode parse mismatch: %#v", feed.Episodes)
    }
    if strings.Contains(feed.Episodes[0].Description, "<") {
        t.Fatalf("episode description not plain text: %q", feed.Episodes[0].Description)
    }
}

func TestParseRSSRejectsDTDAndOversize(t *testing.T) {
    for _, xmlText := range []string{
        `<!DOCTYPE rss [ <!ENTITY xxe SYSTEM "file:///etc/passwd"> ]><rss><channel><title>x</title></channel></rss>`,
        `<rss><channel><title>bad</title>`,
        `<feed><title>Atom unsupported for now</title></feed>`,
        `<rss><channel><item><title>Untitled Feed</title></item></channel></rss>`,
    } {
        _, err := ParseRSS(strings.NewReader(xmlText), "https://example.org/rss")
        if !errors.Is(err, ErrInvalidFeed) { t.Fatalf("unexpected error for %q: %v", xmlText, err) }
    }
    _, err := ParseRSS(strings.NewReader(strings.Repeat("A", int(MaxFeedBytes)+1)), "https://example.org/rss")
    if !errors.Is(err, ErrInvalidFeed) { t.Fatalf("oversized feed accepted: %v", err) }
}

func TestParseDuration(t *testing.T) {
    cases := map[string]int64{"1:05:12":3912,"02:05":125,"30":30,"":0,"bad":0,"30:bad":0,"1:2:3:4":0}
    for input, expected := range cases {
        if got := parseDuration(input); got != expected {
            t.Errorf("%q: got %d, expected %d", input, got, expected)
        }
    }
}
