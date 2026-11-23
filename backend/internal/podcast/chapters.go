// chapters.go parses and fetches Podcasting 2.0 JSON and Podlove Simple
// Chapters documents through ViiB's public-IP DNS-pinning transport.
package podcast

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MaxChaptersBytes int64 = 1 << 20
	MaxChaptersCount       = 1000
)

// ErrInvalidChapters indicates a malformed or unsupported chapters payload.
var ErrInvalidChapters = errors.New("invalid podcast chapters")

// Chapter describes one sanitized podcast chapter marker with optional link and artwork.
type Chapter struct {
	StartTime float64  `json:"startTime"`
	EndTime   *float64 `json:"endTime,omitempty"`
	Title     string   `json:"title"`
	Img       string   `json:"img,omitempty"`
	URL       string   `json:"url,omitempty"`
	TOC       *bool    `json:"toc,omitempty"`
}

type rawJSONChaptersDoc struct {
	Version  string           `json:"version"`
	Chapters []rawJSONChapter `json:"chapters"`
}

type rawJSONChapter struct {
	StartTime float64  `json:"startTime"`
	EndTime   *float64 `json:"endTime"`
	Title     string   `json:"title"`
	Img       string   `json:"img"`
	URL       string   `json:"url"`
	TOC       *bool    `json:"toc"`
}

type rssPodloveChapters struct {
	XMLName  xml.Name            `xml:"chapters"`
	Version  string              `xml:"version,attr"`
	Chapters []rssPodloveChapter `xml:"chapter"`
}

type rssPodloveChapter struct {
	Start string `xml:"start,attr"`
	Title string `xml:"title,attr"`
	Href  string `xml:"href,attr"`
	Image string `xml:"image,attr"`
}

// ParseJSONChapters decodes a bounded Podcasting 2.0 JSON chapters document,
// sanitizing titles and filtering chapter image and link URLs.
func ParseJSONChapters(reader io.Reader) ([]Chapter, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxChaptersBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: unreadable body", ErrInvalidChapters)
	}
	if int64(len(data)) > MaxChaptersBytes {
		return nil, fmt.Errorf("%w: exceeds size limit", ErrInvalidChapters)
	}
	var doc rawJSONChaptersDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrInvalidChapters)
	}
	if doc.Chapters == nil {
		return nil, fmt.Errorf("%w: missing chapters array", ErrInvalidChapters)
	}
	if len(doc.Chapters) > MaxChaptersCount {
		return nil, fmt.Errorf("%w: too many chapters", ErrInvalidChapters)
	}
	chapters := make([]Chapter, 0, len(doc.Chapters))
	for _, raw := range doc.Chapters {
		if math.IsNaN(raw.StartTime) || math.IsInf(raw.StartTime, 0) || raw.StartTime < 0 || raw.StartTime > 86400*30 {
			continue
		}
		title := plainText(raw.Title, 300)
		if title == "" {
			continue
		}
		var endTime *float64
		if raw.EndTime != nil {
			v := *raw.EndTime
			if !math.IsNaN(v) && !math.IsInf(v, 0) && v >= raw.StartTime && v <= 86400*30 {
				val := v
				endTime = &val
			}
		}
		var toc *bool
		if raw.TOC != nil {
			v := *raw.TOC
			toc = &v
		}
		chapters = append(chapters, Chapter{
			StartTime: raw.StartTime,
			EndTime:   endTime,
			Title:     title,
			Img:       SafeArtworkURL(raw.Img),
			URL:       safePublicHTTPSURL(raw.URL),
			TOC:       toc,
		})
	}
	sort.SliceStable(chapters, func(i, j int) bool {
		return chapters[i].StartTime < chapters[j].StartTime
	})
	return chapters, nil
}

