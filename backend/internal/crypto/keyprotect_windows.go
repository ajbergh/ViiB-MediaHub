//go:build windows

// Uses Windows DPAPI (CryptProtectData / CryptUnprotectData) to bind the v2
// master credential key to the current Windows user profile.
package crypto

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dpapiEntropy = []byte("viib-mediahub:credential-master-key:v2")
)

func makeDataBlob(d []byte) *windows.DataBlob {
	if len(d) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{
		Size: uint32(len(d)),
		Data: &d[0],
	}
}

func bytesFromDataBlob(blob windows.DataBlob) []byte {
	if blob.Size == 0 || blob.Data == nil {
		return nil
	}
	src := unsafe.Slice(blob.Data, int(blob.Size))
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

func protectOSMasterKey(rawKey []byte) ([]byte, string, bool) {
	if len(rawKey) == 0 {
		return nil, "", false
	}
	var outBlob windows.DataBlob
	err := windows.CryptProtectData(
		makeDataBlob(rawKey),
		nil,
		makeDataBlob(dpapiEntropy),
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&outBlob,
	)
	if err != nil {
		return nil, "", false
	}
	defer func() {
		if outBlob.Data != nil {
			_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))
		}
	}()
	return bytesFromDataBlob(outBlob), "dpapi", true
}

func unprotectOSMasterKey(scheme string, payload []byte) ([]byte, error) {
	if scheme != "dpapi" {
		return nil, fmt.Errorf("%w: unsupported OS key protection scheme %q", ErrReauthRequired, scheme)
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("%w: empty DPAPI key payload", ErrReauthRequired)
	}
	var outBlob windows.DataBlob
	err := windows.CryptUnprotectData(
		makeDataBlob(payload),
		nil,
		makeDataBlob(dpapiEntropy),
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&outBlob,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: Windows DPAPI key unprotect failed: %v", ErrReauthRequired, err)
	}
	defer func() {
		if outBlob.Data != nil {
			_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))
		}
	}()
	out := bytesFromDataBlob(outBlob)
	if len(out) != keyLength {
		return nil, fmt.Errorf("%w: invalid DPAPI master key length", ErrReauthRequired)
	}
	return out, nil
}
