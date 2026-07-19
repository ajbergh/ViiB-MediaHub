#!/usr/bin/env python3
"""Apply final code-review corrections before PR handoff."""
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]


def rewrite(path: str, transform) -> None:
    target = ROOT / path
    before = target.read_text(encoding="utf-8")
    after = transform(before)
    if after != before:
        target.write_text(after, encoding="utf-8")


def fix_scanner(text: str) -> str:
    old_delete = '''\t\t\tpathLower := strings.ToLower(filepath.Clean(existingPath))
\t\t\tisInScanFolder := false
\t\t\tfor folderPath := range deletionSafeFolderPaths {
\t\t\t\tif strings.HasPrefix(pathLower, folderPath) {
\t\t\t\t\tisInScanFolder = true
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}
'''
    new_delete = '''\t\t\tisInScanFolder := false
\t\t\tfor folderPath := range deletionSafeFolderPaths {
\t\t\t\tif isSubPath(folderPath, existingPath) {
\t\t\t\t\tisInScanFolder = true
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}
'''
    if old_delete in text:
        text = text.replace(old_delete, new_delete, 1)

    old_batch_error = '''\t\t\tif err != nil {
\t\t\t\tlogger.Scanner("ERROR saving batch to database: %v", err)
\t\t\t\t// Continue scanning even if save fails
\t\t\t} else {
'''
    new_batch_error = '''\t\t\tif err != nil {
\t\t\t\tlogger.Scanner("ERROR saving batch to database: %v", err)
\t\t\t\tresult.Errors++
\t\t\t\t// Continue discovery, but make the root ineligible for deletion reconciliation.
\t\t\t} else {
'''
    if old_batch_error in text:
        text = text.replace(old_batch_error, new_batch_error, 1)

    text = text.replace('Message:      fmt.Sprintf("Added %d songs", len(songs)),',
                        'Message:      fmt.Sprintf("Library updated: %d added, %d updated", upsert.Inserted, upsert.Updated),')
    text = text.replace('id := proposedSongID(fingerprint)', 'id := proposedSongID(fingerprint, filePath)', 1)
    return text


def fix_scanner_identity(text: str) -> str:
    if '"path/filepath"' not in text:
        text = text.replace('\t"os"\n', '\t"os"\n\t"path/filepath"\n')
    old = '''func proposedSongID(fingerprint string) string {
\tif len(fingerprint) >= 16 {
\t\treturn fingerprint[:16]
\t}
\treturn fingerprint
}
'''
    new = '''// proposedSongID is path-specific so identical files in two live locations
// remain distinct library entries. Move reconciliation reuses the previous ID
// only when the previous path is confirmed absent.
func proposedSongID(fingerprint, filePath string) string {
\thash := sha256.Sum256([]byte(fingerprint + "\\x00" + filepath.Clean(filePath)))
\treturn hex.EncodeToString(hash[:8])
}
'''
    return text.replace(old, new, 1)


def fix_db_identity(text: str) -> str:
    text = text.replace('import "database/sql"', 'import (\n\t"database/sql"\n\t"errors"\n\t"os"\n)')
    old = '''\tif fingerprint != "" {
\t\terr = d.conn.QueryRow(`SELECT id FROM songs WHERE file_hash = ? LIMIT 1`, fingerprint).Scan(&id)
\t\tif err == nil {
\t\t\treturn id, nil
\t\t}
\t\tif err != sql.ErrNoRows {
\t\t\treturn "", err
\t\t}
\t}
'''
    new = '''\tif fingerprint != "" {
\t\tvar previousPath string
\t\terr = d.conn.QueryRow(`SELECT id, file_path FROM songs WHERE file_hash = ? LIMIT 1`, fingerprint).Scan(&id, &previousPath)
\t\tif err == nil {
\t\t\t// Reuse identity only for a confirmed move. If the old path still exists,
\t\t\t// this is a duplicate copy and must receive its own logical ID.
\t\t\tif _, statErr := os.Stat(previousPath); errors.Is(statErr, os.ErrNotExist) {
\t\t\t\treturn id, nil
\t\t\t}
\t\t\treturn proposedID, nil
\t\t}
\t\tif err != sql.ErrNoRows {
\t\t\treturn "", err
\t\t}
\t}
'''
    return text.replace(old, new, 1)


