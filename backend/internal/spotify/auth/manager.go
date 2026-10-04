// Package auth isolates Spotify credentials by consumer purpose.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Purpose uint8

const (
	WebAPI Purpose = iota + 1
	Playback
	InternalAnalysis
)

type Kind string

const (
	OAuth     Kind = "oauth"
	WebPlayer Kind = "webplayer"
)

var (
	ErrDisabled               = errors.New("spotify provider disabled")
	ErrAuthenticationRequired = errors.New("spotify authentication required")
	ErrTokenKind              = errors.New("spotify token kind does not match purpose")
	ErrInvalidPurpose         = errors.New("invalid spotify token purpose")
)

// Token keeps its bearer private and redacts standard formatting and JSON.
// Bearer is available only to backend request/session consumers.
type Token struct {
	bearer            string
	webPlayerClientID string
	Kind              Kind
	ExpiresAt         time.Time
	Generation        uint64
}

func NewToken(bearer string, kind Kind, expiry time.Time, generation uint64) Token {
	return Token{bearer: bearer, Kind: kind, ExpiresAt: expiry, Generation: generation}
}
func (t Token) Bearer() string { return t.bearer }

// WebPlayerClientID is provider-returned public client context, not a developer credential.
func (t Token) WebPlayerClientID() string { return t.webPlayerClientID }
func (t Token) Format(s fmt.State, verb rune) {
	fmt.Fprintf(s, "SpotifyToken{kind:%s bearer:[REDACTED]}", t.Kind)
}

// Provider owns refresh coordination. Refresh receives the rejected token so
// concurrent callers can reuse a replacement instead of refreshing it again.
type Provider interface {
	Token(context.Context) (Token, error)
	Refresh(context.Context, Token) (Token, error)
}
type Manager struct {
	oauth, internal Provider
	webPlayerOnly   bool
}

func NewManager(oauth, internal Provider) *Manager { return &Manager{oauth: oauth, internal: internal} }

// NewWebPlayerManager explicitly routes every purpose to the cookie provider.
// A missing or rejected cookie provider never falls back to OAuth.
func NewWebPlayerManager(provider Provider) *Manager {
	return &Manager{internal: provider, webPlayerOnly: true}
}

func (m *Manager) provider(p Purpose) (Provider, Kind, error) {
	switch p {
	case WebAPI, Playback:
		if m.webPlayerOnly {
			return m.internal, WebPlayer, nil
		}
		return m.oauth, OAuth, nil
	case InternalAnalysis:
		return m.internal, WebPlayer, nil
	default:
		return nil, "", ErrInvalidPurpose
	}
}
func (m *Manager) Token(ctx context.Context, p Purpose) (Token, error) { return m.get(ctx, p, nil) }
func (m *Manager) Refresh(ctx context.Context, p Purpose, rejected Token) (Token, error) {
	return m.get(ctx, p, &rejected)
}
func (m *Manager) get(ctx context.Context, p Purpose, rejected *Token) (Token, error) {
	if err := ctx.Err(); err != nil {
		return Token{}, err
	}
	provider, kind, err := m.provider(p)
	if err != nil {
		return Token{}, err
	}
	if provider == nil {
		return Token{}, ErrDisabled
	}
	if rejected != nil && rejected.Kind != kind {
		return Token{}, ErrTokenKind
	}
	var token Token
	if rejected == nil {
		token, err = provider.Token(ctx)
	} else {
		token, err = provider.Refresh(ctx, *rejected)
	}
	if err != nil {
		return Token{}, err
	}
	if token.Kind != kind {
		return Token{}, ErrTokenKind
	}
	if token.Bearer() == "" {
		return Token{}, ErrAuthenticationRequired
	}
	return token, nil
}

// ProviderFuncs adapts an existing OAuth owner without moving its persistence.
type ProviderFuncs struct {
	Load  func(context.Context) (Token, error)
	Renew func(context.Context, Token) (Token, error)
}

func (p ProviderFuncs) Token(ctx context.Context) (Token, error) { return p.Load(ctx) }
func (p ProviderFuncs) Refresh(ctx context.Context, rejected Token) (Token, error) {
	return p.Renew(ctx, rejected)
}
