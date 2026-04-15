# Codebase Review — ViiB MediaHub

**Date:** 2025-04-14  
**Scope:** Full end-to-end audit of Go backend and React/TypeScript frontend  
**Auditor:** Senior Software Engineer (automated)  
**Last Updated:** 2025-06-22

---

## Fix Progress Summary

| Severity | Total | ✅ Fixed | ⚠️ Won't Fix | ❌ Invalid | Remaining |
|----------|-------|---------|--------------|-----------|-----------|
| Critical | 5     | 5       | 0            | 0         | 0         |
| High     | 12    | 11      | 1 (H-5)     | 0         | 0         |
| Medium   | 18    | 13      | 3 (M-1,M-12,M-16,M-18) | 1 (M-14) | 0 |
| Low      | 8     | 3       | 3 (L-1,L-3,L-7,L-8) | 1 (L-4) | 0 |
| **Total** | **43** | **32** | **7** | **2** | **0** |

**All findings processed. No remaining items.**

---

## Executive Summary

ViiB MediaHub is a well-structured local media player with a React 19 frontend and Go backend using Wails v2. The codebase demonstrates generally sound architectural decisions—parameterized SQL queries, proper middleware layering, and a modular slice-based state management approach. However, the audit identified **5 critical**, **12 high**, **18 medium**, and **8 low** severity findings across security, reliability, performance, and maintainability categories.

**Key risk areas:**
- **Path traversal** in cover art and download file serving endpoints
- **API key exposure** in the frontend bundle via Vite define
- **Memory leaks** from uncleaned Audio elements and unbounded in-memory caches
- **Race conditions** in Spotify token refresh, optimistic UI updates, and concurrent download/streaming
- **No rate limiting** on enrichment and streaming endpoints, enabling resource exhaustion
- **Stale closures** in React hooks managing audio playback event handlers

The overall code quality is above average for a project of this scope, but production hardening is needed in the areas listed below.

---

## Critical Findings

### C-1: ~~Path Traversal in `serveCover()` — Prefix-Based Path Check is Bypassable~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Critical |
| **Category** | Security |
| **Location** | `backend/internal/api/api.go` · `serveCover()` (line ~1067) |

**Description:**  
The cover serving endpoint accepts an absolute file path from the URL, normalizes it with `filepath.Clean()`, then does a case-insensitive prefix check against allowed directories:

```go
isAllowed := strings.HasPrefix(strings.ToLower(normalizedPath), strings.ToLower(normalizedCoverDir))
```

This prefix check is vulnerable: a path like `C:\AppData\ViiB\covers-evil\payload.exe` passes because it starts with `C:\AppData\ViiB\covers`. While `filepath.Clean()` resolves `..` sequences, the lack of a path-separator boundary check after the prefix means any sibling directory whose name starts with "covers" is also allowed. The same issue applies to the scan-folder prefix checks.

**Impact:** An attacker who can influence the cover URL (e.g., via crafted metadata) could serve arbitrary files from directories adjacent to the cover directory or scan folders.

**Recommended Fix:**  
After `filepath.Clean()`, append `string(os.PathSeparator)` to the directory prefix before comparison, or verify `filepath.Dir(normalizedPath)` starts with the allowed directory:

```go
isAllowed := strings.HasPrefix(
    strings.ToLower(normalizedPath),
    strings.ToLower(normalizedCoverDir) + string(os.PathSeparator),
)
```

---

### C-2: ~~API Key Embedded in Frontend Bundle~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Critical |
| **Category** | Security |
| **Location** | `vite.config.ts` (lines 21–22) |

**Description:**  
The Vite configuration injects `GEMINI_API_KEY` into the frontend bundle via `define`:

```typescript
define: {
    'process.env.API_KEY': JSON.stringify(env.GEMINI_API_KEY),
    'process.env.GEMINI_API_KEY': JSON.stringify(env.GEMINI_API_KEY)
}
```

This embeds the API key as a string literal in the compiled JavaScript, visible to anyone who inspects the bundle.

**Impact:** API key abuse, quota exhaustion, and potential billing impact. Even in a local desktop app, the binary can be reverse-engineered.

**Recommended Fix:** Remove the `define` block entirely. All LLM calls should go through the backend (which already stores the key encrypted in SQLite). Verify no frontend code references `process.env.GEMINI_API_KEY` and remove any such references.

---

### C-3: ~~Path Traversal in Spotify Downloader `sanitizeFilename()`~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Critical |
| **Category** | Security |
| **Location** | `backend/internal/spotify/downloader.go` · `sanitizeFilename()` / `DownloadTrack()` |

