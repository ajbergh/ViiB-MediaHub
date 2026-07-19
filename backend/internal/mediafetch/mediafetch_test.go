package mediafetch

import (
	"context"
	"testing"
)

func TestValidateURLRejectsUnsafeTargets(t *testing.T) {
	tests := []string{
		"http://example.com/image.jpg",
		"https://127.0.0.1/image.jpg",
		"https://localhost/image.jpg",
		"file:///etc/passwd",
	}
	for _, candidate := range tests {
		if _, err := validateURL(context.Background(), candidate); err == nil {
			t.Fatalf("expected %q to be rejected", candidate)
		}
	}
}

func TestPublicIPClassification(t *testing.T) {
	if isPublicIP([]byte{127, 0, 0, 1}) {
		t.Fatal("loopback address must not be public")
	}
}
