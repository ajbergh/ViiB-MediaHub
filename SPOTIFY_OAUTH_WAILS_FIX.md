# Spotify OAuth Cross-Origin Issue in Wails Build

## Issue Summary

**Error Message:**
```
Connection Failed
Failed to read a named property 'origin' from 'Window': Blocked a frame with origin "http://127.0.0.1:65337" from accessing a cross-origin frame.
```

**Environment:**
- Wails Windows build
- API server running on: `http://127.0.0.1:65337`
- Main Wails window origin: `http://wails.localhost` (WebView2)
- OAuth callback URL: `http://127.0.0.1:65337/callback`

---

## Root Cause Analysis

### How Spotify OAuth Works in Wails

1. **Main Window (wails.localhost)**
   - User clicks "Connect to Spotify" button
   - Frontend calls `getOAuthCallbackUrl()` which returns `http://127.0.0.1:PORT/callback`
   - Credentials and PKCE code verifier are saved to backend via `api.saveSpotifyCredentials()`
   - A popup window opens with Spotify's authorization URL

2. **Spotify Authorization Flow**
   - User logs in to Spotify
   - Spotify redirects to: `http://127.0.0.1:65337/callback?code=...`
   - The popup now runs on `http://127.0.0.1:65337` origin

3. **SpotifyCallback Page (127.0.0.1:65337)**
   - React app loads in popup at this origin
   - Tries to fetch credentials from backend
   - Exchanges OAuth code for tokens
   - **THE PROBLEM**: Line 123 of SpotifyCallback.tsx:
     ```tsx
     const isCrossOrigin = !isWailsEnvironment() && window.opener && window.opener.origin !== window.location.origin;
     ```
   - This line attempts to access `window.opener.origin` which triggers a cross-origin security error

### The Cross-Origin Security Issue

The browser blocks access to `window.opener.origin` when:
- Opener window origin: `http://wails.localhost`
- Current window origin: `http://127.0.0.1:65337`

These are different origins, so the browser throws:
```
Failed to read a named property 'origin' from 'Window': Blocked a frame with origin "http://127.0.0.1:65337" from accessing a cross-origin frame.
```

The problematic code:
```tsx
// SpotifyCallback.tsx line 123
const isCrossOrigin = !isWailsEnvironment() && window.opener && window.opener.origin !== window.location.origin;
```

The check `window.opener.origin` throws an error **before** the comparison can even happen because accessing any property on a cross-origin `window.opener` is blocked by browser security.

---

## Current Implementation Details

### Files Involved