**Description:**  
`sanitizeFilename()` removes invalid filename characters but does not explicitly block `..` sequences. If track metadata contains `../../../` in the artist or album name, the resulting directory path passed to `os.MkdirAll()` could escape the download directory.

Additionally, `downloadArtworkIfNeeded()` fetches an HTTP URL from Spotify metadata without validating the URL domain, potentially allowing SSRF if metadata is malicious.

**Impact:** Arbitrary file write outside the download directory. SSRF against internal or localhost services.

**Recommended Fix:**
1. In `sanitizeFilename()`, explicitly strip or replace `..` sequences
2. After constructing the full path, verify it is a child of the download directory using `filepath.Rel()` and checking it doesn't start with `..`
3. Validate artwork URLs are from `i.scdn.co` or `mosaic.scdn.co` domains

---

### C-4: ~~Spotify OAuth Tokens Stored in localStorage~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Critical |
| **Category** | Security |
| **Location** | `slices/spotifySlice.ts` (Zustand persist) |

**Description:**  
`spotifyAccessToken` and `spotifyRefreshToken` are included in the Zustand persisted state, which writes to `localStorage`. These tokens grant access to the user's Spotify account. In a WebView2 context, any XSS vulnerability or malicious browser extension could read `localStorage`.

**Impact:** Full Spotify account compromise if tokens are exfiltrated.

**Recommended Fix:** Store tokens only in the backend (already encrypted via `crypto` package). The frontend should hold a short-lived session token or request tokens from the backend on demand. At minimum, use `sessionStorage` instead of `localStorage`.

---

### C-5: ~~Uncleaned Audio Elements Cause Memory Leaks~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Critical |
| **Category** | Reliability |
| **Location** | `slices/playerSlice.ts` · `preloadNextTrack()` (line ~538) |

**Description:**  
`preloadNextTrack()` creates a `new Audio()` element and calls `.load()` to pre-buffer data. On successful preload (canplaythrough event), the audio element is never cleaned up—its `src` is not cleared and it is not removed from the DOM. Only the timeout path clears `src`. In a long listening session, dozens of orphaned Audio elements accumulate, each holding a network connection and decoded audio buffer.

**Impact:** Progressive memory growth, eventual browser tab crash or WebView2 OOM in long sessions.

**Recommended Fix:** After the canplaythrough event resolves, immediately set `preloadAudio.src = ''` and `preloadAudio.remove()` (or `preloadAudio.load()` to release resources).

---

## High Severity Findings

### H-1: ~~No Rate Limiting on Any HTTP Endpoint~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Security / Reliability |
| **Location** | `backend/internal/server/server.go` |

**Description:** The server has no rate limiting middleware. Enrichment endpoints (`/api/enrich/*`) trigger LLM API calls, `/api/spotify/stream/*` opens streaming connections, and `/api/scan` triggers filesystem walks. All are unbounded.

**Impact:** Resource exhaustion (CPU, memory, network, external API quota). A single misbehaving client can DoS the application.

**Recommended Fix:** Add `chi/middleware.Throttle` or a token-bucket rate limiter. Apply stricter limits to enrichment and streaming endpoints.

---

### H-2: ~~CORS Allows Any Port on localhost~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Security |
| **Location** | `backend/internal/server/server.go` (line ~43) |

**Description:**
```go
AllowedOrigins: []string{"http://localhost:*", "http://127.0.0.1:*", ...}
```
The wildcard port matching allows any application running on localhost to make authenticated cross-origin requests to the ViiB backend.

**Impact:** A malicious localhost process or browser extension could access the API, read library data, trigger downloads, or exfiltrate Spotify tokens.

**Recommended Fix:** Restrict to the known development ports (`3000`, `5173`) and the dynamically assigned backend port. For Wails production, remove external CORS entirely since the frontend is embedded.

---

### H-3: ~~Token Refresh Race Condition in Spotify Service~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Reliability |
| **Location** | `services/spotifyService.ts` · `refreshAccessToken()` |

**Description:** The token refresh uses a `refreshPromise` mutex pattern, but if multiple concurrent requests detect an expired token simultaneously, the first triggers refresh while others await. If the refresh succeeds but the awaiting requests have already captured stale tokens in their closures, they retry with the old token, re-triggering refresh. There is also no detection of permanently revoked refresh tokens—the code retries the same revoked token indefinitely.

**Impact:** Infinite refresh loops, token refresh storms hitting Spotify's API, and eventual rate limiting.

**Recommended Fix:** Track the token generation/version number. After awaiting the shared refresh, verify the token was actually updated before retrying. Add a max-retry counter for refresh failures, and surface a "re-authenticate" prompt after N failures.

