//go:build spotify_research

package main

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	"io"
	"net/http"
	"time"
)

func runCatalogProfileResearch(ctx context.Context, cookie string, contract auth.WebPlayerContract, renew bool, opts auth.WebPlayerOptions, httpClient *http.Client, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	provider, err := auth.NewWebPlayerProvider(cookie, contract, opts)
	cookie = ""
	if err != nil {
		return err
	}
	defer provider.Disconnect()
	token, err := provider.Token(ctx)
	if err != nil {
		return &probeFailure{stage: "authentication", cause: err}
	}
	if renew {
		token, err = provider.Refresh(ctx, token)
		if err != nil {
			return &probeFailure{stage: "token_renewal", authenticated: true, cause: err}
		}
	}
	var wireShape map[string]any
	observed := http.Client{}
	if httpClient != nil {
		observed = *httpClient
	}
	transport := observed.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	observed.Transport = profileShapeTransport{base: transport, report: &wireShape}
	client := catalog.New(catalog.Options{Client: &observed})
	defer client.Close()
	profile, err := client.Profile(ctx, token)
	if err != nil {
		return &probeFailure{stage: "catalog_profile", authenticated: true, renewed: renew, cause: err}
	}
	return json.NewEncoder(out).Encode(struct {
		WireShape              map[string]any `json:"wireShape,omitempty"`
		AuthenticationVerified bool           `json:"authenticationVerified"`
		RenewalVerified        bool           `json:"renewalVerified"`
		ProfileNormalized      bool           `json:"profileNormalized"`
		IdentityAvailable      bool           `json:"identityAvailable"`
		DisplayNameAvailable   bool           `json:"displayNameAvailable"`
		ImageCount             int            `json:"imageCount"`
		CountryAvailable       bool           `json:"countryAvailable"`
		ProductAvailable       bool           `json:"productAvailable"`
		FollowersAvailable     bool           `json:"followersAvailable"`
	}{wireShape, true, renew, true, profile.ID != "", profile.DisplayName != "", len(profile.Images), profile.Country != nil, profile.Product != nil, profile.Followers != nil})
}