def fix_scanner_tests(text: str) -> str:
    addition = '''
func TestProposedSongIDKeepsLiveDuplicatesDistinct(t *testing.T) {
\tfingerprint := "0123456789abcdef0123456789abcdef"
\tfirst := proposedSongID(fingerprint, filepath.Join("library-a", "song.flac"))
\tsecond := proposedSongID(fingerprint, filepath.Join("library-b", "song.flac"))
\tif first == second {
\t\tt.Fatal("identical content at two paths must not collapse to one song ID")
\t}
\tif first != proposedSongID(fingerprint, filepath.Join("library-a", "song.flac")) {
\t\tt.Fatal("proposed ID must be deterministic")
\t}
}
'''
    if "TestProposedSongIDKeepsLiveDuplicatesDistinct" not in text:
        text += addition
    return text


def fix_audio_player_hook(text: str) -> str:
    if "import { PlaybackContext } from '../types';" not in text:
        text = text.replace(
            "import { StreamingErrorType } from '../slices/types';\n",
            "import { StreamingErrorType } from '../slices/types';\nimport { PlaybackContext } from '../types';\n",
        )

    text = text.replace(
        '''        accumulatedPlayTime: number;
        lastPlayStartTime: number | null;
        lastMediaTime: number;
        isTracking: boolean;
''',
        '''        accumulatedPlayTime: number;
        lastMediaTime: number;
        context: PlaybackContext;
        isTracking: boolean;
''',
    )

    text = re.sub(
        r'''\s+let finalPlayTime = listenTrackingRef\.current\.accumulatedPlayTime;\n\s+if \(listenTrackingRef\.current\.lastPlayStartTime !== null\) \{\n\s+finalPlayTime \+= \(Date\.now\(\) - listenTrackingRef\.current\.lastPlayStartTime\) / 1000;\n\s+\}''',
        '\n                const finalPlayTime = listenTrackingRef.current.accumulatedPlayTime;',
        text,
    )
    text = text.replace("currentSong.playbackContext || 'queue'", "listenTrackingRef.current.context")
    text = text.replace(
        '''                accumulatedPlayTime: 0,
                lastPlayStartTime: isPlaying ? Date.now() : null,
                lastMediaTime: 0,
                isTracking: true
''',
        '''                accumulatedPlayTime: 0,
                lastMediaTime: 0,
                context: currentSong.playbackContext || 'queue',
                isTracking: true
''',
    )
    text = re.sub(
        r'''\s+// Resume listen tracking\n\s+if \(listenTrackingRef\.current && listenTrackingRef\.current\.songId === currentSong\.id\) \{\n\s+listenTrackingRef\.current\.lastPlayStartTime = Date\.now\(\);\n\s+\}''',
        '',
        text,
    )
    text = re.sub(
        r'''\s+// Pause listen tracking - accumulate time played so far\n\s+if \(listenTrackingRef\.current\) listenTrackingRef\.current\.lastPlayStartTime = null;''',
        '',
        text,
    )

    text = text.replace(
        '''            if (!element || !audioSettings.mainOutputDevice) return;
            const sinkCapable = element as HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> };
            if (typeof sinkCapable.setSinkId !== 'function') return;
            try {
                await sinkCapable.setSinkId(audioSettings.mainOutputDevice);
''',
        '''            if (!element) return;
            const sinkCapable = element as HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> };
            if (typeof sinkCapable.setSinkId !== 'function') return;
            try {
                await sinkCapable.setSinkId(audioSettings.mainOutputDevice || '');
''',
    )

    text = text.replace(
        '''            nextPlayer.src = currentSong.url;
            nextPlayer.load();
''',
        '''            if (nextPlayer.getAttribute('src') !== currentSong.url) {
                nextPlayer.src = currentSong.url;
                nextPlayer.load();
            }
''',
    )

    preload_marker = '''            // Trigger pre-buffering of next track when approaching end of current track
            if (dur > 0 && (dur - time) <= PRELOAD_THRESHOLD_SECONDS) {
'''
    if "Gapless preload uses the inactive player" not in text and preload_marker in text:
        preload = '''            // Gapless preload uses the inactive player that will perform the handoff,
            // rather than a disposable Audio element whose buffer cannot be reused.
            const state = useStore.getState();
            if (state.audioSettings.gapless && dur > 0 && (dur - time) <= PRELOAD_THRESHOLD_SECONDS) {
                const nextTrack = state.queue[state.currentSongIndex + 1];
                const inactivePlayer = activePlayerIndex.current === 0 ? secondaryRef.current : primaryRef.current;
                if (nextTrack?.url && inactivePlayer && inactivePlayer.getAttribute('src') !== nextTrack.url) {
                    inactivePlayer.preload = 'auto';
                    inactivePlayer.src = nextTrack.url;
                    inactivePlayer.load();
                }
            }

'''
        text = text.replace(preload_marker, preload + preload_marker, 1)
    return text