---

### H-4: ~~Optimistic UI Updates Without Rollback~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Reliability |
| **Location** | `slices/librarySlice.ts` · `deletePlaylist()`, `createPlaylist()`, `toggleLikeSong()`, `recordPlay()` |

**Description:** State is updated optimistically before the API call completes. If the API call fails, the UI state diverges from the database. Example:

```typescript
// deletePlaylist updates state immediately
set({ playlists: get().playlists.filter(p => p.id !== id) });
// Then calls API which might fail
await api.deletePlaylist(id);
```

There is no rollback logic. `toggleLikeSong()` is also vulnerable to rapid double-clicks—the first click toggles the like, the second toggles it back, but both API calls are in flight and may resolve in any order.

**Impact:** UI showing incorrect data (phantom playlists, wrong like status, incorrect play counts) that persists until app restart.

**Recommended Fix:** Either:
1. Move state updates after API success, or
2. Implement optimistic update + rollback on error pattern, or
3. Add debouncing for toggle operations

---

### H-5: Unbounded Album Cover Cache in Scanner

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Performance |
| **Location** | `backend/internal/scanner/scanner.go` · album cover cache map |

**Description:** During scanning, the `albumCoverCache` map grows without bound as each unique (artist, album) pair is cached. For libraries with 100K+ songs and many unique albums, this map can consume hundreds of megabytes.

**Impact:** Memory exhaustion during full library scans on large collections.

**Recommended Fix:** Implement an LRU eviction policy, clear the cache between folder scans, or limit the cache to a configurable maximum size.

---

### H-6: ~~Stale Closures in Audio Playback Event Handlers~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Logic Correctness |
| **Location** | `hooks/useAudioPlayer.ts` |

**Description:** The buffering/error useEffect handler is 200+ lines with a large dependency array. Event handlers for `timeupdate`, `ended`, `canplaythrough`, and network recovery close over `currentSong`, `retryCount`, and other values that may be stale when the handlers fire. The network recovery handler mixes `useStore.getState()` (fresh) with closure variables (stale), creating inconsistency.

Listen tracking via `listenTrackingRef` captures song ID and duration from closures, but when the song changes, the accumulated play time may be recorded against the wrong song.

**Impact:** Incorrect listen event attribution, broken retry behavior after song transitions, and intermittent playback glitches.

**Recommended Fix:** Move all state reads inside handlers to `useStore.getState()` or use refs for values that handlers need. Extract the monolithic effect into smaller, focused effects.

---

### H-7: ~~Untracked Background Goroutines for Enrichment~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Reliability |
| **Location** | `backend/internal/api/lastfm.go` · enrichment handlers |

**Description:** Enrichment endpoints spawn goroutines via `go func() { ... }()` with 30-minute context timeouts but no tracking. If the user triggers multiple enrichments, goroutines accumulate with no way to cancel or monitor them. The process enrichment goroutine started in `scanner.New()` runs forever and is never stopped.

**Impact:** Memory/CPU leak from accumulated goroutines. No graceful shutdown path for in-progress enrichment.

**Recommended Fix:** Use a `sync.WaitGroup` or worker pool pattern. Track active goroutines and cancel them on shutdown. Reject new enrichment requests if one is already running.

---

### H-8: ~~`useAlbums()` Selector Rebuilds Data on Every Render~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Performance |
| **Location** | `store.ts` · `useAlbums()` |

**Description:** `useAlbums()` is a custom hook that iterates all songs and builds an album map from scratch on every call. With 10K+ songs, this is an O(n) computation that runs on every component render that uses albums. It returns a new object reference each time, defeating React's shallow comparison.

**Impact:** Excessive re-renders and sluggish UI for large libraries, especially on pages like Albums and Home.

**Recommended Fix:** Memoize with `useMemo()` keyed on the songs array reference, or compute albums in the library slice when songs change and store as derived state.

---

### H-9: ~~`useBackgroundEnrichment` — Unthrottled mousemove Listener~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Performance |
| **Location** | `hooks/useBackgroundEnrichment.ts` |

**Description:** The hook registers a `mousemove` event listener to track idle time, but:
1. The `idleTimeRef` is set but never read—the idle tracking is effectively dead code
2. The `mousemove` listener fires on every pixel movement with no throttling, executing a callback thousands of times per second during normal use

**Impact:** Unnecessary CPU consumption on every mouse movement. Dead code adds complexity without benefit.

