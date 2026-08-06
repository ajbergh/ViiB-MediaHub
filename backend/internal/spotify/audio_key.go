package spotify

import (
	"errors"
	"fmt"
	"strings"
)

// ErrAudioKeyRejected means Spotify did not return a usable decryption key for
// a track. librespot-go currently reports this as crypto/aes key size zero,
// even though the AES implementation itself is not the source of the failure.
var ErrAudioKeyRejected = errors.New("Spotify rejected the audio key request")

// IsAudioKeyRejected recognizes both the normalized error and messages emitted
// by current and older librespot implementations.
func IsAudioKeyRejected(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAudioKeyRejected) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"crypto/aes: invalid key size 0",
		"audio key error",
		"failed retrieving aes key",
		"failed fetching audio key",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func normalizeAudioKeyError(err error) error {
	if !IsAudioKeyRejected(err) || errors.Is(err, ErrAudioKeyRejected) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrAudioKeyRejected, err)
}
