package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func fixtureProvider(kind Kind, calls *int) Provider {
	return ProviderFuncs{
		Load: func(context.Context) (Token, error) {
			*calls++
			return NewToken("secret-bearer", kind, time.Now().Add(time.Hour), 1), nil
		},
		Renew: func(context.Context, Token) (Token, error) {
			*calls++
			return NewToken("renewed", kind, time.Now().Add(time.Hour), 2), nil
		},
	}
}
func TestManagerPurposeIsolation(t *testing.T) {
	var oauthCalls, internalCalls int
	manager := NewManager(fixtureProvider(OAuth, &oauthCalls), fixtureProvider(WebPlayer, &internalCalls))
	for _, p := range []Purpose{WebAPI, Playback, InternalAnalysis} {
		token, err := manager.Token(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Refresh(context.Background(), p, token); err != nil {
			t.Fatal(err)
		}
	}
	if oauthCalls != 4 || internalCalls != 2 {
		t.Fatalf("wrong routes: %d %d", oauthCalls, internalCalls)
	}
	manager = NewManager(fixtureProvider(OAuth, &oauthCalls), nil)
	if _, err := manager.Token(context.Background(), InternalAnalysis); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if oauthCalls != 4 {
		t.Fatal("disabled internal provider fell back to OAuth")
	}
	if _, err := manager.Token(context.Background(), Purpose(99)); !errors.Is(err, ErrInvalidPurpose) {
		t.Fatal(err)
	}
	manager = NewManager(nil, fixtureProvider(OAuth, &internalCalls))
	if _, err := manager.Token(context.Background(), InternalAnalysis); !errors.Is(err, ErrTokenKind) {
		t.Fatal(err)
	}
}
func TestTokenDoesNotSerializeOrFormatBearer(t *testing.T) {
	token := NewToken("secret-bearer", WebPlayer, time.Now(), 1)
	encoded, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{string(encoded), fmt.Sprintf("%v", token), fmt.Sprintf("%+v", &token), fmt.Sprintf("%#v", token)} {
		if strings.Contains(value, "secret-bearer") {
			t.Fatal("bearer disclosure")
		}
	}
}
func TestTOTPStandardVectors(t *testing.T) {
	// RFC 6238 Appendix B SHA1 results reduced from eight to six digits.
	for _, test := range []struct {
		unix int64
		want string
	}{
		{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"},
		{1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"},
	} {
		got, err := TOTP([]byte("12345678901234567890"), time.Unix(test.unix, 0))
		if err != nil || got != test.want {
			t.Fatalf("time %d: %s %v", test.unix, got, err)
		}
	}
	if _, err := TOTP(nil, time.Now()); err == nil {
		t.Fatal("empty key accepted")
	}
}

func TestWebPlayerManagerAllPurposesWithoutFallback(t *testing.T) {
	var calls int
	manager := NewWebPlayerManager(fixtureProvider(WebPlayer, &calls))
	for _, purpose := range []Purpose{WebAPI, Playback, InternalAnalysis} {
		token, err := manager.Token(context.Background(), purpose)
		if err != nil || token.Kind != WebPlayer {
			t.Fatal(err)
		}
		if _, err := manager.Refresh(context.Background(), purpose, token); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 6 {
		t.Fatal("wrong routes")
	}
	manager = NewWebPlayerManager(nil)
	for _, purpose := range []Purpose{WebAPI, Playback, InternalAnalysis} {
		if _, err := manager.Token(context.Background(), purpose); !errors.Is(err, ErrDisabled) {
			t.Fatal("missing provider did not fail")
		}
	}
	manager = NewWebPlayerManager(fixtureProvider(OAuth, &calls))
	if _, err := manager.Token(context.Background(), Playback); !errors.Is(err, ErrTokenKind) {
		t.Fatal("accepted OAuth as cookie token")
	}
	contract := PinnedWebPlayerContract()
	code, err := TOTP(contract.Secret, time.Unix(1777993436, 0))
	if err != nil || code != "031750" {
		t.Fatal("pinned contract failed known vector")
	}
}
