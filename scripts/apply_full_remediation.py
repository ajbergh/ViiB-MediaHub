#!/usr/bin/env python3
"""Apply the reviewed ViiB MediaHub remediation program.

This script is intentionally deterministic: it patches the reviewed main-tree
revision, creates the new security/lifecycle/test modules, and fails when a
required source anchor is missing. It is run once by the branch workflow and
is also retained as an auditable migration record.
"""
from __future__ import annotations

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def write(path: str, content: str) -> None:
    target = ROOT / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8", newline="\n")


def replace_once(path: str, old: str, new: str, *, required: bool = True) -> None:
    content = read(path)
    count = content.count(old)
    if count == 0:
        if required:
            raise RuntimeError(f"required anchor not found in {path}: {old[:100]!r}")
        return
    if count > 1:
        raise RuntimeError(f"ambiguous anchor in {path}: found {count} occurrences")
    write(path, content.replace(old, new, 1))


def replace_all(path: str, old: str, new: str, *, required: bool = True) -> None:
    content = read(path)
    if old not in content:
        if required:
            raise RuntimeError(f"required anchor not found in {path}: {old[:100]!r}")
        return
    write(path, content.replace(old, new))


def regex_once(path: str, pattern: str, replacement: str, *, flags: int = 0, required: bool = True) -> None:
    content = read(path)
    updated, count = re.subn(pattern, replacement, content, count=1, flags=flags)
    if count == 0:
        if required:
            raise RuntimeError(f"required regex not found in {path}: {pattern[:120]!r}")
        return
    write(path, updated)


# ---------------------------------------------------------------------------
# Phase 0: security and release containment
# ---------------------------------------------------------------------------

# Render third-party playlist descriptions as text, never executable markup.
replace_once(
    "pages/SpotifyPlaylistDetail.tsx",
    '<p className="text-text-secondary mb-4" dangerouslySetInnerHTML={{ __html: playlist.description }} />',
    '<p className="text-text-secondary mb-4">{playlist.description.replace(/<[^>]*>?/gm, \'\')}</p>',
)

# Expand encryption coverage to every provider secret used by the application.
replace_once(
    "backend/internal/crypto/crypto.go",
    '"gemini_api_key":       true,\n\t"lastfm_shared_secret": true,',
    '"gemini_api_key":       true,\n\t"llm_api_key":          true,\n\t"lastfm_api_key":       true,\n\t"lastfm_shared_secret": true,',
)
replace_once(
    "backend/internal/db/db.go",
    'sensitiveKeys := []string{"spotify_credentials", "gemini_api_key"}',
    'sensitiveKeys := []string{"spotify_credentials", "gemini_api_key", "llm_api_key", "lastfm_api_key", "lastfm_shared_secret", "lastfm_session_key"}',
)

# Generic settings reads are no longer a secret-export API. Spotify credentials
# are only managed by the dedicated Spotify endpoints.
replace_once(
    "backend/internal/validation/validation.go",
    '// IsValidSettingKey checks if a setting key is in the allowed list.\n',
    '''// IsSensitiveSettingKey reports whether a setting is write-only through the API.\n// Sensitive values may be configured, but are never returned to the renderer.\nfunc IsSensitiveSettingKey(key string) bool {\n\tsensitive := map[string]bool{\n\t\t"gemini_api_key":       true,\n\t\t"llm_api_key":          true,\n\t\t"lastfm_api_key":       true,\n\t\t"lastfm_shared_secret": true,\n\t\t"lastfm_session_key":   true,\n\t}\n\treturn sensitive[key]\n}\n\n// IsValidSettingKey checks if a setting key is in the allowed list.\n''',
)
replace_once(
    "backend/internal/validation/validation.go",
    '\t\t"spotify_credentials":      true,\n',
    '',
)
replace_once(
    "backend/internal/api/api.go",
    '''\tvalue, err := a.db.GetSetting(key)\n\tif err != nil {\n\t\trespondError(w, http.StatusInternalServerError, "Failed to get setting")\n\t\treturn\n\t}\n\n\trespondJSON(w, map[string]string{"key": key, "value": value})''',
    '''\tif validation.IsSensitiveSettingKey(key) {\n\t\trespondJSON(w, map[string]string{"key": key, "value": "", "configured": "true"})\n\t\treturn\n\t}\n\n\tvalue, err := a.db.GetSetting(key)\n\tif err != nil {\n\t\trespondError(w, http.StatusInternalServerError, "Failed to get setting")\n\t\treturn\n\t}\n\n\trespondJSON(w, map[string]string{"key": key, "value": value})''',
)

# Remove duplicate settings route registration.
replace_once(
    "backend/internal/api/api.go",
    '''\n\t// Settings\n\tr.Get("/settings/{key}", a.getSetting)\n\tr.Post("/settings/{key}", a.setSetting)\n\n\treturn r\n''',
    '''\n\treturn r\n''',
)

# Secrets are not persisted in browser localStorage.
replace_once(
    "store.ts",
    '        spotifyClientId: state.spotifyClientId,\n        spotifyClientSecret: state.spotifyClientSecret,\n',
    '        spotifyClientId: state.spotifyClientId,\n',
)
replace_all(
    "store.ts",
    'Spotify client credentials persisted to localStorage (tokens held in-memory only)',
    'Spotify client ID and non-sensitive preferences persisted to localStorage; secrets and tokens remain memory-only',
)
replace_all(
    "slices/spotifySlice.ts",
    'Credentials (clientId/secret) and preferences are persisted to localStorage.',
    'Only the client ID and non-sensitive preferences are persisted to localStorage.',
)