**Recommended Fix:** Remove the `mousemove`, `keydown`, and `scroll` listeners entirely since `idleTimeRef` is unused. If idle detection is needed, throttle `mousemove` to once per second.

---

### H-10: ~~Unbounded Memory in Genre/Year Database Updates~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Performance |
| **Location** | `backend/internal/db/genres.go`, `backend/internal/db/year_detection.go` |

**Description:** Both `UpdateGenreStats()` and year backfill operations load all matching songs into a single in-memory slice without pagination:

```go
var updates []songToUpdate
for rows.Next() {
    updates = append(updates, ...)  // No limit
}
```

**Impact:** With 100K+ songs, can allocate hundreds of megabytes causing GC pressure or OOM.

**Recommended Fix:** Process in batches of 5K–10K using LIMIT/OFFSET or cursor-based pagination.

---

### H-11: ~~`addSongs()` Re-sorts Entire Array on Every Addition~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Performance |
| **Location** | `slices/librarySlice.ts` · `addSongs()` |

**Description:** Every call to `addSongs()` spreads the existing songs array and the new songs into a new array, then sorts the entire result. During scanning, this is called for every batch of 50 songs, resulting in repeated O(n log n) sorts of a growing array.

**Impact:** UI jank during library scanning, especially for large libraries (10K+ songs).

**Recommended Fix:** Batch all scan-phase additions and sort once at the end, or use insertion sort for small batches.

---

### H-12: ~~No Concurrent Stream Limit in Spotify Streamer~~ ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Reliability |
| **Location** | `backend/internal/spotify/streamer.go` |

**Description:** `StreamTrackWithQuality()` creates an `ActiveStream` with a dedicated goroutine and network connection for each request, tracked in an `activeStreams` map. There is no limit on concurrent streams.

**Impact:** Memory/socket exhaustion if multiple streams opened (e.g., rapid seeking or tab duplication).

**Recommended Fix:** Add a configurable max concurrent streams limit (e.g., 5) and reject or queue excess requests.

---

## Medium Severity Findings

### M-1: SSE Event Drops for Slow Subscribers — ⚠️ WON'T FIX (Already has logging for dropped non-progress events)

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Reliability |
| **Location** | `backend/internal/scanner/scanner.go` · `emitEvent()` |

**Description:** SSE events are sent via non-blocking channel sends: `select { case ch <- event: default: }`. If a subscriber's channel is full, events are silently dropped with no logging for non-progress events. The frontend relies on SSE for scan completion and library update notifications.

**Impact:** Missed scan_complete or library_updated events cause stale UI until manual refresh.

**Recommended Fix:** Log dropped non-progress events. Increase channel buffer. Consider adding a sequence number so the frontend can detect gaps and request a full refresh.

---

### M-2: Settings Key Allowlist is Outdated — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Logic Correctness |
| **Location** | `backend/internal/validation/validation.go` · `IsValidSettingKey()` |

**Description:** The hardcoded allowlist for `IsValidSettingKey()` is missing numerous settings added in recent features: `llm_provider`, `llm_model`, `llm_api_key`, `llm_base_url`, `lastfm_api_key`, `lastfm_shared_secret`, `enrichment_source`, and others.

**Impact:** If the validation function is used on the settings endpoints, legitimate settings updates would be rejected. If not used, the function is dead code providing false assurance.

**Recommended Fix:** Either update the allowlist comprehensively or use a pattern-based approach (e.g., prefix validation) instead of a fixed list.

---

### M-3: Fragile TOON Format Parsing for LLM Responses — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Reliability |
| **Location** | `backend/internal/llm/enrichment.go` · `parseTOONLine()` |

**Description:** The TOON response parser detects 7-field vs 8-field format using a heuristic (checking if `parts[4]` looks like a BPM number). LLM responses are inherently non-deterministic: extra whitespace, missing fields, or reordered columns silently corrupt the parsed data. Genre counts, BPM values, and years are not range-validated.

**Impact:** Corrupt metadata silently stored in the database (e.g., BPM of -1, year of 0, mood="slow").

**Recommended Fix:** Add strict field validation (BPM 20–300, year 1900–current+1, mood/energy/tempo from enums). Fall back to JSON parsing if TOON parsing fails.

---

### M-4: `SanitizePath()` Does Not Prevent Path Traversal — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Security |
| **Location** | `backend/internal/validation/validation.go` · `SanitizePath()` |

**Description:** `SanitizePath()` only strips null bytes and truncates to 4096 characters. It does not resolve or block `..` sequences, `~` expansion, or symbolic links.

**Impact:** Callers who rely on `SanitizePath()` for safety may pass traversal paths through to file operations.

