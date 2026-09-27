package main

import "testing"

func TestGenerateSpotifyCodeChallenge(t *testing.T) {
	// RFC 7636, Appendix B.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := (&App{}).GenerateSpotifyCodeChallenge(verifier); got != want {
		t.Fatalf("challenge = %q, want %q", got, want)
	}
}