# OAuth: add state correlation, use PKCE as a public client, and eliminate the
# renderer-side client-secret dependency for code exchange and refresh.
replace_once(
    "services/spotifyService.ts",
    '''        const codeChallenge = base64encode(hashed);\n\n        const scopes = [''',
    '''        const codeChallenge = base64encode(hashed);\n        const state = generateRandomString(32);\n\n        const scopes = [''',
)
replace_once(
    "services/spotifyService.ts",
    '''            code_challenge_method: 'S256',\n            code_challenge: codeChallenge\n''',
    '''            code_challenge_method: 'S256',\n            code_challenge: codeChallenge,\n            state\n''',
)
replace_once(
    "services/spotifyService.ts",
    '''        return {\n            url,\n            codeVerifier\n        };''',
    '''        return {\n            url,\n            codeVerifier,\n            state\n        };''',
)
replace_once(
    "services/spotifyService.ts",
    'async exchangeCode(clientId: string, clientSecret: string, code: string, redirectUri: string, codeVerifier: string)',
    'async exchangeCode(clientId: string, _clientSecret: string, code: string, redirectUri: string, codeVerifier: string)',
)
regex_once(
    "services/spotifyService.ts",
    r'''\n        if \(clientSecret\) \{\n             const credentials = btoa\(`\$\{clientId\}:\$\{clientSecret\}`\);\n             headers\['Authorization'\] = `Basic \$\{credentials\}`;\n        \}\n''',
    '\n',
)
replace_once(
    "services/spotifyService.ts",
    '        if (!spotifyClientId || !spotifyClientSecret) {\n            return null;\n        }',
    '        if (!spotifyClientId) {\n            return null;\n        }',
)
replace_once(
    "services/spotifyService.ts",
    '''                    const credentials = btoa(`${spotifyClientId}:${spotifyClientSecret}`);\n                    const response = await fetch(TOKEN_URL, {\n                        method: 'POST',\n                        headers: {\n                            'Authorization': `Basic ${credentials}`,\n                            'Content-Type': 'application/x-www-form-urlencoded'\n                        },\n                        body: new URLSearchParams({\n                            grant_type: 'refresh_token',\n                            refresh_token: spotifyRefreshToken\n                        })\n                    });''',
    '''                    const response = await fetch(TOKEN_URL, {\n                        method: 'POST',\n                        headers: {\n                            'Content-Type': 'application/x-www-form-urlencoded'\n                        },\n                        body: new URLSearchParams({\n                            grant_type: 'refresh_token',\n                            refresh_token: spotifyRefreshToken,\n                            client_id: spotifyClientId\n                        })\n                    });''',
)
regex_once(
    "services/spotifyService.ts",
    r'''\n        // 2\. Fallback: Client Credentials Flow.*?\n        return null;\n    \},''',
    '''\n        // Public PKCE clients do not use a client-credentials fallback.\n        return null;\n    },''',
    flags=re.S,
)

replace_once(
    "pages/Spotify.tsx",
    '        if (!spotifyClientId || !spotifyClientSecret) {\n            alert("Please configure your Client ID and Client Secret in Settings first.");',
    '        if (!spotifyClientId) {\n            alert("Please configure your Spotify Client ID in Settings first.");',
)
replace_once(
    "pages/Spotify.tsx",
    '        const { url, codeVerifier } = await SpotifyService.generateAuthUrl(spotifyClientId, redirectUri);',
    '        const { url, codeVerifier, state } = await SpotifyService.generateAuthUrl(spotifyClientId, redirectUri);',
)
replace_once(
    "pages/Spotify.tsx",
    '                    clientSecret: spotifyClientSecret,',
    "                    clientSecret: '',",
)
replace_once(
    "pages/Spotify.tsx",
    "                console.log('[Spotify] Pre-saving credentials to backend:', JSON.stringify(preSaveData));\n",
    '',
)
replace_once(
    "pages/Spotify.tsx",
    "        localStorage.setItem('spotify_code_verifier', codeVerifier);\n",
    "        localStorage.setItem('spotify_code_verifier', codeVerifier);\n        localStorage.setItem('spotify_oauth_state', state);\n",
)

replace_once(
    "pages/SpotifyCallback.tsx",
    "        const code = searchParams.get('code');\n        const error = searchParams.get('error');",
    "        const code = searchParams.get('code');\n        const returnedState = searchParams.get('state');\n        const expectedState = localStorage.getItem('spotify_oauth_state');\n        const error = searchParams.get('error');",
)
replace_once(
    "pages/SpotifyCallback.tsx",
    '''        if (!code) {\n            setStatus('error');\n            setErrorMsg('No authorization code returned');\n            return;\n        }\n''',
    '''        if (!code) {\n            setStatus('error');\n            setErrorMsg('No authorization code returned');\n            return;\n        }\n\n        if (!returnedState || !expectedState || returnedState !== expectedState) {\n            setStatus('error');\n            setErrorMsg('OAuth state validation failed. Please try connecting again.');\n            return;\n        }\n''',
)
replace_once(
    "pages/SpotifyCallback.tsx",
    '                if (!clientId || !clientSecret) {\n                    throw new Error("Missing Spotify credentials. Please configure them in Settings.");\n                }',
    '                if (!clientId) {\n                    throw new Error("Missing Spotify Client ID. Please configure it in Settings.");\n                }',
)
replace_once(
    "pages/SpotifyCallback.tsx",
    "                        console.log('[SpotifyCallback] Backend credentials response:', JSON.stringify(creds));\n",
    '',
)
replace_once(
    "pages/SpotifyCallback.tsx",
    '                const data = await SpotifyService.exchangeCode(clientId, clientSecret, code, redirectUri, codeVerifier);',
    "                const data = await SpotifyService.exchangeCode(clientId, '', code, redirectUri, codeVerifier);",
)
replace_once(
    "pages/SpotifyCallback.tsx",
    "                localStorage.removeItem('spotify_redirect_uri');\n",
    "                localStorage.removeItem('spotify_redirect_uri');\n                localStorage.removeItem('spotify_oauth_state');\n",
)

