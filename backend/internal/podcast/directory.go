package podcast

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPodcastIndexBaseURL = "https://api.podcastindex.org/api/1.0"
	directoryUserAgent           = "ViiB-MediaHub/1.0"
	defaultDirectoryMaxResults   = 20
	maxDirectoryMaxResults       = 40
	maxDirectoryResponseBytes    = 4 << 20
)

var (
	ErrDirectoryNotConfigured = errors.New("podcast directory credentials not configured")
	ErrDirectoryUnauthorized  = errors.New("podcast directory credentials rejected")
	ErrDirectoryRateLimited   = errors.New("podcast directory rate limit exceeded")
	ErrDirectoryUnavailable   = errors.New("podcast directory unavailable")
)

type DirectoryCredentials struct {
	APIKey    string `json:"apiKey"`
	APISecret string `json:"apiSecret"`
}

type DirectoryShow struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Author       string   `json:"author,omitempty"`
	Description  string   `json:"description,omitempty"`
	FeedURL      string   `json:"feedUrl"`
	WebsiteURL   string   `json:"websiteUrl,omitempty"`
	ArtworkURL   string   `json:"artworkUrl,omitempty"`
	Language     string   `json:"language,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	EpisodeCount int      `json:"episodeCount,omitempty"`
}

// DirectoryClient queries the Podcast Index API v1.0 using public-IP
// DNS-pinning transport and normalized, sanitized output structs.
type DirectoryClient struct {
	fetcher *Fetcher
	// baseURL is overridable ONLY in tests via this unexported struct field.
	baseURL string
	now     func() time.Time
}

func NewDirectoryClient() *DirectoryClient {
	return &DirectoryClient{
		fetcher: NewFetcher(false),
		baseURL: defaultPodcastIndexBaseURL,
		now:     time.Now,
	}
}

func podcastIndexAuthHeaders(apiKey, apiSecret string, at time.Time) (authDate, authToken string) {
	authDate = strconv.FormatInt(at.Unix(), 10)
	sum := sha1.Sum([]byte(apiKey + apiSecret + authDate))
	return authDate, fmt.Sprintf("%x", sum)
}

func clampDirectoryMaxResults(maxResults int) int {
	if maxResults <= 0 {
		return defaultDirectoryMaxResults
	}
	if maxResults > maxDirectoryMaxResults {
		return maxDirectoryMaxResults
	}
	return maxResults
}

func (c *DirectoryClient) Search(ctx context.Context, creds DirectoryCredentials, query string, maxResults int) ([]DirectoryShow, error) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []DirectoryShow{}, nil
	}
	params := url.Values{}
	params.Set("q", trimmedQuery)
	params.Set("max", strconv.Itoa(clampDirectoryMaxResults(maxResults)))
	return c.queryShows(ctx, creds, "/search/byterm", params)
}

func (c *DirectoryClient) Trending(ctx context.Context, creds DirectoryCredentials, category, lang string, maxResults int) ([]DirectoryShow, error) {
	params := url.Values{}
	params.Set("max", strconv.Itoa(clampDirectoryMaxResults(maxResults)))
	if cat := strings.TrimSpace(category); cat != "" {
		params.Set("cat", cat)
	}
	if language := strings.TrimSpace(lang); language != "" {
		params.Set("lang", language)
	}
	return c.queryShows(ctx, creds, "/podcasts/trending", params)
}

type podcastIndexFeedResponse struct {
	Feeds []podcastIndexFeed `json:"feeds"`
}

type podcastIndexFeed struct {
	ID           int64           `json:"id"`
	Title        string          `json:"title"`
	URL          string          `json:"url"`
	Link         string          `json:"link"`
	Description  string          `json:"description"`
	Author       string          `json:"author"`
	OwnerName    string          `json:"ownerName"`
	Image        string          `json:"image"`
	Artwork      string          `json:"artwork"`
	Language     string          `json:"language"`
	Categories   json.RawMessage `json:"categories"`
	EpisodeCount int             `json:"episodeCount"`
}

func (c *DirectoryClient) queryShows(ctx context.Context, creds DirectoryCredentials, endpoint string, params url.Values) ([]DirectoryShow, error) {
	apiKey := strings.TrimSpace(creds.APIKey)
	apiSecret := strings.TrimSpace(creds.APISecret)
	if apiKey == "" || apiSecret == "" {
		return nil, ErrDirectoryNotConfigured
	}

	fetcher := c.fetcher
	if fetcher == nil {
		fetcher = NewFetcher(false)
	}
	baseRaw := strings.TrimSpace(c.baseURL)
	if baseRaw == "" {
		baseRaw = defaultPodcastIndexBaseURL
	}
	baseURL, err := validateDirectoryBaseURL(baseRaw, fetcher.AllowHTTP)
	if err != nil {
		return nil, ErrDirectoryUnavailable
	}

	targetURL := *baseURL
	targetURL.Path = strings.TrimRight(baseURL.Path, "/") + endpoint
	targetURL.RawQuery = params.Encode()

	nowFn := c.now
	if nowFn == nil {
		nowFn = time.Now
	}
	authDate, authToken := podcastIndexAuthHeaders(apiKey, apiSecret, nowFn().UTC())

	tr := fetcher.transport()
	defer tr.CloseIdleConnections()
	client := &http.Client{
		Transport: tr,
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return ErrBlockedURL
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return nil, ErrDirectoryUnavailable
	}
	req.Header.Set("User-Agent", directoryUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("X-Auth-Date", authDate)
	req.Header.Set("X-Auth-Key", apiKey)
	req.Header.Set("Authorization", authToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, ErrDirectoryUnavailable
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrDirectoryUnauthorized
	case http.StatusTooManyRequests:
		return nil, ErrDirectoryRateLimited
	default:
		return nil, ErrDirectoryUnavailable
	}

	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return nil, ErrDirectoryUnavailable
	}
	if resp.ContentLength > maxDirectoryResponseBytes {
		return nil, ErrDirectoryUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDirectoryResponseBytes+1))
	if err != nil || int64(len(body)) > maxDirectoryResponseBytes {
		return nil, ErrDirectoryUnavailable
	}

	var decoded podcastIndexFeedResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, ErrDirectoryUnavailable
	}

	shows := make([]DirectoryShow, 0, len(decoded.Feeds))
	for _, item := range decoded.Feeds {
		validatedFeedURL, err := ValidatePublicURL(item.URL, false)
		if err != nil {
			continue
		}
		artworkURL := SafeArtworkURL(item.Artwork)
		if artworkURL == "" {
			artworkURL = SafeArtworkURL(item.Image)
		}
		title := plainText(item.Title, 300)
		if title == "" {
			title = validatedFeedURL.Hostname()
		}
		author := plainText(item.Author, 300)
		if author == "" {
			author = plainText(item.OwnerName, 300)
		}
		episodeCount := item.EpisodeCount
		if episodeCount < 0 {
			episodeCount = 0
		}
		shows = append(shows, DirectoryShow{
			ID:           item.ID,
			Title:        title,
			Author:       author,
			Description:  plainText(item.Description, 4096),
			FeedURL:      validatedFeedURL.String(),
			WebsiteURL:   safeWebsiteURL(item.Link),
			ArtworkURL:   artworkURL,
			Language:     plainText(item.Language, 50),
			Categories:   parseDirectoryCategories(item.Categories),
			EpisodeCount: episodeCount,
		})
	}
	return shows, nil
}

func validateDirectoryBaseURL(raw string, allowHTTP bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil {
		return nil, ErrBlockedURL
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
		return nil, ErrBlockedURL
	}
	if u.Opaque != "" || u.Fragment != "" || u.RawQuery != "" {
		return nil, ErrBlockedURL
	}
	if strings.ContainsAny(u.Hostname(), "% \t\r\n") {
		return nil, ErrBlockedURL
	}
	if literal, err := netip.ParseAddr(u.Hostname()); err == nil && !IsPublicIP(literal) {
		return nil, ErrBlockedURL
	}
	return u, nil
}

func safeWebsiteURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > 2048 {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil {
		return ""
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	if u.Opaque != "" || strings.ContainsAny(u.Hostname(), "% \t\r\n") {
		return ""
	}
	if literal, err := netip.ParseAddr(u.Hostname()); err == nil && !IsPublicIP(literal) {
		return ""
	}
	return u.String()
}

func parseDirectoryCategories(raw json.RawMessage) []string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	addCategory := func(value string) {
		cleaned := plainText(value, 100)
		if cleaned != "" && !seen[cleaned] {
			seen[cleaned] = true
			out = append(out, cleaned)
		}
	}

	var byID map[string]string
	if err := json.Unmarshal(raw, &byID); err == nil {
		for _, v := range byID {
			addCategory(v)
		}
		sort.Strings(out)
		return out
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		for _, v := range list {
			addCategory(v)
		}
		sort.Strings(out)
		return out
	}

	return nil
}