def fix_player_slice(text: str) -> str:
    old = '''        const previousUrl = get().currentSong?.url;
        if (previousUrl && previousUrl !== playableSong.url) managedObjectUrls.release(previousUrl);
'''
    new = '''        const previousUrl = get().currentSong?.url;
        if (previousUrl && previousUrl !== playableSong.url) {
            const settings = get().audioSettings;
            const fadeSeconds = settings.gapless ? 0 : Math.max(0, settings.crossfadeDuration || 0);
            // Keep the outgoing Blob alive until the audio engine has paused the old element.
            setTimeout(() => managedObjectUrls.release(previousUrl), fadeSeconds * 1000 + 1000);
        }
'''
    return text.replace(old, new, 1)


def fix_spotify_service(text: str) -> str:
    old = '''                    if (response.ok) {
                        const data = await response.json();
                        setSpotifyTokens(
                            data.access_token,
                            data.refresh_token || spotifyRefreshToken,
                            Date.now() + (data.expires_in * 1000)
                        );
                        refreshFailureCount = 0; // Reset on success
                        return data.access_token;
'''
    new = '''                    if (response.ok) {
                        const data = await response.json();
                        const nextRefreshToken = data.refresh_token || spotifyRefreshToken;
                        const nextExpiry = Date.now() + (data.expires_in * 1000);
                        setSpotifyTokens(data.access_token, nextRefreshToken, nextExpiry);
                        try {
                            const { api } = await import('./api');
                            await api.saveSpotifyCredentials({
                                clientId: spotifyClientId,
                                clientSecret: '',
                                accessToken: data.access_token,
                                refreshToken: nextRefreshToken,
                                expiry: nextExpiry,
                            });
                        } catch (syncError) {
                            store.addLog('warn', 'Spotify token refreshed locally but backend synchronization failed', syncError);
                        }
                        refreshFailureCount = 0; // Reset on success
                        return data.access_token;
'''
    return text.replace(old, new, 1)


def fix_spotify_handlers(text: str) -> str:
    search_old = '''\trequest, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.spotify.com/v1/search?"+params.Encode(), nil)
\tif err != nil {
\t\trespondError(w, http.StatusInternalServerError, "Failed to create request")
\t\treturn
\t}
\trequest.Header.Set("Authorization", "Bearer "+accessToken)
\tresp, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
'''
    search_new = '''\tresp, err := a.doSpotifyRequest(r.Context(), http.MethodGet, "https://api.spotify.com/v1/search?"+params.Encode(), nil, "")
'''
    if search_old in text:
        text = text.replace(search_old, search_new, 1)

    profile_old = '''\treq, err := http.NewRequestWithContext(r.Context(), "GET", "https://api.spotify.com/v1/me", nil)
\tif err != nil {
\t\trespondError(w, http.StatusInternalServerError, "Failed to create request")
\t\treturn
\t}

\treq.Header.Set("Authorization", "Bearer "+accessToken)

\tclient := &http.Client{Timeout: 30 * time.Second}
\tresp, err := client.Do(req)
'''
    profile_new = '''\tresp, err := a.doSpotifyRequest(r.Context(), http.MethodGet, "https://api.spotify.com/v1/me", nil, "")
'''
    if profile_old in text:
        text = text.replace(profile_old, profile_new, 1)

    proxy_old = '''\treq, err := http.NewRequest(r.Method, spotifyURL, r.Body)
\tif err != nil {
\t\trespondError(w, http.StatusInternalServerError, "Failed to create request")
\t\treturn
\t}

\treq.Header.Set("Authorization", "Bearer "+accessToken)
\treq.Header.Set("Content-Type", "application/json")

\tclient := &http.Client{Timeout: 30 * time.Second}
\tresp, err := client.Do(req)
'''
    proxy_new = '''\trequestBody, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
\tif err != nil {
\t\trespondError(w, http.StatusBadRequest, "Failed to read request body")
\t\treturn
\t}
\tresp, err := a.doSpotifyRequest(r.Context(), r.Method, spotifyURL, requestBody, "application/json")
'''
    if proxy_old in text:
        text = text.replace(proxy_old, proxy_new, 1)

    # Access token variables are no longer needed after routing through the retry helper.
    text = text.replace('''\taccessToken, err := a.validSpotifyAccessToken(r.Context())
\tif err != nil {
\t\trespondError(w, http.StatusUnauthorized, err.Error())
\t\treturn
\t}
\tquery :=''', '\tquery :=', 1)
    text = text.replace('''\taccessToken, err := a.validSpotifyAccessToken(r.Context())
\tif err != nil {
\t\trespondError(w, http.StatusUnauthorized, err.Error())
\t\treturn
\t}
\tresp, err := a.doSpotifyRequest''', '\tresp, err := a.doSpotifyRequest', 1)
    text = text.replace('''\taccessToken, err := a.validSpotifyAccessToken(r.Context())
\tif err != nil {
\t\trespondError(w, http.StatusUnauthorized, err.Error())
\t\treturn
\t}

\t// Get the path after /api/spotify/proxy/''', '\t// Get the path after /api/spotify/proxy/', 1)
    return text