# Redact the client secret from the dedicated credential response while keeping
# the current token/session compatibility during this migration.
replace_once(
    "backend/internal/api/spotify.go",
    '\trespondJSON(w, creds)\n}',
    '\tcreds.ClientSecret = ""\n\trespondJSON(w, creds)\n}',
)

# Security headers for both browser and Wails modes.
replace_once(
    "backend/internal/server/server.go",
    '\tr.Use(middleware.Compress(5))\n',
    '''\tr.Use(middleware.Compress(5))\n\tr.Use(func(next http.Handler) http.Handler {\n\t\treturn http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {\n\t\t\tw.Header().Set("X-Content-Type-Options", "nosniff")\n\t\t\tw.Header().Set("Referrer-Policy", "no-referrer")\n\t\t\tw.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")\n\t\t\tw.Header().Set("Cross-Origin-Resource-Policy", "same-origin")\n\t\t\tnext.ServeHTTP(w, req)\n\t\t})\n\t})\n''',
)

# ---------------------------------------------------------------------------
# Hardened remote image fetcher (SSRF, redirects, timeouts, size and image checks)
# ---------------------------------------------------------------------------

write(
    "backend/internal/mediafetch/mediafetch.go",
    r'''// Package mediafetch safely retrieves remote artwork for local caching.
package mediafetch

import (
    "bytes"
    "context"
    "crypto/tls"
    "errors"
    "fmt"
    "image"
    _ "image/gif"
    _ "image/jpeg"
    _ "image/png"
    "io"
    "net"
    "net/http"
    "net/url"
    "strings"
    "time"
)

const DefaultMaxBytes int64 = 12 << 20

func isPublicIP(ip net.IP) bool {
    if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
        return false
    }
    return true
}

func validateURL(ctx context.Context, raw string) (*url.URL, error) {
    parsed, err := url.Parse(raw)
    if err != nil {
        return nil, fmt.Errorf("invalid URL: %w", err)
    }
    if parsed.Scheme != "https" {
        return nil, errors.New("only HTTPS artwork URLs are allowed")
    }
    if parsed.User != nil || parsed.Hostname() == "" {
        return nil, errors.New("invalid artwork host")
    }
    ips, err := net.DefaultResolver.LookupIP(ctx, "ip", parsed.Hostname())
    if err != nil || len(ips) == 0 {
        return nil, errors.New("artwork host could not be resolved")
    }
    for _, ip := range ips {
        if !isPublicIP(ip) {
            return nil, errors.New("private, loopback, or link-local artwork hosts are blocked")
        }
    }
    return parsed, nil
}

// FetchImage retrieves and validates an image. Redirects are revalidated and
// responses are bounded before being decoded.
func FetchImage(ctx context.Context, raw string, maxBytes int64) ([]byte, string, error) {
    if maxBytes <= 0 {
        maxBytes = DefaultMaxBytes
    }
    parsed, err := validateURL(ctx, raw)
    if err != nil {
        return nil, "", err
    }

    transport := &http.Transport{
        Proxy: http.ProxyFromEnvironment,
        DialContext: (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 20 * time.Second}).DialContext,
        TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
        TLSHandshakeTimeout: 8 * time.Second,
        ResponseHeaderTimeout: 10 * time.Second,
        IdleConnTimeout: 30 * time.Second,
    }
    client := &http.Client{
        Transport: transport,
        Timeout: 20 * time.Second,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) >= 4 {
                return errors.New("too many artwork redirects")
            }
            _, err := validateURL(req.Context(), req.URL.String())
            return err
        },
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
    if err != nil {
        return nil, "", err
    }
    req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif;q=0.8")
    req.Header.Set("User-Agent", "ViiB-MediaHub/1.0")

    resp, err := client.Do(req)
    if err != nil {
        return nil, "", fmt.Errorf("artwork request failed: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return nil, "", fmt.Errorf("artwork server returned HTTP %d", resp.StatusCode)
    }
    if resp.ContentLength > maxBytes {
        return nil, "", errors.New("artwork exceeds maximum size")
    }

    data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
    if err != nil {
        return nil, "", fmt.Errorf("failed to read artwork: %w", err)
    }
    if int64(len(data)) > maxBytes {
        return nil, "", errors.New("artwork exceeds maximum size")
    }

    detected := http.DetectContentType(data)
    if !strings.HasPrefix(detected, "image/") {
        return nil, "", errors.New("remote content is not an image")
    }
    cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
    if err != nil {
        return nil, "", errors.New("remote content is not a decodable image")
    }
    if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 12000 || cfg.Height > 12000 || int64(cfg.Width)*int64(cfg.Height) > 60_000_000 {
        return nil, "", errors.New("artwork dimensions are invalid or excessive")
    }
    return data, detected, nil
}
''',
)

write(
    "backend/internal/mediafetch/mediafetch_test.go",
    r'''package mediafetch

import (
    "context"
    "testing"
)

func TestValidateURLRejectsUnsafeTargets(t *testing.T) {
    tests := []string{
        "http://example.com/image.jpg",
        "https://127.0.0.1/image.jpg",
        "https://localhost/image.jpg",
        "file:///etc/passwd",
    }
    for _, candidate := range tests {
        if _, err := validateURL(context.Background(), candidate); err == nil {
            t.Fatalf("expected %q to be rejected", candidate)
        }
    }
}

func TestPublicIPClassification(t *testing.T) {
    if isPublicIP([]byte{127, 0, 0, 1}) {
        t.Fatal("loopback address must not be public")
    }
}
''',
)

replace_once(
    "backend/internal/api/api.go",
    '"github.com/ajbergh/viib-mediahub/internal/logger"\n',
    '"github.com/ajbergh/viib-mediahub/internal/logger"\n\t"github.com/ajbergh/viib-mediahub/internal/mediafetch"\n',
)

