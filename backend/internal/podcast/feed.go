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
    "net/url"
    "regexp"
    "strconv"
    "strings"
    "time"
    "unicode/utf8"
)

const (
    // MaxFeedBytes bounds both feed response and XML parser memory usage.
    MaxFeedBytes int64 = 8 << 20
    MaxFeedEpisodes = 1000
)
// ErrInvalidFeed indicates a malformed, oversized, or unsupported RSS 2.0 feed document.
var ErrInvalidFeed = errors.New("invalid podcast RSS feed")

// Feed holds parsed podcast show metadata and sanitized episode entries.
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
    GUID            string    `json:"guid,omitempty"`
    Title           string    `json:"title"`
    Description     string    `json:"description,omitempty"`
    RichDescription string    `json:"richDescription,omitempty"`
    EnclosureURL    string    `json:"-"`
    HasAudio        bool      `json:"hasAudio"`
    EnclosureType   string    `json:"enclosureType,omitempty"`
    EnclosureBytes  int64     `json:"enclosureBytes,omitempty"`
    WebsiteURL      string    `json:"websiteUrl,omitempty"`
    EpisodeNumber   int       `json:"episodeNumber,omitempty"`
    SeasonNumber    int       `json:"seasonNumber,omitempty"`
    ChaptersURL     string    `json:"-"`
    ChaptersType    string    `json:"chaptersType,omitempty"`
    HasChapters     bool      `json:"hasChapters"`
    InlineChapters  []Chapter `json:"-"`
    TranscriptURL   string    `json:"-"`
    TranscriptType  string    `json:"transcriptType,omitempty"`
    HasTranscript   bool      `json:"hasTranscript"`
    PublishedAt     time.Time `json:"publishedAt,omitempty"`
    DurationSeconds int64     `json:"durationSeconds,omitempty"`
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
type rssPodcastChapters struct {
    URL  string `xml:"url,attr"`
    Type string `xml:"type,attr"`
}
type rssPodcastTranscript struct {
    URL  string `xml:"url,attr"`
    Type string `xml:"type,attr"`
}
type rssItem struct {
    GUID            string                 `xml:"guid"`
    Title           string                 `xml:"title"`
    Link            string                 `xml:"link"`
    Description     string                 `xml:"description"`
    ContentEncoded  string                 `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
    Published       string                 `xml:"pubDate"`
    Duration        string                 `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd duration"`
    ItunesEpisode   string                 `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd episode"`
    ItunesSeason    string                 `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd season"`
    Chapters        []rssPodcastChapters   `xml:"https://podcastindex.org/namespace/1.0 chapters"`
    ChaptersAlt     []rssPodcastChapters   `xml:"http://podcastindex.org/namespace/1.0 chapters"`
    Transcripts     []rssPodcastTranscript `xml:"https://podcastindex.org/namespace/1.0 transcript"`
    TranscriptsAlt  []rssPodcastTranscript `xml:"http://podcastindex.org/namespace/1.0 transcript"`
    PodloveChapters struct {
        Version  string              `xml:"version,attr"`
        Chapters []rssPodloveChapter `xml:"http://podlove.org/simple-chapters chapter"`
    } `xml:"http://podlove.org/simple-chapters chapters"`
    Enclosure struct {
        URL    string `xml:"url,attr"`
        Length string `xml:"length,attr"`
        Type   string `xml:"type,attr"`
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

func safePublicHTTPSURL(raw string) string {
    raw = strings.TrimSpace(raw)
    if raw == "" || len(raw) > 2048 {
        return ""
    }
    u, err := ValidatePublicURL(raw, false)
    if err != nil {
        return ""
    }
    return u.String()
}

func parsePositiveInt(raw string) int {
    n, err := strconv.Atoi(strings.TrimSpace(raw))
    if err != nil || n <= 0 || n > 1000000 {
        return 0
    }
    return n
}

func parsePositiveInt64(raw string) int64 {
    n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
    if err != nil || n <= 0 {
        return 0
    }
    return n
}

func selectBestChapters(primary, alt []rssPodcastChapters) (string, string) {
    for _, list := range [][]rssPodcastChapters{primary, alt} {
        for _, c := range list {
            if u := safePublicHTTPSURL(c.URL); u != "" {
                return u, plainText(c.Type, 100)
            }
        }
    }
    return "", ""
}

func transcriptTypeScore(mimeType string) int {
    t := strings.ToLower(strings.TrimSpace(mimeType))
    if idx := strings.IndexByte(t, ';'); idx >= 0 {
        t = strings.TrimSpace(t[:idx])
    }
    switch t {
    case "application/json", "text/json":
        return 5
    case "text/vtt":
        return 4
    case "application/x-subrip", "application/srt", "text/srt":
        return 3
    case "text/plain":
        return 2
    case "text/html":
        return 0
    default:
        return 1
    }
}

func selectBestTranscript(primary, alt []rssPodcastTranscript) (string, string) {
    var bestURL, bestType string
    bestScore := 0
    for _, list := range [][]rssPodcastTranscript{primary, alt} {
        for _, tr := range list {
            u := safePublicHTTPSURL(tr.URL)
            if u == "" {
                continue
            }
            score := transcriptTypeScore(tr.Type)
            if score == 0 {
                continue
            }
            if score > bestScore {
                bestScore = score
                bestURL = u
                bestType = plainText(tr.Type, 100)
            }
        }
    }
    return bestURL, bestType
}

const maxRichHTMLBytes = 128 << 10
const maxRichHTMLOutRunes = 16384

type richHTMLStackEntry struct {
    tag     string
    emitted bool
}

// SanitizeRichHTML strips scripts, styles, iframes, objects, event handlers,
// and unsafe URLs, keeping only safe p, br, strong, em, ul, ol, li,
// a[href^=https://], blockquote, and code tags.
func SanitizeRichHTML(raw string) string {
    if len(raw) > maxRichHTMLBytes {
        raw = truncateUTF8Bytes(raw, maxRichHTMLBytes)
    }
    raw = strings.TrimSpace(raw)
    if raw == "" {
        return ""
    }
    var out strings.Builder
    var stack []richHTMLStackEntry
    n := len(raw)
    i := 0
    for i < n {
        if utf8.RuneCountInString(out.String()) >= maxRichHTMLOutRunes {
            break
        }
        lt := strings.IndexByte(raw[i:], '<')
        if lt < 0 {
            appendSafeRichText(&out, raw[i:])
            break
        }
        if lt > 0 {
            appendSafeRichText(&out, raw[i:i+lt])
        }
        i += lt
        if strings.HasPrefix(raw[i:], "<!--") {
            end := strings.Index(raw[i+4:], "-->")
            if end < 0 {
                break
            }
            i += 4 + end + 3
            continue
        }
        if i+1 < n && (raw[i+1] == '!' || raw[i+1] == '?') {
            end := strings.IndexByte(raw[i+1:], '>')
            if end < 0 {
                break
            }
            i += 1 + end + 1
            continue
        }
        gt := findTagClose(raw, i+1)
        if gt < 0 {
            appendSafeRichText(&out, raw[i:])
            break
        }
        tagContent := raw[i+1 : gt]
        i = gt + 1
        trimmedTag := strings.TrimSpace(tagContent)
        if trimmedTag == "" {
            continue
        }
        isClosing := strings.HasPrefix(trimmedTag, "/")
        if isClosing {
            trimmedTag = strings.TrimSpace(trimmedTag[1:])
        }
        selfClosing := strings.HasSuffix(trimmedTag, "/")
        if selfClosing {
            trimmedTag = strings.TrimSpace(trimmedTag[:len(trimmedTag)-1])
        }
        tagName, attrStr := splitTagAndAttrs(trimmedTag)
        if tagName == "" {
            continue
        }
        if !isClosing && isDangerousContainerTag(tagName) {
            if !selfClosing {
                i = skipUntilClosingTag(raw, i, tagName)
            }
            continue
        }
        if isDangerousContainerTag(tagName) {
            continue
        }
        if tagName == "br" {
            if !isClosing {
                out.WriteString("<br>")
            }
            continue
        }
        if !isAllowedRichTag(tagName) {
            continue
        }
        if isClosing {
            closeRichTag(&out, &stack, tagName)
            continue
        }
        if tagName == "a" {
            href := extractSafeHrefAttr(attrStr)
            if href == "" {
                if !selfClosing {
                    stack = append(stack, richHTMLStackEntry{tag: "a", emitted: false})
                }
                continue
            }
            out.WriteString(`<a href="`)
            out.WriteString(html.EscapeString(href))
            out.WriteString(`">`)
            if selfClosing {
                out.WriteString(`</a>`)
            } else {
                stack = append(stack, richHTMLStackEntry{tag: "a", emitted: true})
            }
            continue
        }
        out.WriteByte('<')
        out.WriteString(tagName)
        out.WriteByte('>')
        if selfClosing {
            out.WriteString("</")
            out.WriteString(tagName)
            out.WriteByte('>')
        } else {
            stack = append(stack, richHTMLStackEntry{tag: tagName, emitted: true})
        }
    }
    for j := len(stack) - 1; j >= 0; j-- {
        if stack[j].emitted {
            out.WriteString("</")
            out.WriteString(stack[j].tag)
            out.WriteByte('>')
        }
    }
    return strings.TrimSpace(out.String())
}

func truncateUTF8Bytes(s string, maxBytes int) string {
    if len(s) <= maxBytes {
        return s
    }
    cut := maxBytes
    for cut > 0 && !utf8.RuneStart(s[cut]) {
        cut--
    }
    return s[:cut]
}

func appendSafeRichText(out *strings.Builder, raw string) {
    decoded := html.UnescapeString(raw)
    out.WriteString(html.EscapeString(decoded))
}

func findTagClose(s string, start int) int {
    var quote byte
    for i := start; i < len(s); i++ {
        ch := s[i]
        if quote != 0 {
            if ch == quote {
                quote = 0
            }
            continue
        }
        if ch == '"' || ch == '\'' {
            quote = ch
            continue
        }
        if ch == '>' {
            return i
        }
    }
    return -1
}

func splitTagAndAttrs(s string) (string, string) {
    for i := 0; i < len(s); i++ {
        ch := s[i]
        if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '/' {
            return strings.ToLower(s[:i]), s[i:]
        }
    }
    return strings.ToLower(s), ""
}

func isDangerousContainerTag(tag string) bool {
    switch tag {
    case "script", "style", "iframe", "object", "embed", "svg", "math", "noscript", "template", "textarea", "title":
        return true
    default:
        return false
    }
}

func isAllowedRichTag(tag string) bool {
    switch tag {
    case "p", "br", "strong", "em", "ul", "ol", "li", "a", "blockquote", "code":
        return true
    default:
        return false
    }
}

func skipUntilClosingTag(s string, pos int, tag string) int {
    lower := strings.ToLower(s[pos:])
    needle := "</" + tag
    offset := 0
    for {
        idx := strings.Index(lower[offset:], needle)
        if idx < 0 {
            return len(s)
        }
        after := pos + offset + idx + len(needle)
        if after < len(s) {
            next := s[after]
            if next == '>' {
                return after + 1
            }
            if next == ' ' || next == '\t' || next == '\n' || next == '\r' {
                gt := strings.IndexByte(s[after:], '>')
                if gt < 0 {
                    return len(s)
                }
                return after + gt + 1
            }
        } else {
            return len(s)
        }
        offset += idx + len(needle)
    }
}

func closeRichTag(out *strings.Builder, stack *[]richHTMLStackEntry, tag string) {
    for idx := len(*stack) - 1; idx >= 0; idx-- {
        if (*stack)[idx].tag == tag {
            for j := len(*stack) - 1; j >= idx; j-- {
                if (*stack)[j].emitted {
                    out.WriteString("</")
                    out.WriteString((*stack)[j].tag)
                    out.WriteByte('>')
                }
            }
            *stack = (*stack)[:idx]
            return
        }
    }
}

func extractSafeHrefAttr(attrStr string) string {
    i := 0
    n := len(attrStr)
    for i < n {
        for i < n && (attrStr[i] == ' ' || attrStr[i] == '\t' || attrStr[i] == '\n' || attrStr[i] == '\r' || attrStr[i] == '/') {
            i++
        }
        if i >= n {
            break
        }
        nameStart := i
        for i < n && attrStr[i] != '=' && attrStr[i] != ' ' && attrStr[i] != '\t' && attrStr[i] != '\n' && attrStr[i] != '\r' {
            i++
        }
        name := strings.ToLower(attrStr[nameStart:i])
        for i < n && (attrStr[i] == ' ' || attrStr[i] == '\t' || attrStr[i] == '\n' || attrStr[i] == '\r') {
            i++
        }
        var val string
        if i < n && attrStr[i] == '=' {
            i++
            for i < n && (attrStr[i] == ' ' || attrStr[i] == '\t' || attrStr[i] == '\n' || attrStr[i] == '\r') {
                i++
            }
            if i < n && (attrStr[i] == '"' || attrStr[i] == '\'') {
                q := attrStr[i]
                i++
                valStart := i
                for i < n && attrStr[i] != q {
                    i++
                }
                val = attrStr[valStart:i]
                if i < n {
                    i++
                }
            } else {
                valStart := i
                for i < n && attrStr[i] != ' ' && attrStr[i] != '\t' && attrStr[i] != '\n' && attrStr[i] != '\r' {
                    i++
                }
                val = attrStr[valStart:i]
            }
        }
        if name == "href" {
            return validateRichHref(val)
        }
    }
    return ""
}

func validateRichHref(raw string) string {
    decoded := strings.TrimSpace(html.UnescapeString(raw))
    if decoded == "" || len(decoded) > 2048 {
        return ""
    }
    for i := 0; i < len(decoded); i++ {
        if decoded[i] < 0x20 || decoded[i] == 0x7f {
            return ""
        }
    }
    if !strings.HasPrefix(strings.ToLower(decoded), "https://") {
        return ""
    }
    u, err := url.Parse(decoded)
    if err != nil || u == nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil {
        return ""
    }
    u.Scheme = "https"
    return u.String()
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
        plainSource := item.Description
        if strings.TrimSpace(plainSource) == "" {
            plainSource = item.ContentEncoded
        }
        richSource := item.ContentEncoded
        if strings.TrimSpace(richSource) == "" {
            richSource = item.Description
        }
        chaptersURL, chaptersType := selectBestChapters(item.Chapters, item.ChaptersAlt)
        inlineChapters, _ := normalizePodloveChapters(item.PodloveChapters.Chapters)
        transcriptURL, transcriptType := selectBestTranscript(item.Transcripts, item.TranscriptsAlt)
        result.Episodes = append(result.Episodes, Episode{
            GUID: plainText(item.GUID, 500),
            Title: title,
            Description: plainText(plainSource, 8192),
            RichDescription: SanitizeRichHTML(richSource),
            PublishedAt: parseDate(item.Published),
            DurationSeconds: parseDuration(item.Duration),
            EnclosureURL: strings.TrimSpace(item.Enclosure.URL),
            HasAudio: strings.TrimSpace(item.Enclosure.URL)!="",
            EnclosureType: plainText(item.Enclosure.Type, 150),
            EnclosureBytes: parsePositiveInt64(item.Enclosure.Length),
            WebsiteURL: safePublicHTTPSURL(item.Link),
            EpisodeNumber: parsePositiveInt(item.ItunesEpisode),
            SeasonNumber: parsePositiveInt(item.ItunesSeason),
            ChaptersURL: chaptersURL,
            ChaptersType: chaptersType,
            HasChapters: chaptersURL != "" || len(inlineChapters) > 0,
            InlineChapters: inlineChapters,
            TranscriptURL: transcriptURL,
            TranscriptType: transcriptType,
            HasTranscript: transcriptURL != "",
        })
    }
    return result, nil
}
