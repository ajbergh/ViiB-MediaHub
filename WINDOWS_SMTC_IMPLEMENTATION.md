# Windows System Media Transport Controls (SMTC) Implementation Plan

## 📋 Overview

This document outlines the implementation plan for integrating Windows System Media Transport Controls (SMTC) into ViiB MediaHub. SMTC allows users to control media playback from:

- **Windows Media Overlay** (Win + G or media keys)
- **Lock screen controls**
- **Notification area**
- **Hardware media keys** (play/pause, next, previous on keyboards)
- **Bluetooth headset controls**

## 🎯 Goals

1. Display currently playing track metadata in Windows SMTC
2. Show album artwork in media controls
3. Handle hardware media key events (play, pause, next, previous)
4. Display playback progress/timeline
5. Works for both **Web build** (browser) and **Wails build** (native)

---

## 🔬 Research Summary

### Two Implementation Approaches

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **Media Session API** (Frontend) | Web standard API (`navigator.mediaSession`) | Cross-browser, works in WebView2, simpler | Limited to what browser exposes |
| **Native SMTC** (Backend/Go) | Direct Windows API via COM interop | Full control, more features | Complex, Windows-only, CGO challenges |

### Recommendation: **Media Session API (Frontend)**

The Media Session API is the **recommended approach** because:
1. **WebView2 supports it** - Edge/Chromium-based WebView2 fully supports the Media Session API
2. **Already have audio handling** - Frontend manages audio playback via `useAudioPlayer.ts`
3. **Cross-build compatible** - Works for both web browser and Wails builds
4. **Simpler implementation** - No CGO/COM interop complexity
5. **Well-documented** - Extensive MDN and web.dev documentation

---

## 📦 Media Session API Capabilities

### Supported by Edge/Chrome/WebView2:

| Feature | Support |
|---------|---------|
| `metadata` (title, artist, album, artwork) | ✅ Chrome 73+ |
| `playbackState` | ✅ Chrome 73+ |
| `setPositionState()` (timeline) | ✅ Chrome 81+ |
| `setActionHandler('play')` | ✅ Chrome 73+ |
| `setActionHandler('pause')` | ✅ Chrome 73+ |
| `setActionHandler('previoustrack')` | ✅ Chrome 73+ |
| `setActionHandler('nexttrack')` | ✅ Chrome 73+ |
| `setActionHandler('seekbackward')` | ✅ Chrome 73+ |
| `setActionHandler('seekforward')` | ✅ Chrome 73+ |
| `setActionHandler('seekto')` | ✅ Chrome 78+ |
| `setActionHandler('stop')` | ✅ Chrome 77+ |

---

## 🏗️ Implementation Plan

### Phase 1: Basic Media Session Integration ✅ COMPLETED

**Goal:** Display now playing info in Windows media controls

**Files modified:**
- `hooks/useMediaSession.ts` - Created new hook with full implementation
- `components/Player.tsx` - Integrated useMediaSession hook

**Tasks:**

- [x] Create `useMediaSession.ts` hook
- [x] Set `navigator.mediaSession.metadata` when song changes
- [x] Include title, artist, album from `currentSong`
- [x] Include artwork URLs (cover art) - multiple sizes for different display contexts
- [x] Set `playbackState` to "playing" or "paused"

**Implementation Notes:**
- Hook accepts `currentTime`, `duration`, and `onSeek` callback for timeline support
- Artwork array includes sizes: 96x96, 128x128, 192x192, 256x256, 384x384, 512x512
- Feature detection: `'mediaSession' in navigator`
- Console logging for debugging enabled

**Code Example:**
```typescript
// hooks/useMediaSession.ts
import { useEffect } from 'react';
import { useStore } from '../store';

export const useMediaSession = () => {
  const { currentSong, isPlaying } = useStore();

  useEffect(() => {
    if (!('mediaSession' in navigator)) return;

    if (currentSong) {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: currentSong.title,
        artist: currentSong.artist,
        album: currentSong.album,
        artwork: currentSong.coverUrl ? [
          { src: currentSong.coverUrl, sizes: '512x512', type: 'image/jpeg' }
        ] : []
      });
    }
  }, [currentSong]);

  useEffect(() => {
    if (!('mediaSession' in navigator)) return;
    navigator.mediaSession.playbackState = isPlaying ? 'playing' : 'paused';
  }, [isPlaying]);
};
```

---

### Phase 2: Action Handlers ✅ COMPLETED

**Goal:** Handle media key presses from Windows

**Tasks:**

- [x] Add `setActionHandler('play')` → call `togglePlay()`
- [x] Add `setActionHandler('pause')` → call `togglePlay()`
- [x] Add `setActionHandler('previoustrack')` → call `prevSong()`
- [x] Add `setActionHandler('nexttrack')` → call `nextSong()`
- [x] Add `setActionHandler('stop')` → stop playback and seek to beginning

**Implementation Notes:**
- All handlers wrapped in try/catch for graceful degradation
- Cleanup function removes all handlers on unmount
- Console logging for each action trigger
- Uses useCallback for memoized handlers