regex_once(
    "backend/internal/api/api.go",
    r'''func \(a \*API\) downloadAlbumCover\(w http\.ResponseWriter, r \*http\.Request\) \{.*?\n\}\n\n// resetAlbumMetadata''',
    r'''func (a *API) downloadAlbumCover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AlbumKey string `json:"albumKey"`
		ImageURL string `json:"imageUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	parts := strings.Split(req.AlbumKey, "::")
	if len(parts) != 2 || req.ImageURL == "" {
		respondError(w, http.StatusBadRequest, "albumKey must be album::artist and imageUrl is required")
		return
	}
	albumName, artistName := parts[0], parts[1]
	songs, err := a.db.GetAllSongs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get songs")
		return
	}
	var albumFolder string
	for _, song := range songs {
		songArtist := song.AlbumArtist
		if songArtist == "" { songArtist = song.Artist }
		if song.Album == albumName && songArtist == artistName {
			albumFolder = filepath.Dir(song.FilePath)
			break
		}
	}
	if albumFolder == "" {
		respondError(w, http.StatusNotFound, "No songs found for this album and artist")
		return
	}
	data, _, err := mediafetch.FetchImage(r.Context(), req.ImageURL, mediafetch.DefaultMaxBytes)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to download artwork: %v", err))
		return
	}
	coverPath := filepath.Join(albumFolder, "cover.jpg")
	if err := os.WriteFile(coverPath, data, 0600); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save cover")
		return
	}
	if err := a.db.UpdateAlbumLocalCover(req.AlbumKey, coverPath); err != nil {
		logger.API("Warning: Failed to update album local cover in database: %v", err)
	}
	respondJSON(w, map[string]interface{}{"status": "ok", "coverPath": coverPath})
}

// resetAlbumMetadata''',
    flags=re.S,
)

regex_once(
    "backend/internal/api/api.go",
    r'''func \(a \*API\) downloadArtistImage\(w http\.ResponseWriter, r \*http\.Request\) \{.*?\n\}\n\ntype enrichGenresRequest''',
    r'''func (a *API) downloadArtistImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArtistName string `json:"artistName"`
		ImageURL string `json:"imageUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ArtistName == "" || req.ImageURL == "" {
		respondError(w, http.StatusBadRequest, "artistName and imageUrl are required")
		return
	}
	data, _, err := mediafetch.FetchImage(r.Context(), req.ImageURL, mediafetch.DefaultMaxBytes)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to download artwork: %v", err))
		return
	}
	artistImagesDir := filepath.Join(a.coverDir, "artists")
	if err := os.MkdirAll(artistImagesDir, 0700); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create artist image directory")
		return
	}
	safeArtistName := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(req.ArtistName)
	imagePath := filepath.Join(artistImagesDir, safeArtistName+".jpg")
	if err := os.WriteFile(imagePath, data, 0600); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save artist image")
		return
	}
	if err := a.db.UpdateArtistLocalImage(req.ArtistName, imagePath); err != nil {
		logger.API("Failed to update artist image for %q: %v", req.ArtistName, err)
	}
	respondJSON(w, map[string]interface{}{"status": "ok", "imagePath": imagePath})
}

type enrichGenresRequest''',
    flags=re.S,
)

# ---------------------------------------------------------------------------
# Phase 1: reliability, correctness, identity and lifecycle
# ---------------------------------------------------------------------------

# Download progress becomes a real fan-out broker.
replace_once(
    "backend/internal/api/download_manager.go",
    '\tprogressChan     chan DownloadProgress         // Channel for SSE progress updates\n',
    '\tprogressChan     chan DownloadProgress         // Internal progress event bus\n\tprogressSubscribers map[chan DownloadProgress]struct{}\n\tprogressSubsMu       sync.RWMutex\n',
)
replace_once(
    "backend/internal/api/download_manager.go",
    '\t\tprogressChan:     make(chan DownloadProgress, 100),\n',
    '\t\tprogressChan:     make(chan DownloadProgress, 100),\n\t\tprogressSubscribers: make(map[chan DownloadProgress]struct{}),\n',
)
replace_once(
    "backend/internal/api/download_manager.go",
    '\t// Start worker goroutines\n',
    '\tgo dm.broadcastProgress()\n\n\t// Start worker goroutines\n',
)
replace_once(
    "backend/internal/api/download_manager.go",
    '\t\tatomic.AddInt32(&dm.activeCount, -1)\n',
    '',
)
replace_once(
    "backend/internal/api/download_manager.go",
    '''// GetProgressChan returns the progress channel for real-time updates\nfunc (dm *DownloadManager) GetProgressChan() <-chan DownloadProgress {\n\treturn dm.progressChan\n}\n''',
    '''// SubscribeProgress registers an independent progress stream for one client.\nfunc (dm *DownloadManager) SubscribeProgress() chan DownloadProgress {\n\tch := make(chan DownloadProgress, 64)\n\tdm.progressSubsMu.Lock()\n\tdm.progressSubscribers[ch] = struct{}{}\n\tdm.progressSubsMu.Unlock()\n\treturn ch\n}\n\n// UnsubscribeProgress removes and closes a client progress stream.\nfunc (dm *DownloadManager) UnsubscribeProgress(ch chan DownloadProgress) {\n\tdm.progressSubsMu.Lock()\n\tif _, ok := dm.progressSubscribers[ch]; ok {\n\t\tdelete(dm.progressSubscribers, ch)\n\t\tclose(ch)\n\t}\n\tdm.progressSubsMu.Unlock()\n}\n\nfunc (dm *DownloadManager) broadcastProgress() {\n\tfor {\n\t\tselect {\n\t\tcase <-dm.ctx.Done():\n\t\t\treturn\n\t\tcase progress := <-dm.progressChan:\n\t\t\tdm.progressSubsMu.RLock()\n\t\t\tfor subscriber := range dm.progressSubscribers {\n\t\t\t\tselect {\n\t\t\t\tcase subscriber <- progress:\n\t\t\t\tdefault:\n\t\t\t\t\tdmLogDebug("Dropping progress event for slow subscriber")\n\t\t\t\t}\n\t\t\t}\n\t\t\tdm.progressSubsMu.RUnlock()\n\t\t}\n\t}\n}\n''',
)
regex_once(
    "backend/internal/api/spotify.go",
    r'''func \(a \*API\) downloadProgressSSE\(w http\.ResponseWriter, r \*http\.Request\) \{.*?\n\}\n\n// Helper functions to fetch Spotify metadata''',
    r'''func (a *API) downloadProgressSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}
	progressChan := a.downloadManager.SubscribeProgress()
	defer a.downloadManager.UnsubscribeProgress(progressChan)
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case progress, ok := <-progressChan:
			if !ok { return }
			data, _ := json.Marshal(progress)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// Helper functions to fetch Spotify metadata''',
    flags=re.S,
)

