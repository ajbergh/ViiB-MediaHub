// Package podcast implements isolated public RSS ingestion primitives.
// No network operations happen in this parser.
package podcast

import (
    "bytes"
    "encoding/xml"
    "errors"
    "fmt"
    "html"
    "io"
    "regexp"
    "strings"
    "time"
)

const (
    // MaxFeedBytes bounds both feed response and XML parser memory usage.
    MaxFeedBytes int64 = 8 << 20
    MaxFeedEpisodes = 1000
)
var ErrInvalidFeed = errors.New("invalid podcast RSS feed")

type Feed struct {
    URL string `json:"url"`
    Title string `json:"title"`
    Author string `json:"author,omitempty"`
    Description string `json:"description,omitempty"`
    ImageURL string `json:"imageUrl,omitempty"`
    Language string `json:"language,omitempty"`
    Episodes []Episode `json:"episodes"`
}

type Episode struct {
    GUID string `json:"guid,omitempty"`
    Title string `json:"title"`
    Description string `json:"description,omitempty"`
    EnclosureURL string `json:"-"`
    HasAudio bool `json:"hasAudio"`
    EnclosureType string `json:"enclosureType,omitempty"`
    PublishedAt time.Time `json:"publishedAt,omitempty"`
    DurationSeconds int64 `json:"durationSeconds,omitempty"`
}

type rssDocument struct {
    XMLName xml.Name
    Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
    Title string `xml:"title"`
    Description string `xml:"description"`
    Language string `xml:"language"`
    Author string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd author"`
    ItunesImage struct { Href string `xml:"href,attr"` } `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
    Image struct { URL string `xml:"url"` } `xml:"image"`
    Items []rssItem `xml:"item"`
}
type rssItem struct {
    GUID string `xml:"guid"`
    Title string `xml:"title"`
    Description string `xml:"description"`
    Published string `xml:"pubDate"`
    Duration string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd duration"`
    Enclosure struct {
        URL string `xml:"url,attr"`
        Type string `xml:"type,attr"`
    } `xml:"enclosure"`
}

var (
    htmlScripts = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)\s*>`)
    htmlTags = regexp.MustCompile(`(?s)<[^>]*>`)
)

func plainText(value string, maxRunes int) string {
    value = htmlScripts.ReplaceAllString(value, " ")
    value = htmlTags.ReplaceAllString(value, " ")
    value = html.UnescapeString(value)
    value = strings.Join(strings.Fields(value), " ")
    r := []rune(value)
    if len(r) > maxRunes { return string(r[:maxRunes]) }
    return value
}

func parseDuration(value string) int64 {
    value = strings.TrimSpace(value)
    if value == "" { return 0 }
    parts := strings.Split(value, ":")
    if len(parts) > 3 { return 0 }
    var total int64
    for _, part := range parts {
        if part == "" { return 0 }
        var n int64
        for _, ch := range part {
            if ch < '0' || ch > '9' { return 0 }
            n = n*10 + int64(ch-'0')
            if n > 86400000 { return 0 }
        }
        total = total*60 + n
    }
    if total > 86400*30 { return 0 }
    return total
}
func parseDate(value string) time.Time {
    value = strings.TrimSpace(value)
    for _, layout := range []string{
        time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339,
        "Mon, 2 Jan 2006 15:04:05 -0700",
    } {
        if parsed, err := time.Parse(layout, value); err == nil { return parsed.UTC() }
    }
    return time.Time{}
}

// ParseRSS decodes an untrusted, bounded RSS 2.0 document and emits only
// plain-text descriptions. Enclosures are metadata, not permission to fetch.
func ParseRSS(reader io.Reader, feedURL string) (Feed, error) {
    var result Feed
    contents, err := io.ReadAll(io.LimitReader(reader, MaxFeedBytes+1))
    if err != nil { return result, fmt.Errorf("%w: unreadable XML", ErrInvalidFeed) }
    if int64(len(contents)) > MaxFeedBytes {
        return result, fmt.Errorf("%w: response exceeds limit", ErrInvalidFeed)
    }
    upper := bytes.ToUpper(contents)
    if bytes.Contains(upper, []byte("<!DOCTYPE")) || bytes.Contains(upper, []byte("<!ENTITY")) {
        return result, fmt.Errorf("%w: document type declarations are not allowed", ErrInvalidFeed)
    }
    var doc rssDocument
    decoder := xml.NewDecoder(bytes.NewReader(contents))
    decoder.Strict = true
    if err := decoder.Decode(&doc); err != nil {
        return result, fmt.Errorf("%w: malformed XML", ErrInvalidFeed)
    }
    if doc.XMLName.Local != "rss" || strings.TrimSpace(doc.Channel.Title) == "" {
        return result, fmt.Errorf("%w: RSS channel or title missing", ErrInvalidFeed)
    }
    if len(doc.Channel.Items) > MaxFeedEpisodes {
        return result, fmt.Errorf("%w: too many episodes", ErrInvalidFeed)
    }
    result = Feed{
        URL: feedURL,
        Title: plainText(doc.Channel.Title, 300),
        Author: plainText(doc.Channel.Author, 300),
        Description: plainText(doc.Channel.Description, 4096),
        ImageURL: strings.TrimSpace(doc.Channel.ItunesImage.Href),
        Language: plainText(doc.Channel.Language, 50),
        Episodes: make([]Episode, 0, len(doc.Channel.Items)),
    }
    if result.ImageURL == "" { result.ImageURL = strings.TrimSpace(doc.Channel.Image.URL) }
    result.ImageURL = SafeArtworkURL(result.ImageURL)
    for _, item := range doc.Channel.Items {
        title := plainText(item.Title, 400)
        if title == "" { continue }
        result.Episodes = append(result.Episodes, Episode{
            GUID: plainText(item.GUID, 500),
            Title: title,
            Description: plainText(item.Description, 8192),
            PublishedAt: parseDate(item.Published),
            DurationSeconds: parseDuration(item.Duration),
            EnclosureURL: strings.TrimSpace(item.Enclosure.URL),
            HasAudio: strings.TrimSpace(item.Enclosure.URL)!="",
            EnclosureType: plainText(item.Enclosure.Type, 150),
        })
    }
    return result, nil
}
