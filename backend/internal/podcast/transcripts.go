package podcast

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	MaxTranscriptBytes int64 = 2 << 20
	MaxTranscriptCues        = 10000
)

var ErrInvalidTranscript = errors.New("invalid podcast transcript")

type TranscriptCue struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Speaker string `json:"speaker,omitempty"`
	Text    string `json:"text"`
}

var (
	vttVoiceTagPattern  = regexp.MustCompile(`(?is)^<v(?:\.[^>\s]+)?\s+([^>]+)>(.*)$`)
	srtTimestampPattern = regexp.MustCompile(`^\d{1,2}:\d{2}:\d{2}[,\.]\d{3}\s*-->\s*\d{1,2}:\d{2}:\d{2}[,\.]\d{3}`)
	htmlDangerHint      = regexp.MustCompile(`(?is)(<!doctype\s+html|<html\b|<script\b|<iframe\b|<object\b)`)
)

// ParseTranscript parses Podcasting 2.0 JSON, WebVTT, SRT, or plain-text
// transcripts into bounded, normalized TranscriptCues.
func ParseTranscript(reader io.Reader, contentType string) ([]TranscriptCue, string, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxTranscriptBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%w: unreadable transcript", ErrInvalidTranscript)
	}
	if len(data) == 0 || int64(len(data)) > MaxTranscriptBytes {
		return nil, "", fmt.Errorf("%w: transcript empty or exceeds limit", ErrInvalidTranscript)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, "", fmt.Errorf("%w: binary transcript not allowed", ErrInvalidTranscript)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	format := detectTranscriptFormat(data, contentType)
	if format == "" {
		return nil, "", fmt.Errorf("%w: unsupported transcript format", ErrInvalidTranscript)
	}
	switch format {
	case "json":
		cues, err := parseJSONTranscript(data)
		return cues, "json", err
	case "vtt":
		cues, err := parseVTTTranscript(normalizeTranscriptNewlines(string(data)))
		return cues, "vtt", err
	case "srt":
		cues, err := parseSRTTranscript(normalizeTranscriptNewlines(string(data)))
		return cues, "srt", err
	case "text":
		cues, err := parsePlainTextTranscript(normalizeTranscriptNewlines(string(data)))
		return cues, "text", err
	default:
		return nil, "", ErrInvalidTranscript
	}
}

func normalizeTranscriptNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func detectTranscriptFormat(data []byte, contentType string) string {
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
		if err == nil {
			switch strings.ToLower(mediaType) {
			case "application/json", "text/json":
				return "json"
			case "text/vtt":
				return "vtt"
			case "application/x-subrip", "application/srt", "text/srt":
				return "srt"
			case "text/html", "application/xhtml+xml", "application/xml", "text/xml", "image/svg+xml":
				return ""
			}
		}
	}
	trimmed := strings.TrimSpace(normalizeTranscriptNewlines(string(data)))
	if trimmed == "" {
		return ""
	}
	if htmlDangerHint.MatchString(trimmed) {
		return ""
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return "json"
	}
	if strings.HasPrefix(trimmed, "WEBVTT") {
		rest := trimmed[len("WEBVTT"):]
		if rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n' || rest[0] == '.' {
			return "vtt"
		}
	}
	if looksLikeSRT(trimmed) {
		return "srt"
	}
	return "text"
}

