//go:build !windows

// On non-Windows platforms without a CGO keychain helper, the v2 master key is
// stored in a 0600 keyfile inside the user's application data directory and
// wrapped with AES-256-GCM using a relocation-safe machine/user key (derived
// without os.Executable()).
package crypto

import "fmt"

func protectOSMasterKey(_ []byte) ([]byte, string, bool) {
	return nil, "", false
}

func unprotectOSMasterKey(scheme string, _ []byte) ([]byte, error) {
	return nil, fmt.Errorf("%w: OS key protection scheme %q is not supported on this platform", ErrReauthRequired, scheme)
}