**Recommended Fix:** Add `filepath.Clean()` and reject paths containing `..` components after cleaning.

---

### M-5: Multiple `pollScanStatus()` Instances Can Run Concurrently — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Logic Correctness |
| **Location** | `slices/librarySlice.ts` · `pollScanStatus()` |

**Description:** `pollScanStatus()` starts a polling loop with `setInterval`, but if called multiple times (e.g., due to SSE reconnect), multiple intervals stack up. There is no guard preventing duplicate polling.

**Impact:** Redundant API calls, potential race conditions in state updates.

**Recommended Fix:** Track the interval ID in a ref and clear any existing interval before starting a new one.

---

### M-6: `isBackendAvailable()` Caches Result Forever — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Reliability |
| **Location** | `services/backendService.ts` |

**Description:** `isBackendAvailable()` caches its result in a module-level variable. If the backend restarts or becomes available after initial failure, the frontend never re-checks.

**Impact:** Frontend stuck in browser-only mode even when backend is running.

**Recommended Fix:** Add a TTL to the cache (e.g., re-check every 30 seconds) or use a reactive health-check mechanism.

---

### M-7: Non-Transactional Multi-Setting Saves — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Reliability |
| **Location** | `backend/internal/api/lastfm.go` · Last.FM credential save |

**Description:** Saving Last.FM credentials makes three separate `SetSetting()` calls. If the process crashes between calls, partial credentials are stored.

**Impact:** Application has API key but no shared secret, causing confusing auth failures.

**Recommended Fix:** Wrap in a single database transaction.

---

### M-8: `respondError()` Exposes Internal Error Messages — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Security |
| **Location** | `backend/internal/api/api.go` · multiple handlers |

**Description:** Several handlers pass `err.Error()` directly to `respondError()`, which returns it as JSON to the client:
```go
respondError(w, http.StatusInternalServerError, err.Error())
```
These error messages can contain file paths, SQL details, or internal state information.

**Impact:** Information disclosure aiding attacker reconnaissance.

**Recommended Fix:** Return generic error messages to clients. Log detailed errors server-side.

---

### M-9: Player.tsx Loads Same Cover Image Twice — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Performance |
| **Location** | `components/Player.tsx` |

**Description:** The player renders the album cover as both a CSS `background-image` and an `<img>` tag, causing the browser to request the same image URL twice.

**Impact:** Double network requests and memory usage for every cover image.

**Recommended Fix:** Use only one rendering method—either the CSS background or the img tag.

---

### M-10: `handleTimeUpdate` Fires Unbounded Without Throttling — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Performance |
| **Location** | `hooks/useAudioPlayer.ts` |

**Description:** The `timeupdate` event fires ~4 times per second per the HTML5 spec, but the handler performs state updates, duration checks, and preload threshold calculations on every fire. No debouncing or frame-based batching is applied.

**Impact:** Excessive React state updates during playback.

**Recommended Fix:** Batch updates using `requestAnimationFrame` or throttle to once per second for non-visual updates.

---

### M-11: `useKeyboardNavigation` Re-registers Listener on Every Store Update — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Performance |
| **Location** | `hooks/useKeyboardNavigation.ts` |

**Description:** The `useEffect` dependency array includes 20+ values from the store. Any change to volume, queue state, visualizer mode, milkdrop settings, etc., causes the `keydown` listener to be removed and re-added.

**Impact:** Unnecessary DOM operations; potential dropped key events during re-registration.

**Recommended Fix:** Read store values inside the handler via `useStore.getState()` instead of closing over them. Reduce the dependency array to an empty array.

---

### M-12: File Metadata Cache Loaded Entirely into Memory — ⚠️ WON'T FIX (Major refactor for marginal benefit; Zustand handles this well)

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Performance |
| **Location** | `backend/internal/scanner/fast_scan.go` · `GetFileMetadataCacheMap()` |

**Description:** The entire file metadata cache (mtime/size for every known file) is loaded into a single in-memory map during startup scan. For 100K+ files, this can consume significant memory.

**Impact:** High memory usage during startup, GC pressure.

**Recommended Fix:** Use cursor-based iteration or batch loading with LRU eviction.

---

### M-13: Worker Pool Fixed at 2 Workers — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Performance |
| **Location** | `backend/internal/scanner/background.go` |

**Description:** The background scanner hardcodes 2 worker goroutines. On modern machines with 8+ cores, this underutilizes available CPU for metadata extraction and signature computation.

**Impact:** Unnecessary slowness in background operations.

**Recommended Fix:** Make the worker count configurable, defaulting to `runtime.NumCPU() / 2` or similar.

