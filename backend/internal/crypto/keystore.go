// Binds credential encryption to one database's data directory.
package crypto

import (
	"os"
	"path/filepath"
	"sync"
)

// KeyStore keeps an immutable directory and a lazily loaded master key.
// Opening another database cannot change this store's key selection.
type KeyStore struct {
	dir string
	mu  sync.Mutex
	key []byte
}

// NewKeyStore creates an independent store. Empty directories use the user
// configuration directory, preserving the package helpers' default location.
func NewKeyStore(dir string) (*KeyStore, error) {
	if dir == "" {
		keyMu.Lock()
		dir = resolveKeyStoreDirLocked()
		keyMu.Unlock()
	} else {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		dir = filepath.Clean(absolute)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	return &KeyStore{dir: dir}, nil
}

func (s *KeyStore) getKey() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.key) == keyLength {
		return s.key, nil
	}
	keyMu.Lock()
	defer keyMu.Unlock()
	key, err := loadV2Key(s.dir)
	if err != nil {
		return nil, err
	}
	s.key = key
	return key, nil
}

// Encrypt encrypts a setting with this store's key.
func (s *KeyStore) Encrypt(value string) (string, error) {
	return encryptWithKey(value, s.getKey)
}

// DecryptAndUpgrade reads legacy settings and upgrades them with this store's key.
func (s *KeyStore) DecryptAndUpgrade(value string) (string, string, bool, error) {
	return upgradeWithKey(value, s.getKey)
}