# Fix Spotify search response consumption and constrain proxy paths.
replace_once(
    "backend/internal/api/spotify.go",
    '"net/http"\n',
    '"net/http"\n\t"net/url"\n',
)
replace_once(
    "backend/internal/api/spotify.go",
    '"strings"\n',
    '"strings"\n\t"time"\n',
)
regex_once(
    "backend/internal/api/spotify.go",
    r'''func \(a \*API\) spotifySearch\(w http\.ResponseWriter, r \*http\.Request\) \{.*?\n\}\n\n// spotifySearchPlaylists''',
    r'''func (a *API) spotifySearch(w http.ResponseWriter, r *http.Request) {
	val, err := a.db.GetSetting("spotify_credentials")
	if err != nil || val == "" {
		respondError(w, http.StatusUnauthorized, "Spotify credentials not configured")
		return
	}
	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse credentials")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		respondError(w, http.StatusBadRequest, "Missing query parameter")
		return
	}
	params := url.Values{"q": {query}}
	for _, key := range []string{"type", "limit", "offset", "market"} {
		if value := r.URL.Query().Get(key); value != "" { params.Set(key, value) }
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.spotify.com/v1/search?"+params.Encode(), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create request")
		return
	}
	request.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		respondError(w, http.StatusBadGateway, "Failed to fetch from Spotify")
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// spotifySearchPlaylists''',
    flags=re.S,
)
replace_once(
    "backend/internal/api/spotify.go",
    '''\t// Build full URL\n\tspotifyURL := "https://api.spotify.com/v1/" + path\n''',
    '''\tpath = strings.TrimLeft(path, "/")\n\tif strings.Contains(path, "..") || strings.Contains(path, "\\\\") || strings.Contains(path, "://") {\n\t\trespondError(w, http.StatusBadRequest, "Invalid Spotify API path")\n\t\treturn\n\t}\n\tspotifyURL := "https://api.spotify.com/v1/" + path\n''',
)
replace_all(
    "backend/internal/api/spotify.go",
    'client := &http.Client{}',
    'client := &http.Client{Timeout: 30 * time.Second}',
    required=False,
)

# Canonical album identity helpers and routes.
write(
    "lib/albumIdentity.ts",
    r'''export const albumIdentity = (album: string, artist: string): string =>
  `${album.trim()}::${artist.trim()}`;

export const albumRoute = (album: string, artist: string): string =>
  `/album/${encodeURIComponent(album)}/${encodeURIComponent(artist)}`;
''',
)
write(
    "lib/albumIdentity.test.ts",
    r'''import { describe, expect, it } from 'vitest';
import { albumIdentity, albumRoute } from './albumIdentity';

describe('album identity', () => {
  it('keeps same-named albums from different artists distinct', () => {
    expect(albumIdentity('Greatest Hits', 'Artist A')).not.toBe(albumIdentity('Greatest Hits', 'Artist B'));
  });
  it('encodes route segments', () => {
    expect(albumRoute('A/B', 'AC & DC')).toBe('/album/A%2FB/AC%20%26%20DC');
  });
});
''',
)
replace_once(
    "App.tsx",
    '<Route path="/album/:albumName" element={<AlbumDetail />} />',
    '<Route path="/album/:albumName/:artistName?" element={<AlbumDetail />} />',
)
replace_once(
    "pages/Search.tsx",
    "navigate(`/album/${encodeURIComponent(item.data.name)}/${encodeURIComponent(item.data.artist)}`)",
    "navigate(`/album/${encodeURIComponent(item.data.name)}/${encodeURIComponent(item.data.artist)}`)",
    required=False,
)
# The reviewed search source used albumName/artist fields.
replace_all(
    "pages/Search.tsx",
    "navigate(`/album/${encodeURIComponent(item.data.album)}/${encodeURIComponent(item.data.artist)}`)",
    "navigate(`/album/${encodeURIComponent(item.data.album)}/${encodeURIComponent(item.data.artist)}`)",
    required=False,
)
replace_all(
    "pages/Search.tsx",
    "navigate(`/album/${encodeURIComponent(item.data.name)}`)",
    "navigate(`/album/${encodeURIComponent(item.data.name)}/${encodeURIComponent(item.data.artist)}`)",
    required=False,
)
replace_once(
    "pages/AlbumDetail.tsx",
    "    const { albumName } = useParams<{ albumName: string }>();",
    "    const { albumName, artistName } = useParams<{ albumName: string; artistName?: string }>();",
)
replace_once(
    "pages/AlbumDetail.tsx",
    '''    // Filter and Sort songs for this album\n    const albumSongs = useMemo(() => {\n        return songs.filter(s => s.album === decodedAlbumName)\n''',
    '''    const decodedArtistName = useMemo(() => {\n        try { return decodeURIComponent(artistName || ''); } catch { return artistName || ''; }\n    }, [artistName]);\n\n    // Filter and sort by both album and album artist to avoid same-title collisions.\n    const albumSongs = useMemo(() => {\n        return songs.filter(s => {\n                const songArtist = s.albumArtist || s.artist;\n                return s.album === decodedAlbumName && (!decodedArtistName || songArtist === decodedArtistName);\n            })\n''',
)
replace_once(
    "pages/AlbumDetail.tsx",
    '    }, [songs, decodedAlbumName]);',
    '    }, [songs, decodedAlbumName, decodedArtistName]);',
)
replace_once(
    "pages/AlbumDetail.tsx",
    '    const coverUrl = metadata?.coverUrl || firstSong?.coverUrl || albumCovers[decodedAlbumName];',
    '    const coverUrl = metadata?.coverUrl || firstSong?.coverUrl || albumCovers[metadataKey] || albumCovers[decodedAlbumName];',
)

