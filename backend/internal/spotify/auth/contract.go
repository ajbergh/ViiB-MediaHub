package auth

import (
	"strconv"
	"strings"
)

// PinnedWebPlayerContract returns reviewed public client protocol material.
// Source: stephancill/stupid-social f0f8c219e43d394c84516a3bcc7af7c9fd41f713,
// scripts/spotify-web-client.py (Apache-2.0); see research probe UPSTREAM_LICENSE.
func PinnedWebPlayerContract() WebPlayerContract {
	obfuscated := ",7/*F(\"rLJ2oxaKL^f+E1xvP@N"
	var secret strings.Builder
	for i, c := range []byte(obfuscated) {
		secret.WriteString(strconv.Itoa(int(c) ^ ((i % 33) + 9)))
	}
	return WebPlayerContract{Version: "61", Secret: []byte(secret.String()), AppVersion: "1.2.90.229.g33aad738", Revision: "stupid-social:f0f8c219e43d394c84516a3bcc7af7c9fd41f713"}
}
