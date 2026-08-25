package plex

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const (
	ProductName   = "ViiB MediaHub"
	PMSAPIVersion = "1.2.2"
	DefaultPort   = 32400
	GDMPort       = 32414
	TrackType     = 10
)

// Server describes a Plex Media Server discovered or validated by ViiB.
// Token material is intentionally never part of this public structure.
type Server struct {
	Name              string `json:"name"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Scheme            string `json:"scheme"`
	URL               string `json:"url"`
	MachineIdentifier string `json:"machineIdentifier"`
	Version           string `json:"version,omitempty"`
	Claimed           bool   `json:"claimed"`
	AuthRequired      bool   `json:"authRequired"`
}

// Library is a selectable music library. ContentKey is the documented provider
// key used to browse the section; TrackKey is the type pivot that returns tracks.
type Library struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	ContentKey string `json:"contentKey,omitempty"`
	TrackKey   string `json:"trackKey,omitempty"`
}

// Track is the subset of PMS track metadata required by ViiB's unified catalog.
type Track struct {
	RatingKey       string
	MetadataKey     string
	Title           string
	Artist          string
	Album           string
	AlbumArtist     string
	TrackNumber     int
	DiscNumber      int
	Genres          []string
	Year            int
	DurationSeconds float64
	ArtworkKey      string
	MediaKey        string
	Container       string
	AudioCodec      string
	AddedAt         int64
	UpdatedAt       int64
}

// SyncResult is returned after a complete, authoritative PMS library read.
type SyncResult struct {
	Tracks   []Track
	Started  time.Time
	Finished time.Time
}

// Credentials contains authentication state persisted through ViiB's encrypted
// sensitive-settings store. JSON responses must never expose this type directly.
type Credentials struct {
	ClientIdentifier   string            `json:"clientIdentifier"`
	KeyID              string            `json:"keyId"`
	PrivateKey         string            `json:"privateKey"`
	AccountToken       string            `json:"accountToken,omitempty"`
	AccountTokenExpiry int64             `json:"accountTokenExpiry,omitempty"`
	ServerTokens       map[string]string `json:"serverTokens,omitempty"`
	PendingPINID       int64             `json:"pendingPinId,omitempty"`
	PendingPINCode     string            `json:"pendingPinCode,omitempty"`
	PendingPINExpiry   int64             `json:"pendingPinExpiry,omitempty"`
}

// AuthStart contains only browser-safe PIN flow state.
type AuthStart struct {
	AuthURL   string `json:"authUrl"`
	ExpiresAt int64  `json:"expiresAt"`
}

// AuthStatus is safe to return to the frontend.
type AuthStatus struct {
	Authenticated bool   `json:"authenticated"`
	Pending       bool   `json:"pending"`
	ExpiresAt     int64  `json:"expiresAt,omitempty"`
	Message       string `json:"message,omitempty"`
}

// StableSourceID and StableTrackID namespace PMS identities inside ViiB so
// local and remote IDs can never collide while remaining stable across syncs.
func StableSourceID(machineIdentifier string) string {
	sum := sha256.Sum256([]byte("plex-source\x00" + machineIdentifier))
	return "plexsrc_" + hex.EncodeToString(sum[:12])
}

func StableTrackID(machineIdentifier, ratingKey string) string {
	sum := sha256.Sum256([]byte("plex-track\x00" + machineIdentifier + "\x00" + ratingKey))
	return "plex_" + hex.EncodeToString(sum[:16])
}
