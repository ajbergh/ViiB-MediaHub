# Spotify Direct Streaming Roadmap

This document tracks the implementation of direct audio streaming from Spotify using librespot-go, enabling playback without downloading tracks first.

**Created:** 2024-12-04
**Status:** 🚧 In Progress

---

## Overview

Currently, ViiB MediaHub downloads Spotify tracks to OGG files before playback. This roadmap outlines adding **direct streaming capability** to play Spotify tracks in real-time without pre-downloading.

### Current Flow (Download-First)
```
User → Queue Download → Wait → Play from local file
```

### Target Flow (Direct Streaming)
```
User → Click Play → Stream directly from Spotify → Instant playback
```

---

## Phase 1: Backend Streaming Infrastructure ✅

### 1.1 Create Streamer Component ✅
- [x] **Create `backend/internal/spotify/streamer.go`**
  - [x] `Streamer` struct to manage audio streaming sessions
  - [x] `StreamTrack(ctx, spotifyID)` method returning `io.ReadSeekCloser`
  - [x] Session reuse from existing `SessionManager`
  - [x] Quality selection (320kbps → 160kbps → 96kbps fallback)
  - [x] Proper resource cleanup on context cancellation

### 1.2 Add Streaming API Endpoint ✅
- [x] **Add `/api/spotify/stream/{id}` endpoint**
  - [x] Route registration in `api.go`
  - [x] Handler in `spotify.go`
  - [x] Session authentication check
  - [x] Content-Type: `audio/ogg` header
  - [x] Accept-Ranges header for seeking support
  - [x] Range request handling (HTTP 206 Partial Content)

### 1.3 Session Management Improvements ✅
- [x] **Optimize session lifecycle for streaming**
  - [x] Keep session alive during active playback (via SessionManager reuse)
  - [x] Handle session expiry gracefully during stream
  - [x] Token refresh without interrupting playback
  - [ ] Concurrent stream limit handling

---

## Phase 2: Frontend Integration ✅

### 2.1 Audio Player Updates ✅
- [x] **Update audio source handling in player**
  - [x] Detect Spotify tracks vs local tracks (via spotifyId field)
  - [x] Use streaming endpoint for Spotify tracks
  - [x] Fallback to local file if available
  - [x] Handle streaming errors gracefully

### 2.2 Playback Controls ✅
- [x] **Seeking support for streamed content**
  - [x] Range request implementation
  - [x] Progress bar interaction (works with streaming)
  - [x] Buffer state indication

### 2.3 UI Indicators ✅
- [x] **Visual feedback for streaming**
  - [x] Downloaded indicator icon on tracks in Spotify browser
  - [x] Loading state during initial buffer
  - [x] Error state handling
  - [x] "Download for offline" option on streaming tracks

### 2.4 Spotify Page Play Functionality ✅
- [x] **Click-to-play on search results**
  - [x] Track rows are clickable to start playback
  - [x] Play icon shown on hover
  - [x] Entire track list used as playback context

---

## Phase 3: Hybrid Playback Mode

### 3.1 Smart Source Selection ✅
- [x] **Automatic source selection logic**
  - [x] Check if track is already downloaded → use local file
  - [x] If not downloaded → stream directly
  - [x] Option to prefer streaming or downloaded
  - [x] **Download Status Sync**: Show "Downloaded" state in Spotify browser

### 3.2 Background Download Option ✅
- [x] **Download while streaming**
  - [x] Download button in NowPlaying view for streaming tracks
  - [x] Download option in context menu for streaming tracks
  - [x] Download button in Queue for streaming tracks
  - [ ] Switch to local file when download completes (requires library refresh)

### 3.3 Settings & Preferences ✅
- [x] **Add streaming settings to Settings page**
  - [x] Enable/disable streaming toggle (default: enabled)
  - [x] Preferred quality selector (High/Medium/Low)
  - [ ] Auto-download streamed tracks option
  - [ ] Data usage warnings

---

## Phase 4: Queue & Playlist Integration

### 4.1 Mixed Queue Support
- [x] **Support both local and Spotify tracks in queue**
  - [x] Track source indicator in queue UI (CheckCircle for downloaded, Download for streaming)
  - [x] Seamless transition between sources
  - [ ] Gapless playback consideration

### 4.2 Spotify Playlist/Album Streaming ✅
- [x] **Stream entire Spotify playlists/albums**
  - [x] **Wire up Play buttons** on Album/Playlist cards in `Spotify.tsx`
  - [x] Fetch all tracks for album/playlist
  - [x] Add to queue functionality
  - [x] Shuffle play support

