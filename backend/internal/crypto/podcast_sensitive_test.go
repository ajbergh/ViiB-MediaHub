package crypto

import "testing"

func TestPodcastIndexCredentialsAreSensitive(t *testing.T) {
	if !IsSensitiveKey("podcast_index_credentials") {
		t.Fatal("podcast index credentials must use the encrypted setting mechanism")
	}
}