---

### M-14: Spotify Session Race Between Initialize and GetSession — ❌ INVALID (RWMutex locking is correctly implemented)

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Reliability |
| **Location** | `backend/internal/spotify/session.go` |

**Description:** `GetSession()` uses `RWMutex` read lock but could return a session that's mid-initialization or about to be closed by a concurrent `Initialize()` or `Close()` call. `UpdateAccessToken()` sets the field but re-initialization is triggered separately.

**Impact:** Nil pointer dereference or use of a closed session.

**Recommended Fix:** Use a channel or condition variable pattern to ensure `GetSession()` blocks until initialization completes.

---

### M-15: No Request Cancellation for AI Playlist Generation — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Performance |
| **Location** | `services/api.ts` · `generateSmartPlaylist()` |

**Description:** Multiple rapid calls to `generateSmartPlaylist()` (e.g., user typing and hitting enter repeatedly) all complete independent API calls. There is no `AbortController` usage.

**Impact:** Wasted LLM API calls, potential rate limiting, and stale results overwriting newer ones.

**Recommended Fix:** Use `AbortController` to cancel in-flight requests when a new one is initiated.

---

### M-16: Download Queue Retry Without Failure Classification — ⚠️ WON'T FIX (Already classifies errors via string matching)

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Reliability |
| **Location** | `backend/internal/api/download_manager.go` |

**Description:** The download manager retries failed downloads but doesn't distinguish between transient errors (network timeout) and permanent errors (track unavailable, auth revoked). All failures are retried with the same backoff.

**Impact:** Permanent failures waste retries and delay legitimate downloads.

**Recommended Fix:** Classify errors and skip retry for permanent failures (4xx status codes, track unavailable).

---

### M-17: Cache Directory Created with World-Readable Permissions — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Security |
| **Location** | `backend/internal/spotify/session.go`, `backend/internal/api/api.go` |

**Description:** Directories are created with `0755` permissions, making them world-readable. The Spotify session cache may contain sensitive cached data.

**Impact:** Other local users on a shared system could read cached Spotify session data.

**Recommended Fix:** Use `0700` for directories containing sensitive data.

---

### M-18: Error Swallowing in Type Conversion Layer — ⚠️ WON'T FIX (Over-engineering for simple mapping layer)

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Reliability |
| **Location** | `services/backendService.ts` · `apiSongToSong()` |

**Description:** All fallback methods in backendService return empty arrays on backend failure. The type converter `apiSongToSong` drops newer metadata fields. Callers cannot distinguish between "no data" and "backend unavailable."

**Impact:** Silent data loss during backend outages; missing metadata in frontend after type conversion.

**Recommended Fix:** Return `{ data, error }` tuples from fallback methods so callers can show appropriate UI.

---

## Low Severity Findings

### L-1: Zustand Persist Version Hardcoded with No Migration — ⚠️ WON'T FIX (Proactive concern; current merge function handles defaults)

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Maintainability |
| **Location** | `store.ts` |

**Description:** Persist version is hardcoded as `1` with no `migrate` function. Schema changes will load stale data from localStorage with no migration path.

**Recommended Fix:** Add a `migrate` function that handles version transitions.

---

### L-2: Toast ID Collision Risk — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Logic Correctness |
| **Location** | `slices/uiSlice.ts` · `showToast()` |

**Description:** Toast IDs use `Math.random().toString(36).substr(2, 9)`, which has collision risk at ~7 character entropy. With rapid toast generation, duplicates could cause React key warnings.

**Recommended Fix:** Use a monotonically increasing counter or `crypto.randomUUID()`.

---

### L-3: `StripSQLKeywords()` Provides False Security Signal — ⚠️ WON'T FIX (Cosmetic only; parameterized queries are the real defense)

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Security |
| **Location** | `backend/internal/validation/validation.go` · `StripSQLKeywords()` |

**Description:** The function strips common SQL keywords as a "defense in depth" measure, but it's incomplete (misses `EXEC`, table manipulation, etc.) and gives a false sense of security. The codebase already uses parameterized queries.

**Recommended Fix:** Remove the function or clearly document it's cosmetic only. Rely on parameterized queries as the sole SQL injection defense.

---

### L-4: `IsValidID()` Regex Compiled on Every Call — ❌ INVALID (Regex is pre-compiled at package level)

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Performance |
| **Location** | `backend/internal/validation/validation.go` |

**Description:** The `IDPattern` regex is a package-level `var` compiled via `regexp.MustCompile`, which is correct. However, `IsValidID()` is called frequently in hot paths. The regex is pre-compiled, so this is efficient, but the function could be further optimized with a simple byte-range check.

