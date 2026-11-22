package podcast

import (
    "encoding/json"
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

func TestParseRSSPodcasting2AndPodloveMetadata(t *testing.T) {
    rssXML := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"
  xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"
  xmlns:content="http://purl.org/rss/1.0/modules/content/"
  xmlns:podcast="https://podcastindex.org/namespace/1.0"
  xmlns:psc="http://podlove.org/simple-chapters">
  <channel>
    <title>Pod2 Show</title>
    <item>
      <guid>ep-42</guid>
      <title>Episode 42</title>
      <link>https://example.org/episodes/42</link>
      <description>Plain summary</description>
      <content:encoded><![CDATA[<p>Rich <strong>notes</strong> with <a href="https://example.org/ref" onclick="alert(1)">link</a> and <a href="javascript:alert(1)">bad</a></p><script>evil()</script><iframe src="https://evil.test">bad</iframe>]]></content:encoded>
      <itunes:episode>42</itunes:episode>
      <itunes:season>3</itunes:season>
      <enclosure url="https://cdn.example.org/ep42.mp3" length="987654" type="audio/mpeg"/>
      <podcast:chapters url="https://cdn.example.org/ep42.chapters.json" type="application/json+chapters"/>
      <podcast:transcript url="http://insecure.example.org/tr.vtt" type="text/vtt"/>
      <podcast:transcript url="https://cdn.example.org/ep42.vtt" type="text/vtt"/>
      <podcast:transcript url="https://cdn.example.org/ep42.json" type="application/json"/>
      <psc:chapters version="1.2">
        <psc:chapter start="00:01:00" title="Intro" href="https://example.org/intro" image="https://cdn.example.org/c.png"/>
      </psc:chapters>
    </item>
  </channel>
</rss>`
    feed, err := ParseRSS(strings.NewReader(rssXML), "https://example.org/rss.xml")
    if err != nil {
        t.Fatal(err)
    }
    if len(feed.Episodes) != 1 {
        t.Fatalf("expected 1 episode, got %d", len(feed.Episodes))
    }
    ep := feed.Episodes[0]
    if ep.WebsiteURL != "https://example.org/episodes/42" ||
        ep.EpisodeNumber != 42 || ep.SeasonNumber != 3 ||
        ep.EnclosureBytes != 987654 {
        t.Fatalf("metadata mismatch: %#v", ep)
    }
    if !ep.HasChapters || ep.ChaptersURL != "https://cdn.example.org/ep42.chapters.json" ||
        ep.ChaptersType != "application/json+chapters" || len(ep.InlineChapters) != 1 {
        t.Fatalf("chapters mismatch: %#v", ep)
    }
    if !ep.HasTranscript || ep.TranscriptURL != "https://cdn.example.org/ep42.json" ||
        ep.TranscriptType != "application/json" {
        t.Fatalf("transcript mismatch: %#v", ep)
    }
    if strings.Contains(ep.RichDescription, "<script") ||
        strings.Contains(ep.RichDescription, "<iframe") ||
        strings.Contains(ep.RichDescription, "onclick") ||
        strings.Contains(ep.RichDescription, "javascript:") {
        t.Fatalf("unsafe HTML leaked into RichDescription: %q", ep.RichDescription)
    }
    if !strings.Contains(ep.RichDescription, `<a href="https://example.org/ref">link</a>`) ||
        !strings.Contains(ep.RichDescription, `<strong>notes</strong>`) {
        t.Fatalf("safe tags/links stripped unexpectedly: %q", ep.RichDescription)
    }
    encoded, err := json.Marshal(ep)
    if err != nil {
        t.Fatal(err)
    }
    s := string(encoded)
    if strings.Contains(s, "ep42.chapters.json") || strings.Contains(s, "ep42.json") || strings.Contains(s, "ep42.mp3") {
        t.Fatalf("raw internal URLs leaked in Episode JSON: %s", s)
    }
    if !strings.Contains(s, `"hasChapters":true`) || !strings.Contains(s, `"hasTranscript":true`) {
        t.Fatalf("hasChapters/hasTranscript booleans missing from JSON: %s", s)
    }
}

func TestSanitizeRichHTML(t *testing.T) {
    raw := `<div class="wrap"><p onclick="bad()">Hi <b>there</b> <em>friend</em><br/><code>x=1</code></p><ul><li>One</li></ul><ol><li>Two</li></ol><blockquote>Quote</blockquote><style>body{display:none}</style><object data="x">obj</object><a href="&#106;avascript:alert(1)">no</a> <a href="https://example.org/ok?a=1">yes</a></div>`
    got := SanitizeRichHTML(raw)
    want := `<p>Hi there <em>friend</em><br><code>x=1</code></p><ul><li>One</li></ul><ol><li>Two</li></ol><blockquote>Quote</blockquote>no <a href="https://example.org/ok?a=1">yes</a>`
    if got != want {
        t.Fatalf("SanitizeRichHTML mismatch:\ngot:  %q\nwant: %q", got, want)
    }
}