func looksLikeSRT(trimmed string) bool {
	lines := strings.Split(trimmed, "\n")
	limit := len(lines)
	if limit > 8 {
		limit = 8
	}
	for _, line := range lines[:limit] {
		if srtTimestampPattern.MatchString(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

type rawJSONTranscript struct {
	Version  string                 `json:"version"`
	Segments []rawJSONTranscriptCue `json:"segments"`
	Cues     []rawJSONTranscriptCue `json:"cues"`
}

type rawJSONTranscriptCue struct {
	Speaker   string   `json:"speaker"`
	StartTime *float64 `json:"startTime"`
	EndTime   *float64 `json:"endTime"`
	Start     *float64 `json:"start"`
	End       *float64 `json:"end"`
	Body      string   `json:"body"`
	Text      string   `json:"text"`
}

func parseJSONTranscript(data []byte) ([]TranscriptCue, error) {
	var raw rawJSONTranscript
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrInvalidTranscript)
	}
	items := raw.Segments
	if items == nil {
		items = raw.Cues
	}
	if items == nil {
		return nil, fmt.Errorf("%w: missing transcript segments", ErrInvalidTranscript)
	}
	if len(items) > MaxTranscriptCues {
		return nil, fmt.Errorf("%w: too many cues", ErrInvalidTranscript)
	}
	cues := make([]TranscriptCue, 0, len(items))
	for _, item := range items {
		startSec := item.StartTime
		if startSec == nil {
			startSec = item.Start
		}
		if startSec == nil || math.IsNaN(*startSec) || math.IsInf(*startSec, 0) || *startSec < 0 || *startSec > 86400*30 {
			continue
		}
		startMs := int64(math.Round(*startSec * 1000))
		endMs := startMs
		endSec := item.EndTime
		if endSec == nil {
			endSec = item.End
		}
		if endSec != nil && !math.IsNaN(*endSec) && !math.IsInf(*endSec, 0) && *endSec >= *startSec && *endSec <= 86400*30 {
			endMs = int64(math.Round(*endSec * 1000))
		}
		body := item.Body
		if body == "" {
			body = item.Text
		}
		text := plainText(body, 4096)
		if text == "" {
			continue
		}
		cues = append(cues, TranscriptCue{
			StartMs: startMs,
			EndMs:   endMs,
			Speaker: plainText(item.Speaker, 120),
			Text:    text,
		})
	}
	return cues, nil
}

func parseVTTTranscript(content string) ([]TranscriptCue, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "WEBVTT") {
		return nil, fmt.Errorf("%w: missing WEBVTT header", ErrInvalidTranscript)
	}
	blocks := splitBlankLineBlocks(content)
	cues := make([]TranscriptCue, 0)
	for i, block := range blocks {
		if i == 0 {
			continue
		}
		blockLines := strings.Split(strings.TrimSpace(block), "\n")
		if len(blockLines) == 0 {
			continue
		}
		first := strings.TrimSpace(blockLines[0])
		if strings.HasPrefix(first, "NOTE") || strings.HasPrefix(first, "STYLE") || strings.HasPrefix(first, "REGION") {
			continue
		}
		timingIdx := -1
		for idx, line := range blockLines {
			if strings.Contains(line, "-->") {
				timingIdx = idx
				break
			}
		}
		if timingIdx < 0 || timingIdx+1 >= len(blockLines) {
			continue
		}
		startMs, endMs, ok := parseCueTimingLine(blockLines[timingIdx])
		if !ok {
			continue
		}
		rawText := strings.Join(blockLines[timingIdx+1:], " ")
		speaker, text := extractVTTVoice(rawText)
		if text == "" {
			continue
		}
		cues = append(cues, TranscriptCue{
			StartMs: startMs,
			EndMs:   endMs,
			Speaker: speaker,
			Text:    text,
		})
		if len(cues) > MaxTranscriptCues {
			return nil, fmt.Errorf("%w: too many cues", ErrInvalidTranscript)
		}
	}
	return cues, nil
}

func extractVTTVoice(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if match := vttVoiceTagPattern.FindStringSubmatch(trimmed); len(match) == 3 {
		speaker := plainText(match[1], 120)
		text := plainText(match[2], 4096)
		return speaker, text
	}
	return "", plainText(trimmed, 4096)
}

func parseSRTTranscript(content string) ([]TranscriptCue, error) {
	blocks := splitBlankLineBlocks(content)
	cues := make([]TranscriptCue, 0)
	for _, block := range blocks {
		blockLines := strings.Split(strings.TrimSpace(block), "\n")
		if len(blockLines) < 2 {
			continue
		}
		timingIdx := -1
		for idx, line := range blockLines {
			if strings.Contains(line, "-->") {
				timingIdx = idx
				break
			}
		}
		if timingIdx < 0 || timingIdx+1 >= len(blockLines) {
			continue
		}
		startMs, endMs, ok := parseCueTimingLine(blockLines[timingIdx])
		if !ok {
			continue
		}
		text := plainText(strings.Join(blockLines[timingIdx+1:], " "), 4096)
		if text == "" {
			continue
		}
		cues = append(cues, TranscriptCue{
			StartMs: startMs,
			EndMs:   endMs,
			Text:    text,
		})
		if len(cues) > MaxTranscriptCues {
			return nil, fmt.Errorf("%w: too many cues", ErrInvalidTranscript)
		}
	}
	return cues, nil
}