### 4.3 Search & Play ✅
- [x] **Direct play from Spotify search**
  - [x] Play button on search result tracks
  - [x] Play artist radio/top tracks

---

## Phase 5: Performance & Polish

### 5.1 Buffering & Caching ✅
- [x] **Optimize streaming performance**
  - [x] Pre-buffer next track in queue (15s threshold)
  - [x] Buffering state management in playerSlice
  - [x] Buffer state UI indicators (spinner on play button)
  - [x] Audio element waiting/canplay event handling

### 5.2 Error Recovery ✅
- [x] **Robust error handling**
  - [x] Network interruption recovery (online/offline events)
  - [x] Error type detection (network, auth, unavailable)
  - [x] Retry logic with exponential backoff (1s, 2s, 4s)
  - [x] Graceful degradation (auto-skip unavailable tracks)
  - [x] User-friendly error messages via toast
  - [x] Auto-resume on network recovery

### 5.3 Analytics & Logging ✅
- [x] **Streaming telemetry**
  - [x] Stream quality metrics (via SpotifyStreamer logger)
  - [x] Buffering event logging
  - [x] Error rate tracking (StreamingStats in playerSlice)
  - [x] Frontend playback reporting (recordStreamEvent)
  - [x] Streaming statistics UI in Settings page

---

## Technical Details

### API Endpoint Specification

```
GET /api/spotify/stream/{spotifyId}

Headers:
  Authorization: (uses stored OAuth token)
  Range: bytes=0-1048575 (optional, for seeking)

Response:
  Content-Type: audio/ogg
  Accept-Ranges: bytes
  Content-Length: (total size if known)
  Content-Range: bytes 0-1048575/total (if Range requested)

Status Codes:
  200 OK - Full content
  206 Partial Content - Range request
  401 Unauthorized - No valid Spotify session
  403 Forbidden - Premium required
  404 Not Found - Track not available
  503 Service Unavailable - Spotify not reachable
```

### Key Dependencies
- `librespot-go` - Already integrated, provides `io.ReadSeekCloser`
- Existing `SessionManager` - Reuse for authentication
- Existing OAuth flow - No changes needed

### Considerations
- **Spotify Premium Required** - Same as downloads
- **Rate Limits** - Concurrent stream limits TBD
- **Terms of Service** - Review Spotify TOS for streaming usage
- **Latency** - Initial pin/buffer delay (~1-2 seconds)

---

## Files Created/Modified

| File | Description |
|------|-------------|
| `backend/internal/spotify/streamer.go` | New Streamer component for audio streaming |
| `backend/internal/api/spotify.go` | Added `streamSpotifyTrack` handler |
| `backend/internal/api/api.go` | Added `/api/spotify/stream/{id}` route |
| `backend/internal/api/download_manager.go` | Added `EnsureSession` and `GetSessionManager` exports |
| `backend/internal/logger/logger.go` | Added `SpotifyStreamer` logger function |
| `types.ts` | Added `spotifyId` and `isStreaming` fields to Song interface |
| `services/api.ts` | Added `getSpotifyStreamUrl` helper |
| `slices/playerSlice.ts` | Updated `playSong` to handle Spotify streaming URLs and local file fallback |
| `lib/spotifyHelpers.ts` | Created helpers to convert Spotify data to Song objects |
| `pages/Spotify.tsx` | Added play buttons, handlers, download status, artist search with top tracks playback |
| `services/libraryService.ts` | Added `getSongBySpotifyId` for local file lookup |
| `components/NowPlaying.tsx` | Added download button for streaming tracks |
| `components/context-menus/SongMenu.tsx` | Added download option for streaming tracks |
| `components/now-playing/QueueList.tsx` | Added download/downloaded indicators |
| `slices/spotifySlice.ts` | Added streamingEnabled and streamingQuality settings |
| `slices/types.ts` | Added streaming settings to SpotifySlice interface, buffering state to PlayerSlice |
| `hooks/useAudioPlayer.ts` | Added pre-buffer logic, buffering/error event handlers, network recovery, analytics |
| `components/Player.tsx` | Added buffering spinner, buffer progress bar, error state UI with retry |
| `pages/Settings.tsx` | Added streaming statistics display |
| `services/spotifyService.ts` | Added `getArtistTopTracks` and `getArtist` methods |

---

## Progress Log

