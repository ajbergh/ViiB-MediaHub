// Fetches optional Spotify reference data with purpose-scoped credentials, bounded responses, and typed failures.
package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/metadata"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Code string

const (
	Disabled               Code = "disabled"
	InvalidTrackID         Code = "invalid_track_id"
	AuthenticationRequired Code = "authentication_required"
	AccessDenied           Code = "access_denied"
	NotFound               Code = "not_found"
	AnalysisUnavailable    Code = "analysis_unavailable"
	RateLimited            Code = "rate_limited"
	TemporarilyUnavailable Code = "temporarily_unavailable"
	ProviderChanged        Code = "provider_changed"
)

type Error struct {
	Code       Code
	HTTPStatus int
	RetryAfter time.Duration
}

func (e *Error) Error() string             { return "spotify analysis: " + string(e.Code) }
func failure(code Code, status int) *Error { return &Error{Code: code, HTTPStatus: status} }

type Tokens interface {
	Token(context.Context, auth.Purpose) (auth.Token, error)
	Refresh(context.Context, auth.Purpose, auth.Token) (auth.Token, error)
}
type Options struct {
	Enabled        bool
	AppVersion     string
	Client         *http.Client
	Now            func() time.Time
	AccountContext string
}
type Client struct {
	appVersion     string
	enabled        bool
	http           *http.Client
	tokens         Tokens
	now            func() time.Time
	accountContext string
}

func NewClient(tokens Tokens, opts Options) *Client {
	client := http.Client{Timeout: 20 * time.Second}
	if opts.Client != nil {
		client = *opts.Client
	}
	// Reject even same-origin redirects; a private endpoint change is evidence to
	// investigate, not permission to forward credentials.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client.Jar = nil
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Client{appVersion: opts.AppVersion, enabled: opts.Enabled, http: &client, tokens: tokens, now: now, accountContext: opts.AccountContext}
}

var trackID = regexp.MustCompile("^[A-Za-z0-9]{22}$")

func (c *Client) Fetch(ctx context.Context, id string) (Observation, error) {
	return c.fetch(ctx, id, false)
}

// FetchFeatures explicitly requests scalar audio features. It is not an automatic
// fallback and supplies no detailed timing arrays or invented confidence values.
func (c *Client) FetchFeatures(ctx context.Context, id string) (Observation, error) {
	return c.fetch(ctx, id, true)
}