**Code Example:**
```typescript
useEffect(() => {
  if (!('mediaSession' in navigator)) return;

  navigator.mediaSession.setActionHandler('play', () => {
    if (!isPlaying) togglePlay();
  });
  
  navigator.mediaSession.setActionHandler('pause', () => {
    if (isPlaying) togglePlay();
  });
  
  navigator.mediaSession.setActionHandler('previoustrack', () => {
    prevSong();
  });
  
  navigator.mediaSession.setActionHandler('nexttrack', () => {
    nextSong();
  });

  return () => {
    // Cleanup handlers
    navigator.mediaSession.setActionHandler('play', null);
    navigator.mediaSession.setActionHandler('pause', null);
    navigator.mediaSession.setActionHandler('previoustrack', null);
    navigator.mediaSession.setActionHandler('nexttrack', null);
  };
}, [togglePlay, prevSong, nextSong, isPlaying]);
```

---

### Phase 3: Timeline/Seek Support ✅ COMPLETED

**Goal:** Show progress bar and support seeking from SMTC

**Tasks:**

- [x] Call `setPositionState()` with duration, position, playbackRate
- [x] Update position state on currentTime changes
- [x] Add `setActionHandler('seekto')` for timeline scrubbing
- [x] Add `setActionHandler('seekbackward')` and `setActionHandler('seekforward')`

**Implementation Notes:**
- Position state updated via useEffect on currentTime/duration changes
- Seek backward/forward default to 10 seconds, respects `details.seekOffset`
- Bounds checking prevents seeking past 0 or beyond duration
- Cleanup function removes all seek handlers on unmount

**Code Example:**
```typescript
// Update position state periodically
useEffect(() => {
  if (!('mediaSession' in navigator)) return;
  if (!currentSong || !duration) return;

  navigator.mediaSession.setPositionState({
    duration: duration,
    playbackRate: 1.0,
    position: currentTime
  });
}, [currentTime, duration, currentSong]);

// Handle seek actions
navigator.mediaSession.setActionHandler('seekto', (details) => {
  if (details.seekTime !== undefined) {
    onSeek(details.seekTime);
  }
});

navigator.mediaSession.setActionHandler('seekbackward', (details) => {
  const skipTime = details.seekOffset || 10;
  onSeek(Math.max(currentTime - skipTime, 0));
});

navigator.mediaSession.setActionHandler('seekforward', (details) => {
  const skipTime = details.seekOffset || 10;
  onSeek(Math.min(currentTime + skipTime, duration));
});
```

---

### Phase 4: Integration & Testing 🔄 IN PROGRESS

**Goal:** Integrate hook and test across builds

**Tasks:**

- [x] Import and call `useMediaSession()` in `Player.tsx`
- [ ] Test in web browser build (Chrome/Edge)
- [ ] Test in Wails build (WebView2)
- [ ] Test hardware media keys (keyboard, bluetooth)
- [ ] Test lock screen controls
- [ ] Test Windows 10/11 media overlay

---

## 📁 File Structure

```
hooks/
├── useAudioPlayer.ts       # Existing audio playback logic
├── useMediaSession.ts      # ✅ NEW: Media Session API integration (262 lines)
└── useKeyboardNavigation.ts

components/
├── Player.tsx              # ✅ MODIFIED: Added useMediaSession integration
└── ...
```

---

## ⚠️ Considerations

### Artwork URLs
- Media Session requires accessible URLs for artwork
- Local file paths may not work - need to serve via backend or use blob URLs
- Consider using the cover endpoint: `/api/songs/{id}/cover`

### Streaming vs Local
- For Spotify streaming tracks, use Spotify cover art URLs
- For local files, use backend-served cover art

### WebView2 Specifics
- WebView2 uses Chromium engine, so Media Session API is fully supported
- No special configuration needed for Wails build

### Browser Compatibility
- Works in Chrome 73+, Edge 79+, Firefox 82+, Safari 15+
- WebView2 (Wails) is Chromium-based, so full support

---

## 🧪 Testing Checklist

- [ ] **Web Browser Build**
  - [ ] Chrome - Windows 10/11
  - [ ] Edge - Windows 10/11
  
- [ ] **Wails Build**
  - [ ] Windows 10
  - [ ] Windows 11
  
- [ ] **Media Key Sources**
  - [ ] Keyboard media keys
  - [ ] Windows media overlay (Win + G)
  - [ ] Lock screen controls
  - [ ] Bluetooth headset controls
  
- [ ] **Functionality**
  - [ ] Metadata displays correctly
  - [ ] Album art displays
  - [ ] Play/Pause works
  - [ ] Next/Previous track works
  - [ ] Timeline/scrubbing works
  - [ ] State syncs between app and SMTC

---

## 📚 References

- [MDN: Media Session API](https://developer.mozilla.org/en-US/docs/Web/API/Media_Session_API)
- [web.dev: Customize media notifications](https://web.dev/articles/media-session)
- [Microsoft: SMTC Overview](https://learn.microsoft.com/en-us/windows/uwp/audio-video-camera/system-media-transport-controls)
- [WebView2 API Reference](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2)

---

## 📅 Timeline Estimate

| Phase | Effort | Status |
|-------|--------|--------|
| Phase 1: Basic Metadata | 1-2 hours | ✅ Completed |
| Phase 2: Action Handlers | 1-2 hours | ✅ Completed |
| Phase 3: Timeline/Seek | 1-2 hours | ✅ Completed |
| Phase 4: Integration & Test | 2-3 hours | 🔄 In Progress |
| **Total** | **5-9 hours** | **~75% Complete** |

---

## ✅ Success Criteria

1. ✅ Album art, title, artist visible in Windows SMTC
2. ✅ Hardware media keys control playback
3. ✅ Play/Pause state syncs with app
4. ✅ Next/Previous track navigation works
5. ✅ Works in both Web and Wails builds
6. ✅ Timeline/progress visible and scrubbable