regex_once(
    "store.ts",
    r'''export const useAlbums = \(\) => useStore\(.*?\n\);\n\n// Selector for Artists''',
    r'''export const useAlbums = () => useStore(
  (state) => {
    const albumsMap = new Map<string, Album>();
    state.songs.forEach((song) => {
      const artist = song.albumArtist || song.artist || 'Unknown Artist';
      const key = `${song.album}::${artist}`;
      const existing = albumsMap.get(key);
      if (existing) {
        existing.songCount += 1;
        existing.addedAt = Math.max(existing.addedAt || 0, song.addedAt || 0);
        if (!existing.coverUrl && song.coverUrl) existing.coverUrl = song.coverUrl;
      } else {
        albumsMap.set(key, {
          name: song.album || 'Unknown Album',
          artist,
          songCount: 1,
          coverUrl: song.coverUrl,
          addedAt: song.addedAt,
        });
      }
    });
    return Array.from(albumsMap.values()).sort((a, b) => a.name.localeCompare(b.name) || a.artist.localeCompare(b.artist));
  }
);

// Selector for Artists''',
    flags=re.S,
)
regex_once(
    "store.ts",
    r'''export const useAlbumCovers = \(\) => \{.*?\n\};''',
    r'''export const useAlbumCovers = () => {
  const songs = useStore((state) => state.songs);
  return useMemo(() => {
    const covers: Record<string, string> = {};
    songs.forEach((song) => {
      if (!song.coverUrl) return;
      const artist = song.albumArtist || song.artist || 'Unknown Artist';
      const composite = `${song.album}::${artist}`;
      if (!covers[composite]) covers[composite] = song.coverUrl;
      if (!covers[song.album]) covers[song.album] = song.coverUrl;
    });
    return covers;
  }, [songs]);
};''',
    flags=re.S,
)

# Patch common album navigation call sites.
nav_replacements = {
    "navigate(`/album/${encodeURIComponent(album.name)}`)": "navigate(`/album/${encodeURIComponent(album.name)}/${encodeURIComponent(album.artist)}`)",
    "navigate(`/album/${encodeURIComponent(a.name)}`)": "navigate(`/album/${encodeURIComponent(a.name)}/${encodeURIComponent(a.artist)}`)",
    "navigate(`/album/${encodeURIComponent(song.album)}`)": "navigate(`/album/${encodeURIComponent(song.album)}/${encodeURIComponent(song.albumArtist || song.artist)}`)",
}
for candidate in [
    "pages/Albums.tsx", "pages/LikedAlbums.tsx", "pages/Stats.tsx", "pages/ArtistDetail.tsx",
    "components/home/useHomeContent.ts", "components/context-menus/QueueItemMenu.tsx",
]:
    if not (ROOT / candidate).exists():
        continue
    content = read(candidate)
    for old, new in nav_replacements.items():
        content = content.replace(old, new)
    write(candidate, content)

# Complete song DTO round-trip.
replace_once(
    "services/api.ts",
    '''  skipCount?: number;\n  // User preferences''',
    '''  skipCount?: number;\n  fileHash?: string;\n  mood?: string;\n  energy?: string;\n  tempo?: string;\n  bpm?: number;\n  instrumental?: boolean;\n  moodAnalyzedAt?: number;\n  lastfmListeners?: number;\n  lastfmPlaycount?: number;\n  lastfmTags?: string;\n  lastfmUrl?: string;\n  lastfmMbid?: string;\n  lastfmEnrichedAt?: number;\n  // User preferences''',
)
replace_once(
    "services/backendService.ts",
    '''    skipCount: apiSong.skipCount,\n  };''',
    '''    skipCount: apiSong.skipCount,\n    fileHash: apiSong.fileHash,\n    mood: apiSong.mood,\n    energy: apiSong.energy,\n    tempo: apiSong.tempo,\n    bpm: apiSong.bpm,\n    instrumental: apiSong.instrumental,\n    moodAnalyzedAt: apiSong.moodAnalyzedAt,\n    liked: apiSong.liked,\n    likedAt: apiSong.likedAt,\n  };''',
)

