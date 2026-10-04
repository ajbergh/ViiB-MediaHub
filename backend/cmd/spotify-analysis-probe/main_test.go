//go:build spotify_research

// Tests the pinned Web Player authentication contract against a known TOTP vector.
package main

import (
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"testing"
	"time"
)

func TestPinnedContractVector(t *testing.T) {
	contract := researchContract()
	got, err := auth.TOTP(contract.Secret, time.Unix(1777993436, 0))
	if err != nil || got != "031750" {
		t.Fatalf("contract vector: %s %v", got, err)
	}
}
