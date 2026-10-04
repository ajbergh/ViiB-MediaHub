package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// TOTP implements the six-digit, 30-second HMAC-SHA1 form of RFC 6238.
// The key is supplied by a reviewed provider contract; no keys are downloaded.
func TOTP(secret []byte, at time.Time) (string, error) {
	if len(secret) == 0 || at.Unix() < 0 {
		return "", errors.New("invalid TOTP input")
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(at.Unix()/30))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	value := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000), nil
}
