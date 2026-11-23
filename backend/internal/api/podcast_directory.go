// podcast_directory.go exposes redacted Podcast Index credential status and
// SSRF-guarded search and trending HTTP endpoints under /api/v2/podcasts.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ajbergh/viib-mediahub/internal/crypto"
	"github.com/ajbergh/viib-mediahub/internal/podcast"
)

const (
	maxPodcastDirectoryCredentialBodyBytes = 1024
	maxPodcastDirectoryCredentialLen       = 256
	defaultPodcastDirectoryMaxResults      = 20
	maxPodcastDirectoryResults             = 40
)

type podcastDirectoryClient interface {
	Search(ctx context.Context, creds podcast.DirectoryCredentials, query string, maxResults int) ([]podcast.DirectoryShow, error)
	Trending(ctx context.Context, creds podcast.DirectoryCredentials, category, lang string, maxResults int) ([]podcast.DirectoryShow, error)
	LookupByFeedURL(ctx context.Context, creds podcast.DirectoryCredentials, feedURL string) (podcast.DirectoryShow, bool, error)
}

var podcastDirectoryClients sync.Map // map[*API]podcastDirectoryClient

func (a *API) hasPodcastDirectoryClientForTest() bool {
	_, ok := podcastDirectoryClients.Load(a)
	return ok
}

func (a *API) setPodcastDirectoryClientForTest(client podcastDirectoryClient) {
	if client == nil {
		podcastDirectoryClients.Delete(a)
		return
	}
	podcastDirectoryClients.Store(a, client)
}

func (a *API) getPodcastDirectoryClient() podcastDirectoryClient {
	if v, ok := podcastDirectoryClients.Load(a); ok {
		if client, ok := v.(podcastDirectoryClient); ok && client != nil {
			return client
		}
	}
	return podcast.NewDirectoryClient()
}

type podcastDirectoryCredentialsRequest struct {
	APIKey    string `json:"apiKey"`
	APISecret string `json:"apiSecret"`
}

func (a *API) getPodcastDiscoverStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.db.GetPodcastDirectoryStatus(r.Context())
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "directory_status_unavailable",
			"Unable to read podcast directory status", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"status": status})
}

func (a *API) putPodcastDiscoverCredentials(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_directory_credentials",
			"Provide JSON Podcast Index API credentials", false, nil)
		return
	}

	var input podcastDirectoryCredentialsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodcastDirectoryCredentialBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_directory_credentials",
			"Invalid Podcast Index credentials payload", false, nil)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_directory_credentials",
			"Unexpected trailing content in credentials payload", false, nil)
		return
	}

	if strings.ContainsAny(input.APIKey, "\r\n\x00") || strings.ContainsAny(input.APISecret, "\r\n\x00") {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_directory_credentials",
			"Provide non-empty apiKey and apiSecret up to 256 characters", false, nil)
		return
	}
	apiKey := strings.TrimSpace(input.APIKey)
	apiSecret := strings.TrimSpace(input.APISecret)
	if apiKey == "" || apiSecret == "" ||
		len(apiKey) > maxPodcastDirectoryCredentialLen || len(apiSecret) > maxPodcastDirectoryCredentialLen {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_directory_credentials",
			"Provide non-empty apiKey and apiSecret up to 256 characters", false, nil)
		return
	}

	if err := a.db.SetPodcastDirectoryCredentials(r.Context(), apiKey, apiSecret); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "directory_credentials_save_failed",
			"Unable to save podcast directory credentials", true, nil)
		return
	}

	status, err := a.db.GetPodcastDirectoryStatus(r.Context())
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "directory_status_unavailable",
			"Credentials saved but status could not be read", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"status": status})
}

func (a *API) deletePodcastDiscoverCredentials(w http.ResponseWriter, r *http.Request) {
	if err := a.db.DeletePodcastDirectoryCredentials(r.Context()); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "directory_credentials_delete_failed",
			"Unable to delete podcast directory credentials", true, nil)
		return
	}
	status, err := a.db.GetPodcastDirectoryStatus(r.Context())
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "directory_status_unavailable",
			"Unable to read podcast directory status", true, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{"status": status})
}

