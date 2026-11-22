package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/podcast"
)

const (
	podcastIndexCredentialsKey = "podcast_index_credentials"
	maxDirectoryCredentialLen  = 256
)

type PodcastDirectoryStatus struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	KeyHint    string `json:"keyHint"`
}

func maskDirectoryAPIKey(apiKey string) string {
	runes := []rune(strings.TrimSpace(apiKey))
	if len(runes) == 0 {
		return ""
	}
	if len(runes) <= 4 {
		return "••••"
	}
	return "••••" + string(runes[len(runes)-4:])
}

func validateDirectoryCredentialPart(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("podcast directory credential contains unsupported control characters")
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > maxDirectoryCredentialLen {
		return "", errors.New("podcast directory credential must be between 1 and 256 characters")
	}
	return trimmed, nil
}

// GetPodcastDirectoryCredentials returns decrypted Podcast Index credentials
// for internal backend callers only. Never expose its return value in API JSON or logs.
func (d *DB) GetPodcastDirectoryCredentials(_ context.Context) (podcast.DirectoryCredentials, error) {
	var creds podcast.DirectoryCredentials
	raw, err := d.GetSetting(podcastIndexCredentialsKey)
	if err != nil {
		return creds, err
	}
	if strings.TrimSpace(raw) == "" {
		return creds, podcast.ErrDirectoryNotConfigured
	}
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return podcast.DirectoryCredentials{}, fmt.Errorf("invalid podcast directory credentials: %w", err)
	}
	creds.APIKey = strings.TrimSpace(creds.APIKey)
	creds.APISecret = strings.TrimSpace(creds.APISecret)
	if creds.APIKey == "" || creds.APISecret == "" {
		return podcast.DirectoryCredentials{}, podcast.ErrDirectoryNotConfigured
	}
	return creds, nil
}

func (d *DB) GetPodcastDirectoryStatus(ctx context.Context) (PodcastDirectoryStatus, error) {
	creds, err := d.GetPodcastDirectoryCredentials(ctx)
	if errors.Is(err, podcast.ErrDirectoryNotConfigured) {
		return PodcastDirectoryStatus{
			Configured: false,
			Provider:   "none",
			KeyHint:    "",
		}, nil
	}
	if err != nil {
		return PodcastDirectoryStatus{
			Configured: false,
			Provider:   "none",
			KeyHint:    "",
		}, err
	}
	return PodcastDirectoryStatus{
		Configured: true,
		Provider:   "podcast_index",
		KeyHint:    maskDirectoryAPIKey(creds.APIKey),
	}, nil
}

func (d *DB) SetPodcastDirectoryCredentials(_ context.Context, apiKey, apiSecret string) error {
	cleanKey, err := validateDirectoryCredentialPart(apiKey)
	if err != nil {
		return err
	}
	cleanSecret, err := validateDirectoryCredentialPart(apiSecret)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(podcast.DirectoryCredentials{
		APIKey:    cleanKey,
		APISecret: cleanSecret,
	})
	if err != nil {
		return err
	}
	return d.SetSetting(podcastIndexCredentialsKey, string(encoded))
}

func (d *DB) DeletePodcastDirectoryCredentials(ctx context.Context) error {
	_, err := d.conn.ExecContext(ctx, "DELETE FROM settings WHERE key = ?", podcastIndexCredentialsKey)
	return err
}
