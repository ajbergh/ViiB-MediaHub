// podcast_playback_settings.go owns global and per-show podcast playback defaults
// (speed, skip intervals, intro/outro skip) without altering music player settings.
package db

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
)

const (
    podcastPlaybackSettingsKey = "podcast_playback_settings"
    MaxPodcastSkipTrimSeconds  = 600
    MinPodcastStepSeconds      = 5
    MaxPodcastStepSeconds      = 120
)

// PodcastShowPlaybackSettings configures per-show speed and intro/outro skip defaults.
type PodcastShowPlaybackSettings struct {
    DefaultSpeed     float64 `json:"defaultSpeed"`
    SkipIntroSeconds int     `json:"skipIntroSeconds"`
    SkipOutroSeconds int     `json:"skipOutroSeconds"`
}

// PodcastGlobalPlaybackSettings configures app-wide podcast default speed and seek steps.
type PodcastGlobalPlaybackSettings struct {
    DefaultSpeed       float64 `json:"defaultSpeed"`
    SkipBackSeconds    int     `json:"skipBackSeconds"`
    SkipForwardSeconds int     `json:"skipForwardSeconds"`
}

// DefaultPodcastGlobalPlaybackSettings returns ViiB's built-in podcast playback defaults (1x, -15s/+30s).
func DefaultPodcastGlobalPlaybackSettings() PodcastGlobalPlaybackSettings {
    return PodcastGlobalPlaybackSettings{
        DefaultSpeed:       1,
        SkipBackSeconds:    15,
        SkipForwardSeconds: 30,
    }
}

// IsAllowedPodcastPlaybackSpeed validates that speed is one of ViiB's supported playback rates (or 0 when global fallback is allowed).
func IsAllowedPodcastPlaybackSpeed(speed float64, allowGlobalZero bool) bool {
    if allowGlobalZero && speed == 0 {
        return true
    }
    switch speed {
    case 0.75, 1, 1.25, 1.5, 1.75, 2:
        return true
    default:
        return false
    }
}

func normalizePodcastShowSpeed(custom int, speed float64) float64 {
    if custom != 1 || !IsAllowedPodcastPlaybackSpeed(speed, false) {
        return 0
    }
    return speed
}

func clampSkipTrim(seconds int) int {
    if seconds < 0 {
        return 0
    }
    if seconds > MaxPodcastSkipTrimSeconds {
        return MaxPodcastSkipTrimSeconds
    }
    return seconds
}

// GetPodcastShowPlaybackSettings reads a subscribed show's speed and intro/outro skip defaults.
func (d *DB) GetPodcastShowPlaybackSettings(ctx context.Context, feedID string) (PodcastShowPlaybackSettings, error) {
    var out PodcastShowPlaybackSettings
    if err := d.EnsurePodcastSchema(); err != nil {
        return out, err
    }
    var custom int
    var speed float64
    var intro, outro int
    err := d.conn.QueryRowContext(ctx, `
SELECT playback_speed_custom, default_speed, skip_intro_seconds, skip_outro_seconds
FROM podcast_subscriptions WHERE feed_id=?`, feedID).Scan(&custom, &speed, &intro, &outro)
    if err != nil {
        return out, err
    }
    out.DefaultSpeed = normalizePodcastShowSpeed(custom, speed)
    out.SkipIntroSeconds = clampSkipTrim(intro)
    out.SkipOutroSeconds = clampSkipTrim(outro)
    return out, nil
}