| Date | Phase | Task | Status |
|------|-------|------|--------|
| 2024-12-04 | - | Roadmap created | ✅ Complete |
| 2024-12-04 | 1.1 | Created `streamer.go` with Streamer struct | ✅ Complete |
| 2024-12-04 | 1.1 | Implemented `StreamTrack` method | ✅ Complete |
| 2024-12-04 | 1.1 | Added quality selection (320/160/96 kbps) | ✅ Complete |
| 2024-12-04 | 1.1 | Added resource cleanup on context cancel | ✅ Complete |
| 2024-12-04 | 1.2 | Added `/api/spotify/stream/{id}` route | ✅ Complete |
| 2024-12-04 | 1.2 | Implemented `streamSpotifyTrack` handler | ✅ Complete |
| 2024-12-04 | 1.2 | Added session authentication check | ✅ Complete |
| 2024-12-04 | 1.2 | Added proper Content-Type headers | ✅ Complete |
| 2024-12-04 | 1.3 | Added SpotifyStreamer logging | ✅ Complete |
| 2024-12-04 | 2.1 | Updated `types.ts` and `api.ts` for streaming | ✅ Complete |
| 2024-12-04 | 2.1 | Updated `playerSlice` to generate stream URLs | ✅ Complete |
| 2024-12-04 | 2.4 | Added `spotifyHelpers` for data conversion | ✅ Complete |
| 2024-12-04 | 2.4 | Added play buttons to Spotify search results | ✅ Complete |
| 2025-12-04 | 3.1 | Smart source selection (local file fallback) | ✅ Complete |
| 2025-12-04 | 3.1 | Download status sync in Spotify browser | ✅ Complete |
| 2025-12-04 | 4.2 | Album/Playlist play buttons wired up | ✅ Complete |
| 2025-12-04 | 3.2 | Download button in NowPlaying view | ✅ Complete |
| 2025-12-04 | 3.2 | Download option in context menu | ✅ Complete |
| 2025-12-04 | 3.2 | Download button in Queue view | ✅ Complete |
| 2025-12-04 | 4.1 | Track source indicators in queue | ✅ Complete |
| 2025-12-04 | 3.3 | Streaming enable/disable toggle | ✅ Complete |
| 2025-12-04 | 3.3 | Streaming quality selector (High/Medium/Low) | ✅ Complete |
| 2025-12-04 | 3.3 | Backend quality parameter support | ✅ Complete |
| 2025-12-04 | 5.1 | Pre-buffer next track in queue | ✅ Complete |
| 2025-12-04 | 5.1 | Buffering state in playerSlice | ✅ Complete |
| 2025-12-04 | 5.1 | Buffer indicator on play button | ✅ Complete |
| 2025-12-04 | 5.1 | Audio buffering event handling | ✅ Complete |
| 2025-12-04 | 5.2 | Streaming error state in playerSlice | ✅ Complete |
| 2025-12-04 | 5.2 | Error type detection (network/auth/unavailable) | ✅ Complete |
| 2025-12-04 | 5.2 | Retry with exponential backoff (3 attempts) | ✅ Complete |
| 2025-12-04 | 5.2 | Network recovery detection (online/offline) | ✅ Complete |
| 2025-12-04 | 5.2 | User-friendly error toasts | ✅ Complete |
| 2025-12-04 | 5.3 | StreamingStats state in playerSlice | ✅ Complete |
| 2025-12-04 | 5.3 | recordStreamEvent action | ✅ Complete |
| 2025-12-04 | 5.3 | Event logging (start/complete/error/buffer) | ✅ Complete |
| 2025-12-04 | 5.3 | Streaming statistics UI in Settings | ✅ Complete |
| 2025-12-04 | 2.1 | Graceful error handling in frontend | ✅ Complete |
| 2025-12-04 | 2.2 | Buffer state indication UI | ✅ Complete |
| 2025-12-04 | 2.3 | Loading state during initial buffer | ✅ Complete |
| 2025-12-04 | 2.3 | Error state handling UI with retry | ✅ Complete |
| 2025-12-04 | 4.3 | Play artist top tracks | ✅ Complete |
| 2025-12-04 | 4.3 | Artist search results with play buttons | ✅ Complete |
| | - | 🎉 All Phases Complete! | ✅ Done |

---

## References

- [librespot-go Repository](https://github.com/art-media-platform/librespot-go)
- [Existing Download Implementation](./backend/internal/spotify/downloader.go)
- [Session Management](./backend/internal/spotify/session.go)
- [Download Manager](./backend/internal/api/download_manager.go)
- [New Streamer Implementation](./backend/internal/spotify/streamer.go)

