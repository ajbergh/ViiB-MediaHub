// Exchanges backend-held cookies for Web Player tokens using the pinned contract and coalesced refreshes.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WebPlayerContract is a reviewed protocol revision, not a public API guarantee.
// The application does not enable it by default.
type WebPlayerContract struct {
	Version    string
	Secret     []byte
	AppVersion string
	Revision   string
}
type WebPlayerOptions struct {
	Client *http.Client
	Now    func() time.Time
}
type webPlayerFlight struct {
	done   chan struct{}
	cancel context.CancelFunc
	token  Token
	err    error
}

// WebPlayerProvider retains session material and derived tokens only in memory.
// A new instance is required to reconnect after Disconnect.
type WebPlayerProvider struct {
	mu         sync.Mutex
	cookie     string
	contract   WebPlayerContract
	client     *http.Client
	now        func() time.Time
	generation uint64
	cached     Token
	issued     uint64
	flight     *webPlayerFlight
}

func NewWebPlayerProvider(cookie string, contract WebPlayerContract, opts WebPlayerOptions) (*WebPlayerProvider, error) {
	if cookie == "" || strings.ContainsAny(cookie, ";\r\n\t ") {
		return nil, ErrAuthenticationRequired
	}
	for _, c := range cookie {
		if c < 33 || c > 126 {
			return nil, ErrAuthenticationRequired
		}
	}
	if contract.Version == "" || len(contract.Secret) == 0 || contract.AppVersion == "" || contract.Revision == "" {
		return nil, ErrDisabled
	}
	for _, c := range contract.Version {
		if c < '0' || c > '9' {
			return nil, ErrDisabled
		}
	}
	if strings.ContainsAny(contract.AppVersion, "\r\n") {
		return nil, ErrDisabled
	}
	client := http.Client{Timeout: 20 * time.Second}
	if opts.Client != nil {
		client = *opts.Client
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	contract.Secret = append([]byte(nil), contract.Secret...)
	return &WebPlayerProvider{cookie: cookie, contract: contract, client: &client, now: now, generation: 1}, nil
}

// CachedLifetime exposes only non-secret renewal evidence, without fetching a
// token or changing its expiry. Disconnect makes the cached expiry unavailable.
func (p *WebPlayerProvider) CachedLifetime() (time.Time, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cached.ExpiresAt, p.issued
}

func (p *WebPlayerProvider) Token(ctx context.Context) (Token, error) { return p.get(ctx, nil) }
func (p *WebPlayerProvider) Refresh(ctx context.Context, rejected Token) (Token, error) {
	return p.get(ctx, &rejected)
}
func (p *WebPlayerProvider) Disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cookie = ""
	p.cached = Token{}
	p.generation++
	if p.flight != nil {
		p.flight.cancel()
	}
}
func (p *WebPlayerProvider) get(ctx context.Context, rejected *Token) (Token, error) {
	if err := ctx.Err(); err != nil {
		return Token{}, err
	}
	p.mu.Lock()
	if p.cookie == "" {
		p.mu.Unlock()
		return Token{}, ErrAuthenticationRequired
	}
	fresh := p.cached.Bearer() != "" && p.now().Add(time.Minute).Before(p.cached.ExpiresAt)
	if fresh && (rejected == nil || rejected.bearer != p.cached.bearer || rejected.Generation != p.cached.Generation) {
		token := p.cached
		p.mu.Unlock()
		return token, nil
	}
	if flight := p.flight; flight != nil {
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return Token{}, ctx.Err()
		case <-flight.done:
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.cookie == "" {
				return Token{}, ErrAuthenticationRequired
			}
			return flight.token, flight.err
		}
	}
	workCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	flight := &webPlayerFlight{done: make(chan struct{}), cancel: cancel}
	p.flight = flight
	cookie, generation := p.cookie, p.generation
	p.mu.Unlock()
	token, err := p.fetch(workCtx, cookie, generation)
	cancel()
	p.mu.Lock()
	if generation != p.generation || p.cookie == "" {
		token, err = Token{}, ErrAuthenticationRequired
	}
	if err == nil {
		p.cached = token
		p.issued++
	}
	flight.token, flight.err = token, err
	p.flight = nil
	close(flight.done)
	p.mu.Unlock()
	return token, err
}

// Error responses deliberately exclude URLs, cookie values and response bodies.
var ErrProviderChanged = errors.New("spotify WebPlayer contract changed")
var ErrTemporarilyUnavailable = errors.New("spotify WebPlayer temporarily unavailable")

type WebPlayerHTTPError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *WebPlayerHTTPError) Error() string {
	return "spotify WebPlayer request denied (status " + strconv.Itoa(e.Status) + ")"
}

func (p *WebPlayerProvider) fetch(ctx context.Context, cookie string, generation uint64) (Token, error) {
	var server struct {
		ServerTime *int64 `json:"serverTime"`
	}
	if err := p.getJSON(ctx, "https://open.spotify.com/api/server-time", "", &server); err != nil {
		return Token{}, err
	}
	if server.ServerTime == nil || *server.ServerTime <= 0 {
		return Token{}, ErrProviderChanged
	}
	clientCode, err := TOTP(p.contract.Secret, p.now())
	if err != nil {
		return Token{}, ErrProviderChanged
	}
	serverCode, err := TOTP(p.contract.Secret, time.Unix(*server.ServerTime, 0))
	if err != nil {
		return Token{}, ErrProviderChanged
	}
	params := url.Values{
		"reason": {"transport"}, "productType": {"web-player"},
		"totp": {clientCode}, "totpServer": {serverCode}, "totpVer": {p.contract.Version},
	}
	var reply struct {
		AccessToken string `json:"accessToken"`
		ClientID    string `json:"clientId"`
		Expiry      int64  `json:"accessTokenExpirationTimestampMs"`
		IsAnonymous *bool  `json:"isAnonymous"`
	}
	if err := p.getJSON(ctx, "https://open.spotify.com/api/token?"+params.Encode(), cookie, &reply); err != nil {
		return Token{}, err
	}
	if reply.IsAnonymous == nil {
		return Token{}, ErrProviderChanged
	}
	if *reply.IsAnonymous {
		return Token{}, ErrAuthenticationRequired
	}
	expiry := time.UnixMilli(reply.Expiry)
	if reply.AccessToken == "" || !p.now().Add(time.Minute).Before(expiry) {
		return Token{}, ErrProviderChanged
	}
	token := NewToken(reply.AccessToken, WebPlayer, expiry, generation)
	token.webPlayerClientID = reply.ClientID
	return token, nil
}
func (p *WebPlayerProvider) getJSON(ctx context.Context, target, cookie string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ErrProviderChanged
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("App-Platform", "WebPlayer")
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: "sp_dc", Value: cookie})
	}
	response, err := p.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrTemporarilyUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusUnauthorized {
			return ErrAuthenticationRequired
		}
		return &WebPlayerHTTPError{Status: response.StatusCode, RetryAfter: RetryAfter(response.Header.Get("Retry-After"), p.now())}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrTemporarilyUnavailable
	}
	if len(body) > 1<<20 || json.Unmarshal(body, out) != nil {
		return ErrProviderChanged
	}
	return nil
}

// RetryAfter supports both delay-seconds and the HTTP-date representation.
func RetryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

// Format prevents accidental session disclosure through ordinary diagnostics.
func (p *WebPlayerProvider) Format(s fmt.State, verb rune) {
	fmt.Fprint(s, "SpotifyWebPlayerProvider{session:[REDACTED]}")
}