func (a *API) searchPodcastDirectory(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	qRunes := utf8.RuneCountInString(query)
	if qRunes < 1 || qRunes > 200 {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_search_query",
			"Provide a search query between 1 and 200 characters", false, nil)
		return
	}

	maxResults, ok := parseDirectoryMaxResults(r.URL.Query().Get("max"))
	if !ok {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_pagination",
			"Result limit must be between 1 and 40", false, nil)
		return
	}

	creds, ok := a.loadPodcastDirectoryCredentials(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	shows, err := a.getPodcastDirectoryClient().Search(ctx, creds, query, maxResults)
	if err != nil {
		respondPodcastDirectoryError(w, r, err)
		return
	}
	if shows == nil {
		shows = []podcast.DirectoryShow{}
	}

	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{
		"shows":    shows,
		"provider": "podcast_index",
		"query":    query,
		"count":    len(shows),
	})
}

func (a *API) trendingPodcastDirectory(w http.ResponseWriter, r *http.Request) {
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	if utf8.RuneCountInString(category) > 100 {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_directory_filter",
			"Category filter must be 100 characters or fewer", false, nil)
		return
	}
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	if utf8.RuneCountInString(lang) > 32 {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_directory_filter",
			"Language filter must be 32 characters or fewer", false, nil)
		return
	}

	maxResults, ok := parseDirectoryMaxResults(r.URL.Query().Get("max"))
	if !ok {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_pagination",
			"Result limit must be between 1 and 40", false, nil)
		return
	}

	creds, ok := a.loadPodcastDirectoryCredentials(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	shows, err := a.getPodcastDirectoryClient().Trending(ctx, creds, category, lang, maxResults)
	if err != nil {
		respondPodcastDirectoryError(w, r, err)
		return
	}
	if shows == nil {
		shows = []podcast.DirectoryShow{}
	}

	w.Header().Set("Cache-Control", "no-store")
	respondV2JSON(w, http.StatusOK, map[string]any{
		"shows":    shows,
		"provider": "podcast_index",
		"count":    len(shows),
	})
}

func parseDirectoryMaxResults(raw string) (int, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultPodcastDirectoryMaxResults, true
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed < 1 || parsed > maxPodcastDirectoryResults {
		return 0, false
	}
	return parsed, true
}

func (a *API) loadPodcastDirectoryCredentials(w http.ResponseWriter, r *http.Request) (podcast.DirectoryCredentials, bool) {
	creds, err := a.db.GetPodcastDirectoryCredentials(r.Context())
	if err == nil {
		return creds, true
	}
	switch {
	case errors.Is(err, podcast.ErrDirectoryNotConfigured):
		respondV2Error(w, r, http.StatusPreconditionFailed, "provider_not_configured",
			"Podcast Index credentials are not configured. Add your Podcast Index API key and secret in Settings, or subscribe directly with an RSS feed URL.",
			false, nil)
	case errors.Is(err, crypto.ErrReauthRequired):
		respondV2Error(w, r, http.StatusPreconditionFailed, "directory_reauth_required",
			"Stored Podcast Index credentials could not be decrypted; please re-enter your Podcast Index API key and secret, or subscribe directly with an RSS feed URL.",
			false, nil)
	default:
		respondV2Error(w, r, http.StatusInternalServerError, "directory_credentials_unavailable",
			"Unable to read podcast directory credentials", true, nil)
	}
	return podcast.DirectoryCredentials{}, false
}

func respondPodcastDirectoryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, podcast.ErrDirectoryNotConfigured):
		respondV2Error(w, r, http.StatusPreconditionFailed, "provider_not_configured",
			"Podcast Index credentials are not configured. Add your Podcast Index API key and secret in Settings, or subscribe directly with an RSS feed URL.",
			false, nil)
	case errors.Is(err, podcast.ErrDirectoryUnauthorized):
		respondV2Error(w, r, http.StatusUnauthorized, "directory_unauthorized",
			"Podcast Index rejected the configured API credentials", false, nil)
	case errors.Is(err, podcast.ErrDirectoryRateLimited):
		respondV2Error(w, r, http.StatusTooManyRequests, "directory_rate_limited",
			"Podcast Index rate limit exceeded; retry shortly", true, nil)
	default:
		respondV2Error(w, r, http.StatusBadGateway, "directory_unavailable",
			"Podcast directory search is temporarily unavailable", true, nil)
	}
}
