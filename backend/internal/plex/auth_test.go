package plex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJWTAuthenticationStartAndPoll(t *testing.T) {
	var sawJWK bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/pins":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			jwk, _ := body["jwk"].(map[string]any)
			sawJWK = jwk["kty"] == "OKP" && jwk["crv"] == "Ed25519" && jwk["alg"] == "EdDSA" && body["strong"] == true
			_, _ = w.Write([]byte(`{"id":77,"code":"ABCD","expiresIn":300}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/pins/77":
			jwt := r.URL.Query().Get("deviceJWT")
			if len(strings.Split(jwt, ".")) != 3 {
				t.Errorf("deviceJWT not signed JWT: %q", jwt)
			}
			_, _ = w.Write([]byte(`{"id":77,"code":"ABCD","authToken":"header.eyJleHAiOjQxMDI0NDQ4MDB9.signature"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	credentials := Credentials{}
	auth := NewAuthClientWithHTTP(server.URL, server.Client())
	start, err := auth.StartPIN(context.Background(), &credentials)
	if err != nil {
		t.Fatal(err)
	}
	if !sawJWK || !strings.HasPrefix(start.AuthURL, "https://app.plex.tv/auth#?") || credentials.PrivateKey == "" || credentials.ClientIdentifier == "" {
		t.Fatalf("unexpected start state: %#v credentials=%#v", start, credentials)
	}
	status, err := auth.PollPIN(context.Background(), &credentials)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Authenticated || credentials.AccountToken == "" || credentials.PendingPINID != 0 || credentials.AccountTokenExpiry <= time.Now().Unix() {
		t.Fatalf("unexpected auth status: %#v credentials=%#v", status, credentials)
	}
}

func TestRedactError(t *testing.T) {
	err := RedactError(&testError{"request failed deviceJWT=abc.def.ghi&foo=bar X-Plex-Token=topsecret authToken=also-secret"})
	text := err.Error()
	for _, secret := range []string{"abc.def.ghi", "topsecret", "also-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("secret %q was not redacted: %s", secret, text)
		}
	}
	if strings.Count(text, "[REDACTED]") < 3 {
		t.Fatalf("expected redaction markers: %s", text)
	}
}

type testError struct{ text string }
func (e *testError) Error() string { return e.text }
