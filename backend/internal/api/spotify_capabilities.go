package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

// Product state is independent of catalog access. Never infer Premium from a
// successful login or an unavailable profile.product field.
type spotifyProductCache struct {
	mu       sync.Mutex
	lifetime context.Context
	product  string
	until    time.Time
}

func spotifyProduct(catalogue, product string) string {
	known := func(value string) string {
		switch value {
		case "premium":
			return "premium"
		case "free", "open":
			return "free"
		}
		return "unknown"
	}
	if catalogue == "" {
		return known(product)
	}
	tier := known(catalogue)
	if product != "" && known(product) != tier {
		return "unknown"
	}
	return tier
}

func (a *API) spotifyAccountProduct(parent context.Context, refresh ...bool) string {
	parent, stop := context.WithTimeout(parent, 12*time.Second)
	defer stop()
	s := a.spotifyTokens()
	ctx, cancel := s.requestContext(parent)
	defer cancel()
	lifetime, _ := ctx.Value(spotifyAccountContextKey{}).(context.Context)
	c := &s.productCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || lifetime == nil || lifetime.Err() != nil {
		return "unknown"
	}
	if c.lifetime == lifetime && time.Now().Before(c.until) && !(len(refresh) > 0 && refresh[0]) {
		return c.product
	}
	product := "unknown"
	defer func() {
		if ctx.Err() == nil && lifetime.Err() == nil {
			c.lifetime, c.product = lifetime, product
			ttl := 5 * time.Minute
			if product == "unknown" {
				ttl = 30 * time.Second
			}
			c.until = time.Now().Add(ttl)
		}
	}()
	token, err := s.Token(ctx, spotifyauth.WebAPI)
	if err != nil {
		return product
	}
	release, err := s.acquireWebAPI(ctx)
	if err != nil {
		return product
	}
	defer release()
	client := http.Client{Timeout: 10 * time.Second}
	if a.spotifyHTTPClient != nil {
		client = *a.spotifyHTTPClient
	}
	if client.Timeout == 0 || client.Timeout > 10*time.Second {
		client.Timeout = 10 * time.Second
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for attempt := 0; attempt < 2; attempt++ {
		if s.checkWebAPICooldown(ctx) != nil {
			return product
		}
		// Fixed destination; never expose this bearer through the generic proxy.
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://spclient.wg.spotify.com/melody/v1/product_state", nil)
		req.Header.Set("Authorization", "Bearer "+token.Bearer())
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return product
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			token, err = s.Refresh(ctx, spotifyauth.WebAPI, token)
			if err != nil {
				return product
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			s.recordWebAPICooldown(resp.Header.Get("Retry-After"))
		}
		var state struct {
			Catalogue string `json:"catalogue"`
			Product   string `json:"product"`
		}
		if resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&state) == nil {
			product = spotifyProduct(state.Catalogue, state.Product)
		}
		resp.Body.Close()
		if ctx.Err() != nil || lifetime.Err() != nil {
			return "unknown"
		}
		return product
	}
	return product
}

func (a *API) requireSpotifyMedia(w http.ResponseWriter, r *http.Request) bool {
	status := a.spotifyTokens().status()
	if !status.Connected || status.AuthRequired {
		respondError(w, http.StatusUnauthorized, "Sign in to Spotify to continue")
		return false
	}
	switch a.spotifyAccountProduct(r.Context()) {
	case "premium":
		return true
	case "free":
		respondError(w, http.StatusForbidden, "Spotify Premium is required for streaming and downloads. Metadata browsing is available with Spotify Free.")
	default:
		respondError(w, http.StatusServiceUnavailable, "Could not verify Spotify Premium. Metadata browsing is available; retry the account check before streaming or downloading.")
	}
	return false
}

// Capture the account before checking its tier and retain that lifetime through
// queue insertion/session preparation, so replacement cannot reuse permission.
func (a *API) spotifyMediaOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := a.spotifyTokens().requestContext(r.Context())
		defer cancel()
		r = r.WithContext(ctx)
		if a.requireSpotifyMedia(w, r) {
			next(w, r)
		}
	}
}