def fix_spotify_tests(text: str) -> str:
    if "TestSpotifyRequestRetriesOnceAfterUnauthorized" in text:
        return text
    addition = r'''

func TestSpotifyRequestRetriesOnceAfterUnauthorized(t *testing.T) {
    var resourceCalls int
    resource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        resourceCalls++
        if r.Header.Get("Authorization") == "Bearer old-access-token" {
            w.WriteHeader(http.StatusUnauthorized)
            return
        }
        if r.Header.Get("Authorization") != "Bearer refreshed-access-token" {
            t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
        }
        w.WriteHeader(http.StatusOK)
    }))
    defer resource.Close()

    tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _ = json.NewEncoder(w).Encode(map[string]any{
            "access_token": "refreshed-access-token",
            "expires_in":   3600,
        })
    }))
    defer tokenServer.Close()

    previousEndpoint := spotifyTokenEndpoint
    spotifyTokenEndpoint = tokenServer.URL
    defer func() { spotifyTokenEndpoint = previousEndpoint }()

    database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
    if err != nil {
        t.Fatal(err)
    }
    defer database.Close()
    credentials := SpotifyCredentials{
        ClientId:     "client-id",
        AccessToken:  "old-access-token",
        RefreshToken: "refresh-token",
        Expiry:       time.Now().Add(time.Hour).UnixMilli(),
    }
    raw, _ := json.Marshal(credentials)
    if err := database.SetSetting("spotify_credentials", string(raw)); err != nil {
        t.Fatal(err)
    }

    api := &API{db: database}
    response, err := api.doSpotifyRequest(context.Background(), http.MethodGet, resource.URL, nil, "")
    if err != nil {
        t.Fatal(err)
    }
    response.Body.Close()
    if response.StatusCode != http.StatusOK || resourceCalls != 2 {
        t.Fatalf("expected one retry and 200, got calls=%d status=%d", resourceCalls, response.StatusCode)
    }
}
'''
    return text + addition


rewrite("backend/internal/scanner/scanner.go", fix_scanner)
rewrite("backend/internal/scanner/identity.go", fix_scanner_identity)
rewrite("backend/internal/db/identity.go", fix_db_identity)
rewrite("backend/internal/scanner/identity_test.go", fix_scanner_tests)
rewrite("hooks/useAudioPlayer.ts", fix_audio_player_hook)
rewrite("slices/playerSlice.ts", fix_player_slice)
rewrite("services/spotifyService.ts", fix_spotify_service)
rewrite("backend/internal/api/spotify.go", fix_spotify_handlers)
rewrite("backend/internal/api/spotify_token_test.go", fix_spotify_tests)
print("Final review fixes applied")
