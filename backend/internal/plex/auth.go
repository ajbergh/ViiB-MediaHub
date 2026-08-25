package plex

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const plexClientsBaseURL = "https://clients.plex.tv"

// AuthClient implements Plex's documented JWT PIN flow for new applications.
type AuthClient struct {
	httpClient *http.Client
	baseURL    string
}

func NewAuthClient() *AuthClient {
	return &AuthClient{httpClient: &http.Client{Timeout: 20 * time.Second}, baseURL: plexClientsBaseURL}
}

func NewAuthClientWithHTTP(baseURL string, client *http.Client) *AuthClient {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &AuthClient{httpClient: client, baseURL: strings.TrimRight(baseURL, "/")}
}

func randomHex(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func ensureDeviceCredentials(credentials *Credentials) error {
	if credentials.ClientIdentifier != "" && credentials.KeyID != "" && credentials.PrivateKey != "" {
		if credentials.ServerTokens == nil {
			credentials.ServerTokens = map[string]string{}
		}
		return nil
	}
	clientID, err := randomHex(16)
	if err != nil {
		return fmt.Errorf("generate Plex client identifier: %w", err)
	}
	kid, err := randomHex(12)
	if err != nil {
		return fmt.Errorf("generate Plex key identifier: %w", err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate Plex device key: %w", err)
	}
	credentials.ClientIdentifier = clientID
	credentials.KeyID = kid
	credentials.PrivateKey = base64.RawURLEncoding.EncodeToString(privateKey)
	if credentials.ServerTokens == nil {
		credentials.ServerTokens = map[string]string{}
	}
	return nil
}

func privateKey(credentials Credentials) (ed25519.PrivateKey, error) {
	raw, err := base64.RawURLEncoding.DecodeString(credentials.PrivateKey)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("stored Plex device key is invalid")
	}
	return ed25519.PrivateKey(raw), nil
}

func deviceJWK(credentials Credentials) (map[string]string, error) {
	key, err := privateKey(credentials)
	if err != nil {
		return nil, err
	}
	public := key.Public().(ed25519.PublicKey)
	return map[string]string{
		"kty": "OKP",
		"crv": "Ed25519",
		"x":   base64.RawURLEncoding.EncodeToString(public),
		"kid": credentials.KeyID,
		"alg": "EdDSA",
	}, nil
}

func (a *AuthClient) requestJSON(ctx context.Context, method, endpoint, clientID string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", ProductName)
	if clientID != "" {
		req.Header.Set("X-Plex-Client-Identifier", clientID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("contact Plex authentication service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("plex authentication service returned HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out); err != nil {
		return fmt.Errorf("decode Plex authentication response: %w", err)
	}
	return nil
}

type pinResponse struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
	ExpiresIn int64  `json:"expiresIn"`
	ExpiresAt int64  `json:"expiresAt"`
}

// StartPIN begins Plex's recommended JWT PIN authentication flow.
func (a *AuthClient) StartPIN(ctx context.Context, credentials *Credentials) (AuthStart, error) {
	if err := ensureDeviceCredentials(credentials); err != nil {
		return AuthStart{}, err
	}
	jwk, err := deviceJWK(*credentials)
	if err != nil {
		return AuthStart{}, err
	}
	var pin pinResponse
	if err := a.requestJSON(ctx, http.MethodPost, "/api/v2/pins", credentials.ClientIdentifier, map[string]any{
		"jwk": jwk, "strong": true,
	}, &pin); err != nil {
		return AuthStart{}, err
	}
	if pin.ID == 0 || pin.Code == "" {
		return AuthStart{}, errors.New("plex authentication service returned an incomplete PIN")
	}
	expiresAt := pin.ExpiresAt
	if expiresAt == 0 {
		expiresIn := pin.ExpiresIn
		if expiresIn <= 0 {
			expiresIn = 300
		}
		expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second).Unix()
	}
	credentials.PendingPINID = pin.ID
	credentials.PendingPINCode = pin.Code
	credentials.PendingPINExpiry = expiresAt
	fragment := url.Values{}
	fragment.Set("clientID", credentials.ClientIdentifier)
	fragment.Set("code", pin.Code)
	fragment.Set("context[device][product]", ProductName)
	return AuthStart{AuthURL: "https://app.plex.tv/auth#?" + fragment.Encode(), ExpiresAt: expiresAt}, nil
}

func signDeviceJWT(credentials Credentials, claims map[string]any) (string, error) {
	key, err := privateKey(credentials)
	if err != nil {
		return "", err
	}
	headerJSON, _ := json.Marshal(map[string]string{"alg": "EdDSA", "kid": credentials.KeyID, "typ": "JWT"})
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	unsigned := header + "." + payload
	signature := ed25519.Sign(key, []byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func tokenExpiry(token string) int64 {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0
	}
	var claims struct{ Exp int64 `json:"exp"` }
	if json.Unmarshal(payload, &claims) != nil {
		return 0
	}
	return claims.Exp
}

// PollPIN checks the pending PIN using the device-signed JWT required by the
// current Plex flow. It returns authenticated=false,nil while user action is pending.
func (a *AuthClient) PollPIN(ctx context.Context, credentials *Credentials) (AuthStatus, error) {
	if credentials.PendingPINID == 0 {
		return AuthStatus{Authenticated: credentials.AccountToken != "", Pending: false}, nil
	}
	if credentials.PendingPINExpiry > 0 && time.Now().Unix() >= credentials.PendingPINExpiry {
		credentials.PendingPINID, credentials.PendingPINCode, credentials.PendingPINExpiry = 0, "", 0
		return AuthStatus{Pending: false, Message: "Plex sign-in expired; start authentication again"}, nil
	}
	now := time.Now()
	jwt, err := signDeviceJWT(*credentials, map[string]any{
		"aud": "plex.tv", "iss": credentials.ClientIdentifier, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		return AuthStatus{}, err
	}
	endpoint := fmt.Sprintf("/api/v2/pins/%d?deviceJWT=%s", credentials.PendingPINID, url.QueryEscape(jwt))
	var pin pinResponse
	if err := a.requestJSON(ctx, http.MethodGet, endpoint, credentials.ClientIdentifier, nil, &pin); err != nil {
		return AuthStatus{}, RedactError(err)
	}
	if pin.AuthToken == "" {
		return AuthStatus{Pending: true, ExpiresAt: credentials.PendingPINExpiry}, nil
	}
	credentials.AccountToken = pin.AuthToken
	credentials.AccountTokenExpiry = tokenExpiry(pin.AuthToken)
	if credentials.AccountTokenExpiry == 0 {
		credentials.AccountTokenExpiry = time.Now().Add(7 * 24 * time.Hour).Unix()
	}
	credentials.PendingPINID, credentials.PendingPINCode, credentials.PendingPINExpiry = 0, "", 0
	return AuthStatus{Authenticated: true, Pending: false, ExpiresAt: credentials.AccountTokenExpiry}, nil
}

type nonceResponse struct{ Nonce string `json:"nonce"` }
type tokenResponse struct{ AuthToken string `json:"auth_token"` }

// Refresh refreshes a Plex JWT using the stored ED25519 device key.
func (a *AuthClient) Refresh(ctx context.Context, credentials *Credentials) error {
	if credentials.ClientIdentifier == "" || credentials.PrivateKey == "" || credentials.KeyID == "" {
		return ErrAuthenticationRequired
	}
	var nonce nonceResponse
	if err := a.requestJSON(ctx, http.MethodGet, "/api/v2/auth/nonce", credentials.ClientIdentifier, nil, &nonce); err != nil {
		return err
	}
	if nonce.Nonce == "" {
		return errors.New("plex authentication service returned no nonce")
	}
	now := time.Now()
	jwt, err := signDeviceJWT(*credentials, map[string]any{
		"nonce": nonce.Nonce, "scope": "username,email,friendly_name", "aud": "plex.tv",
		"iss": credentials.ClientIdentifier, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		return err
	}
	var response tokenResponse
	if err := a.requestJSON(ctx, http.MethodPost, "/api/v2/auth/token", credentials.ClientIdentifier, map[string]string{"jwt": jwt}, &response); err != nil {
		return RedactError(err)
	}
	if response.AuthToken == "" {
		return errors.New("plex authentication refresh returned no token")
	}
	credentials.AccountToken = response.AuthToken
	credentials.AccountTokenExpiry = tokenExpiry(response.AuthToken)
	if credentials.AccountTokenExpiry == 0 {
		credentials.AccountTokenExpiry = time.Now().Add(7 * 24 * time.Hour).Unix()
	}
	return nil
}

// EnsureFreshToken refreshes JWTs within one day of expiration.
func (a *AuthClient) EnsureFreshToken(ctx context.Context, credentials *Credentials) error {
	if credentials.AccountToken == "" {
		return ErrAuthenticationRequired
	}
	if credentials.AccountTokenExpiry == 0 || time.Until(time.Unix(credentials.AccountTokenExpiry, 0)) > 24*time.Hour {
		return nil
	}
	return a.Refresh(ctx, credentials)
}

type resourceConnection struct {
	URI   string `json:"uri"`
	Local bool   `json:"local"`
	Relay bool   `json:"relay"`
}
type resource struct {
	Name             string               `json:"name"`
	Provides         string               `json:"provides"`
	ClientIdentifier string               `json:"clientIdentifier"`
	AccessToken      string               `json:"accessToken"`
	Connections      []resourceConnection `json:"connections"`
}

// ResolveServerToken follows Plex's documented resources endpoint to obtain the
// PMS-specific access token for a machine. It prefers local non-relay URLs.
func (a *AuthClient) ResolveServerToken(ctx context.Context, credentials *Credentials, machineIdentifier string) (token, preferredURL string, err error) {
	if credentials.ServerTokens == nil {
		credentials.ServerTokens = map[string]string{}
	}
	if err := a.EnsureFreshToken(ctx, credentials); err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/api/v2/resources?includeHttps=1&includeRelay=1&includeIPv6=1", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", ProductName)
	req.Header.Set("X-Plex-Client-Identifier", credentials.ClientIdentifier)
	req.Header.Set("X-Plex-Token", credentials.AccountToken)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("get Plex server resources: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("plex resources service returned HTTP %d", resp.StatusCode)
	}
	var resources []resource
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&resources); err != nil {
		return "", "", fmt.Errorf("decode Plex resources: %w", err)
	}
	for _, candidate := range resources {
		if candidate.ClientIdentifier != machineIdentifier || !strings.Contains(candidate.Provides, "server") {
			continue
		}
		if candidate.AccessToken == "" {
			return "", "", errors.New("plex server resource did not provide an access token")
		}
		credentials.ServerTokens[machineIdentifier] = candidate.AccessToken
		for _, connection := range candidate.Connections {
			if connection.Local && !connection.Relay && connection.URI != "" {
				return candidate.AccessToken, connection.URI, nil
			}
		}
		for _, connection := range candidate.Connections {
			if !connection.Relay && connection.URI != "" {
				return candidate.AccessToken, connection.URI, nil
			}
		}
		for _, connection := range candidate.Connections {
			if connection.URI != "" {
				return candidate.AccessToken, connection.URI, nil
			}
		}
		return candidate.AccessToken, "", nil
	}
	return "", "", errors.New("authenticated Plex account cannot access this server")
}

// RedactError removes credential-like values from errors before they reach logs
// or API responses.
func RedactError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, marker := range []string{"deviceJWT=", "X-Plex-Token=", "authToken=", "auth_token="} {
		cursor := 0
		for cursor < len(message) {
			relative := strings.Index(strings.ToLower(message[cursor:]), strings.ToLower(marker))
			if relative < 0 {
				break
			}
			index := cursor + relative
			valueStart := index + len(marker)
			valueEnd := valueStart
			for valueEnd < len(message) && !strings.ContainsRune("& \t\r\n\"'", rune(message[valueEnd])) {
				valueEnd++
			}
			const replacement = "[REDACTED]"
			message = message[:valueStart] + replacement + message[valueEnd:]
			cursor = valueStart + len(replacement)
		}
	}
	return errors.New(message)
}
