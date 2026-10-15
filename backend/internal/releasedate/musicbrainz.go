package releasedate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/time/rate"
)

type Match struct {
	Date        string
	RecordingID string
	State       string
}
type Client struct {
	HTTP    *http.Client
	BaseURL string
	limiter *rate.Limiter
	mu      sync.Mutex
	retryAt time.Time
}

type CooldownError struct{ Until time.Time }

func (e *CooldownError) Error() string { return "MusicBrainz is temporarily rate limited" }

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 12 * time.Second}, BaseURL: "https://musicbrainz.org/ws/2", limiter: rate.NewLimiter(rate.Every(time.Second), 1)}
}

var remasterSuffix = regexp.MustCompile(`(?i)\s*(?:[-–]\s*|\()(?:\d{4}\s+)?remaster(?:ed)?(?:\s+\d{4})?\)?\s*$`)
var recordingID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func matchText(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, s)), " ")
}
func luceneQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// Search only admits an unambiguous recording with matching title, artist and
// duration. Remaster suffixes may differ; live/remix/edit identity is retained.
func (c *Client) Lookup(ctx context.Context, title, artist string, duration float64) (Match, error) {
	result := Match{State: "not_found"}
	if title == "" || artist == "" || len(title) > 300 || len(artist) > 300 || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return result, nil
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return result, err
	}
	c.mu.Lock()
	retryAt := c.retryAt
	c.mu.Unlock()
	if time.Now().Before(retryAt) {
		return result, &CooldownError{Until: retryAt}
	}
	title = remasterSuffix.ReplaceAllString(title, "")
	query := "recording:" + luceneQuote(title) + " AND artist:" + luceneQuote(artist)
	target := c.BaseURL + "/recording/?" + url.Values{"query": {query}, "fmt": {"json"}, "limit": {"25"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("User-Agent", "ViiB-MediaHub/1.0 (https://github.com/ajbergh/ViiB-MediaHub)")
	req.Header.Set("Accept", "application/json")
	response, err := c.HTTP.Do(req)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == 429 || response.StatusCode == 503 {
			until := time.Now().Add(time.Minute)
			if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
				until = time.Now().Add(time.Duration(min(seconds, 86400)) * time.Second)
			} else if parsed, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil && parsed.After(time.Now()) {
				until = parsed
				if until.After(time.Now().Add(24 * time.Hour)) {
					until = time.Now().Add(24 * time.Hour)
				}
			}
			c.mu.Lock()
			if until.After(c.retryAt) {
				c.retryAt = until
			}
			c.mu.Unlock()
			return result, &CooldownError{Until: until}
		}
		return result, fmt.Errorf("MusicBrainz returned HTTP %d", response.StatusCode)
	}
	var body struct {
		Recordings []struct {
			ID             string `json:"id"`
			Title          string `json:"title"`
			Disambiguation string `json:"disambiguation"`
			Length         int    `json:"length"`
			Score          int    `json:"score"`
			Date           string `json:"first-release-date"`
			Credits        []struct {
				Name   string `json:"name"`
				Artist struct {
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"artist-credit"`
		} `json:"recordings"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	if err != nil || len(data) > 2<<20 {
		return result, fmt.Errorf("MusicBrainz response exceeds limit or could not be read")
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return result, err
	}
	if len(body.Recordings) > 25 {
		return result, fmt.Errorf("MusicBrainz recording limit exceeded")
	}
	for _, recording := range body.Recordings {
		if recording.Score < 95 || matchText(recording.Title) != matchText(title) || recording.Length <= 0 || math.Abs(float64(recording.Length)/1000-duration) > math.Max(3, duration*.01) || len(recording.Credits) == 0 {
			continue
		}
		credit := recording.Credits[0]
		if matchText(credit.Name) != matchText(artist) && matchText(credit.Artist.Name) != matchText(artist) {
			continue
		}
		wrongVersion := false
		for _, version := range []string{"live", "remix", "instrumental", "demo", "karaoke", "edit", "acoustic"} {
			if strings.Contains(matchText(recording.Disambiguation), version) && !strings.Contains(matchText(title), version) {
				wrongVersion = true
			}
		}
		date, _, _ := Parse(recording.Date)
		if wrongVersion || date == "" || !recordingID.MatchString(recording.ID) {
			continue
		}
		if result.Date != "" && (result.Date != date || result.RecordingID != recording.ID) {
			return Match{State: "ambiguous"}, nil
		}
		result = Match{Date: date, RecordingID: recording.ID, State: "available"}
	}
	return result, nil
}