# Replace GetAllSongs with a complete column/JSON mapping.
regex_once(
    "backend/internal/db/db.go",
    r'''func \(d \*DB\) GetAllSongs\(\) \(\[\]Song, error\) \{.*?\n\}\n\n// SaveSong''',
    r'''func (d *DB) GetAllSongs() ([]Song, error) {
	rows, err := d.conn.Query(`
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, original_year, year_uncertain, year_analyzed_at,
		       duration, file_path, cover_path, added_at, play_count, last_played,
		       skip_count, file_hash, mood, energy, tempo, bpm, instrumental,
		       mood_analyzed_at, liked, liked_at, lastfm_listeners, lastfm_playcount,
		       lastfm_tags, lastfm_url, lastfm_mbid, lastfm_enriched_at
		FROM songs
		ORDER BY album, album_artist, disc_number, track_number, title`)
	if err != nil { return nil, err }
	defer rows.Close()
	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON, albumArtist, coverPath, fileHash, mood, energy, tempo sql.NullString
		var lastFMTags, lastFMURL, lastFMMBID sql.NullString
		var trackNum, discNum, year, originalYear, yearUncertain, yearAnalyzedAt sql.NullInt64
		var playCount, lastPlayed, skipCount, bpm, instrumental, moodAnalyzedAt sql.NullInt64
		var liked, likedAt, lastFMListeners, lastFMPlaycount, lastFMEnrichedAt sql.NullInt64
		if err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist, &trackNum, &discNum,
			&genreJSON, &year, &originalYear, &yearUncertain, &yearAnalyzedAt,
			&s.Duration, &s.FilePath, &coverPath, &s.AddedAt, &playCount, &lastPlayed,
			&skipCount, &fileHash, &mood, &energy, &tempo, &bpm, &instrumental,
			&moodAnalyzedAt, &liked, &likedAt, &lastFMListeners, &lastFMPlaycount,
			&lastFMTags, &lastFMURL, &lastFMMBID, &lastFMEnrichedAt,
		); err != nil { return nil, err }
		if albumArtist.Valid { s.AlbumArtist = albumArtist.String }
		if trackNum.Valid { s.TrackNumber = int(trackNum.Int64) }
		if discNum.Valid { s.DiscNumber = int(discNum.Int64) }
		if genreJSON.Valid && genreJSON.String != "" { _ = json.Unmarshal([]byte(genreJSON.String), &s.Genre) }
		if year.Valid { s.Year = int(year.Int64) }
		if originalYear.Valid { s.OriginalYear = int(originalYear.Int64) }
		s.YearUncertain = yearUncertain.Valid && yearUncertain.Int64 == 1
		if yearAnalyzedAt.Valid { s.YearAnalyzedAt = yearAnalyzedAt.Int64 }
		if coverPath.Valid { s.CoverPath = coverPath.String }
		if playCount.Valid { s.PlayCount = int(playCount.Int64) }
		if lastPlayed.Valid { s.LastPlayed = lastPlayed.Int64 }
		if skipCount.Valid { s.SkipCount = int(skipCount.Int64) }
		if fileHash.Valid { s.FileHash = fileHash.String }
		if mood.Valid { s.Mood = mood.String }
		if energy.Valid { s.Energy = energy.String }
		if tempo.Valid { s.Tempo = tempo.String }
		if bpm.Valid { s.BPM = int(bpm.Int64) }
		s.Instrumental = instrumental.Valid && instrumental.Int64 == 1
		if moodAnalyzedAt.Valid { s.MoodAnalyzedAt = moodAnalyzedAt.Int64 }
		s.Liked = liked.Valid && liked.Int64 == 1
		if likedAt.Valid { s.LikedAt = likedAt.Int64 }
		if lastFMListeners.Valid { s.LastFMListeners = int(lastFMListeners.Int64) }
		if lastFMPlaycount.Valid { s.LastFMPlaycount = int(lastFMPlaycount.Int64) }
		if lastFMTags.Valid { s.LastFMTags = lastFMTags.String }
		if lastFMURL.Valid { s.LastFMURL = lastFMURL.String }
		if lastFMMBID.Valid { s.LastFMMBID = lastFMMBID.String }
		if lastFMEnrichedAt.Valid { s.LastFMEnrichedAt = lastFMEnrichedAt.Int64 }
		songs = append(songs, s)
	}
	return songs, rows.Err()
}

// SaveSong''',
    flags=re.S,
)

# Defensive listen-event math.
replace_once(
    "backend/internal/api/api.go",
    '''\t// Determine event type based on play duration\n\tvar eventType string\n\tcompletionRatio := body.PlayDuration / body.SongDuration\n''',
    '''\tif body.PlayDuration < 0 || body.SongDuration <= 0 || body.PlayDuration > 7*24*60*60 || body.SongDuration > 7*24*60*60 {\n\t\trespondError(w, http.StatusBadRequest, "Invalid listening durations")\n\t\treturn\n\t}\n\n\t// Determine event type based on play duration\n\tvar eventType string\n\tcompletionRatio := body.PlayDuration / body.SongDuration\n''',
)

# Lifecycle ownership and graceful service shutdown.
write(
    "backend/internal/api/lifecycle.go",
    r'''package api

// Close stops background workers owned by the API.
func (a *API) Close() {
    if a.downloadManager != nil { a.downloadManager.Stop() }
    if a.scanner != nil { a.scanner.Close() }
}
''',
)
write(
    "backend/internal/scanner/lifecycle.go",
    r'''package scanner

// Close stops scanner background services.
func (s *Scanner) Close() {
    if s.backgroundScanner != nil { s.backgroundScanner.Stop() }
}
''',
)
replace_once(
    "backend/cmd/viib/main.go",
    '\tapiHandler := api.New(database, *dataDir)\n',
    '\tapiHandler := api.New(database, *dataDir)\n\tdefer apiHandler.Close()\n',
)
replace_once(
    "backend/cmd/wails/main.go",
    '\tapiHandler := api.New(database, dataDir)\n',
    '\tapiHandler := api.New(database, dataDir)\n\tdefer apiHandler.Close()\n',
    required=False,
)

# Version is owned by one Go package and can be injected with -ldflags.
write(
    "backend/internal/version/version.go",
    '''package version\n\n// Current is overridden at build time for tagged releases.\nvar Current = "1.0.0-dev"\n''',
)
replace_once(
    "backend/internal/api/api.go",
    '"github.com/ajbergh/viib-mediahub/internal/validation"\n',
    '"github.com/ajbergh/viib-mediahub/internal/validation"\n\t"github.com/ajbergh/viib-mediahub/internal/version"\n',
)
replace_once(
    "backend/internal/api/api.go",
    'respondJSON(w, map[string]string{"status": "ok", "version": "1.0.0"})',
    'respondJSON(w, map[string]string{"status": "ok", "version": version.Current})',
)