1. **[utils.ts](utils.ts#L60-L97)** - Environment detection and OAuth URL generation
   - `isWailsEnvironment()` - Checks if running in Wails by hostname `wails.localhost`
   - `getOAuthCallbackUrl()` - Returns appropriate callback URL (uses 127.0.0.1 for Wails)

2. **[pages/Spotify.tsx](pages/Spotify.tsx#L320-L420)** - Main Spotify page
   - `handleLogin()` - Initiates OAuth flow, saves credentials to backend, opens popup
   - Polling mechanism to detect auth completion via backend

3. **[pages/SpotifyCallback.tsx](pages/SpotifyCallback.tsx)** - OAuth callback handler
   - Runs in the popup window
   - Fetches credentials from backend API
   - Exchanges OAuth code for tokens
   - **BUG**: Line 123 incorrectly accesses `window.opener.origin`

4. **[backend/cmd/wails/main.go](backend/cmd/wails/main.go)** - Wails application setup
   - `GetServerURL()` - Returns API server URL for frontend
   - API proxy handler for `/api/*` routes

5. **[backend/internal/api/spotify.go](backend/internal/api/spotify.go)** - Backend credential storage
   - `saveSpotifyCredentials()` - Stores OAuth credentials in database
   - `getSpotifyCredentials()` - Retrieves stored credentials

### Flow Diagram

```
┌──────────────────────┐        ┌──────────────────────┐
│   Wails WebView      │        │   Popup Window       │
│ (wails.localhost)    │        │ (127.0.0.1:65337)    │
├──────────────────────┤        ├──────────────────────┤
│                      │        │                      │
│ 1. User clicks       │        │                      │
│    "Connect"         │        │                      │
│         │            │        │                      │
│ 2. Save creds to     │        │                      │
│    backend API       │        │                      │
│         │            │        │                      │
│ 3. Open popup ───────┼───────>│ 4. Spotify redirects │
│                      │        │    back with code    │
│ 5. Poll backend ─────┼──────┐ │         │            │
│    for auth          │      │ │ 5. SpotifyCallback   │
│         │            │      │ │    tries to access   │
│         │            │      │ │    window.opener     │
│         │            │      │ │         │            │
│         │            │      │ │ ❌ CROSS-ORIGIN     │
│         │            │      │ │    ERROR!           │
│         │            │      │ │         │            │
│         │            │      └─┤ 6. Callback fails   │
│                      │        │                      │
└──────────────────────┘        └──────────────────────┘
```

---

## Fix Implementation

### Step 1: Add Cross-Origin Detection Helper (COMPLETED ✅)

Added a safe helper function `detectCrossOriginContext()` that:
- Checks if popup is running on API server (127.0.0.1) vs Wails (wails.localhost)
- Wraps `window.opener.location.origin` access in try-catch
- Returns true for any cross-origin scenario

**New Code (lines 33-66):**
```tsx
/**
 * Detects if we're running in a cross-origin context where we cannot
 * directly communicate with window.opener.
 * 
 * Cross-origin scenarios:
 * 1. Wails build: popup on 127.0.0.1, main window on wails.localhost
 * 2. Any case where opener origin differs from current origin
 * 
 * IMPORTANT: We cannot access window.opener.origin directly when cross-origin
 * as the browser will throw a security error. We must use indirect detection.
 * 
 * @returns true if we're in a cross-origin popup context
 */
const detectCrossOriginContext = (): boolean => {
    // In Wails, the popup runs on the API server (127.0.0.1) while
    // the main window runs on wails.localhost - these are cross-origin
    const isPopupOnApiServer = window.location.hostname === '127.0.0.1';
    
    // If we're on the API server (not wails.localhost), we're definitely cross-origin
    if (isPopupOnApiServer && !isWailsEnvironment()) {
        return true;
    }
    
    // For non-Wails environments, try to detect via window.opener
    if (window.opener) {
        try {
            // Attempting to access window.opener.origin will throw if cross-origin
            // If it doesn't throw, compare origins
            const openerOrigin = window.opener.location.origin;
            return openerOrigin !== window.location.origin;
        } catch {
            // SecurityError thrown - we're definitely cross-origin
            return true;
        }
    }
    
    // No opener means we navigated here directly (not a popup)
    return false;
};
```

### Step 2: Fix Cross-Origin Check Usage (COMPLETED ✅)

**Before (problematic):**
```tsx
const isCrossOrigin = !isWailsEnvironment() && window.opener && window.opener.origin !== window.location.origin;
```

**After (fixed):**
```tsx
const isCrossOrigin = detectCrossOriginContext();
console.log('[SpotifyCallback] Cross-origin context:', isCrossOrigin);
```

### Step 3: Fix postMessage Logic (COMPLETED ✅)

**Before (used wildcard origin):**
```tsx
window.opener.postMessage({
    type: 'SPOTIFY_AUTH_SUCCESS',
    // ...
}, '*'); // Use wildcard for cross-origin
```

**After (only posts if same-origin, relies on backend polling for cross-origin):**
```tsx
if (!isCrossOrigin) {
    try {
        window.opener.postMessage({
            type: 'SPOTIFY_AUTH_SUCCESS',
            // ...
        }, window.location.origin);
        console.log('[SpotifyCallback] Posted auth success to opener');
    } catch (e) {
        console.log('[SpotifyCallback] Could not post message to opener:', e);
    }
} else {
    console.log('[SpotifyCallback] Cross-origin: skipping postMessage, main window will poll backend');
}
```

### Step 4: Updated File Documentation (COMPLETED ✅)

The module-level JSDoc comment was updated to explain the cross-origin handling:

```tsx
/**
 * ViiB MediaHub - Spotify OAuth Callback Page
 * 
 * Handles the OAuth 2.0 PKCE callback from Spotify authorization.
 * 
 * Flow:
 * 1. User redirected here after Spotify login
 * 2. Extracts authorization code from URL params
 * 3. Fetches client credentials from backend (supports cross-origin popups)
 * 4. Exchanges code for access/refresh tokens
 * 5. Fetches user profile
 * 6. Saves credentials to backend for download functionality
 * 7. Closes popup - main window detects auth via backend polling
 * 
 * Cross-Origin Handling (Wails):
 * - Main window: runs on http://wails.localhost (WebView2)
 * - OAuth popup: runs on http://127.0.0.1:PORT (API server)
 * - These are different origins, so localStorage is NOT shared
 * - We CANNOT access window.opener properties (throws security error)
 * - Communication happens via backend: popup saves tokens → main window polls
 * 
 * @module SpotifyCallback
 */
```

---

## Complete Fix Summary

### Changes Made

| File | Change Description |
|------|-------------------|
| `pages/SpotifyCallback.tsx` | Added `detectCrossOriginContext()` helper function (lines 33-66) |
| `pages/SpotifyCallback.tsx` | Replaced unsafe `window.opener.origin` access with safe helper |
| `pages/SpotifyCallback.tsx` | Fixed postMessage to only attempt when same-origin |
| `pages/SpotifyCallback.tsx` | Updated module JSDoc with cross-origin handling explanation |

### Testing Steps

1. **Build Wails app**: `scripts/build-wails.ps1`
2. **Launch application**
3. **Navigate to Spotify page**
4. **Click "Connect to Spotify"**
5. **Complete Spotify authorization in popup**
6. **Verify popup closes and main window shows "Connected as [user]"**
7. **Verify credentials persist on app restart**

### Additional Considerations

1. **Spotify Redirect URI**: Ensure `http://127.0.0.1:65337/callback` is registered in Spotify Developer Dashboard
   - Since the port is dynamic, you may need to register multiple ports or use a fixed port

2. **Port Consistency**: Consider using a fixed port for the API server in production builds:
   - Add `-port 65337` flag or similar fixed port
   - Register that specific callback URL in Spotify Dashboard

3. **Development vs Production**: The OAuth flow differs:
   - **Development (Vite)**: `http://127.0.0.1:3000/callback`
   - **Production (Wails)**: `http://127.0.0.1:PORT/callback` (dynamic port)

---

## Related Files Reference

- [SpotifyCallback.tsx](pages/SpotifyCallback.tsx) - OAuth callback handler (NEEDS FIX)
- [Spotify.tsx](pages/Spotify.tsx) - Main Spotify page with login/polling
- [utils.ts](utils.ts) - OAuth URL generation and environment detection
- [spotifyService.ts](services/spotifyService.ts) - Spotify API service
- [api.ts](services/api.ts) - Backend API client
- [backend/internal/api/spotify.go](backend/internal/api/spotify.go) - Backend credential storage
- [backend/cmd/wails/main.go](backend/cmd/wails/main.go) - Wails app configuration

---

## Status

- [x] Issue analyzed and documented
- [x] Fix implemented in SpotifyCallback.tsx
  - [x] Added `detectCrossOriginContext()` helper function
  - [x] Replaced unsafe `window.opener.origin` access
  - [x] Fixed postMessage to only send when same-origin
  - [x] Updated module documentation
- [ ] Tested in Wails build
- [ ] Verified with Spotify authorization flow