func (c *Client) fetch(ctx context.Context, id string, features bool) (Observation, error) {
	if !c.enabled || c.appVersion == "" || strings.ContainsAny(c.appVersion, "\r\n") {
		return Observation{}, failure(Disabled, 0)
	}
	if !trackID.MatchString(id) {
		return Observation{}, failure(InvalidTrackID, 0)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	if c.tokens == nil {
		return Observation{}, failure(Disabled, 0)
	}
	token, err := c.tokens.Token(ctx, auth.InternalAnalysis)
	if err != nil {
		return Observation{}, tokenError(ctx, err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if token.Kind != auth.WebPlayer || token.Bearer() == "" {
			return Observation{}, failure(AuthenticationRequired, 0)
		}
		endpoint := "audio-analysis/" + id
		if features {
			endpoint = "audio-features/" + id + "?format=json"
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://spclient.wg.spotify.com/audio-attributes/v1/"+endpoint, nil)
		if err != nil {
			return Observation{}, failure(ProviderChanged, 0)
		}
		request.Header.Set("Authorization", "Bearer "+token.Bearer())
		request.Header.Set("App-Platform", "WebPlayer")
		request.Header.Set("Spotify-App-Version", c.appVersion)
		request.Header.Set("Accept", "application/json")
		response, err := c.http.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return Observation{}, ctx.Err()
			}
			return Observation{}, failure(TemporarilyUnavailable, 0)
		}
		if response.StatusCode == http.StatusUnauthorized && attempt == 0 {
			response.Body.Close()
			token, err = c.tokens.Refresh(ctx, auth.InternalAnalysis, token)
			if err != nil {
				return Observation{}, tokenError(ctx, err)
			}
			continue
		}
		var observation Observation
		var decodeErr error
		if features {
			observation, decodeErr = c.decodeFeatures(response, id)
		} else {
			observation, decodeErr = c.decode(response, id)
		}
		response.Body.Close()
		if ctx.Err() != nil {
			return Observation{}, ctx.Err()
		}
		return observation, decodeErr
	}
	return Observation{}, failure(AuthenticationRequired, http.StatusUnauthorized)
}
func (c *Client) decode(response *http.Response, id string) (Observation, error) {
	status := response.StatusCode
	if status != http.StatusOK {
		code := TemporarilyUnavailable
		switch {
		case status == http.StatusUnauthorized:
			code = AuthenticationRequired
		case status == http.StatusForbidden:
			code = AccessDenied
		case status == http.StatusNotFound:
			code = NotFound
		case status == http.StatusTooManyRequests:
			code = RateLimited
		case status >= 300 && status < 500:
			code = ProviderChanged
		}
		err := failure(code, status)
		if code == RateLimited {
			err.RetryAfter = auth.RetryAfter(response.Header.Get("Retry-After"), c.now())
		}
		return Observation{}, err
	}
	return c.decodePayload(response, id, false)
}

func (c *Client) decodeFeatures(response *http.Response, id string) (Observation, error) {
	if response.StatusCode != http.StatusOK {
		return c.decode(response, id)
	}
	return c.decodePayload(response, id, true)
}

func (c *Client) decodePayload(response *http.Response, id string, features bool) (Observation, error) {
	status := response.StatusCode
	const maxBody = 8 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return Observation{}, failure(TemporarilyUnavailable, status)
	}
	if len(body) > maxBody {
		return Observation{}, failure(ProviderChanged, status)
	}
	body, rejected, sanitizeErr := sanitizeScalars(body, features)
	if sanitizeErr != nil {
		return Observation{}, failure(ProviderChanged, status)
	}
	if !features {
		var arrayRejected []FieldRejection
		body, arrayRejected, sanitizeErr = sanitizeDetailedArrays(body)
		if sanitizeErr != nil {
			return Observation{}, failure(ProviderChanged, status)
		}
		rejected = append(rejected, arrayRejected...)
	}
	var data payload
	if features {
		var feature struct {
			track
			ID         string   `json:"id"`
			Type       string   `json:"type"`
			DurationMS *float64 `json:"duration_ms"`
		}
		if json.Unmarshal(body, &feature) != nil || feature.ID != id || (feature.Type != "" && feature.Type != "audio_features") || !finite(feature.DurationMS) || (feature.DurationMS != nil && *feature.DurationMS <= 0) {
			return Observation{}, failure(ProviderChanged, status)
		}
		if feature.DurationMS != nil {
			seconds := *feature.DurationMS / 1000
			feature.Duration = &seconds
		}
		data.Track = &feature.track
	} else {
		var identity struct {
			ID    string `json:"id"`
			Track struct {
				ID string `json:"id"`
			} `json:"track"`
		}
		if json.Unmarshal(body, &identity) != nil || (identity.ID != "" && identity.ID != id) || (identity.Track.ID != "" && identity.Track.ID != id) || json.Unmarshal(body, &data) != nil {
			return Observation{}, failure(ProviderChanged, status)
		}
	}
	observation, err := normalize(id, data, c.now())
	var analysisErr *Error
	if errors.As(err, &analysisErr) {
		analysisErr.HTTPStatus = status
	}
	observation.RejectedFields = rejected
	if err != nil && len(rejected) > 0 {
		return Observation{}, failure(ProviderChanged, status)
	}
	if features {
		var original struct {
			DurationMS *float64 `json:"duration_ms"`
		}
		_ = json.Unmarshal(body, &original)
		observation.DurationMilliseconds = original.DurationMS
		observation.SourceEndpoint = "audio_features"
	}
	if err == nil {
		limit := metadata.DetailedLimit
		if features {
			limit = metadata.ScalarLimit
		}
		safe, sanitizeErr := metadata.Sanitize(body, limit)
		if sanitizeErr != nil {
			return Observation{}, failure(ProviderChanged, status)
		}
		observation.DomainPayload = safe
		observation.AccountContext = c.accountContext
	}
	return observation, err
}

func tokenError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch {
	case errors.Is(err, auth.ErrDisabled):
		return failure(Disabled, 0)
	case errors.Is(err, auth.ErrAuthenticationRequired):
		return failure(AuthenticationRequired, 0)
	case errors.Is(err, auth.ErrProviderChanged), errors.Is(err, auth.ErrTokenKind):
		return failure(ProviderChanged, 0)
	}
	var httpErr *auth.WebPlayerHTTPError
	if errors.As(err, &httpErr) {
		code := TemporarilyUnavailable
		if httpErr.Status == http.StatusForbidden {
			code = AccessDenied
		}
		if httpErr.Status >= 300 && httpErr.Status < 400 {
			code = ProviderChanged
		}
		if httpErr.Status == http.StatusTooManyRequests {
			code = RateLimited
		}
		return &Error{Code: code, HTTPStatus: httpErr.Status, RetryAfter: httpErr.RetryAfter}
	}
	return failure(TemporarilyUnavailable, 0)
}

// ValidateDomainPayload applies the transport's field and artifact validation to
// already sanitized persistence input. No network or credentials are involved.
func ValidateDomainPayload(id, resource string, raw []byte) (Observation, error) {
	if !trackID.MatchString(id) || (resource != "audio_analysis" && resource != "audio_features") {
		return Observation{}, failure(InvalidTrackID, 0)
	}
	client := NewClient(nil, Options{})
	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(raw))}
	observation, err := client.decodePayload(response, id, resource == "audio_features")
	if err != nil {
		return Observation{}, err
	}
	if len(observation.RejectedFields) > 0 {
		return Observation{}, failure(ProviderChanged, http.StatusOK)
	}
	return observation, nil
}