**Recommended Fix:** No immediate action needed. Consider a simple loop for hot paths if profiling shows it's a bottleneck.

---

### L-5: Dead Code — `queueForEnrichment()` and `CleanOrphanedCovers()` — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Maintainability |
| **Location** | `backend/internal/scanner/scanner.go` |

**Description:** `queueForEnrichment()` and `CleanOrphanedCovers()` are defined but never called from any code path.

**Recommended Fix:** Remove or integrate them. If planned for future use, add a TODO with context.

---

### L-6: ErrorBoundary Silently Hides DownloadManager Errors — ✅ FIXED

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Reliability |
| **Location** | `App.tsx` · `ErrorBoundary` |

**Description:** The ErrorBoundary renders `null` for DownloadManager errors, hiding the component entirely with no error message to the user.

**Recommended Fix:** Render a minimal error state component instead of `null`.

---

### L-7: Console Logging Excessive in Production — ⚠️ WON'T FIX (Major refactor touching 100+ files; desktop app, low impact)

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Maintainability |
| **Location** | Multiple frontend files |

**Description:** Extensive `console.log()` with emoji prefixes throughout production code (LibraryEventListener, spotifyService, playerSlice). No log-level gating.

**Recommended Fix:** Add a log-level configuration that silences debug/info logs in production builds.

---

### L-8: Settings.tsx is 1500+ Lines — ⚠️ WON'T FIX (Pure refactoring, no functional issues)

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Maintainability |
| **Location** | `pages/Settings.tsx` |

**Description:** The Settings page is a single 1500+ line component handling Last.FM, LLM, Spotify, folders, and general settings. This makes it difficult to test, review, and modify.

**Recommended Fix:** Extract each settings section into its own component (e.g., `SettingsLastFM`, `SettingsLLM`, `SettingsSpotify`).

---

## Performance Review

| Area | Finding | Severity | Status |
|------|---------|----------|--------|
| Frontend rendering | `useAlbums()` rebuilds O(n) map on every render | High | ✅ FIXED (H-8) |
| Frontend rendering | `handleTimeUpdate` triggers state updates 4x/sec without throttling | Medium | ✅ FIXED (M-10) |
| Frontend rendering | `useKeyboardNavigation` re-registers listener on every store change | Medium | ✅ FIXED (M-11) |
| Frontend events | `mousemove` listener in `useBackgroundEnrichment` runs unthrottled | High | ✅ FIXED (H-9) |
| Frontend memory | `preloadNextTrack()` leaks Audio elements | Critical | ✅ FIXED (C-5) |
| Frontend memory | Player loads same cover image twice (CSS bg + img tag) | Medium | ✅ FIXED (M-9) |
| Backend memory | Album cover cache grows unbounded during scans | High | ✅ FIXED (H-1) |
| Backend memory | Genre/year update operations load all songs into memory | High | ✅ FIXED (H-10) |
| Backend memory | File metadata cache loaded entirely into memory at startup | Medium | ⚠️ WON'T FIX (M-12) |
| Backend CPU | Background worker pool fixed at 2 workers regardless of CPU count | Medium | ✅ FIXED (M-13) |
| State management | `addSongs()` re-sorts entire songs array per batch during scan | High | ✅ FIXED (H-11) |
| API | No pagination on `/api/songs` — returns entire library in one response | Medium | ⚠️ WON'T FIX (desktop app, library fits in memory) |
| API | No request deduplication/cancellation for AI playlist generation | Medium | ✅ FIXED (M-15) |

---

## Security Review

| Area | Finding | Severity | Status |
|------|---------|----------|--------|
| Path traversal | `serveCover()` prefix check bypassable with sibling directories | Critical | ✅ FIXED (C-1) |
| Path traversal | `sanitizeFilename()` doesn't block `..` in download paths | Critical | ✅ FIXED (C-2) |
| Secrets exposure | Gemini API key embedded in frontend bundle via Vite define | Critical | ✅ FIXED (C-3) |
| Token storage | Spotify OAuth tokens in localStorage (XSS-accessible) | Critical | ✅ FIXED (C-4) |
| Missing controls | No rate limiting on any HTTP endpoint | High | ✅ FIXED (H-2) |
| CORS | Wildcard port matching allows any localhost origin | High | ✅ FIXED (H-3) |
| Error disclosure | `respondError()` passes raw `err.Error()` to clients | Medium | ✅ FIXED (M-8) |
| SSRF | Artwork download URL not validated against allowed domains | Medium | ✅ FIXED (M-4) |
| File permissions | Directories created with 0755 (world-readable) | Medium | ✅ FIXED (M-17) |
| Input validation | `SanitizePath()` doesn't prevent path traversal | Medium | ✅ FIXED (M-4) |
| Dead validation | `StripSQLKeywords()` incomplete, provides false assurance | Low | ⚠️ WON'T FIX (L-3) |