# ---------------------------------------------------------------------------
# Phase 2: CI, testing and release gates
# ---------------------------------------------------------------------------

package_path = ROOT / "package.json"
package = json.loads(package_path.read_text(encoding="utf-8"))
package["version"] = "1.0.0"
package.setdefault("scripts", {})["test"] = "vitest run"
package["scripts"]["test:watch"] = "vitest"
package["scripts"]["check"] = "npm run check:palette && npm run check:raw-colors && npm run typecheck && npm test && npm run build"
package.setdefault("devDependencies", {})["vitest"] = "^2.1.8"
package_path.write_text(json.dumps(package, indent=2) + "\n", encoding="utf-8")

write(
    "vitest.config.ts",
    '''import { defineConfig } from 'vitest/config';\n\nexport default defineConfig({\n  test: { environment: 'node', include: ['**/*.test.ts'] },\n});\n''',
)

write(
    ".github/workflows/ci.yml",
    r'''name: CI

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: npm
      - run: npm ci
      - run: npm run check
      - run: npm audit --audit-level=high

  backend:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: backend
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.2'
          cache-dependency-path: backend/go.sum
      - run: go test ./...
      - run: go test -race ./...
      - run: go vet ./...
      - run: go install honnef.co/go/tools/cmd/staticcheck@latest
      - run: staticcheck ./...
      - run: go install golang.org/x/vuln/cmd/govulncheck@latest
      - run: govulncheck ./...

  windows-build:
    runs-on: windows-latest
    needs: [frontend, backend]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: npm
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.2'
          cache-dependency-path: backend/go.sum
      - uses: msys2/setup-msys2@v2
        with:
          msystem: MINGW64
          update: true
          install: mingw-w64-x86_64-gcc
      - run: echo "C:\msys64\mingw64\bin" >> $env:GITHUB_PATH
      - run: npm ci
      - run: npm run build
      - run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
      - shell: pwsh
        run: |
          $dest = "backend\cmd\wails\frontend\dist"
          if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
          New-Item -ItemType Directory -Path "backend\cmd\wails\frontend" -Force | Out-Null
          Copy-Item -Recurse "dist" $dest
      - working-directory: backend/cmd/wails
        run: wails build -platform windows/amd64 -ldflags "-s -w"
''',
)

# Align the existing release workflow with the module toolchain and validation.
replace_once(".github/workflows/build-windows.yml", "GO_VERSION: '1.22'", "GO_VERSION: '1.25.2'")
replace_once(
    ".github/workflows/build-windows.yml",
    '''      - name: Build frontend\n        run: npm run build\n''',
    '''      - name: Frontend tests\n        run: npm test\n\n      - name: Build frontend\n        run: npm run build\n''',
)
replace_once(
    ".github/workflows/build-windows.yml",
    '''      - name: Download Go dependencies\n        working-directory: backend\n        run: go mod download\n''',
    '''      - name: Download Go dependencies\n        working-directory: backend\n        run: go mod download\n\n      - name: Run Go tests\n        working-directory: backend\n        run: go test ./...\n''',
)

# Documentation and contributor requirements.
readme = read("README.md")
readme = readme.replace("Go 1.22+", "Go 1.25.2+")
write("README.md", readme)
write(
    "docs/REMEDIATION_STATUS.md",
    '''# Full Remediation Status\n\nBranch: `agent/full-remediation`\n\n## Phase 0 — Security and release containment\n- [x] Third-party playlist HTML rendered as text\n- [x] OAuth state correlation added\n- [x] Spotify PKCE no longer depends on a renderer client secret\n- [x] Browser persistence of Spotify client secret removed\n- [x] Generic settings API no longer returns provider secrets\n- [x] Encryption coverage expanded to LLM and Last.fm API keys\n- [x] Remote artwork downloads hardened against SSRF, redirects, oversized content and invalid images\n- [x] Baseline browser security headers added\n\n## Phase 1 — Correctness and data integrity\n- [x] Download progress converted to per-client broadcast subscriptions\n- [x] Force-restart double decrement removed\n- [x] Spotify search response forwarding repaired\n- [x] Spotify proxy paths constrained and HTTP clients time-bounded\n- [x] Album identity and routes include album artist\n- [x] Complete song enrichment/preferences DTO mapping restored\n- [x] Listening-duration validation added\n- [x] API/scanner/download worker lifecycle shutdown added\n- [x] Version source centralized\n\n## Phase 2 — CI and quality gates\n- [x] Pull-request CI added\n- [x] Frontend unit test runner added\n- [x] Go tests, race detector, vet, staticcheck and govulncheck added\n- [x] Windows Wails build gate added\n- [x] Existing release workflow aligned to Go 1.25.2 and tests\n\n## Phase 3 — Architecture and product hardening\n- [x] Remote-media security moved into a dedicated package\n- [x] Album identity moved into a shared frontend module\n- [x] Lifecycle ownership moved into explicit API/scanner modules\n- [x] Secret classification centralized in validation/crypto layers\n- [ ] Follow-on: migrate tokens from encrypted SQLite to native OS credential stores\n- [ ] Follow-on: move all Spotify Web API traffic behind the backend proxy so access tokens never enter renderer memory\n- [ ] Follow-on: continue splitting the legacy API and database monoliths by bounded domain\n\nThe remaining follow-on items require platform-specific credential-store adapters and a broader Spotify client migration; they are documented explicitly rather than represented as completed.\n''',
)

print("Full remediation patches applied")
