package waveform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

type Error struct {
	Code         spotifyanalysis.Code
	HTTPStatus   int
	EntityStatus int
	RetryAfter   time.Duration
}

func (e *Error) Error() string { return "Spotify waveform: " + string(e.Code) }

type Options struct {
	Enabled       bool
	AppVersion    string
	Client        *http.Client
	BeforeRequest func(context.Context) error
	OnRateLimit   func(string) time.Duration
}
type Client struct {
	tokens spotifyanalysis.Tokens
	http   http.Client
	opts   Options
}

func NewClient(tokens spotifyanalysis.Tokens, opts Options) *Client {
	httpClient := http.Client{Timeout: 20 * time.Second}
	if opts.Client != nil {
		httpClient = *opts.Client
		if httpClient.Timeout == 0 || httpClient.Timeout > 20*time.Second {
			httpClient.Timeout = 20 * time.Second
		}
	}
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{tokens: tokens, http: httpClient, opts: opts}
}
func codeForStatus(status int) spotifyanalysis.Code {
	switch status {
	case 401:
		return spotifyanalysis.AuthenticationRequired
	case 403, 451:
		return spotifyanalysis.AccessDenied
	case 404:
		return spotifyanalysis.NotFound
	case 429:
		return spotifyanalysis.RateLimited
	default:
		if status >= 300 && status < 500 {
			return spotifyanalysis.ProviderChanged
		}
		return spotifyanalysis.TemporarilyUnavailable
	}
}
func tokenFailure(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	code := spotifyanalysis.TemporarilyUnavailable
	if errors.Is(err, auth.ErrAuthenticationRequired) {
		code = spotifyanalysis.AuthenticationRequired
	}
	if errors.Is(err, auth.ErrDisabled) {
		code = spotifyanalysis.Disabled
	}
	return &Error{Code: code}
}

// Fetch requests only extension 237. It never falls back to scalar/detailed routes.
// A 304 is reusable only for a validated cache with this ID and exact provider ETag.
func (c *Client) Fetch(ctx context.Context, id, etag string, cached *Waveform) (Waveform, error) {
	if !c.opts.Enabled || c.tokens == nil || c.opts.AppVersion == "" || strings.ContainsAny(c.opts.AppVersion, "\r\n") {
		return Waveform{}, &Error{Code: spotifyanalysis.Disabled}
	}
	if !validID.MatchString(id) || len(etag) > 1024 {
		return Waveform{}, &Error{Code: spotifyanalysis.InvalidTrackID}
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	token, err := c.tokens.Token(ctx, auth.InternalAnalysis)
	if err != nil {
		return Waveform{}, tokenFailure(ctx, err)
	}
	renewed, revalidated := false, false
	for attempt := 0; attempt < 4; attempt++ {
		if ctx.Err() != nil {
			return Waveform{}, ctx.Err()
		}
		if token.Kind != auth.WebPlayer || token.Bearer() == "" {
			return Waveform{}, &Error{Code: spotifyanalysis.AuthenticationRequired}
		}
		body, _ := json.Marshal(map[string]any{"entityRequest": []any{map[string]any{"entityUri": "spotify:track:" + id, "query": []any{map[string]any{"extensionKind": ExtensionKind, "etag": etag}}}}})
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://spclient.wg.spotify.com/extended-metadata/v0/extended-metadata", bytes.NewReader(body))
		if err != nil {
			return Waveform{}, &Error{Code: spotifyanalysis.ProviderChanged}
		}
		request.Header.Set("Authorization", "Bearer "+token.Bearer())
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/protobuf")
		request.Header.Set("App-Platform", "WebPlayer")
		request.Header.Set("Spotify-App-Version", c.opts.AppVersion)
		if c.opts.BeforeRequest != nil {
			if err = c.opts.BeforeRequest(ctx); err != nil {
				return Waveform{}, err
			}
		}
		response, err := c.http.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return Waveform{}, ctx.Err()
			}
			return Waveform{}, &Error{Code: spotifyanalysis.TemporarilyUnavailable}
		}
		status := response.StatusCode
		if status == 401 && !renewed {
			response.Body.Close()
			renewed = true
			token, err = c.tokens.Refresh(ctx, auth.InternalAnalysis, token)
			if err != nil {
				return Waveform{}, tokenFailure(ctx, err)
			}
			continue
		}
		var waveform Waveform
		entityStatus := 0
		if status == 200 {
			raw, readErr := io.ReadAll(io.LimitReader(response.Body, MaxBody+1))
			response.Body.Close()
			if readErr != nil || len(raw) > MaxBody {
				return Waveform{}, &Error{Code: spotifyanalysis.ProviderChanged, HTTPStatus: status}
			}
			waveform, err = DecodeResponse(raw, id)
			if ctx.Err() != nil {
				return Waveform{}, ctx.Err()
			}
			if err == nil {
				return waveform, nil
			}
			var entity *EntityError
			if errors.As(err, &entity) {
				entityStatus = entity.Status
			} else {
				return Waveform{}, &Error{Code: spotifyanalysis.ProviderChanged, HTTPStatus: status}
			}
		} else {
			response.Body.Close()
		}
		if status == 304 || entityStatus == 304 {
			if etag != "" && cached != nil && cached.TrackID == id && cached.ETag == etag && cached.Validate() == nil {
				return *cached, nil
			}
			if !revalidated {
				revalidated = true
				etag = ""
				continue
			}
			return Waveform{}, &Error{Code: spotifyanalysis.ProviderChanged, HTTPStatus: status, EntityStatus: entityStatus}
		}
		effectiveStatus := status
		if entityStatus != 0 {
			effectiveStatus = entityStatus
		}
		failure := &Error{Code: codeForStatus(effectiveStatus), HTTPStatus: status, EntityStatus: entityStatus}
		if failure.Code == spotifyanalysis.RateLimited {
			failure.RetryAfter = auth.RetryAfter(response.Header.Get("Retry-After"), time.Now())
			if c.opts.OnRateLimit != nil {
				failure.RetryAfter = c.opts.OnRateLimit(response.Header.Get("Retry-After"))
			}
		}
		return Waveform{}, failure
	}
	return Waveform{}, &Error{Code: spotifyanalysis.ProviderChanged}
}