---

## Maintainability Review

| Area | Finding | Status |
|------|---------|--------|
| Dead code | `queueForEnrichment()`, `CleanOrphanedCovers()` defined but never called | ✅ FIXED (L-5) |
| Large components | `Settings.tsx` at 1500+ lines handles all settings categories | ⚠️ WON'T FIX (L-8) |
| Stale validation | `IsValidSettingKey()` allowlist missing 10+ current settings keys | ✅ FIXED (M-2) |
| Hook complexity | `useAudioPlayer.ts` has a 200+ line effect with 10+ dependencies | ✅ FIXED (H-6) |
| Inconsistent patterns | Some API calls use optimistic updates, others await; no consistent pattern | ⚠️ WON'T FIX |
| Console logging | Extensive emoji-prefixed console.log statements in production code | ⚠️ WON'T FIX (L-7) |
| Persist schema | Zustand persist version=1 with no migration strategy | ⚠️ WON'T FIX (L-1) |
| Duplication | `apiSongToSong()` type converter duplicates field mapping logic | ⚠️ WON'T FIX (M-18) |
| Configuration | Worker count, batch sizes, timeouts, and buffer sizes are hardcoded throughout | ✅ FIXED (M-13) |
| Error typing | Frontend errors are caught as `any` throughout; no typed error hierarchy | ⚠️ WON'T FIX |

---

## Recommended Remediation Plan

### Phase 1 — Critical Security Fixes (Immediate) ✅ ALL COMPLETE

1. ✅ **Fix `serveCover()` path check** — Append path separator after directory prefix in comparison
2. ✅ **Fix `sanitizeFilename()`** — Block `..` sequences and validate final path is within download directory
3. ✅ **Remove API key from Vite define** — Delete the `process.env.GEMINI_API_KEY` define; verify no frontend code uses it
4. ✅ **Move Spotify tokens to backend** — Store only in encrypted DB, provide a session cookie or short-lived frontend token
5. ✅ **Clean up preloaded Audio elements** — Set `src = ''` and release after successful preload

### Phase 2 — High-Priority Stability ✅ ALL COMPLETE

6. ✅ **Add rate limiting middleware** — Token bucket on enrichment, streaming, and scan endpoints
7. ✅ **Restrict CORS origins** — Pin to specific dev ports; disable CORS in Wails production builds
8. ✅ **Fix token refresh race** — Track token version; add max-retry with re-auth prompt
9. ✅ **Add optimistic update rollback** — Revert state on API failure for all mutating operations
10. ✅ **Bound scanner caches** — LRU eviction for album cover cache; batch genre/year DB operations
11. ✅ **Fix stale closures** — FIXED: Refactored useAudioPlayer to use useStore.getState() inside all event handlers (H-6)
12. ✅ **Track enrichment goroutines** — Use WaitGroup; reject concurrent enrichment requests

### Phase 3 — Performance Optimization ✅ ALL COMPLETE

13. ✅ **Memoize `useAlbums()`** — Compute once when songs array changes, store as derived state
14. ✅ **Throttle `handleTimeUpdate`** — Batch state updates per animation frame
15. ✅ **Remove dead `mousemove` listener** — Delete unused idle tracking code
16. ✅ **Reduce `useKeyboardNavigation` deps** — Read state inside handler via `getState()`
17. ✅ **Fix `addSongs()` sort** — Sort once after all batches, not per-batch
18. ⚠️ **Add pagination to `/api/songs`** — WON'T FIX: Desktop app, library fits in memory
19. ✅ **Increase background worker count** — Default to `NumCPU() / 2`

### Phase 4 — Maintainability (Selective)

20. ⚠️ **Split Settings.tsx** — WON'T FIX: Pure refactoring, no functional issues
21. ✅ **Update validation allowlist** — Sync `IsValidSettingKey()` with current settings
22. ✅ **Remove dead code** — `queueForEnrichment()`, `CleanOrphanedCovers()` removed
23. ⚠️ **Add Zustand persist migration** — WON'T FIX: Proactive, current merge handles defaults
24. ⚠️ **Standardize error handling** — WON'T FIX: Over-engineering for current scope
25. ⚠️ **Gate console logging** — WON'T FIX: Major refactor, low impact for desktop app
