package crypto

import "testing"

func TestPlexCredentialsAreSensitive(t *testing.T) {
	if !IsSensitiveKey("plex_credentials") {
		t.Fatal("Plex credentials must use the existing encrypted setting mechanism")
	}
}