// ParsePodloveChaptersXML parses an XML Podlove Simple Chapters document.
func ParsePodloveChaptersXML(reader io.Reader) ([]Chapter, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxChaptersBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: unreadable XML", ErrInvalidChapters)
	}
	if int64(len(data)) > MaxChaptersBytes {
		return nil, fmt.Errorf("%w: exceeds size limit", ErrInvalidChapters)
	}
	upper := bytes.ToUpper(data)
	if bytes.Contains(upper, []byte("<!DOCTYPE")) || bytes.Contains(upper, []byte("<!ENTITY")) {
		return nil, fmt.Errorf("%w: DTD not allowed", ErrInvalidChapters)
	}
	var doc rssPodloveChapters
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: malformed XML", ErrInvalidChapters)
	}
	return normalizePodloveChapters(doc.Chapters)
}

func normalizePodloveChapters(items []rssPodloveChapter) ([]Chapter, error) {
	if len(items) > MaxChaptersCount {
		return nil, fmt.Errorf("%w: too many chapters", ErrInvalidChapters)
	}
	chapters := make([]Chapter, 0, len(items))
	for _, item := range items {
		start, ok := parsePodloveTimestamp(item.Start)
		if !ok {
			continue
		}
		title := plainText(item.Title, 300)
		if title == "" {
			continue
		}
		chapters = append(chapters, Chapter{
			StartTime: start,
			Title:     title,
			Img:       SafeArtworkURL(item.Image),
			URL:       safePublicHTTPSURL(item.Href),
		})
	}
	sort.SliceStable(chapters, func(i, j int) bool {
		return chapters[i].StartTime < chapters[j].StartTime
	})
	return chapters, nil
}

func parsePodloveTimestamp(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, false
	}
	var totalSeconds float64
	for i, part := range parts {
		if part == "" {
			return 0, false
		}
		if i < len(parts)-1 {
			n, err := strconv.ParseInt(part, 10, 64)
			if err != nil || n < 0 || n > 86400 {
				return 0, false
			}
			totalSeconds = totalSeconds*60 + float64(n)
		} else {
			sec, err := strconv.ParseFloat(part, 64)
			if err != nil || math.IsNaN(sec) || math.IsInf(sec, 0) || sec < 0 || sec > 86400*30 {
				return 0, false
			}
			totalSeconds = totalSeconds*60 + sec
		}
	}
	if totalSeconds < 0 || totalSeconds > 86400*30 {
		return 0, false
	}
	return totalSeconds, true
}

// FetchChapters fetches and parses a Podcasting 2.0 JSON (or Podlove XML)
// chapters resource using the public-IP-pinned transport.
func (f *Fetcher) FetchChapters(ctx context.Context, raw string) ([]Chapter, error) {
	u, err := ValidatePublicURL(raw, f.AllowHTTP)
	if err != nil {
		return nil, err
	}
	tr := f.transport()
	defer tr.CloseIdleConnections()
	redirects := 0
	client := &http.Client{
		Transport: tr,
		Timeout:   15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			redirects++
			if redirects > 4 {
				return ErrBlockedURL
			}
			if _, err := ValidatePublicURL(req.URL.String(), f.AllowHTTP); err != nil {
				return ErrBlockedURL
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return ErrBlockedURL
			}
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrBlockedURL
	}
	req.Header.Set("User-Agent", "ViiB-MediaHub-Podcast/1.0")
	req.Header.Set("Accept", "application/json+chapters, application/json;q=0.9, application/xml;q=0.5")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, ErrBlockedURL) {
			return nil, ErrBlockedURL
		}
		return nil, ErrFeedUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrFeedUnavailable, resp.StatusCode)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && !strings.EqualFold(enc, "identity") {
		return nil, fmt.Errorf("%w: compressed chapters not supported", ErrInvalidChapters)
	}
	if resp.ContentLength > MaxChaptersBytes {
		return nil, ErrInvalidChapters
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxChaptersBytes+1))
	if err != nil || int64(len(body)) > MaxChaptersBytes {
		return nil, ErrInvalidChapters
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	trimmed := bytes.TrimSpace(body)
	if strings.Contains(ct, "xml") || bytes.HasPrefix(trimmed, []byte("<")) {
		return ParsePodloveChaptersXML(bytes.NewReader(body))
	}
	return ParseJSONChapters(bytes.NewReader(body))
}