// UpdatePodcastShowPlaybackSettings validates and persists per-show speed and intro/outro skip defaults.
func (d *DB) UpdatePodcastShowPlaybackSettings(ctx context.Context, feedID string, settings PodcastShowPlaybackSettings) (PodcastShowPlaybackSettings, error) {
    if !IsAllowedPodcastPlaybackSpeed(settings.DefaultSpeed, true) {
        return PodcastShowPlaybackSettings{}, errors.New("invalid podcast playback speed")
    }
    if settings.SkipIntroSeconds < 0 || settings.SkipIntroSeconds > MaxPodcastSkipTrimSeconds ||
        settings.SkipOutroSeconds < 0 || settings.SkipOutroSeconds > MaxPodcastSkipTrimSeconds {
        return PodcastShowPlaybackSettings{}, errors.New("invalid podcast skip duration")
    }
    if err := d.EnsurePodcastSchema(); err != nil {
        return PodcastShowPlaybackSettings{}, err
    }
    custom := 0
    storedSpeed := 1.0
    if settings.DefaultSpeed > 0 {
        custom = 1
        storedSpeed = settings.DefaultSpeed
    }
    result, err := d.conn.ExecContext(ctx, `
UPDATE podcast_subscriptions
SET playback_speed_custom=?, default_speed=?, skip_intro_seconds=?, skip_outro_seconds=?
WHERE feed_id=?`,
        custom, storedSpeed, settings.SkipIntroSeconds, settings.SkipOutroSeconds, feedID)
    if err != nil {
        return PodcastShowPlaybackSettings{}, err
    }
    n, err := result.RowsAffected()
    if err != nil {
        return PodcastShowPlaybackSettings{}, err
    }
    if n == 0 {
        return PodcastShowPlaybackSettings{}, sql.ErrNoRows
    }
    return settings, nil
}

// GetPodcastGlobalPlaybackSettings returns persisted global podcast playback defaults, falling back safely on missing/corrupt data.
func (d *DB) GetPodcastGlobalPlaybackSettings(_ context.Context) (PodcastGlobalPlaybackSettings, error) {
    defaults := DefaultPodcastGlobalPlaybackSettings()
    raw, err := d.GetSetting(podcastPlaybackSettingsKey)
    if err != nil {
        return defaults, err
    }
    if raw == "" {
        return defaults, nil
    }
    var parsed PodcastGlobalPlaybackSettings
    if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
        return defaults, nil
    }
    if !IsAllowedPodcastPlaybackSpeed(parsed.DefaultSpeed, false) {
        parsed.DefaultSpeed = defaults.DefaultSpeed
    }
    if parsed.SkipBackSeconds < MinPodcastStepSeconds || parsed.SkipBackSeconds > MaxPodcastStepSeconds {
        parsed.SkipBackSeconds = defaults.SkipBackSeconds
    }
    if parsed.SkipForwardSeconds < MinPodcastStepSeconds || parsed.SkipForwardSeconds > MaxPodcastStepSeconds {
        parsed.SkipForwardSeconds = defaults.SkipForwardSeconds
    }
    return parsed, nil
}

// UpdatePodcastGlobalPlaybackSettings validates and saves global podcast speed and skip intervals.
func (d *DB) UpdatePodcastGlobalPlaybackSettings(_ context.Context, settings PodcastGlobalPlaybackSettings) (PodcastGlobalPlaybackSettings, error) {
    if !IsAllowedPodcastPlaybackSpeed(settings.DefaultSpeed, false) {
        return PodcastGlobalPlaybackSettings{}, errors.New("invalid podcast playback speed")
    }
    if settings.SkipBackSeconds < MinPodcastStepSeconds || settings.SkipBackSeconds > MaxPodcastStepSeconds ||
        settings.SkipForwardSeconds < MinPodcastStepSeconds || settings.SkipForwardSeconds > MaxPodcastStepSeconds {
        return PodcastGlobalPlaybackSettings{}, errors.New("invalid podcast skip interval")
    }
    encoded, err := json.Marshal(settings)
    if err != nil {
        return PodcastGlobalPlaybackSettings{}, err
    }
    if err := d.SetSetting(podcastPlaybackSettingsKey, string(encoded)); err != nil {
        return PodcastGlobalPlaybackSettings{}, err
    }
    return settings, nil
}
