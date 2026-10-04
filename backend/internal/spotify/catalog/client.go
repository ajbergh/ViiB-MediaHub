// Package catalog implements fixed Web Player catalog operations.
// Contract source: stupid-social f0f8c219e43d394c84516a3bcc7af7c9fd41f713
// scripts/spotify-web-client.py; Apache-2.0, license retained beside auth contract.
package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

var ErrSchema = errors.New("Spotify catalog response is incompatible")

type HTTPError struct {
	Stage      string
	Status     int
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string { return "Spotify catalog request denied" }
func (e *HTTPError) Unwrap() error {
	return &auth.WebPlayerHTTPError{Status: e.Status, RetryAfter: e.RetryAfter}
}

type Options struct {
	Client *http.Client
	Now    func() time.Time
	// Hooks enforce an application-wide cooldown on every upstream dispatch,
	// including client-token minting and nested library hydration requests.
	BeforeRequest func(context.Context) error
	OnRateLimit   func(string) time.Duration
}
type Client struct {
	mu                    sync.Mutex
	gate                  chan struct{}
	http                  http.Client
	now                   func() time.Time
	lifetime              context.Context
	cancel                context.CancelFunc
	closed                bool
	clientToken, clientID string
	generation            uint64
	deadline              time.Time
	beforeRequest         func(context.Context) error
	onRateLimit           func(string) time.Duration
}

func (c *Client) Format(state fmt.State, verb rune) {
	fmt.Fprint(state, "SpotifyCatalogClient{credentials:[REDACTED]}")
}

func New(options Options) *Client {
	client := http.Client{Timeout: 15 * time.Second}
	if options.Client != nil {
		client = *options.Client
		if client.Timeout == 0 || client.Timeout > 15*time.Second {
			client.Timeout = 15 * time.Second
		}
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	now := options.Now
	if now == nil {
		now = time.Now
	}
	lifetime, cancel := context.WithCancel(context.Background())
	return &Client{http: client, now: now, gate: make(chan struct{}, 1), lifetime: lifetime, cancel: cancel, beforeRequest: options.BeforeRequest, onRateLimit: options.OnRateLimit}
}
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.cancel()
	c.clientToken = ""
	c.clientID = ""
	c.deadline = time.Time{}
}
func (c *Client) InvalidateClientToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientToken = ""
	c.deadline = time.Time{}
}
func (c *Client) context(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	stop := context.AfterFunc(c.lifetime, cancel)
	if c.lifetime.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}
func (c *Client) clientContext(ctx context.Context, token auth.Token) (string, error) {
	if token.Kind != auth.WebPlayer || token.Bearer() == "" {
		return "", auth.ErrTokenKind
	}
	if token.WebPlayerClientID() == "" {
		return "", auth.ErrProviderChanged
	}
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-c.gate }()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return "", context.Canceled
	}
	if c.clientToken != "" && c.clientID == token.WebPlayerClientID() && c.generation == token.Generation && c.now().Add(15*time.Second).Before(c.deadline) {
		result := c.clientToken
		c.mu.Unlock()
		return result, nil
	}
	c.mu.Unlock()
	payload := map[string]any{"client_data": map[string]any{"client_version": auth.PinnedWebPlayerContract().AppVersion, "client_id": token.WebPlayerClientID(), "js_sdk_data": map[string]string{"device_brand": "", "device_id": "", "device_model": "", "device_type": "", "os": "", "os_version": ""}}}
	var reply struct {
		Granted struct {
			Token   string `json:"token"`
			Expires int64  `json:"expires_after_seconds"`
			Refresh int64  `json:"refresh_after_seconds"`
		} `json:"granted_token"`
	}
	if err := c.request(ctx, "client_token", "https://clienttoken.spotify.com/v1/clienttoken", payload, "", "", &reply); err != nil {
		return "", err
	}
	if reply.Granted.Token == "" || reply.Granted.Expires < 0 {
		return "", ErrSchema
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || ctx.Err() != nil {
		return "", context.Canceled
	}
	lifetime := reply.Granted.Expires
	if lifetime > 86400 {
		lifetime = 86400
	}
	if reply.Granted.Refresh > 0 && reply.Granted.Refresh < lifetime {
		lifetime = reply.Granted.Refresh
	}
	c.clientToken = reply.Granted.Token
	c.clientID = token.WebPlayerClientID()
	c.generation = token.Generation
	// A missing expiry allows this transaction but never cache reuse.
	c.deadline = c.now().Add(time.Duration(lifetime) * time.Second)
	return c.clientToken, nil
}
func (c *Client) request(ctx context.Context, stage, target string, payload any, bearer, clientToken string, result any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return ErrSchema
	}
	defer clear(data)
	request, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(data))
	if err != nil {
		return auth.ErrTemporarilyUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("App-Platform", "WebPlayer")
	request.Header.Set("Spotify-App-Version", auth.PinnedWebPlayerContract().AppVersion)
	request.Header.Set("Origin", "https://open.spotify.com")
	request.Header.Set("Referer", "https://open.spotify.com/")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if clientToken != "" {
		request.Header.Set("Client-Token", clientToken)
	}
	if c.beforeRequest != nil {
		if err := c.beforeRequest(ctx); err != nil {
			return err
		}
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return auth.ErrTemporarilyUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		retryAfter := auth.RetryAfter(response.Header.Get("Retry-After"), c.now())
		if response.StatusCode == http.StatusTooManyRequests && c.onRateLimit != nil {
			retryAfter = c.onRateLimit(response.Header.Get("Retry-After"))
		}
		return &HTTPError{Stage: stage, Status: response.StatusCode, RetryAfter: retryAfter}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	defer clear(raw)
	if err != nil || len(raw) > 1<<20 {
		return ErrSchema
	}
	if json.Unmarshal(raw, result) != nil {
		return ErrSchema
	}
	return ctx.Err()
}