func parsePlainTextTranscript(content string) ([]TranscriptCue, error) {
	if htmlDangerHint.MatchString(content) {
		return nil, fmt.Errorf("%w: HTML content not allowed", ErrInvalidTranscript)
	}
	lines := strings.Split(content, "\n")
	cues := make([]TranscriptCue, 0)
	for _, line := range lines {
		cleaned := plainText(line, 4096)
		if cleaned == "" {
			continue
		}
		cues = append(cues, TranscriptCue{
			StartMs: 0,
			EndMs:   0,
			Text:    cleaned,
		})
		if len(cues) > MaxTranscriptCues {
			return nil, fmt.Errorf("%w: too many cues", ErrInvalidTranscript)
		}
	}
	return cues, nil
}

func splitBlankLineBlocks(content string) []string {
	lines := strings.Split(content, "\n")
	blocks := make([]string, 0)
	var cur []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(cur) > 0 {
				blocks = append(blocks, strings.Join(cur, "\n"))
				cur = cur[:0]
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		blocks = append(blocks, strings.Join(cur, "\n"))
	}
	return blocks
}

func parseCueTimingLine(line string) (int64, int64, bool) {
	parts := strings.SplitN(line, "-->", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	startStr := strings.TrimSpace(parts[0])
	endFields := strings.Fields(strings.TrimSpace(parts[1]))
	if len(endFields) == 0 {
		return 0, 0, false
	}
	startMs, ok := parseSubtitleTimestamp(startStr)
	if !ok {
		return 0, 0, false
	}
	endMs, ok := parseSubtitleTimestamp(endFields[0])
	if !ok {
		return 0, 0, false
	}
	if endMs < startMs {
		endMs = startMs
	}
	return startMs, endMs, true
}

func parseSubtitleTimestamp(raw string) (int64, bool) {
	s := strings.TrimSpace(strings.ReplaceAll(raw, ",", "."))
	if s == "" {
		return 0, false
	}
	mainAndMs := strings.SplitN(s, ".", 2)
	hms := strings.Split(mainAndMs[0], ":")
	if len(hms) != 2 && len(hms) != 3 {
		return 0, false
	}
	var totalSec int64
	for _, part := range hms {
		if part == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil || n < 0 || n > 86400 {
			return 0, false
		}
		totalSec = totalSec*60 + n
	}
	var ms int64
	if len(mainAndMs) == 2 {
		frac := mainAndMs[1]
		if len(frac) == 0 || len(frac) > 3 {
			if len(frac) > 3 {
				frac = frac[:3]
			} else {
				return 0, false
			}
		}
		for len(frac) < 3 {
			frac += "0"
		}
		parsedMs, err := strconv.ParseInt(frac, 10, 64)
		if err != nil || parsedMs < 0 || parsedMs > 999 {
			return 0, false
		}
		ms = parsedMs
	}
	totalMs := totalSec*1000 + ms
	if totalMs < 0 || totalMs > 86400*30*1000 {
		return 0, false
	}
	return totalMs, true
}

// FetchTranscript fetches and parses a podcast transcript from a validated
// public HTTPS target using the public-IP-pinned transport.
func (f *Fetcher) FetchTranscript(ctx context.Context, raw, declaredType string) ([]TranscriptCue, string, error) {
	u, err := ValidatePublicURL(raw, f.AllowHTTP)
	if err != nil {
		return nil, "", err
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
		return nil, "", ErrBlockedURL
	}
	req.Header.Set("User-Agent", "ViiB-MediaHub-Podcast/1.0")
	req.Header.Set("Accept", "application/json, text/vtt, application/x-subrip, text/srt, text/plain;q=0.8")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, ErrBlockedURL) {
			return nil, "", ErrBlockedURL
		}
		return nil, "", ErrFeedUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("%w: HTTP %d", ErrFeedUnavailable, resp.StatusCode)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && !strings.EqualFold(enc, "identity") {
		return nil, "", fmt.Errorf("%w: compressed transcript not supported", ErrInvalidTranscript)
	}
	if resp.ContentLength > MaxTranscriptBytes {
		return nil, "", ErrInvalidTranscript
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxTranscriptBytes+1))
	if err != nil || int64(len(body)) > MaxTranscriptBytes {
		return nil, "", ErrInvalidTranscript
	}
	ct := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if ct == "" {
		ct = declaredType
	}
	return ParseTranscript(bytes.NewReader(body), ct)
}
